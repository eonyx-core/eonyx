package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDryRunDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new.txt")
	out, err := newWriteTool(dir).Handler(context.Background(),
		json.RawMessage(`{"path":"new.txt","content":"hi","dry_run":true}`))
	if err != nil {
		t.Fatalf("write dry_run: %v", err)
	}
	if !strings.Contains(out, "[dry run]") {
		t.Errorf("result should be marked dry run, got %q", out)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("dry-run write must not create the file")
	}
}

func TestEditDryRunKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := newEditTool(dir).Handler(context.Background(),
		json.RawMessage(`{"path":"f.txt","old_string":"alpha","new_string":"beta","dry_run":true}`))
	if err != nil {
		t.Fatalf("edit dry_run: %v", err)
	}
	if !strings.Contains(out, "[dry run]") {
		t.Errorf("result should be marked dry run, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "alpha" {
		t.Errorf("dry-run edit changed the file to %q", got)
	}
}

func TestMultiEditDryRunKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("a b"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := newMultiEditTool(dir).Handler(context.Background(),
		json.RawMessage(`{"path":"f.txt","dry_run":true,"edits":[{"old_string":"a","new_string":"x"},{"old_string":"b","new_string":"y"}]}`))
	if err != nil {
		t.Fatalf("multi_edit dry_run: %v", err)
	}
	if !strings.Contains(out, "[dry run]") {
		t.Errorf("result should be marked dry run, got %q", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "a b" {
		t.Errorf("dry-run multi_edit changed the file to %q", got)
	}
}
