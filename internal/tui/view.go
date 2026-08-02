package tui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"eonyx/internal/llm"
	"eonyx/internal/skills"
	versionpkg "eonyx/internal/version"
)

// scrollbarWidth is the column reserved to the right of the viewport for the
// scroll-position indicator.
const scrollbarWidth = 1

// narrowWidth is the terminal width below which the header and welcome banner
// switch to a compact layout that fits without wrapping.
const narrowWidth = 60

// blankRunRe matches a run of two or more blank lines (lines that are empty or
// only whitespace), so a model that pads its output with newlines does not blow
// up the transcript.
var blankRunRe = regexp.MustCompile(`\n(?:[ \t]*\n){2,}`)

// collapseBlankLines reduces any run of blank lines to a single blank line.
func collapseBlankLines(s string) string {
	return blankRunRe.ReplaceAllString(s, "\n\n")
}

// View satisfies tea.Model.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.viewport.View(), m.renderScrollbar()))

	if m.dialog != dialogNone {
		b.WriteString("\n")
		b.WriteString(m.renderDialog())
	} else {
		b.WriteString("\n")
		b.WriteString(m.renderRule())
		b.WriteString("\n")
		b.WriteString(m.textarea.View())
		b.WriteString("\n")
		b.WriteString(m.footerStatus())
	}

	return b.String()
}

// footerStatus shows the agent's live status while a turn runs (spinner, what it
// is doing, and elapsed time), or the key hints when idle. While the inline
// completion is open it becomes the vertical completion menu instead; the
// viewport is shrunk by acExtraRows so the extra rows fit.
func (m *Model) footerStatus() string {
	if m.acActive {
		return m.renderAutocompleteMenu()
	}
	if !m.isRunning {
		return statusHint()
	}
	// Distinguish the model thinking/streaming from a tool executing.
	state := "Thinking"
	if m.activeTool != "" {
		state = "Running " + m.activeTool
	}
	if m.stepMax > 0 {
		state += fmt.Sprintf(" %d/%d", m.step, m.stepMax)
	}
	elapsed := time.Since(m.runStart).Round(time.Second)
	status := workingStyle.Render(fmt.Sprintf("%s %s… (%s%s)", m.spin.View(), state, elapsed, m.streamRate()))
	return status + lipgloss.NewStyle().Foreground(dimColor).Render("  esc to interrupt")
}

// renderScrollbar draws a one-column scroll indicator the height of the
// viewport: a bright thumb sized to the visible fraction of the transcript,
// positioned by the scroll offset, over a dim track. When everything fits it
// renders a blank column so the layout width stays stable.
func (m *Model) renderScrollbar() string {
	h := m.viewport.Height
	if h <= 0 {
		return ""
	}
	total := m.viewport.TotalLineCount()
	lines := make([]string, h)
	if total <= h {
		for i := range lines {
			lines[i] = " "
		}
		return strings.Join(lines, "\n")
	}
	thumb := max(1, h*h/total)
	pos := int(math.Round(float64(h-thumb) * m.viewport.ScrollPercent()))
	pos = max(0, min(h-thumb, pos))
	for i := 0; i < h; i++ {
		if i >= pos && i < pos+thumb {
			lines[i] = scrollThumbStyle.Render("█")
		} else {
			lines[i] = scrollTrackStyle.Render("│")
		}
	}
	return strings.Join(lines, "\n")
}

// streamRate returns a " · N tok/s" suffix estimating the current output rate
// from streamed characters (~4 chars per token), or "" before there is enough
// signal (under a second, or nothing streamed yet).
func (m *Model) streamRate() string {
	secs := time.Since(m.runStart).Seconds()
	if secs < 1 || m.streamChars == 0 {
		return ""
	}
	return fmt.Sprintf(" · %.0f tok/s", float64(m.streamChars)/4/secs)
}

// renderViewport rebuilds the content shown in the viewport. It only auto-scrolls
// to the bottom if the user was already at the bottom, so earlier text stays
// readable while scrolling.
func (m *Model) renderViewport() {
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.renderItems())
	if atBottom {
		m.viewport.GotoBottom()
	}
}

// renderItems renders all chat bubbles into one string, or a welcome banner
// when the conversation is empty.
func (m *Model) renderItems() string {
	if len(m.items) == 0 {
		return m.renderWelcome()
	}
	var b strings.Builder
	for i := range m.items {
		rendered := m.renderItem(&m.items[i])
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		b.WriteString(rendered)
		b.WriteString("\n\n")
	}
	return b.String()
}

// robotLines is the little robot-head mark shown above the wordmark.
var robotLines = []string{
	` ┌─────┐`,
	`╾┤ ● ● ├╼`,
	` └─┬─┬─┘`,
}

// bannerLines is the block ASCII wordmark shown on the welcome screen.
var bannerLines = []string{
	`▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄ ▄▄▄ ▄▄▄ ▄▄▄`,
	`█ ▄▄▄▄█ █ ▄▄▄ █ █ ▄▄▄ █ █ █▄█ █ █ █▄█ █`,
	`█ ▄▄▄█▄ █ █▄█ █ █ █ █ █ ▀▀█ █▀▀ ▄▄█ █▄▄`,
	`█▄▄▄▄▄█ █▄▄▄▄▄█ █▄█ █▄█   █▄█   █▄█ █▄█`,
}

// styleRobotLine renders a robot line in the brand colour, with dimmed eyes.
// While blinking, the right eye ("●") closes to a dash so the robot winks.
func (m *Model) styleRobotLine(s string) string {
	eye := lipgloss.NewStyle().Foreground(dimColor)
	var b strings.Builder
	seen := 0
	for _, r := range s {
		if r == '●' {
			seen++
			ch := "●"
			if m.blinkClosed && seen == 2 {
				ch = "-"
			}
			b.WriteString(eye.Render(ch))
			continue
		}
		b.WriteString(brandStyle.Render(string(r)))
	}
	return b.String()
}

// renderWelcome shows the wordmark, tagline, version, and — beneath them — the
// winking robot mark, for the empty transcript.
func (m *Model) renderWelcome() string {
	if m.width < narrowWidth {
		title := brandStyle.Render("eonyx")
		tagline := lipgloss.NewStyle().Foreground(dimColor).Render("terminal-first AI coding agent")
		version := lipgloss.NewStyle().Foreground(dimColor).Render(versionpkg.String())
		return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(title + "\n" + tagline + "\n" + version)
	}
	logo := centerBlock(bannerLines, func(s string) string { return brandStyle.Render(s) })
	robot := centerBlock(robotLines, m.styleRobotLine)
	tagline := lipgloss.NewStyle().Foreground(dimColor).Render("terminal-first AI coding agent")
	version := lipgloss.NewStyle().Foreground(dimColor).Render("eonyx " + versionpkg.String())
	banner := logo + "\n\n" + tagline + "\n" + version + "\n\n" + robot
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(banner)
}

// centerBlock styles each line with style and centers it within the block's
// widest line, so the block stays rectangular when the whole banner is centered.
func centerBlock(lines []string, style func(string) string) string {
	width := 0
	for _, ln := range lines {
		width = max(width, lipgloss.Width(ln))
	}
	out := make([]string, len(lines))
	for i, ln := range lines {
		total := width - lipgloss.Width(ln)
		left := total / 2
		out[i] = strings.Repeat(" ", left) + style(ln) + strings.Repeat(" ", total-left)
	}
	return strings.Join(out, "\n")
}

func (m *Model) renderItem(item *messageItem) string {
	body := m.renderItemBody(item)
	if m.showTimestamps && !item.createdAt.IsZero() {
		ts := lipgloss.NewStyle().Foreground(dimColor).Render(item.createdAt.Format("15:04:05"))
		return ts + "\n" + body
	}
	return body
}

func (m *Model) renderItemBody(item *messageItem) string {
	switch item.role {
	case llm.RoleUser:
		return userStyle.Render(renderUserText(item.text))
	case llm.RoleAssistant:
		var parts []string
		if r := m.renderReasoning(item); r != "" {
			parts = append(parts, r)
		}
		if strings.TrimSpace(item.text) != "" {
			parts = append(parts, m.assistantBody(item))
		}
		for _, tc := range item.toolCalls {
			parts = append(parts, m.renderToolCall(tc))
		}
		return assistantStyle.Render(strings.Join(parts, "\n"))
	case llm.RoleSystem:
		if item.isErr {
			return errBadgeStyle.Render(" error ") + " " + errorStyle.Render(item.text)
		}
		return noteBadgeStyle.Render(" system ") + " " + noteStyle.Render(item.text)
	default:
		return item.text
	}
}

// renderReasoning renders a reasoning model's thinking as a collapsible block:
// a one-line hint by default, or the full dimmed text when expanded (Ctrl+R).
func (m *Model) renderReasoning(item *messageItem) string {
	r := strings.TrimSpace(item.reasoning)
	if r == "" {
		return ""
	}
	if !m.reasoningExpanded {
		return reasoningStyle.Render("▸ thinking — Ctrl+R to expand")
	}
	wrapped := ansi.Wrap(collapseBlankLines(r), m.contentWidth(), "")
	body := styleLines(indentText(wrapped, bodyIndent), reasoningStyle)
	return reasoningStyle.Render("▾ thinking") + "\n" + body
}

// assistantBody renders an assistant turn's prose. A finished turn is rendered
// through the markdown renderer once and cached on the item (re-rendered only
// when the content width changes); an in-flight turn is shown as raw,
// word-wrapped text so streaming never reflows the transcript.
func (m *Model) assistantBody(item *messageItem) string {
	cw := m.contentWidth()
	if item.done {
		if item.rendered == "" || item.renderedWidth != cw {
			item.rendered = indentText(renderMarkdown(collapseBlankLines(item.text), cw), bodyIndent)
			item.renderedWidth = cw
		}
		return item.rendered
	}
	return indentText(ansi.Wrap(collapseBlankLines(item.text), cw, ""), bodyIndent)
}

// contentWidth is the wrap width for assistant prose: the viewport width less
// the body gutter and a small right margin, so a full line never reaches the
// terminal edge — which would force a phantom wrap and a jittery extra row.
func (m *Model) contentWidth() int {
	w := m.width - len(bodyIndent) - 2
	if w < 20 {
		w = 20
	}
	return w
}

// renderUserText prefixes a user turn with a "> " prompt, aligning any
// continuation lines under the first, the way Claude Code shows your input.
func renderUserText(text string) string {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if i == 0 {
			lines[i] = userPromptStyle.Render(">") + " " + ln
		} else {
			lines[i] = "  " + ln
		}
	}
	return strings.Join(lines, "\n")
}

// renderToolCall renders one tool call in the Claude Code style: a coloured
// bullet (green on success, red on error), the tool name, and a short argument,
// followed by the result. For write/edit the result is shown as a coloured
// unified diff (green additions, red deletions); other tools collapse to a
// one-line summary so file contents and long listings are never dumped.
func (m *Model) renderToolCall(tc toolCallItem) string {
	bulletColor := sageColor
	if tc.done && tc.isError {
		bulletColor = errColor
	}
	bullet := lipgloss.NewStyle().Foreground(bulletColor).Render("●")
	name := toolStyle.Bold(true).Render(tc.name)
	head := bullet + " " + name
	if arg := toolArg(tc); arg != "" {
		head += " " + lipgloss.NewStyle().Foreground(dimColor).Render(arg)
	}
	if !tc.done {
		return head
	}
	if !tc.isError && (tc.name == "write" || tc.name == "edit" || tc.name == "multi_edit") {
		if body := renderDiff(tc.result, m.expandTools); body != "" {
			return head + "\n" + body
		}
	}
	color := dimColor
	if tc.isError {
		color = errColor
	}
	// Expanded: show the full result text indented; collapsed: a one-line summary.
	if m.expandTools && strings.TrimSpace(tc.result) != "" {
		body := indentText(strings.TrimRight(tc.result, "\n"), "    ")
		return head + "\n" + styleLines(body, lipgloss.NewStyle().Foreground(color))
	}
	return head + "\n  " + lipgloss.NewStyle().Foreground(color).Render("⎿ "+toolResultSummary(tc))
}

func (m *Model) renderHeader() string {
	if m.width < narrowWidth {
		return m.renderCompactHeader()
	}
	title := m.title
	if title == "" {
		title = "untitled"
	}
	// Each segment is labelled (Path/Branch/Session) and colour-coded so it is
	// clear what each value is. Two spaces keep the labels from running together.
	label := lipgloss.NewStyle().Foreground(dimColor)
	sessionValue := lipgloss.NewStyle().Foreground(lipgloss.Color("#B19CD9")).Bold(true)
	leftSegs := []string{label.Render("Path ") + projectStyle.Render(leftTruncate(m.displayPath(), 48))}
	if m.branch != "" {
		leftSegs = append(leftSegs, label.Render("Branch ")+lipgloss.NewStyle().Foreground(sageColor).Render(m.branch))
	}
	leftSegs = append(leftSegs, label.Render("Session ")+sessionValue.Render(sessionLabel(title)))
	left := headerLogoStyle.Render(logo) + " " + strings.Join(leftSegs, "  ")

	rightSegs := []string{}
	if m.planMode {
		rightSegs = append(rightSegs, planStyle.Render(" PLAN "))
	}
	if m.yolo {
		rightSegs = append(rightSegs, yoloStyle.Render(" YOLO "))
	} else {
		rightSegs = append(rightSegs, safeStyle.Render(" SAFE "))
	}
	tokens := fmt.Sprintf("%s · %d↑ %d↓", m.modelID, m.inTokens, m.outTokens)
	if m.cachedTokens > 0 {
		tokens += fmt.Sprintf(" ⚡%d cached", m.cachedTokens)
	}
	rightSegs = append(rightSegs, headerStyle.Render(tokens))
	if !m.session.CreatedAt.IsZero() {
		rightSegs = append(rightSegs, lipgloss.NewStyle().Foreground(dimColor).Render("⏱ "+formatDuration(time.Since(m.session.CreatedAt))))
	}
	if pct := m.contextFill(); pct >= 0 {
		style := contextFillStyle
		if pct >= 80 {
			// Warn before auto-summarization kicks in (~80% of the window).
			style = contextWarnStyle
		}
		rightSegs = append(rightSegs, style.Render(fmt.Sprintf("{ctx %d%%}", pct)))
	}
	if m.cost > 0 {
		rightSegs = append(rightSegs, costStyle.Render(fmt.Sprintf("$%.4f", m.cost)))
	}
	right := strings.Join(rightSegs, " ")

	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	// Keep a space on each side of the hatch so it never butts against text.
	var mid string
	if gap >= 4 {
		mid = " " + fillerStyle.Render(strings.Repeat("/", gap-2)) + " "
	} else {
		mid = strings.Repeat(" ", gap)
	}
	topLine := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	return topLine + "\n" + m.renderContextLine() + "\n" + m.renderRule()
}

// renderCompactHeader renders a three-line header that fits a narrow terminal:
// session and model on top, capabilities below, then the rule — each truncated
// to the width so nothing wraps.
func (m *Model) renderCompactHeader() string {
	sessionValue := lipgloss.NewStyle().Foreground(lipgloss.Color("#B19CD9")).Bold(true)
	mode, modeStyle := " SAFE ", safeStyle
	if m.yolo {
		mode, modeStyle = " YOLO ", yoloStyle
	}
	top := headerLogoStyle.Render(logo) + " " + sessionValue.Render(sessionLabel(m.title))
	if m.planMode {
		top += " " + planStyle.Render(" PLAN ")
	}
	top += " " + modeStyle.Render(mode) + " " + headerStyle.Render(m.modelID)
	top = ansi.Truncate(top, max(0, m.width), "…")
	ctx := ansi.Truncate(m.renderContextLine(), max(0, m.width), "…")
	return top + "\n" + ctx + "\n" + m.renderRule()
}

// renderRule draws a thin full-width rule, used below the header and above the
// input box to frame the conversation.
func (m *Model) renderRule() string {
	return headerRuleStyle.Render(strings.Repeat("─", max(0, m.width)))
}

// formatDuration renders an elapsed duration compactly for the header: seconds
// under a minute, minutes under an hour, then hours+minutes, then days.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}

// sessionLabel renders the session title for the header: a sequential numeric
// title ("1", or legacy "Session 1") collapses to "[N]" (the "Session" word is
// already the label), while a custom title is shown as-is.
func sessionLabel(title string) string {
	var n int
	if _, err := fmt.Sscanf(title, "Session %d", &n); err == nil {
		return fmt.Sprintf("[%d]", n)
	}
	if n, err := strconv.Atoi(title); err == nil {
		return fmt.Sprintf("[%d]", n)
	}
	return truncate(title, 36)
}

// displayPath is the agent's current working directory with the home directory
// collapsed to ~, shown with forward slashes.
func (m *Model) displayPath() string {
	p := m.workDir
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = "~/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}

// leftTruncate keeps the tail of s (the most specific path segments) when it is
// longer than max, prefixing an ellipsis.
func leftTruncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-max+1:])
}

// contextFill returns the estimated share of the model's context window the
// current conversation occupies, or -1 when the window size is not configured.
func (m *Model) contextFill() int {
	win := contextWindow(m.cfg, m.modelID)
	if win <= 0 {
		return -1
	}
	return min(100, estimateContextTokens(m.history)*100/win)
}

// estimateContextTokens approximates the tokens held in the conversation
// (~4 chars per token), used only for the context-fill indicator.
func estimateContextTokens(history []llm.Message) int {
	chars := 0
	for _, msg := range history {
		for _, part := range msg.Content {
			switch v := part.(type) {
			case llm.Text:
				chars += len(v.Text)
			case llm.ToolCall:
				chars += len(v.Name) + len(v.Input)
			case llm.ToolResult:
				chars += len(v.Content)
			}
		}
	}
	return chars / 4
}

// renderContextLine shows the active skills and MCP servers on one line, so it
// is clear which extra capabilities are currently switched on.
func (m *Model) renderContextLine() string {
	skills := capSegment("# skills", skillNames(m.activeSkillSet()), accentColor)
	mcp := capSegment("» mcp", m.activeMCPServers(), accentColor)
	return skills + " " + mcp
}

// activeMCPServers returns the MCP servers currently switched on.
func (m *Model) activeMCPServers() []string {
	if m.activeMCP == nil {
		return m.mcpServers
	}
	var out []string
	for _, s := range m.mcpServers {
		if m.activeMCP[s] {
			out = append(out, s)
		}
	}
	return out
}

// capSegment renders "<label> a · b · c (+k)" with a colored label and dim
// names, or "<label> —" when nothing is loaded.
func capSegment(label string, names []string, color lipgloss.Color) string {
	lbl := lipgloss.NewStyle().Foreground(color).Bold(true).Render(label)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	if len(names) == 0 {
		return lbl + " " + dim.Render("—")
	}
	shown, suffix := names, ""
	if len(names) > 4 {
		shown, suffix = names[:4], fmt.Sprintf(" +%d", len(names)-4)
	}
	return lbl + " " + dim.Render(strings.Join(shown, " · ")+suffix)
}

func skillNames(s []*skills.Skill) []string {
	out := make([]string, len(s))
	for i, sk := range s {
		out[i] = sk.Meta.Name
	}
	return out
}
