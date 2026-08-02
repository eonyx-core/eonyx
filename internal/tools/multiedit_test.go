package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func runMultiEdit(t *testing.T, workdir, args string) (string, error) {
	t.Helper()
	tool := newMultiEditTool(workdir)
	return tool.Handler(context.Background(), json.RawMessage(args))
}

func TestMultiEditAppliesAllEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runMultiEdit(t, dir, `{"path":"f.txt","edits":[
		{"old_string":"alpha","new_string":"ALPHA"},
		{"old_string":"gamma","new_string":"GAMMA"}
	]}`)
	if err != nil {
		t.Fatalf("multi_edit: %v", err)
	}
	if out == "" {
		t.Error("expected a result summary")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "ALPHA beta GAMMA" {
		t.Errorf("content = %q, want %q", got, "ALPHA beta GAMMA")
	}
}

func TestMultiEditIsAtomicOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	original := "alpha beta gamma"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	// The second edit's old_string is absent, so nothing should be written.
	_, err := runMultiEdit(t, dir, `{"path":"f.txt","edits":[
		{"old_string":"alpha","new_string":"ALPHA"},
		{"old_string":"missing","new_string":"X"}
	]}`)
	if err == nil {
		t.Fatal("expected an error when an edit does not match")
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Errorf("file changed despite failure: %q", got)
	}
}

func TestMultiEditRejectsNonUniqueWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x x x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runMultiEdit(t, dir, `{"path":"f.txt","edits":[{"old_string":"x","new_string":"y"}]}`); err == nil {
		t.Error("expected an error for a non-unique old_string without replace_all")
	}
	out, err := runMultiEdit(t, dir, `{"path":"f.txt","edits":[{"old_string":"x","new_string":"y","replace_all":true}]}`)
	if err != nil {
		t.Fatalf("replace_all multi_edit: %v", err)
	}
	if out == "" {
		t.Error("expected a summary")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "y y y" {
		t.Errorf("content = %q, want %q", got, "y y y")
	}
}
