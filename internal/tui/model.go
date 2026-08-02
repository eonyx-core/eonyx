package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/eonyx-core/eonyx/internal/agent"
	"github.com/eonyx-core/eonyx/internal/config"
	"github.com/eonyx-core/eonyx/internal/git"
	"github.com/eonyx-core/eonyx/internal/hooks"
	"github.com/eonyx-core/eonyx/internal/llm"
	"github.com/eonyx-core/eonyx/internal/permission"
	"github.com/eonyx-core/eonyx/internal/session"
	"github.com/eonyx-core/eonyx/internal/skills"
	"github.com/eonyx-core/eonyx/internal/tools"
)

// nextSessionTitle returns a sequential numeric title ("1", "2", …) for a new
// session, one past the highest existing numeric or legacy "Session N" title.
// Titles are never changed automatically afterwards — only an explicit /rename
// does that.
func nextSessionTitle(store session.Store) string {
	list, _ := store.List()
	highest := 0
	for _, s := range list {
		var n int
		if _, err := fmt.Sscanf(s.Title, "Session %d", &n); err == nil && n > highest {
			highest = n
		}
		if n, err := strconv.Atoi(s.Title); err == nil && n > highest {
			highest = n
		}
	}
	return strconv.Itoa(highest + 1)
}

// Model is the Bubble Tea model for the interactive chat TUI.
type Model struct {
	// Services.
	cfg        *config.Config
	prov       llm.Provider
	store      session.Store
	registry   *tools.Registry
	workDir    string
	ctx        context.Context
	hooks      *hooks.Engine
	skillSet   []*skills.Skill
	mcpServers []string
	branch     string

	// Agent state.
	agent   *agent.Agent
	session session.Session
	history []llm.Message
	title   string
	modelID string

	// deleteTarget is the session id pending deletion confirmation.
	deleteTarget string

	// Interactive permission state: session-wide "allow always" grants, and the
	// prompt currently awaiting the user's decision.
	sessionAllowed map[string]bool
	permPending    *permissionRequestMsg
	permName       string // tool of the active permission prompt (for rendering)
	permArg        string // argument summary shown in the prompt

	// Capability toggles.
	activeSkills map[string]bool
	activeMCP    map[string]bool

	// UI components.
	viewport viewport.Model
	textarea textarea.Model
	spin     spinner.Model

	// Input history: previously submitted inputs (oldest first) and the browse
	// cursor. historyIdx == len(inputHistory) means "not browsing"; historyDraft
	// holds the in-progress line saved when browsing starts.
	inputHistory []string
	historyIdx   int
	historyDraft string

	// Inline autocomplete for "/" commands and "@" file mentions in the input.
	acActive bool
	acKind   acKind
	acItems  []acItem
	acCursor int

	// Progress state while a turn is running.
	runStart   time.Time
	activeTool string
	// step is the current tool round-trip (1-based) and stepMax the configured
	// limit, shown as "step/limit" in the footer while a turn runs.
	step    int
	stepMax int
	// streamChars counts assistant characters streamed in the current turn, used
	// to show a rough live tokens/sec rate in the footer.
	streamChars int

	// blinkClosed winks the welcome robot's eye while the transcript is empty.
	blinkClosed bool

	// Runtime state.
	width, height       int
	headerHeight        int
	footerHeight        int
	isRunning           bool
	err                 error
	dialog              dialogState
	dialogCursor        int
	dialogOptions       []dialogOption
	dialogSection       int // tab index: capabilities (skills/mcp) and command palette (work/system)
	pickerDir           string
	pickerAll           []dialogOption // unfiltered entries of the current picker dir
	pickerFilter        string         // type-to-filter query in the file picker
	yolo                bool
	planMode            bool
	showTimestamps      bool
	expandTools         bool
	reasoningExpanded   bool
	cancel              context.CancelFunc
	program             atomic.Pointer[tea.Program]
	inTokens, outTokens int
	cachedTokens        int
	cost                float64

	// Rendered conversation.
	items []messageItem
}

// dialogOption is one selectable row in a modal dialog. value carries the id or
// action the selection maps to; isDir marks a directory entry in the file
// picker so selecting it navigates rather than attaches.
type dialogOption struct {
	label string
	value string
	isDir bool
}

type messageItem struct {
	role      llm.Role
	text      string
	reasoning string // a reasoning model's thinking, shown as a collapsible block
	toolCalls []toolCallItem
	done      bool
	// createdAt is when the turn was added, shown as a subtle timestamp when
	// timestamps are toggled on. Zero for turns restored from history.
	createdAt time.Time
	// isErr marks a system line as an error (red) rather than an info note.
	isErr bool
	// rendered caches the markdown-rendered body of a finished assistant turn;
	// renderedWidth records the content width it was rendered at, so a terminal
	// resize invalidates it.
	rendered      string
	renderedWidth int
}

type toolCallItem struct {
	name    string
	input   string
	result  string
	isError bool
	done    bool
}

// newModel builds the initial TUI model. It creates a fresh session seeded with
// the system prompt or resumes the most recent session.
func newModel(ctx context.Context, cfg *config.Config, prov llm.Provider, store session.Store, reg *tools.Registry, wd string, skillSet []*skills.Skill, hk *hooks.Engine, mcpServers []string) (*Model, error) {
	ta := textarea.New()
	ta.Placeholder = "Ask eonyx..."
	ta.Focus()
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	// Free Enter for sending; newlines are inserted with Ctrl+J. Alt+Enter is
	// deliberately not bound: Windows terminals intercept it to toggle
	// fullscreen, and many terminals do not deliver a distinct Ctrl+Enter.
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j")

	vp := viewport.New(0, 0)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(accentColor)

	m := &Model{
		cfg:          cfg,
		prov:         prov,
		store:        store,
		registry:     reg,
		workDir:      wd,
		ctx:          ctx,
		hooks:        hk,
		skillSet:     skillSet,
		mcpServers:   mcpServers,
		branch:       git.Branch(wd),
		textarea:     ta,
		viewport:     vp,
		spin:         sp,
		modelID:      cfg.DefaultModel,
		headerHeight: 3,
		// rule + 3-line textarea + status line.
		footerHeight: 5,
	}

	// Drop sessions left empty by earlier runs (a fresh chat that was never used,
	// an abandoned /new) so they do not accumulate in the store and picker.
	if n, err := session.PruneEmpty(store, ""); err != nil {
		slog.Warn("prune empty sessions on startup", "err", err)
	} else if n > 0 {
		slog.Info("pruned empty sessions on startup", "count", n)
	}

	list, err := store.List()
	if err != nil {
		return nil, err
	}
	resumed := false
	if len(list) > 0 {
		history, mErr := store.Messages(list[0].ID)
		switch {
		case mErr == nil:
			m.session = list[0]
			m.history = history
			m.title = m.session.Title
			m.items = itemsFromHistory(m.history)
			resumed = true
		case errors.Is(mErr, session.ErrNotFound):
			// Stale index entry whose message file is gone: start fresh
			// rather than resuming a session that can never be appended to.
		default:
			return nil, mErr
		}
	}
	if !resumed {
		m.session, err = store.Create(nextSessionTitle(store))
		if err != nil {
			return nil, err
		}
		sysMsg := llm.SystemText(systemPrompt(wd, cfg.ProjectRoot(), skillSet))
		m.history = []llm.Message{sysMsg}
		if err := store.Append(m.session.ID, sysMsg); err != nil {
			return nil, err
		}
		m.title = m.session.Title
	}

	return m, nil
}

// itemsFromHistory rebuilds the displayed conversation from stored messages so a
// resumed session shows its prior turns. The leading system message is skipped.
func itemsFromHistory(history []llm.Message) []messageItem {
	var items []messageItem
	for _, msg := range history {
		switch msg.Role {
		case llm.RoleUser:
			items = append(items, messageItem{role: llm.RoleUser, text: messageText(msg), done: true})
		case llm.RoleAssistant:
			item := messageItem{role: llm.RoleAssistant, text: messageText(msg), done: true}
			for _, part := range msg.Content {
				if tc, ok := part.(llm.ToolCall); ok {
					item.toolCalls = append(item.toolCalls, toolCallItem{
						name:  tc.Name,
						input: string(tc.Input),
					})
				}
			}
			items = append(items, item)
		case llm.RoleTool:
			applyToolResults(items, msg)
		}
	}
	return items
}

// applyToolResults attaches a tool message's results to the matching tool calls
// of the most recent assistant item.
func applyToolResults(items []messageItem, msg llm.Message) {
	if len(items) == 0 {
		return
	}
	last := &items[len(items)-1]
	for _, part := range msg.Content {
		res, ok := part.(llm.ToolResult)
		if !ok {
			continue
		}
		for i := range last.toolCalls {
			if !last.toolCalls[i].done {
				last.toolCalls[i].result = res.Content
				last.toolCalls[i].isError = res.IsError
				last.toolCalls[i].done = true
				break
			}
		}
	}
}

// bodyIndent indents message body text so user and assistant turns line up
// under their labels.
const bodyIndent = "  "

// indentText prefixes every line of s with prefix.
func indentText(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}

// messageText concatenates the text parts of a message.
func messageText(msg llm.Message) string {
	var b strings.Builder
	for _, part := range msg.Content {
		if t, ok := part.(llm.Text); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// SetProgram injects the running tea.Program so agent callbacks can send
// messages back into the update loop. This must be called before Run.
func (m *Model) SetProgram(p *tea.Program) {
	m.program.Store(p)
	m.rebuildAgent()
}

// rebuildAgent reconstructs the agent with the current history and callbacks
// that forward events into the Bubble Tea update loop.
func (m *Model) rebuildAgent() {
	policy := permission.New(m.cfg.Permissions.AllowedTools, m.yolo)
	planMode := m.planMode
	sessionID := m.session.ID
	m.agent = agent.New(m.prov, m.registry, agent.Options{
		History:       m.history,
		ContextTokens: contextWindow(m.cfg, m.modelID),
		MaxSteps:      m.cfg.MaxSteps,
		Events: agent.Events{
			StepStart: func(step, maxSteps int) {
				if p := m.program.Load(); p != nil {
					p.Send(agentStepMsg{step, maxSteps})
				}
			},
			Text: func(delta string) {
				if p := m.program.Load(); p != nil {
					p.Send(agentDeltaMsg{delta})
				}
			},
			Reasoning: func(delta string) {
				if p := m.program.Load(); p != nil {
					p.Send(agentReasoningMsg{delta})
				}
			},
			ToolStart: func(name string, input []byte) {
				if p := m.program.Load(); p != nil {
					p.Send(agentToolStartMsg{name, input})
				}
			},
			ToolEnd: func(name, result string, isError bool) {
				if p := m.program.Load(); p != nil {
					p.Send(agentToolEndMsg{name, result, isError})
				}
			},
		},
		Authorize: func(name string, input []byte) error { return m.authorizeTool(policy, planMode, name, input) },
		Persist:   func(msg llm.Message) error { return m.store.Append(sessionID, msg) },
		Hooks:     m.hooks,
	})
}

// planAllowedTools are the tools permitted while plan mode is on: read-only
// exploration, research, and planning, but nothing that writes files or runs
// commands.
var planAllowedTools = map[string]bool{
	"read": true, "ls": true, "glob": true, "grep": true, "picker": true,
	"git_status": true, "git_diff": true, "git_log": true,
	"fetch": true, "task": true, "todo_write": true,
}

// authorizeTool decides whether a tool may run. In plan mode, only read-only
// planning tools are allowed, regardless of the allowlist or yolo. Otherwise
// statically-allowed tools (safe, allowlisted, or yolo) pass immediately, and
// anything else is escalated to an interactive prompt handled by the UI
// goroutine — this call then blocks until the decision arrives over a channel.
// It runs on the agent goroutine, so it touches no UI-owned state directly.
func (m *Model) authorizeTool(policy *permission.Policy, planMode bool, name string, input []byte) error {
	if planMode && !planAllowedTools[name] {
		return fmt.Errorf("plan mode is on: %q is blocked until you approve the plan — run /plan to exit plan mode and enable edits", name)
	}
	if policy.Authorize(name) == nil {
		return nil
	}
	p := m.program.Load()
	if p == nil {
		return policy.Authorize(name) // no UI to ask: keep the static denial
	}
	respond := make(chan permDecision, 1)
	p.Send(permissionRequestMsg{name: name, input: input, respond: respond})
	switch <-respond {
	case permAllowOnce, permAllowAlways:
		return nil
	default:
		return fmt.Errorf("permission denied for tool %q", name)
	}
}

// Init satisfies tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.blinkTick(), m.branchTick())
}

// blinkTick schedules the next eye-blink: the eye stays open a long beat and
// shut for a brief one, so the welcome robot winks every few seconds.
func (m *Model) blinkTick() tea.Cmd {
	d := 2500 * time.Millisecond
	if m.blinkClosed {
		d = 180 * time.Millisecond
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return blinkTickMsg{} })
}

// branchInterval is how often the header's git branch is re-read, so it tracks
// checkouts made during the session (by the agent, a !command, or another
// terminal) instead of showing the branch as it was at startup.
const branchInterval = 2 * time.Second

// branchTick schedules the next git-branch refresh.
func (m *Model) branchTick() tea.Cmd {
	return tea.Tick(branchInterval, func(time.Time) tea.Msg { return branchTickMsg{} })
}

// readBranch reads the current git branch off the UI goroutine (git is a
// subprocess) and reports it back as a branchMsg.
func (m *Model) readBranch() tea.Cmd {
	dir := m.workDir
	return func() tea.Msg { return branchMsg{branch: git.Branch(dir)} }
}

func (m *Model) estimateCost() float64 {
	if m.modelID == "" {
		return 0
	}
	mod, ok := m.cfg.Model(m.modelID)
	if !ok || (mod.InputPrice == 0 && mod.OutputPrice == 0) {
		return 0
	}
	return float64(m.inTokens)*mod.InputPrice/1e6 + float64(m.outTokens)*mod.OutputPrice/1e6
}

// submit starts the agent turn for the user's input.
func (m *Model) submit(input string) tea.Cmd {
	if strings.TrimSpace(input) == "" || m.agent == nil {
		return nil
	}
	m.items = append(m.items, messageItem{role: llm.RoleUser, text: input, done: true, createdAt: time.Now()})
	m.isRunning = true
	m.activeTool = ""
	m.step, m.stepMax = 0, 0
	m.streamChars = 0
	m.runStart = time.Now()

	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	agent := m.agent
	send := func() tea.Msg {
		defer cancel()
		final, err := agent.Send(ctx, input)
		if err != nil {
			return agentDoneMsg{err: err}
		}
		return agentDoneMsg{text: final}
	}
	// Run the agent and animate the spinner alongside it.
	return tea.Batch(send, m.spin.Tick)
}

// systemNote appends an informational line to the transcript.
func (m *Model) systemNote(text string) {
	m.items = append(m.items, messageItem{role: llm.RoleSystem, text: text, done: true, createdAt: time.Now()})
	m.renderViewport()
}

// fail records an error and surfaces it as a system message in the transcript.
func (m *Model) fail(err error) {
	m.err = err
	m.items = append(m.items, messageItem{role: llm.RoleSystem, text: fmt.Sprintf("%v", err), done: true, isErr: true, createdAt: time.Now()})
	m.renderViewport()
}

// finalizeAssistants marks every streaming assistant item of the turn complete.
func (m *Model) finalizeAssistants() {
	for i := range m.items {
		item := &m.items[i]
		if item.role == llm.RoleAssistant && !item.done {
			item.done = true
		}
	}
}
