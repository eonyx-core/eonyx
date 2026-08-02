package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"eonyx/internal/config"
	"eonyx/internal/llm"
	"eonyx/internal/provider"
)

// runSlashCommand interprets a "/command" typed in the input box instead of
// sending it to the model. Unknown commands are reported in the transcript.
func (m *Model) runSlashCommand(input string) tea.Cmd {
	fields := strings.Fields(input)
	switch strings.ToLower(fields[0]) {
	case "/clear":
		m.clearChat()
	case "/new":
		m.newChat()
	case "/add", "/attach":
		m.startAttach(strings.TrimSpace(input[len(fields[0]):]))
	case "/ip":
		return m.runIP()
	case "/rename":
		m.renameSession(strings.TrimSpace(input[len(fields[0]):]))
	case "/fork":
		m.forkSession(m.session.ID, strings.TrimSpace(input[len(fields[0]):]))
	case "/export":
		m.exportSession()
	case "/yolo":
		m.toggleYolo()
	case "/plan":
		m.togglePlan()
	case "/timestamps", "/time":
		m.toggleTimestamps()
	case "/model":
		m.openModelPicker()
	case "/sessions", "/session":
		if q := strings.TrimSpace(input[len(fields[0]):]); q != "" {
			m.openSessionSearch(q)
		} else {
			m.openSessionPicker()
		}
	case "/quit", "/exit":
		return tea.Quit
	case "/help":
		m.openHelp()
	default:
		m.systemNote(fmt.Sprintf("Unknown command: %s", input))
	}
	return nil
}

// runShellEscape runs a "!command" directly in the agent's shell session and,
// per variant B, records the command and its output into the conversation so the
// model stays in sync with the shell state.
func (m *Model) runShellEscape(cmd string) tea.Cmd {
	if cmd == "" {
		return nil
	}
	bash, ok := m.registry.Get("bash")
	if !ok {
		m.systemNote("shell is not available")
		return nil
	}
	m.items = append(m.items, messageItem{role: llm.RoleUser, text: "! " + cmd, done: true, createdAt: time.Now()})
	m.isRunning = true
	m.activeTool = "shell"
	m.step, m.stepMax = 0, 0
	m.streamChars = 0
	m.runStart = time.Now()
	m.renderViewport()

	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	input, _ := json.Marshal(struct {
		Command string `json:"command"`
	}{cmd})
	run := func() tea.Msg {
		defer cancel()
		out, err := bash.Handler(ctx, input)
		return shellResultMsg{cmd: cmd, output: out, err: err}
	}
	return tea.Batch(run, m.spin.Tick)
}

// maxAttachBytes caps the size of a file attached via /add.
const maxAttachBytes = 256 << 10

// startAttach attaches a file directly when arg names one, or opens the file
// picker (rooted at arg if it is a directory, else the working directory).
func (m *Model) startAttach(arg string) {
	if arg == "" {
		m.openFilePicker(m.workDir)
		return
	}
	full := m.resolvePath(arg)
	fi, err := os.Stat(full)
	if err != nil {
		m.fail(err)
		return
	}
	if fi.IsDir() {
		m.openFilePicker(full)
		return
	}
	m.attachFile(full)
}

// runIP looks up the machine's public IPv4 along with its city and country via
// ip-api.com and shows the result as a system note — without going through the
// model or the shell.
func (m *Model) runIP() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://ip-api.com/json/?fields=status,message,query,city,regionName,country,isp", nil)
		if err != nil {
			return ipResultMsg{err: err}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return ipResultMsg{err: err}
		}
		defer resp.Body.Close()

		var r struct {
			Status     string `json:"status"`
			Message    string `json:"message"`
			Query      string `json:"query"`
			City       string `json:"city"`
			RegionName string `json:"regionName"`
			Country    string `json:"country"`
			ISP        string `json:"isp"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return ipResultMsg{err: err}
		}
		if r.Status != "success" {
			msg := r.Message
			if msg == "" {
				msg = "lookup failed"
			}
			return ipResultMsg{err: fmt.Errorf("ip lookup: %s", msg)}
		}

		location := strings.Join(nonEmpty(r.City, r.RegionName, r.Country), ", ")
		parts := nonEmpty(r.Query, location, r.ISP)
		return ipResultMsg{info: strings.Join(parts, " · ")}
	}
}

// nonEmpty returns the arguments that are not empty strings.
func nonEmpty(vals ...string) []string {
	out := vals[:0:0]
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// resolvePath resolves p against the working directory unless it is absolute.
func (m *Model) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(m.workDir, p)
}

// openFilePicker shows a browsable listing of dir for attaching a file. The
// entries are kept unfiltered in pickerAll; typing narrows the visible list.
func (m *Model) openFilePicker(dir string) {
	m.pickerDir = dir
	m.pickerFilter = ""
	var all []dialogOption
	if parent := filepath.Dir(dir); parent != dir {
		all = append(all, dialogOption{label: "../", value: parent, isDir: true})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.fail(err)
		return
	}
	var dirs, files []dialogOption
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			label := name + "/"
			if n, ok := dirEntryCount(full); ok {
				label += fmt.Sprintf(" (%d)", n)
			}
			dirs = append(dirs, dialogOption{label: label, value: full, isDir: true})
		} else {
			files = append(files, dialogOption{label: name, value: full})
		}
	}
	m.pickerAll = append(append(all, dirs...), files...)
	m.applyPickerFilter()
	m.openDialog(dialogFilePicker)
}

// dirEntryCount returns the number of non-hidden entries in a directory, so the
// picker can annotate a folder with how many items it holds. It reports false
// when the directory cannot be read (e.g. a permission error).
func dirEntryCount(path string) (int, bool) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, false
	}
	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n, true
}

// applyPickerFilter rebuilds the visible picker options from pickerAll, keeping
// entries whose label contains the (case-insensitive) filter.
func (m *Model) applyPickerFilter() {
	f := strings.ToLower(m.pickerFilter)
	var opts []dialogOption
	for _, o := range m.pickerAll {
		if f == "" || strings.Contains(strings.ToLower(o.label), f) {
			opts = append(opts, o)
		}
	}
	m.dialogOptions = opts
	if m.dialogCursor >= len(opts) {
		m.dialogCursor = 0
	}
}

// attachFile reads a file and adds it to the conversation as context.
func (m *Model) attachFile(full string) {
	data, err := os.ReadFile(full)
	if err != nil {
		m.fail(err)
		return
	}
	truncated := false
	if len(data) > maxAttachBytes {
		data = data[:maxAttachBytes]
		truncated = true
	}
	rel, err := filepath.Rel(m.workDir, full)
	if err != nil {
		rel = full
	}
	content := fmt.Sprintf("Attached file %s:\n```\n%s\n```", rel, string(data))
	if truncated {
		content += "\n(truncated)"
	}
	msg := llm.UserText(content)
	if err := m.store.Append(m.session.ID, msg); err != nil {
		m.fail(err)
		return
	}
	m.history = append(m.history, msg)
	m.items = append(m.items, messageItem{role: llm.RoleUser, text: "⧉ attached " + rel, done: true, createdAt: time.Now()})
	m.rebuildAgent()
	m.renderViewport()
}

// toggleYolo flips automatic approval of every tool for the session.
func (m *Model) toggleYolo() {
	m.yolo = !m.yolo
	m.rebuildAgent()
	if m.yolo {
		m.systemNote("yolo on: all tools run without permission checks")
	} else {
		m.systemNote("yolo off: you'll be asked before other tools run")
	}
}

// togglePlan flips plan mode: a read-only "think first" phase where the agent
// may research and propose a plan but every writing or command tool is blocked
// until the user exits plan mode to approve it.
func (m *Model) togglePlan() {
	m.planMode = !m.planMode
	m.rebuildAgent()
	if m.planMode {
		m.systemNote("plan mode on: I'll research and propose a plan; edits and commands are blocked. When a plan is ready you'll be asked to approve it (or run /plan to exit).")
	} else {
		m.systemNote("plan mode off: edits and commands enabled.")
	}
}

// toggleTimestamps flips the per-message timestamps in the transcript.
func (m *Model) toggleTimestamps() {
	m.showTimestamps = !m.showTimestamps
	if m.showTimestamps {
		m.systemNote("Timestamps on")
	} else {
		m.systemNote("Timestamps off")
	}
}

// renameSession changes the current session's title in the store.
func (m *Model) renameSession(title string) {
	if title == "" {
		m.systemNote("Usage: /rename <new title>")
		return
	}
	if err := m.store.Rename(m.session.ID, title); err != nil {
		m.fail(err)
		return
	}
	m.session.Title = title
	m.title = title
	m.systemNote(fmt.Sprintf("Renamed session to %q", title))
}

// forkSession branches session id into a new session and switches to it,
// leaving the original untouched. An empty title derives one from the source
// session's title.
func (m *Model) forkSession(id, title string) {
	if title == "" {
		src, err := m.store.Get(id)
		if err != nil {
			m.fail(err)
			return
		}
		title = src.Title + " (fork)"
	}
	sess, err := m.store.Fork(id, title)
	if err != nil {
		m.fail(err)
		return
	}
	m.applySession(sess.ID)
	m.systemNote("Forked into a new session: " + title)
}

// prefillCommand puts a command into the input box ready to edit and send,
// used for commands that take an argument (e.g. /rename <title>).
func (m *Model) prefillCommand(cmd string) {
	m.textarea.SetValue(cmd)
	m.textarea.CursorEnd()
}

// exportSession writes the current conversation to a markdown file in the
// working directory.
func (m *Model) exportSession() {
	exportsDir := filepath.Join(config.ProjectDataDir(m.workDir), "exports")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		m.fail(err)
		return
	}
	path := filepath.Join(exportsDir, "eonyx-"+m.session.ID+".md")
	if err := os.WriteFile(path, []byte(exportMarkdown(m.title, m.history)), 0o644); err != nil {
		m.fail(err)
		return
	}
	m.systemNote("Exported conversation to " + path)
}

// deleteSession removes a session. If others remain it switches to the most
// recent remaining one (so sessions can be deleted in order) and reopens the
// picker; a fresh chat is started only when the last session is removed.
func (m *Model) deleteSession(id string) {
	if id == "" {
		return
	}
	wasCurrent := id == m.session.ID
	if err := m.store.Delete(id); err != nil {
		m.fail(err)
		return
	}
	list, err := m.store.List()
	if err != nil {
		m.fail(err)
		return
	}
	if len(list) == 0 {
		m.systemNote("Deleted the last session")
		m.newChat()
		return
	}
	if wasCurrent {
		m.applySession(list[0].ID)
	}
	m.openSessionPicker()
}

// exportMarkdown renders the conversation as markdown, skipping the system turn.
func exportMarkdown(title string, history []llm.Message) string {
	if title == "" {
		title = "eonyx session"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", title)
	for _, msg := range history {
		switch msg.Role {
		case llm.RoleUser:
			fmt.Fprintf(&b, "\n## You\n\n%s\n", messageText(msg))
		case llm.RoleAssistant:
			if txt := strings.TrimSpace(messageText(msg)); txt != "" {
				fmt.Fprintf(&b, "\n## Eonyx\n\n%s\n", txt)
			}
			for _, part := range msg.Content {
				if tc, ok := part.(llm.ToolCall); ok {
					fmt.Fprintf(&b, "\n- **tool** `%s` %s\n", tc.Name, string(tc.Input))
				}
			}
		case llm.RoleTool:
			for _, part := range msg.Content {
				if res, ok := part.(llm.ToolResult); ok {
					fmt.Fprintf(&b, "  ↳ %s\n", strings.TrimSpace(res.Content))
				}
			}
		}
	}
	return b.String()
}

// applyModel switches the active model, rebinding the provider and agent.
func (m *Model) applyModel(id string) {
	if id == m.modelID {
		return
	}
	prov, err := provider.For(m.cfg, id, nil)
	if err != nil {
		m.fail(err)
		return
	}
	m.prov = prov
	m.modelID = id
	m.rebuildAgent()
}

// applySession loads another stored conversation into the view.
func (m *Model) applySession(id string) {
	if id == m.session.ID {
		return
	}
	sess, err := m.store.Get(id)
	if err != nil {
		m.fail(err)
		return
	}
	history, err := m.store.Messages(id)
	if err != nil {
		m.fail(err)
		return
	}
	m.session = sess
	m.history = history
	m.title = sess.Title
	m.items = itemsFromHistory(history)
	m.inTokens, m.outTokens = 0, 0
	m.cost = 0
	m.rebuildAgent()
	m.renderViewport()
}

// clearChat resets the current session's context: it discards the stored
// messages and the transcript, keeping the same session id and title, then
// reseeds the system prompt. Unlike newChat it does not create a new session.
func (m *Model) clearChat() {
	if err := m.store.Reset(m.session.ID); err != nil {
		m.fail(err)
		return
	}
	sysMsg := llm.SystemText(systemPrompt(m.workDir, m.cfg.ProjectRoot(), m.activeSkillSet()))
	if err := m.store.Append(m.session.ID, sysMsg); err != nil {
		m.fail(err)
		return
	}
	m.history = []llm.Message{sysMsg}
	m.items = nil
	m.inTokens, m.outTokens = 0, 0
	m.cost = 0
	m.rebuildAgent()
	m.renderViewport()
	m.systemNote("Context cleared.")
}

// newChat starts a fresh session seeded with the system prompt.
func (m *Model) newChat() {
	sess, err := m.store.Create(nextSessionTitle(m.store))
	if err != nil {
		m.fail(err)
		return
	}
	sysMsg := llm.SystemText(systemPrompt(m.workDir, m.cfg.ProjectRoot(), m.activeSkillSet()))
	if err := m.store.Append(sess.ID, sysMsg); err != nil {
		m.fail(err)
		return
	}
	m.session = sess
	m.history = []llm.Message{sysMsg}
	m.title = sess.Title
	m.items = nil
	m.inTokens, m.outTokens = 0, 0
	m.cost = 0
	m.rebuildAgent()
	m.renderViewport()
}
