package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilePickerShowsDirItemCount(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(sub, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newTestModel()
	m.workDir = dir
	m.openFilePicker(dir)

	var label string
	for _, o := range m.dialogOptions {
		if o.isDir && strings.HasPrefix(o.label, "pkg/") {
			label = o.label
		}
	}
	if label != "pkg/ (2)" {
		t.Errorf("directory label = %q, want %q", label, "pkg/ (2)")
	}
}

func TestFilePickerTypeToFilter(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"alpha.txt", "readme.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := newTestModel()
	m.workDir = dir
	m.openFilePicker(dir)

	// Typing narrows the list to matching labels.
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alph")})
	if m.pickerFilter != "alph" {
		t.Fatalf("filter = %q, want alph", m.pickerFilter)
	}
	if len(m.dialogOptions) != 1 || m.dialogOptions[0].label != "alpha.txt" {
		t.Errorf("filtered options = %+v, want only alpha.txt", m.dialogOptions)
	}

	// Backspace widens it again.
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.pickerFilter != "alp" {
		t.Errorf("filter after backspace = %q, want alp", m.pickerFilter)
	}
	if len(m.dialogOptions) != 1 {
		t.Errorf("options after backspace = %+v, want alpha.txt only", m.dialogOptions)
	}

	// A non-matching filter yields no entries (and does not crash).
	for range "alp" {
		m.handleDialogKey(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m.handleDialogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	if len(m.dialogOptions) != 0 {
		t.Errorf("no entries should match zzz, got %+v", m.dialogOptions)
	}

	// Closing the picker clears the filter state.
	m.closeDialog()
	if m.pickerFilter != "" || m.pickerAll != nil {
		t.Error("closeDialog should reset the picker filter state")
	}
}
