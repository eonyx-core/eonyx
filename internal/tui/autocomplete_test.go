package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSlashAcceptCompletesCommand(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("/ne")
	m.updateAutocomplete()
	if !m.acActive {
		t.Fatal("expected completion for /ne")
	}
	m.acceptAutocomplete()
	if got := m.textarea.Value(); got != "/new " {
		t.Errorf("accepted value = %q, want %q", got, "/new ")
	}
}

func TestFileMentionCompletion(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestModel()
	m.workDir = dir

	m.textarea.SetValue("look at @mai")
	m.updateAutocomplete()
	if !m.acActive || m.acKind != acFile {
		t.Fatalf("file completion inactive: active=%v kind=%v", m.acActive, m.acKind)
	}
	m.acceptAutocomplete()
	if got := m.textarea.Value(); got != "look at @main.go " {
		t.Errorf("accepted file = %q, want %q", got, "look at @main.go ")
	}

	// A directory match completes with a trailing slash and no space, so the
	// user can descend into it.
	m.textarea.SetValue("see @inter")
	m.updateAutocomplete()
	m.acceptAutocomplete()
	if got := m.textarea.Value(); got != "see @internal/" {
		t.Errorf("accepted dir = %q, want %q", got, "see @internal/")
	}
}

func TestAutocompleteMenuResizesViewport(t *testing.T) {
	m := newTestModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	base := m.viewport.Height

	m.textarea.SetValue("/m")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if !m.acActive {
		t.Fatal("expected completion menu for /mo")
	}
	want := base - m.acExtraRows()
	if m.viewport.Height != want {
		t.Errorf("viewport height with menu = %d, want %d", m.viewport.Height, want)
	}
	// The menu renders one row per candidate plus a hint line.
	if got := strings.Count(m.renderAutocompleteMenu(), "\n") + 1; got != m.acExtraRows()+1 {
		t.Errorf("menu rows = %d, want %d", got, m.acExtraRows()+1)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.acActive {
		t.Fatal("esc should dismiss the menu")
	}
	if m.viewport.Height != base {
		t.Errorf("viewport height after dismiss = %d, want %d", m.viewport.Height, base)
	}
}

func TestAutocompleteSelectionWraps(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("/")
	m.updateAutocomplete()
	if !m.acActive || len(m.acItems) < 2 {
		t.Fatalf("expected several command completions, got %d", len(m.acItems))
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.acCursor != len(m.acItems)-1 {
		t.Errorf("cursor = %d, want last item after up from first", m.acCursor)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.acCursor != 0 {
		t.Errorf("cursor = %d, want 0 after down from last", m.acCursor)
	}
}

func TestAutocompleteNavAndDismiss(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("/")
	m.updateAutocomplete()
	if !m.acActive || len(m.acItems) < 2 {
		t.Fatalf("expected several command completions, got %d", len(m.acItems))
	}
	// Arrow keys are consumed by the completion bar, not history/textarea.
	if consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown}); !consumed {
		t.Error("down should be consumed while completing")
	}
	if m.acCursor != 1 {
		t.Errorf("cursor = %d, want 1 after one down", m.acCursor)
	}
	if consumed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc}); !consumed || m.acActive {
		t.Errorf("esc should dismiss completion: consumed=%v active=%v", consumed, m.acActive)
	}
}
