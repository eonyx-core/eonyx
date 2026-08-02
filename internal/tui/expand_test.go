package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestToolOutputCollapseExpand(t *testing.T) {
	m := newTestModel()
	tc := toolCallItem{name: "read", input: `{"path":"a.txt"}`, result: "line1\nline2\nline3", done: true}

	collapsed := m.renderToolCall(tc)
	if !strings.Contains(collapsed, "3 lines") {
		t.Errorf("collapsed output should show a summary, got:\n%s", collapsed)
	}
	if strings.Contains(collapsed, "line2") {
		t.Errorf("collapsed output should hide the full text, got:\n%s", collapsed)
	}

	m.expandTools = true
	expanded := m.renderToolCall(tc)
	if !strings.Contains(expanded, "line2") || !strings.Contains(expanded, "line3") {
		t.Errorf("expanded output should show the full text, got:\n%s", expanded)
	}
}

func TestCtrlOTogglesExpand(t *testing.T) {
	m := newTestModel()
	if consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlO}); !consumed || !m.expandTools {
		t.Fatalf("ctrl+o should turn expansion on: consumed=%v expand=%v", consumed, m.expandTools)
	}
	if _, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlO}); m.expandTools {
		t.Error("ctrl+o should turn expansion off again")
	}
}
