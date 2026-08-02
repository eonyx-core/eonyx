package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/eonyx-core/eonyx/internal/llm"
	"github.com/eonyx-core/eonyx/internal/session"
	"github.com/eonyx-core/eonyx/internal/skills"
)

func (m *Model) openModelPicker() {
	m.dialogOptions = m.dialogOptions[:0]
	for _, mod := range m.cfg.Models {
		label := mod.ID
		if mod.ID == m.modelID {
			label += " (current)"
		}
		m.dialogOptions = append(m.dialogOptions, dialogOption{label: label, value: mod.ID})
	}
	m.openDialog(dialogModelPicker)
}

func (m *Model) openSessionPicker() {
	m.dialogOptions = nil
	list, err := m.store.List()
	if err != nil {
		m.fail(err)
		return
	}
	for _, s := range list {
		title := s.Title
		if title == "" {
			title = s.ID
		}
		label := truncate(title, 50)
		if s.ID == m.session.ID {
			label += " (current)"
		}
		m.dialogOptions = append(m.dialogOptions, dialogOption{label: label, value: s.ID})
	}
	m.openDialog(dialogSessionPicker)
}

// openSessionSearch opens the session picker filtered to sessions whose title
// or messages contain query, each labelled with a match snippet.
func (m *Model) openSessionSearch(query string) {
	hits, err := session.Search(m.store, query)
	if err != nil {
		m.fail(err)
		return
	}
	if len(hits) == 0 {
		m.systemNote(fmt.Sprintf("No sessions match %q", query))
		return
	}
	m.dialogOptions = nil
	for _, h := range hits {
		label := truncate(h.Session.Title, 40)
		if h.Snippet != "" && h.Snippet != h.Session.Title {
			label += " — " + truncate(h.Snippet, 40)
		}
		if h.Session.ID == m.session.ID {
			label += " (current)"
		}
		m.dialogOptions = append(m.dialogOptions, dialogOption{label: label, value: h.Session.ID})
	}
	m.openDialog(dialogSessionPicker)
}

func (m *Model) openCommandPalette() {
	m.dialogSection = 0
	m.dialogCursor = 0
	m.loadCommandSection()
	m.openDialog(dialogCommandPalette)
}

// openHelp shows the full keybinding and command reference.
func (m *Model) openHelp() {
	m.openDialog(dialogHelp)
}

// openPlanApproval asks the user to approve the plan proposed during a plan-mode
// turn: implement it now, just enable edits, or keep planning.
func (m *Model) openPlanApproval() {
	m.dialogOptions = []dialogOption{
		{label: "Approve & implement", value: "implement"},
		{label: "Approve (enable edits, I'll continue)", value: "enable"},
		{label: "Keep planning", value: "keep"},
	}
	m.openDialog(dialogPlanApproval)
}

// openPermissionPrompt shows the allow-once / allow-always / deny dialog for a
// tool the static policy did not clear, storing the request to answer.
func (m *Model) openPermissionPrompt(req permissionRequestMsg) {
	m.permPending = &req
	m.permName = req.name
	m.permArg = toolArg(toolCallItem{name: req.name, input: string(req.input)})
	m.dialogOptions = []dialogOption{
		{label: "Allow once", value: "once"},
		{label: "Allow always (this session)", value: "always"},
		{label: "Deny", value: "deny"},
	}
	m.openDialog(dialogPermission)
}

// resolvePermission answers the pending permission request, records an
// "always" grant for the session, and closes the dialog. It is safe to call
// with no pending request.
func (m *Model) resolvePermission(dec permDecision) {
	if m.permPending != nil {
		if dec == permAllowAlways {
			if m.sessionAllowed == nil {
				m.sessionAllowed = map[string]bool{}
			}
			m.sessionAllowed[m.permPending.name] = true
		}
		m.permPending.respond <- dec
		m.permPending = nil
	}
	m.closeDialog()
}

// switchDialogSection flips between the two tabs of a sectioned dialog (the
// command palette and the capabilities picker) and reloads its options.
func (m *Model) switchDialogSection() {
	switch m.dialog {
	case dialogCommandPalette:
		m.dialogSection = 1 - m.dialogSection
		m.dialogCursor = 0
		m.loadCommandSection()
	case dialogCapabilities:
		m.dialogSection = 1 - m.dialogSection
		m.dialogCursor = 0
		m.loadCapabilitySection()
	}
}

// loadCommandSection fills the palette with either the work commands (task-
// oriented, specialized) or the system commands (session/app controls),
// depending on the active tab.
func (m *Model) loadCommandSection() {
	if m.dialogSection == 0 {
		m.dialogOptions = []dialogOption{
			{label: "New session (/new)", value: "new"},
			{label: "Clear context (/clear)", value: "clear"},
			{label: "Toggle yolo (/yolo)", value: "yolo"},
			{label: "Toggle plan mode (/plan)", value: "plan"},
			{label: "Toggle timestamps (/timestamps)", value: "timestamps"},
			{label: "Switch model", value: "model"},
			{label: "Manage sessions", value: "session"},
			{label: "Capabilities", value: "capabilities"},
			{label: "Quit", value: "quit"},
		}
		return
	}
	m.dialogOptions = []dialogOption{
		{label: "Attach file (/add)", value: "add"},
		{label: "Show public IP (/ip)", value: "ip"},
		{label: "Export conversation (/export)", value: "export"},
	}
}

func (m *Model) openCapabilities() {
	m.dialogSection = 0
	m.dialogCursor = 0
	m.loadCapabilitySection()
	m.openDialog(dialogCapabilities)
}

func (m *Model) loadCapabilitySection() {
	m.dialogOptions = m.dialogOptions[:0]
	if m.dialogSection == 0 {
		// Skills section.
		for _, sk := range m.skillSet {
			active := true
			if m.activeSkills != nil {
				active = m.activeSkills[sk.Meta.Name]
			}
			mark := "✓"
			if !active {
				mark = "✗"
			}
			label := fmt.Sprintf("%s %s", mark, sk.Meta.Name)
			m.dialogOptions = append(m.dialogOptions, dialogOption{label: label, value: sk.Meta.Name})
		}
	} else {
		// MCP section.
		for _, s := range m.mcpServers {
			active := true
			if m.activeMCP != nil {
				active = m.activeMCP[s]
			}
			mark := "✓"
			if !active {
				mark = "✗"
			}
			label := fmt.Sprintf("%s %s", mark, s)
			m.dialogOptions = append(m.dialogOptions, dialogOption{label: label, value: s})
		}
	}
}

func (m *Model) toggleCapability() {
	if len(m.dialogOptions) == 0 || m.dialogCursor < 0 || m.dialogCursor >= len(m.dialogOptions) {
		return
	}
	opt := m.dialogOptions[m.dialogCursor]
	if m.dialogSection == 0 {
		if m.activeSkills == nil {
			m.activeSkills = make(map[string]bool)
			for _, sk := range m.skillSet {
				m.activeSkills[sk.Meta.Name] = true
			}
		}
		m.activeSkills[opt.value] = !m.activeSkills[opt.value]
		m.applyCapabilities()
	} else {
		if m.activeMCP == nil {
			m.activeMCP = make(map[string]bool)
			for _, s := range m.mcpServers {
				m.activeMCP[s] = true
			}
		}
		m.activeMCP[opt.value] = !m.activeMCP[opt.value]
		m.applyCapabilities()
	}
	m.loadCapabilitySection()
	if m.dialogCursor >= len(m.dialogOptions) {
		m.dialogCursor = len(m.dialogOptions) - 1
	}
}

// activeSkillSet returns the skills currently switched on (all when no toggles
// have been made). It never mutates the full skillSet.
func (m *Model) activeSkillSet() []*skills.Skill {
	if m.activeSkills == nil {
		return m.skillSet
	}
	var out []*skills.Skill
	for _, sk := range m.skillSet {
		if m.activeSkills[sk.Meta.Name] {
			out = append(out, sk)
		}
	}
	return out
}

// disabledMCPTools returns the registry tool names belonging to MCP servers that
// are switched off, so they can be hidden from the model.
func (m *Model) disabledMCPTools() map[string]bool {
	if m.activeMCP == nil {
		return nil
	}
	off := map[string]bool{}
	for _, s := range m.mcpServers {
		if !m.activeMCP[s] {
			off[s] = true
		}
	}
	if len(off) == 0 {
		return nil
	}
	disabled := map[string]bool{}
	for _, name := range m.registry.Names() {
		for s := range off {
			if strings.HasPrefix(name, s+"_") {
				disabled[name] = true
			}
		}
	}
	return disabled
}

// applyCapabilities makes the current skill/MCP toggles take effect: it rebuilds
// the in-memory system prompt catalog, rebinds the load_skill tool to the active
// skills, hides tools of disabled MCP servers, and rebuilds the agent.
func (m *Model) applyCapabilities() {
	active := m.activeSkillSet()
	if len(m.history) > 0 && m.history[0].Role == llm.RoleSystem {
		m.history[0] = llm.SystemText(systemPrompt(m.workDir, m.cfg.ProjectRoot(), active))
	}
	m.registry.Replace(skills.NewLoadTool(active))
	m.registry.SetDisabled(m.disabledMCPTools())
	m.rebuildAgent()
}

func (m *Model) openDialog(state dialogState) {
	m.dialog = state
	m.dialogCursor = 0
}

func (m *Model) closeDialog() {
	m.dialog = dialogNone
	m.dialogCursor = 0
	m.dialogOptions = nil
	m.pickerFilter = ""
	m.pickerAll = nil
}

// selectDialogOption applies the highlighted option for the active dialog.
func (m *Model) selectDialogOption() tea.Cmd {
	if m.dialogCursor < 0 || m.dialogCursor >= len(m.dialogOptions) {
		m.closeDialog()
		return nil
	}
	opt := m.dialogOptions[m.dialogCursor]
	if m.dialog == dialogFilePicker {
		if opt.isDir {
			m.openFilePicker(opt.value)
		} else {
			m.closeDialog()
			m.attachFile(opt.value)
		}
		return nil
	}
	if m.dialog == dialogCapabilities {
		// Toggle in place and keep the dialog open so several can be flipped;
		// closing here would wipe dialogOptions before toggleCapability reads it.
		m.toggleCapability()
		return nil
	}
	if m.dialog == dialogPermission {
		dec := permDeny
		switch opt.value {
		case "once":
			dec = permAllowOnce
		case "always":
			dec = permAllowAlways
		}
		m.resolvePermission(dec)
		return nil
	}
	value := opt.value
	dialog := m.dialog
	m.closeDialog()

	switch dialog {
	case dialogModelPicker:
		m.applyModel(value)
	case dialogSessionPicker:
		m.applySession(value)
	case dialogCommandPalette:
		switch value {
		case "add":
			m.startAttach("")
		case "ip":
			return m.runIP()
		case "clear":
			m.clearChat()
		case "new":
			m.newChat()
		case "export":
			m.exportSession()
		case "yolo":
			m.toggleYolo()
		case "plan":
			m.togglePlan()
		case "timestamps":
			m.toggleTimestamps()
		case "model":
			m.openModelPicker()
		case "session":
			m.openSessionPicker()
		case "capabilities":
			m.openCapabilities()
		case "quit":
			return tea.Quit
		}
	case dialogDeleteConfirm:
		if value == "yes" {
			m.deleteSession(m.deleteTarget)
		}
	case dialogPlanApproval:
		switch value {
		case "implement":
			m.planMode = false
			m.rebuildAgent()
			m.systemNote("Plan approved — implementing.")
			return m.submit("The plan is approved. Go ahead and implement it.")
		case "enable":
			m.planMode = false
			m.rebuildAgent()
			m.systemNote("Plan approved — edits enabled.")
		}
	}
	return nil
}

// confirmDeleteSession opens a confirmation dialog before deleting session id.
func (m *Model) confirmDeleteSession(id string) {
	m.deleteTarget = id
	m.dialogOptions = []dialogOption{
		{label: "Cancel", value: "no"},
		{label: "Delete", value: "yes"},
	}
	m.openDialog(dialogDeleteConfirm)
}

// helpEntry is one "key — description" row in the help screen.
type helpEntry struct{ key, desc string }

var helpKeys = []helpEntry{
	{"Enter", "send message"},
	{"Ctrl+J", "insert a newline"},
	{"↑ / ↓", "recall previous inputs"},
	{"Esc", "stop the running turn"},
	{"/", "command palette (empty prompt)"},
	{"@", "inline file completion (Tab accepts)"},
	{"?", "this help (empty prompt)"},
	{"Ctrl+P", "command palette"},
	{"Ctrl+N", "switch model"},
	{"Ctrl+S", "sessions"},
	{"Ctrl+K", "capabilities (skills / MCP)"},
	{"Ctrl+O", "expand / collapse tool output"},
	{"Ctrl+R", "expand / collapse model reasoning"},
	{"Ctrl+C", "quit"},
	{"! <cmd>", "run a shell command"},
	{"wheel", "scroll the transcript"},
}

var helpCommands = []helpEntry{
	{"/new", "start a new session"},
	{"/clear", "clear the current context"},
	{"/add [path]", "attach a file"},
	{"/ip", "show the public IP"},
	{"/rename <t>", "rename the session"},
	{"/fork", "branch the conversation into a new session"},
	{"/export", "export the conversation"},
	{"/yolo", "toggle the permission gate"},
	{"/plan", "toggle plan mode (read-only until approved)"},
	{"/timestamps", "toggle message timestamps"},
	{"/model", "switch model"},
	{"/sessions", "manage sessions"},
	{"/quit", "exit"},
}

// renderHelp builds the help screen body: two labelled sections of aligned
// "key — description" rows.
func (m *Model) renderHelp() string {
	keyStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(dimColor)
	section := func(label string, entries []helpEntry) string {
		var b strings.Builder
		b.WriteString(headerStyle.Render(label))
		b.WriteString("\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "%s  %s\n", keyStyle.Render(padRight(e.key, 12)), descStyle.Render(e.desc))
		}
		return b.String()
	}
	return section("Keys", helpKeys) + "\n" + section("Commands", helpCommands) +
		"\n" + descStyle.Render("press any key to close")
}

// padRight pads s with spaces to width n (display width aware).
func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func (m *Model) renderDialog() string {
	if m.dialog == dialogHelp {
		return dialogStyle.Render(m.renderHelp())
	}

	var title string
	switch m.dialog {
	case dialogModelPicker:
		title = "Select model"
	case dialogSessionPicker:
		title = "Select session"
	case dialogCommandPalette:
		title = "Commands"
	case dialogFilePicker:
		title = "Attach file"
		if rel, err := filepath.Rel(m.workDir, m.pickerDir); err == nil {
			title = "Attach file · ./" + filepath.ToSlash(rel)
		}
		if m.pickerFilter != "" {
			title += "  [" + m.pickerFilter + "]"
		}
	case dialogDeleteConfirm:
		title = "Delete this session?"
	case dialogPermission:
		title = "Allow " + m.permName + "?"
		if m.permArg != "" {
			title += "  " + m.permArg
		}
	case dialogPlanApproval:
		title = "Plan ready — approve?"
	case dialogCapabilities:
		if m.dialogSection == 0 {
			title = "Skills (toggle on/off)"
		} else {
			title = "MCP Servers (toggle on/off)"
		}
	default:
		return ""
	}

	var b strings.Builder
	switch m.dialog {
	case dialogCommandPalette:
		b.WriteString(m.renderDialogTabs("System", "Work"))
	case dialogCapabilities:
		b.WriteString(m.renderDialogTabs("Skills", "MCP"))
	default:
		b.WriteString(headerStyle.Render(title))
	}
	b.WriteString("\n\n")
	if len(m.dialogOptions) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render("(nothing to show)"))
	}
	for i, opt := range m.dialogOptions {
		cursor := "  "
		line := opt.label
		if i == m.dialogCursor {
			cursor = "› "
			line = lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(line)
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, line)
	}
	hint := "↑/↓ • Enter • Esc"
	if m.dialog == dialogPermission {
		hint = "↑/↓ • Enter • Esc denies"
	}
	if m.dialog == dialogPlanApproval {
		hint = "↑/↓ • Enter • Esc keeps planning"
	}
	if m.dialog == dialogFilePicker {
		hint = "↑/↓ • Enter • type to filter • ⌫ • Esc"
	}
	if m.dialog == dialogSessionPicker {
		hint = "↑/↓ • Enter • r rename • f fork • d delete • Esc"
	}
	if m.dialog == dialogCommandPalette {
		hint = "↑/↓ • Enter • Tab System/Work • Esc"
	}
	if m.dialog == dialogCapabilities {
		hint = "↑/↓ • Enter toggle • Tab Skills/MCP • Esc"
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(hint))
	return dialogStyle.Render(b.String())
}

// renderDialogTabs draws the dialog's section tabs as pills, highlighting the
// active one, the way the question chips are styled.
func (m *Model) renderDialogTabs(labels ...string) string {
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(accentColor).Bold(true).Padding(0, 1)
	idle := lipgloss.NewStyle().Foreground(dimColor).Padding(0, 1)
	tabs := make([]string, len(labels))
	for i, l := range labels {
		if i == m.dialogSection {
			tabs[i] = active.Render(l)
		} else {
			tabs[i] = idle.Render(l)
		}
	}
	return strings.Join(tabs, " ")
}
