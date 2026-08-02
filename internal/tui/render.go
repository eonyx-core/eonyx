package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// toolArg extracts a single meaningful argument (path, pattern, command, …) from
// a tool's JSON input for a clean header, falling back to the raw input.
func toolArg(tc toolCallItem) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(tc.input), &fields) == nil {
		for _, key := range []string{"path", "pattern", "command", "query", "url"} {
			raw, ok := fields[key]
			if !ok {
				continue
			}
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				return truncate(oneLine(s), 80)
			}
		}
	}
	return truncate(oneLine(tc.input), 80)
}

// maxDiffLines caps how many diff lines a single write/edit result shows before
// the remainder is collapsed to a count, keeping the transcript compact.
const maxDiffLines = 40

// renderDiff colours the unified diff embedded in a write/edit result —
// additions green, deletions red, hunk headers and context dimmed. It returns
// "" when the result carries no diff body (e.g. a freshly created file). When
// expanded is true the whole diff is shown; otherwise it is capped.
func renderDiff(result string, expanded bool) string {
	idx := strings.Index(result, "--- a/")
	if idx < 0 {
		return ""
	}
	summary := firstLine(result[:idx])

	var lines []string
	for ln := range strings.SplitSeq(strings.TrimRight(result[idx:], "\n"), "\n") {
		if strings.HasPrefix(ln, "--- ") || strings.HasPrefix(ln, "+++ ") {
			continue
		}
		lines = append(lines, ln)
	}

	var out []string
	if summary != "" {
		out = append(out, "  "+diffContextStyle.Render("⎿ "+summary))
	}
	limit := maxDiffLines
	if expanded {
		limit = len(lines)
	}
	shown, extra := lines, 0
	if len(lines) > limit {
		shown, extra = lines[:limit], len(lines)-limit
	}
	for i := 0; i < len(shown); i++ {
		ln := shown[i]
		// A lone deletion immediately followed by a lone addition is a single-line
		// replacement: highlight the differing span within it.
		if isIsolatedReplacement(shown, i) {
			del, add := styleWordDiff(ln, shown[i+1])
			out = append(out, "    "+del, "    "+add)
			i++
			continue
		}
		out = append(out, "    "+styleDiffLine(ln))
	}
	if extra > 0 {
		out = append(out, "    "+diffContextStyle.Render(fmt.Sprintf("… +%d more lines", extra)))
	}
	return strings.Join(out, "\n")
}

// isIsolatedReplacement reports whether lines[i] is a deletion paired with a
// single following addition (not part of a multi-line deletion or addition
// block), so a word-level diff between the two lines is meaningful.
func isIsolatedReplacement(lines []string, i int) bool {
	if !strings.HasPrefix(lines[i], "-") || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+") {
		return false
	}
	if i > 0 && strings.HasPrefix(lines[i-1], "-") {
		return false
	}
	if i+2 < len(lines) && strings.HasPrefix(lines[i+2], "+") {
		return false
	}
	return true
}

// styleWordDiff renders a deletion/addition pair, keeping the shared leading and
// trailing runes in the base +/- colour and inverting only the changed middle
// span. del starts with "-", add with "+".
func styleWordDiff(del, add string) (string, string) {
	db, ab := []rune(del[1:]), []rune(add[1:])
	pre := commonPrefixRunes(db, ab)
	suf := commonSuffixRunes(db[pre:], ab[pre:])

	render := func(marker byte, body []rune, base, word lipgloss.Style) string {
		head := base.Render(string(marker) + string(body[:pre]))
		mid := word.Render(string(body[pre : len(body)-suf]))
		tail := base.Render(string(body[len(body)-suf:]))
		return head + mid + tail
	}
	return render('-', db, diffDelStyle, diffDelWordStyle),
		render('+', ab, diffAddStyle, diffAddWordStyle)
}

// commonPrefixRunes returns the number of leading runes a and b share.
func commonPrefixRunes(a, b []rune) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// commonSuffixRunes returns the number of trailing runes a and b share.
func commonSuffixRunes(a, b []rune) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}

// styleDiffLine colours one unified-diff line by its leading marker.
func styleDiffLine(ln string) string {
	switch {
	case strings.HasPrefix(ln, "@@"):
		return diffHunkStyle.Render(ln)
	case strings.HasPrefix(ln, "+"):
		return diffAddStyle.Render(ln)
	case strings.HasPrefix(ln, "-"):
		return diffDelStyle.Render(ln)
	default:
		return diffContextStyle.Render(ln)
	}
}

// toolResultSummary condenses a tool result to one line: a count for read/list
// tools, otherwise the first line truncated. It keeps the transcript readable.
func toolResultSummary(tc toolCallItem) string {
	if tc.isError {
		return truncate(oneLine(tc.result), 100)
	}
	switch tc.name {
	case "read":
		return fmt.Sprintf("%d lines", lineCount(tc.result))
	case "ls", "glob", "grep":
		return fmt.Sprintf("%d results", lineCount(tc.result))
	case "write", "edit":
		// The result is a one-line summary (with +N -M) followed by the diff
		// body; show only the summary line to keep the transcript compact.
		return truncate(firstLine(tc.result), 100)
	default:
		return truncate(oneLine(tc.result), 100)
	}
}

// firstLine returns the first line of s, trimmed.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// oneLine collapses newlines and tabs to single spaces for inline display.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// lineCount returns the number of non-empty trailing-trimmed lines in s.
func lineCount(s string) int {
	s = strings.TrimRight(s, "\n")
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
