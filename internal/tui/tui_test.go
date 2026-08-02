package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"eonyx/internal/agent"
	"eonyx/internal/config"
	"eonyx/internal/llm"
	"eonyx/internal/session"
	"eonyx/internal/skills"
	"eonyx/internal/tools"
)

func newTestModel() *Model {
	return &Model{
		ctx:          context.Background(),
		textarea:     textarea.New(),
		viewport:     viewport.New(0, 0),
		spin:         spinner.New(),
		headerHeight: 1,
		footerHeight: 5,
	}
}

func TestModelHandlesWindowSize(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.width != 80 || m.height != 24 {
		t.Errorf("size = %dx%d, want 80x24", m.width, m.height)
	}
	// The viewport reserves one column on the right for the scroll indicator.
	if m.viewport.Width != 80-scrollbarWidth || m.viewport.Height != 18 {
		t.Errorf("viewport size = %dx%d, want %dx18", m.viewport.Width, m.viewport.Height, 80-scrollbarWidth)
	}
}

func TestModelAppendsDeltaAsAssistant(t *testing.T) {
	m := newTestModel()
	m.Update(agentDeltaMsg{delta: "hi"})
	if len(m.items) != 1 || m.items[0].role != llm.RoleAssistant || m.items[0].text != "hi" {
		t.Errorf("items = %+v, want one assistant message", m.items)
	}
}

func TestModelUpdatesBranchFromMsg(t *testing.T) {
	m := newTestModel()
	m.branch = "main"
	m.Update(branchMsg{branch: "feature"})
	if m.branch != "feature" {
		t.Errorf("branch = %q, want feature", m.branch)
	}
}

func TestModelAppendsToolStartToAssistant(t *testing.T) {
	m := newTestModel()
	m.Update(agentDeltaMsg{delta: "listing"})
	m.Update(agentToolStartMsg{name: "ls", input: []byte(`{"path":"."}`)})
	last := m.items[len(m.items)-1]
	if len(last.toolCalls) != 1 || last.toolCalls[0].name != "ls" {
		t.Errorf("tool calls = %+v, want one ls", last.toolCalls)
	}
}

func TestModelMarksToolDone(t *testing.T) {
	m := newTestModel()
	m.Update(agentDeltaMsg{delta: "listing"})
	m.Update(agentToolStartMsg{name: "ls", input: []byte(`{}`)})
	m.Update(agentToolEndMsg{name: "ls", result: "file.txt", isError: false})
	last := m.items[len(m.items)-1]
	if !last.toolCalls[0].done || last.toolCalls[0].result != "file.txt" {
		t.Errorf("tool call = %+v, want done with result", last.toolCalls[0])
	}
}

func TestModelErrorState(t *testing.T) {
	m := newTestModel()
	m.Update(agentDoneMsg{err: errors.New("boom")})
	if m.err == nil {
		t.Error("expected error to be stored")
	}
	if m.isRunning {
		t.Error("expected isRunning to be reset on error")
	}
}

func TestCtrlPOpensPalette(t *testing.T) {
	m := newTestModel()
	consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	if !consumed || m.dialog != dialogCommandPalette {
		t.Errorf("dialog = %d, consumed = %v, want command palette", m.dialog, consumed)
	}
}

func TestSlashOpensPaletteWhenEmpty(t *testing.T) {
	m := newTestModel()
	consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !consumed || m.dialog != dialogCommandPalette {
		t.Errorf("'/' on empty input should open palette: consumed=%v dialog=%d", consumed, m.dialog)
	}
}

func TestTypedSlashTextStillCompletesInline(t *testing.T) {
	// Slash text that reaches the input another way (prefill, paste) still gets
	// inline completion, even though "/" on an empty prompt opens the palette.
	m := newTestModel()
	m.textarea.SetValue("/ex")
	m.updateAutocomplete()
	if !m.acActive || m.acKind != acSlash {
		t.Fatalf("slash completion inactive: active=%v kind=%v", m.acActive, m.acKind)
	}
	var found bool
	for _, it := range m.acItems {
		if it.insert == "/export" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /export among %+v", m.acItems)
	}
}

func TestSlashIsLiteralMidText(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("path is ")
	consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if consumed {
		t.Error("'/' mid-text should be typed, not consumed")
	}
	if m.dialog != dialogNone {
		t.Errorf("dialog = %d, want none", m.dialog)
	}
}

func TestRenameFromSessionPickerPrefillsInput(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("my session")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel()
	m.store = store
	m.session = sess
	m.title = "my session"
	m.openSessionPicker()
	// Select the only session entry.
	m.dialogCursor = 0
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if got := m.textarea.Value(); got != "/rename my session" {
		t.Errorf("prefill = %q, want %q", got, "/rename my session")
	}
	if m.dialog != dialogNone {
		t.Errorf("dialog = %d, should be closed after rename hotkey", m.dialog)
	}
}

func TestForkFromSessionPickerSwitchesToFork(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("my session")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel()
	m.cfg = &config.Config{}
	m.store = store
	m.session = sess
	m.title = "my session"
	m.openSessionPicker()
	// Select the only session entry and fork it.
	m.dialogCursor = 0
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.dialog != dialogNone {
		t.Errorf("dialog = %d, should be closed after fork hotkey", m.dialog)
	}
	if m.title != "my session (fork)" {
		t.Errorf("title = %q, want %q", m.title, "my session (fork)")
	}
	if m.session.ID == sess.ID {
		t.Error("expected to switch to the forked session")
	}
}

func TestSubmitDoesNotChangeTitle(t *testing.T) {
	store, err := session.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("Session 1")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel()
	m.store = store
	m.session = sess
	m.title = sess.Title
	m.agent = agent.New(nil, tools.NewRegistry(), agent.Options{})

	m.submit("Refactor the parser please")
	if m.title != "Session 1" {
		t.Errorf("title = %q, want it unchanged (Session 1)", m.title)
	}
	got, _ := store.Get(sess.ID)
	if got.Title != "Session 1" {
		t.Errorf("stored title = %q, want it unchanged (Session 1)", got.Title)
	}
}

func TestNextSessionTitleIsSequential(t *testing.T) {
	store, err := session.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := nextSessionTitle(store); got != "1" {
		t.Errorf("first title = %q, want 1", got)
	}
	if _, err := store.Create("1"); err != nil {
		t.Fatal(err)
	}
	if got := nextSessionTitle(store); got != "2" {
		t.Errorf("second title = %q, want 2", got)
	}
	// Legacy "Session N" titles keep the numbering going.
	if _, err := store.Create("Session 5"); err != nil {
		t.Fatal(err)
	}
	if got := nextSessionTitle(store); got != "6" {
		t.Errorf("title after legacy sessions = %q, want 6", got)
	}
}

func TestDeleteCurrentSwitchesToPreviousNotNew(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := store.Create("one")
	s2, _ := store.Create("two")

	m := newTestModel()
	m.cfg = &config.Config{}
	m.store = store
	m.workDir = dir
	m.session = s2

	m.deleteSession(s2.ID)

	list, _ := store.List()
	if len(list) != 1 || list[0].ID != s1.ID {
		t.Fatalf("expected only s1 to remain, got %d sessions", len(list))
	}
	if m.session.ID != s1.ID {
		t.Errorf("current session = %q, want previous %q (no new session)", m.session.ID, s1.ID)
	}
}

func TestCapabilityToggleSkillsReversible(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.registry = tools.NewRegistry()
	m.history = []llm.Message{llm.SystemText("sys")}
	m.skillSet = []*skills.Skill{
		{Meta: skills.SkillMeta{Name: "a"}},
		{Meta: skills.SkillMeta{Name: "b"}},
	}
	m.openCapabilities()

	m.dialogCursor = 0 // "a"
	m.toggleCapability()
	if got := skillNames(m.activeSkillSet()); len(got) != 1 || got[0] != "b" {
		t.Fatalf("after disabling a, active = %v", got)
	}
	if len(m.skillSet) != 2 {
		t.Fatalf("full skill list must be preserved, got %d", len(m.skillSet))
	}

	m.dialogCursor = 0 // "a" again — must be re-enableable
	m.toggleCapability()
	if got := skillNames(m.activeSkillSet()); len(got) != 2 {
		t.Errorf("re-enabling a should restore both, got %v", got)
	}
}

func TestCapabilityToggleViaEnterKeepsDialogOpen(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	m.registry = tools.NewRegistry()
	m.history = []llm.Message{llm.SystemText("sys")}
	m.skillSet = []*skills.Skill{{Meta: skills.SkillMeta{Name: "a"}}}
	m.openCapabilities()

	// Enter must toggle the highlighted skill AND keep the dialog open, not
	// close it with no effect.
	m.dialogCursor = 0
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.dialog != dialogCapabilities {
		t.Fatalf("dialog = %d, want capabilities to stay open", m.dialog)
	}
	if len(m.activeSkillSet()) != 0 {
		t.Errorf("Enter should have disabled skill a, active = %v", skillNames(m.activeSkillSet()))
	}
}

func TestCapabilityToggleMCPDisablesTools(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{}
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "db_list", Schema: json.RawMessage(`{}`),
		Handler: func(context.Context, json.RawMessage) (string, error) { return "", nil }})
	m.registry = reg
	m.history = []llm.Message{llm.SystemText("sys")}
	m.mcpServers = []string{"db"}

	m.openCapabilities()
	m.dialogSection = 1 // MCP section
	m.loadCapabilitySection()
	m.dialogCursor = 0
	m.toggleCapability() // turn db off
	if _, ok := reg.Get("db_list"); ok {
		t.Error("db_list should be disabled after turning off the db server")
	}

	m.dialogCursor = 0
	m.toggleCapability() // turn db back on
	if _, ok := reg.Get("db_list"); !ok {
		t.Error("db_list should be enabled again")
	}
}

func TestConfirmDeleteOpensConfirmDialog(t *testing.T) {
	m := newTestModel()
	m.confirmDeleteSession("sess-1")
	if m.dialog != dialogDeleteConfirm {
		t.Fatalf("dialog = %d, want delete confirm", m.dialog)
	}
	if m.deleteTarget != "sess-1" || len(m.dialogOptions) != 2 {
		t.Errorf("deleteTarget=%q options=%d, want sess-1 and 2 options", m.deleteTarget, len(m.dialogOptions))
	}
}

func TestEscCancelsRunningTurn(t *testing.T) {
	m := newTestModel()
	m.isRunning = true
	cancelled := false
	m.cancel = func() { cancelled = true }
	consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed || !cancelled {
		t.Errorf("Esc should cancel running turn: consumed=%v cancelled=%v", consumed, cancelled)
	}
}

func TestCommandPaletteQuitSelected(t *testing.T) {
	m := newTestModel()
	m.openCommandPalette()
	// The palette opens on the "System" tab, where Quit lives.
	if len(m.dialogOptions) != 9 {
		t.Fatalf("system options = %d, want 9", len(m.dialogOptions))
	}
	// Switch to the "Work" tab with the task commands.
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyTab})
	if len(m.dialogOptions) != 3 {
		t.Fatalf("work options = %d, want 3", len(m.dialogOptions))
	}
	// Back to System and select the "Quit" entry (last).
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyTab})
	for range m.dialogOptions[:len(m.dialogOptions)-1] {
		m.handleDialogKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, cmd := m.handleDialogKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command from selecting Quit")
	}
	if m.dialog != dialogNone {
		t.Errorf("dialog not closed after selection: %d", m.dialog)
	}
}

func TestModelPickerListsConfiguredModels(t *testing.T) {
	m := newTestModel()
	m.cfg = &config.Config{
		Models:       []config.Model{{ID: "a"}, {ID: "b"}},
		DefaultModel: "a",
	}
	m.modelID = "a"
	m.openModelPicker()
	if len(m.dialogOptions) != 2 {
		t.Fatalf("options = %d, want 2", len(m.dialogOptions))
	}
	if !strings.Contains(m.dialogOptions[0].label, "current") {
		t.Errorf("current model not marked: %q", m.dialogOptions[0].label)
	}
}

func TestItemsFromHistorySkipsSystemAndPairsResults(t *testing.T) {
	history := []llm.Message{
		llm.SystemText("system"),
		llm.UserText("hello"),
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{
			llm.Text{Text: "running"},
			llm.ToolCall{ID: "1", Name: "ls", Input: json.RawMessage(`{}`)},
		}},
		{Role: llm.RoleTool, Content: []llm.ContentPart{
			llm.ToolResult{ToolCallID: "1", Content: "file.txt"},
		}},
	}
	items := itemsFromHistory(history)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (system skipped)", len(items))
	}
	if items[0].role != llm.RoleUser || items[0].text != "hello" {
		t.Errorf("first item = %+v, want user hello", items[0])
	}
	tc := items[1].toolCalls
	if len(tc) != 1 || !tc[0].done || tc[0].result != "file.txt" {
		t.Errorf("tool call not paired with result: %+v", tc)
	}
}

func TestSlashCommandQuit(t *testing.T) {
	m := newTestModel()
	if cmd := m.runSlashCommand("/quit"); cmd == nil {
		t.Error("/quit should return a command")
	}
}

func TestSlashCommandUnknownNotes(t *testing.T) {
	m := newTestModel()
	if cmd := m.runSlashCommand("/bogus"); cmd != nil {
		t.Error("unknown command should not return a command")
	}
	if len(m.items) != 1 || m.items[0].role != llm.RoleSystem {
		t.Errorf("expected a system note, got %+v", m.items)
	}
	if !strings.Contains(m.items[0].text, "Unknown") {
		t.Errorf("note = %q, want Unknown", m.items[0].text)
	}
}

func TestOpenFilePickerListsEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel()
	m.workDir = dir
	m.openFilePicker(dir)
	if m.dialog != dialogFilePicker {
		t.Fatalf("dialog = %d, want file picker", m.dialog)
	}
	var hasParent, hasDir, hasFile bool
	for _, o := range m.dialogOptions {
		switch {
		case o.label == "../" && o.isDir:
			hasParent = true
		case strings.HasPrefix(o.label, "sub/") && o.isDir:
			hasDir = true
		case o.label == "note.txt" && !o.isDir:
			hasFile = true
		}
	}
	if !hasParent || !hasDir || !hasFile {
		t.Errorf("picker options = %+v, want parent+dir+file", m.dialogOptions)
	}
}

func TestExportMarkdown(t *testing.T) {
	history := []llm.Message{
		llm.SystemText("system"),
		llm.UserText("hi"),
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{
			llm.Text{Text: "hello"},
			llm.ToolCall{Name: "ls", Input: json.RawMessage(`{}`)},
		}},
		{Role: llm.RoleTool, Content: []llm.ContentPart{
			llm.ToolResult{Content: "a.txt"},
		}},
	}
	out := exportMarkdown("My chat", history)
	for _, want := range []string{"# My chat", "## You", "hi", "## Eonyx", "hello", "`ls`", "a.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "system") {
		t.Error("export should skip the system turn")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate(short) = %q", got)
	}
	if got := truncate(strings.Repeat("a", 20), 10); got != strings.Repeat("a", 10)+"…" {
		t.Errorf("truncate(long) = %q", got)
	}
}

func TestShellEscapeRecordsToHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := store.Create("s")
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "bash", Schema: json.RawMessage(`{}`),
		Handler: func(context.Context, json.RawMessage) (string, error) { return "hello", nil }})

	m := newTestModel()
	m.cfg = &config.Config{}
	m.store = store
	m.session = sess
	m.registry = reg
	m.history = []llm.Message{llm.SystemText("sys")}

	// Immediate effects: shows the command and marks the turn running.
	if cmd := m.runShellEscape("echo hello"); cmd == nil {
		t.Fatal("expected a command")
	}
	if !m.isRunning || m.items[len(m.items)-1].text != "! echo hello" {
		t.Fatalf("running=%v lastItem=%q", m.isRunning, m.items[len(m.items)-1].text)
	}

	// Result: output shown and recorded into history for the model (variant B).
	m.Update(shellResultMsg{cmd: "echo hello", output: "hello"})
	if m.isRunning {
		t.Error("turn should be finished")
	}
	rec := messageText(m.history[len(m.history)-1])
	if !strings.Contains(rec, "echo hello") || !strings.Contains(rec, "hello") {
		t.Errorf("history record = %q, want command and output", rec)
	}
}

func TestToolResultSummaryIsCompact(t *testing.T) {
	if got := toolResultSummary(toolCallItem{name: "read", done: true, result: "a\nb\nc\n"}); got != "3 lines" {
		t.Errorf("read summary = %q, want \"3 lines\"", got)
	}
	if got := toolResultSummary(toolCallItem{name: "ls", done: true, result: "x\ny"}); got != "2 results" {
		t.Errorf("ls summary = %q, want \"2 results\"", got)
	}
	// Any tool result is condensed to a single line — no content dump.
	if got := toolResultSummary(toolCallItem{name: "bash", done: true, result: "l1\nl2\nl3\nl4"}); strings.Contains(got, "\n") {
		t.Errorf("summary must be single line, got %q", got)
	}
}

func TestRenderDiffColoursAddsAndDeletes(t *testing.T) {
	result := "edited main.go (1 replacement(s), +1 -1)\n\n" +
		"--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,2 @@\n ctx\n-old line\n+new line\n"
	out := renderDiff(result, false)
	if out == "" {
		t.Fatal("expected a rendered diff body")
	}
	// File headers are dropped; the summary and hunk header survive.
	if strings.Contains(out, "--- a/") || strings.Contains(out, "+++ b/") {
		t.Errorf("diff headers should be hidden, got:\n%s", out)
	}
	for _, want := range []string{"edited main.go", "@@"} {
		if !strings.Contains(out, want) {
			t.Errorf("diff missing %q in:\n%s", want, out)
		}
	}
	// The single-line replacement is word-diffed: only the changed span ("old" →
	// "new") gets the inverted highlight, while the shared " line" suffix keeps
	// the base +/- colour.
	if !strings.Contains(out, diffDelWordStyle.Render("old")) {
		t.Errorf("deleted word not highlighted:\n%s", out)
	}
	if !strings.Contains(out, diffAddWordStyle.Render("new")) {
		t.Errorf("added word not highlighted:\n%s", out)
	}
	if !strings.Contains(out, diffDelStyle.Render(" line")) || !strings.Contains(out, diffAddStyle.Render(" line")) {
		t.Errorf("shared suffix should keep the base colour:\n%s", out)
	}
}

func TestRenderDiffEmptyWithoutBody(t *testing.T) {
	if got := renderDiff("wrote 12 bytes to a.txt (new file, 1 lines)", false); got != "" {
		t.Errorf("renderDiff without a body = %q, want empty", got)
	}
}

func TestToolArgExtractsPath(t *testing.T) {
	tc := toolCallItem{name: "edit", input: `{"path":"cmd/run.go","old_string":"a","new_string":"b"}`}
	if got := toolArg(tc); got != "cmd/run.go" {
		t.Errorf("toolArg = %q, want cmd/run.go", got)
	}
}

func TestFooterStatusShowsAgentState(t *testing.T) {
	m := newTestModel()
	if !strings.Contains(m.footerStatus(), "Enter send") {
		t.Errorf("idle footer should show key hints, got %q", m.footerStatus())
	}
	m.isRunning = true
	m.runStart = time.Now()
	if got := m.footerStatus(); !strings.Contains(got, "Thinking") || !strings.Contains(got, "interrupt") {
		t.Errorf("running footer = %q, want Thinking + interrupt", got)
	}
	m.activeTool = "bash"
	if got := m.footerStatus(); !strings.Contains(got, "Running bash") {
		t.Errorf("footer with active tool = %q, want Running bash", got)
	}
	m.step, m.stepMax = 3, 50
	if got := m.footerStatus(); !strings.Contains(got, "3/50") {
		t.Errorf("footer with step counter = %q, want 3/50", got)
	}
}

func TestStatusHint(t *testing.T) {
	hint := statusHint()
	if !strings.Contains(hint, "Enter send") {
		t.Errorf("hint = %q, want Enter send", hint)
	}
}

func TestEnterSubmitsAndClearsInput(t *testing.T) {
	m := newTestModel()
	m.agent = agent.New(nil, tools.NewRegistry(), agent.Options{})
	m.textarea.SetValue("hello")
	consumed, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !consumed || cmd == nil {
		t.Fatalf("Enter should submit: consumed=%v cmd=%v", consumed, cmd)
	}
	if m.textarea.Value() != "" {
		t.Errorf("input not cleared: %q", m.textarea.Value())
	}
	if !m.isRunning {
		t.Error("expected isRunning after submit")
	}
}

func TestStylesRender(t *testing.T) {
	for name, style := range map[string]lipgloss.Style{
		"user":      userStyle,
		"assistant": assistantStyle,
		"tool":      toolStyle,
		"error":     errorStyle,
		"header":    headerStyle,
		"dialog":    dialogStyle,
	} {
		out := style.Render(name)
		if out == "" {
			t.Errorf("style %s produced empty output", name)
		}
	}
}
