package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/eonyx-core/eonyx/internal/llm"
)

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(0, msg.Width)
		m.height = max(0, msg.Height)
		m.viewport.Width = max(0, m.width-scrollbarWidth)
		m.viewport.Height = m.mainHeight()
		m.textarea.SetWidth(m.width)
		m.renderViewport()

	case tea.KeyMsg:
		consumed, cmd := m.handleKey(msg)
		if consumed {
			m.syncLayout()
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

	case agentDeltaMsg:
		// Text emitted after a tool call belongs below it: start a fresh assistant
		// item so the transcript stays chronological (text → tools → text), the way
		// the conversation actually unfolded.
		if len(m.items) == 0 || m.items[len(m.items)-1].role != llm.RoleAssistant || len(m.items[len(m.items)-1].toolCalls) > 0 {
			m.items = append(m.items, messageItem{role: llm.RoleAssistant, createdAt: time.Now()})
		}
		last := &m.items[len(m.items)-1]
		last.text += msg.delta
		m.streamChars += len(msg.delta)
		m.renderViewport()

	case agentReasoningMsg:
		// Reasoning streams before the answer; attach it to the current assistant
		// item, starting one if needed.
		if len(m.items) == 0 || m.items[len(m.items)-1].role != llm.RoleAssistant || len(m.items[len(m.items)-1].toolCalls) > 0 {
			m.items = append(m.items, messageItem{role: llm.RoleAssistant})
		}
		m.items[len(m.items)-1].reasoning += msg.delta
		m.renderViewport()

	case agentStepMsg:
		m.step, m.stepMax = msg.step, msg.maxSteps

	case permissionRequestMsg:
		// Auto-answer when the tool was already granted "always" this session.
		if m.sessionAllowed[msg.name] {
			msg.respond <- permAllowOnce
			return m, nil
		}
		m.openPermissionPrompt(msg)
		return m, nil

	case spinner.TickMsg:
		// Keep the spinner animating only while a turn is in flight.
		if !m.isRunning {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case agentToolStartMsg:
		m.activeTool = msg.name
		if len(m.items) == 0 || m.items[len(m.items)-1].role != llm.RoleAssistant {
			m.items = append(m.items, messageItem{role: llm.RoleAssistant, createdAt: time.Now()})
		}
		last := &m.items[len(m.items)-1]
		last.toolCalls = append(last.toolCalls, toolCallItem{
			name:  msg.name,
			input: string(msg.input),
		})
		m.renderViewport()

	case agentToolEndMsg:
		m.activeTool = ""
		if len(m.items) > 0 {
			last := &m.items[len(m.items)-1]
			for i := range last.toolCalls {
				if !last.toolCalls[i].done {
					last.toolCalls[i].result = msg.result
					last.toolCalls[i].isError = msg.isError
					last.toolCalls[i].done = true
					break
				}
			}
		}
		m.renderViewport()

	case shellResultMsg:
		m.isRunning = false
		m.cancel = nil
		m.activeTool = ""
		out, isErr := msg.output, false
		if msg.err != nil {
			out, isErr = msg.err.Error(), true
		}
		if strings.TrimSpace(out) == "" {
			out = "(no output)"
		}
		m.items = append(m.items, messageItem{role: llm.RoleSystem, text: out, done: true, isErr: isErr, createdAt: time.Now()})
		// Variant B: record the command and output into the conversation so the
		// model sees what was run and any cwd/env changes it caused.
		rec := llm.UserText(fmt.Sprintf("I ran a shell command:\n$ %s\n\nOutput:\n%s", msg.cmd, out))
		m.history = append(m.history, rec)
		if m.store != nil {
			if err := m.store.Append(m.session.ID, rec); err != nil {
				m.fail(err)
			}
		}
		m.rebuildAgent()
		m.renderViewport()

	case ipResultMsg:
		if msg.err != nil {
			m.fail(msg.err)
		} else {
			m.systemNote(msg.info)
		}

	case blinkTickMsg:
		m.blinkClosed = !m.blinkClosed
		if len(m.items) == 0 {
			m.renderViewport()
		}
		return m, m.blinkTick()

	case branchTickMsg:
		return m, tea.Batch(m.readBranch(), m.branchTick())

	case branchMsg:
		m.branch = msg.branch
		return m, nil

	case agentDoneMsg:
		m.isRunning = false
		m.cancel = nil
		m.activeTool = ""
		m.finalizeAssistants()
		switch {
		case errors.Is(msg.err, context.Canceled):
			m.systemNote("⏹ cancelled")
		case msg.err != nil:
			m.fail(msg.err)
		}
		if m.agent != nil {
			// Send has returned, so reading the agent here cannot race with it.
			m.inTokens, m.outTokens = m.agent.Usage()
			m.cachedTokens = m.agent.Cached()
			m.cost = m.estimateCost()
			m.history = m.agent.History()
			m.rebuildAgent()
		}
		m.renderViewport()
		// A completed plan-mode turn is a proposed plan: ask the user to approve.
		if m.planMode && msg.err == nil {
			m.openPlanApproval()
		}
	}

	// Forward to sub-models.
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Refresh inline autocomplete after any key that reached the input.
	if _, ok := msg.(tea.KeyMsg); ok {
		m.updateAutocomplete()
		m.syncLayout()
	}

	return m, tea.Batch(cmds...)
}

// mainHeight is the viewport height: the terminal minus the header, the
// footer, and any extra rows the open completion menu needs.
func (m *Model) mainHeight() int {
	return max(0, m.height-m.headerHeight-m.footerHeight-m.acExtraRows())
}

// syncLayout resizes the viewport when the completion menu opens, closes, or
// changes size, keeping the transcript pinned to the bottom when it was there.
func (m *Model) syncLayout() {
	h := m.mainHeight()
	if h == m.viewport.Height {
		return
	}
	atBottom := m.viewport.AtBottom()
	m.viewport.Height = h
	if atBottom {
		m.viewport.GotoBottom()
	}
}

// handleKey processes global keys. It returns true when the key was consumed
// and the command (if any) to execute.
func (m *Model) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return true, tea.Quit
	}
	if m.dialog != dialogNone {
		return m.handleDialogKey(msg)
	}
	if m.acActive {
		if consumed, cmd := m.handleAutocompleteKey(msg); consumed {
			return consumed, cmd
		}
	}

	switch msg.String() {
	case "enter", "ctrl+enter":
		input := strings.TrimSpace(m.textarea.Value())
		if m.isRunning || input == "" {
			return true, nil
		}
		m.textarea.SetValue("")
		m.updateAutocomplete()
		m.recordHistory(input)
		if strings.HasPrefix(input, "!") {
			return true, m.runShellEscape(strings.TrimSpace(input[1:]))
		}
		if strings.HasPrefix(input, "/") {
			return true, m.runSlashCommand(input)
		}
		return true, m.submit(input)
	case "up":
		if m.recallHistoryPrev() {
			return true, nil
		}
		return false, nil
	case "down":
		if m.recallHistoryNext() {
			return true, nil
		}
		return false, nil
	case "/":
		// On an empty prompt, "/" opens the command menu; mid-text it is a
		// literal character so paths and slashes can still be typed.
		if !m.isRunning && strings.TrimSpace(m.textarea.Value()) == "" {
			m.openCommandPalette()
			return true, nil
		}
		return false, nil
	case "?":
		// On an empty prompt, "?" opens the help screen; mid-text it is literal.
		if !m.isRunning && strings.TrimSpace(m.textarea.Value()) == "" {
			m.openHelp()
			return true, nil
		}
		return false, nil
	case "ctrl+p":
		if !m.isRunning {
			m.openCommandPalette()
		}
		return true, nil
	case "esc":
		if m.isRunning && m.cancel != nil {
			m.cancel()
		}
		return true, nil
	case "ctrl+n":
		if !m.isRunning {
			m.openModelPicker()
		}
		return true, nil
	case "ctrl+s":
		if !m.isRunning {
			m.openSessionPicker()
		}
		return true, nil
	case "ctrl+k":
		if !m.isRunning {
			m.openCapabilities()
		}
		return true, nil
	case "ctrl+o":
		// Toggle between collapsed tool-output summaries and their full text.
		m.expandTools = !m.expandTools
		m.renderViewport()
		return true, nil
	case "ctrl+r":
		// Expand or collapse the reasoning ("thinking") blocks.
		m.reasoningExpanded = !m.reasoningExpanded
		m.renderViewport()
		return true, nil
	}
	return false, nil
}

// handlePickerKey extends or trims the file picker's filter query for printable
// keys and backspace, re-narrowing the list. It leaves navigation keys (arrows,
// enter, esc) for the common dialog handler.
func (m *Model) handlePickerKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch {
	case msg.String() == "backspace":
		if r := []rune(m.pickerFilter); len(r) > 0 {
			m.pickerFilter = string(r[:len(r)-1])
			m.applyPickerFilter()
		}
		return true, nil
	case msg.Type == tea.KeyRunes:
		m.pickerFilter += string(msg.Runes)
		m.applyPickerFilter()
		return true, nil
	}
	return false, nil
}

// handleDialogKey processes keys while a modal dialog is open.
func (m *Model) handleDialogKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	// The help screen is not a list: any key dismisses it.
	if m.dialog == dialogHelp {
		m.closeDialog()
		return true, nil
	}
	// Dismissing the permission prompt (Esc) denies the tool, so the blocked
	// agent goroutine is always released.
	if m.dialog == dialogPermission && msg.String() == "esc" {
		m.resolvePermission(permDeny)
		return true, nil
	}
	// In the file picker, printable keys extend the type-to-filter query before
	// the list-navigation keys are considered.
	if m.dialog == dialogFilePicker {
		if consumed, cmd := m.handlePickerKey(msg); consumed {
			return consumed, cmd
		}
	}
	switch msg.String() {
	case "esc":
		m.closeDialog()
	case "up", "ctrl+p":
		if m.dialogCursor > 0 {
			m.dialogCursor--
		} else {
			m.dialogCursor = len(m.dialogOptions) - 1
		}
	case "down", "ctrl+n":
		if m.dialogCursor < len(m.dialogOptions)-1 {
			m.dialogCursor++
		} else {
			m.dialogCursor = 0
		}
	case "ctrl+d", "delete", "d":
		if m.dialog == dialogSessionPicker && m.dialogCursor >= 0 && m.dialogCursor < len(m.dialogOptions) {
			m.confirmDeleteSession(m.dialogOptions[m.dialogCursor].value)
		}
		return true, nil
	case "f":
		if m.dialog == dialogSessionPicker && m.dialogCursor >= 0 && m.dialogCursor < len(m.dialogOptions) {
			id := m.dialogOptions[m.dialogCursor].value
			m.closeDialog()
			m.forkSession(id, "")
		}
		return true, nil
	case "r":
		if m.dialog == dialogSessionPicker && m.dialogCursor >= 0 && m.dialogCursor < len(m.dialogOptions) {
			id := m.dialogOptions[m.dialogCursor].value
			m.closeDialog()
			sess, err := m.store.Get(id)
			title := ""
			if err == nil {
				title = sess.Title
			}
			m.prefillCommand("/rename " + title)
		}
		return true, nil
	case "tab", "right", "left":
		m.switchDialogSection()
		return true, nil
	case "enter":
		return true, m.selectDialogOption()
	}
	return true, nil
}
