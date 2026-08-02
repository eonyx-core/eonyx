package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReasoningAttachedAndCollapsible(t *testing.T) {
	m := newTestModel()
	m.width = 80 // wide enough that the short reasoning does not wrap
	m.Update(agentReasoningMsg{delta: "step-one then "})
	m.Update(agentReasoningMsg{delta: "step-two."})
	m.Update(agentDeltaMsg{delta: "The answer is 42."})

	last := &m.items[len(m.items)-1]
	if last.reasoning == "" || last.text == "" {
		t.Fatalf("reasoning and text should share the assistant item: %+v", *last)
	}

	// Collapsed by default: a hint, not the reasoning text.
	collapsed := m.renderItem(last)
	if !strings.Contains(collapsed, "thinking") {
		t.Errorf("collapsed block should show a thinking hint:\n%s", collapsed)
	}
	if strings.Contains(collapsed, "step-one") {
		t.Errorf("collapsed block should hide the reasoning text:\n%s", collapsed)
	}
	if !strings.Contains(collapsed, "The answer is 42.") {
		t.Errorf("the answer should still show:\n%s", collapsed)
	}

	// Expanded: the reasoning text is visible.
	m.reasoningExpanded = true
	expanded := m.renderItem(last)
	if !strings.Contains(expanded, "step-one") || !strings.Contains(expanded, "step-two") {
		t.Errorf("expanded block should show the reasoning:\n%s", expanded)
	}
}

func TestReasoningAbsentRendersNothing(t *testing.T) {
	m := newTestModel()
	m.Update(agentDeltaMsg{delta: "just an answer"})
	out := m.renderItem(&m.items[len(m.items)-1])
	if strings.Contains(out, "thinking") {
		t.Errorf("no thinking block should appear without reasoning:\n%s", out)
	}
}

func TestCtrlRTogglesReasoning(t *testing.T) {
	m := newTestModel()
	if consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR}); !consumed || !m.reasoningExpanded {
		t.Fatalf("ctrl+r should expand reasoning: consumed=%v expanded=%v", true, m.reasoningExpanded)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if m.reasoningExpanded {
		t.Error("ctrl+r should collapse reasoning again")
	}
}
