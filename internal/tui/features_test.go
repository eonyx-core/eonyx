package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputHistoryRecall(t *testing.T) {
	m := newTestModel()
	m.recordHistory("first")
	m.recordHistory("second")

	up := func() { m.handleKey(tea.KeyMsg{Type: tea.KeyUp}) }
	down := func() { m.handleKey(tea.KeyMsg{Type: tea.KeyDown}) }

	up()
	if got := m.textarea.Value(); got != "second" {
		t.Fatalf("after 1st up = %q, want second", got)
	}
	up()
	if got := m.textarea.Value(); got != "first" {
		t.Fatalf("after 2nd up = %q, want first", got)
	}
	up() // already oldest: stays put
	if got := m.textarea.Value(); got != "first" {
		t.Fatalf("after 3rd up = %q, want first (clamped)", got)
	}
	down()
	if got := m.textarea.Value(); got != "second" {
		t.Fatalf("after down = %q, want second", got)
	}
	down() // back to the (empty) draft
	if got := m.textarea.Value(); got != "" {
		t.Fatalf("after down to draft = %q, want empty", got)
	}
}

func TestInputHistorySkipsConsecutiveDuplicates(t *testing.T) {
	m := newTestModel()
	m.recordHistory("same")
	m.recordHistory("same")
	if len(m.inputHistory) != 1 {
		t.Errorf("history = %v, want a single entry", m.inputHistory)
	}
}

func TestUpDeclinedWithoutHistory(t *testing.T) {
	m := newTestModel()
	if consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp}); consumed {
		t.Error("up with no history should not be consumed (textarea handles it)")
	}
}

func TestQuestionMarkOpensHelpWhenEmpty(t *testing.T) {
	m := newTestModel()
	consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !consumed || m.dialog != dialogHelp {
		t.Errorf("'?' on empty input should open help: consumed=%v dialog=%d", consumed, m.dialog)
	}
}

func TestQuestionMarkLiteralMidText(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("why ")
	consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if consumed || m.dialog != dialogNone {
		t.Errorf("'?' mid-text should be literal: consumed=%v dialog=%d", consumed, m.dialog)
	}
}

func TestSlashHelpOpensDialog(t *testing.T) {
	m := newTestModel()
	m.runSlashCommand("/help")
	if m.dialog != dialogHelp {
		t.Errorf("/help should open the help dialog, dialog=%d", m.dialog)
	}
}

func TestHelpDialogAnyKeyCloses(t *testing.T) {
	m := newTestModel()
	m.openHelp()
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.dialog != dialogNone {
		t.Errorf("any key should close help, dialog=%d", m.dialog)
	}
}

func TestScrollbarBlankWhenContentFits(t *testing.T) {
	m := newTestModel()
	m.viewport.Height = 5
	m.viewport.Width = 20
	m.viewport.SetContent("one\ntwo")
	bar := m.renderScrollbar()
	// Fits: five blank rows, no thumb glyph.
	if bar == "" {
		t.Fatal("scrollbar should occupy the reserved column")
	}
	for _, r := range bar {
		if r == '█' {
			t.Errorf("no thumb expected when content fits, got %q", bar)
			break
		}
	}
}

func TestScrollbarHasThumbWhenOverflowing(t *testing.T) {
	m := newTestModel()
	m.viewport.Height = 3
	m.viewport.Width = 20
	m.viewport.SetContent("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10")
	bar := m.renderScrollbar()
	found := false
	for _, r := range bar {
		if r == '█' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a thumb glyph when content overflows, got %q", bar)
	}
}
