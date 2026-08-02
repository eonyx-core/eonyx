package tui

import "strings"

// recordHistory appends a submitted input to the recall history (skipping an
// immediate duplicate) and resets the browse cursor to the end.
func (m *Model) recordHistory(input string) {
	if n := len(m.inputHistory); n == 0 || m.inputHistory[n-1] != input {
		m.inputHistory = append(m.inputHistory, input)
	}
	m.historyIdx = len(m.inputHistory)
	m.historyDraft = ""
}

// recallHistoryPrev loads the previous input into the textarea. It reports
// whether it consumed the key: it declines (so the textarea can move the cursor)
// when a turn is running, when there is no history, or when starting from a
// multi-line draft. Once browsing, it keeps handling ↑ regardless of newlines.
func (m *Model) recallHistoryPrev() bool {
	if m.isRunning || len(m.inputHistory) == 0 {
		return false
	}
	if m.historyIdx >= len(m.inputHistory) {
		if strings.Contains(m.textarea.Value(), "\n") {
			return false
		}
		m.historyDraft = m.textarea.Value()
		m.historyIdx = len(m.inputHistory)
	}
	if m.historyIdx == 0 {
		return true
	}
	m.historyIdx--
	m.textarea.SetValue(m.inputHistory[m.historyIdx])
	m.textarea.CursorEnd()
	return true
}

// recallHistoryNext moves forward through history toward the saved draft. It
// only handles ↓ while browsing; otherwise it declines so the textarea can move
// the cursor.
func (m *Model) recallHistoryNext() bool {
	if m.isRunning || m.historyIdx >= len(m.inputHistory) {
		return false
	}
	m.historyIdx++
	if m.historyIdx >= len(m.inputHistory) {
		m.historyIdx = len(m.inputHistory)
		m.textarea.SetValue(m.historyDraft)
	} else {
		m.textarea.SetValue(m.inputHistory[m.historyIdx])
	}
	m.textarea.CursorEnd()
	return true
}
