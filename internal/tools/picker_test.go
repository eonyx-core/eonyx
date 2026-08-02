package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickerListDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "zebra.txt"), "z")
	mustWrite(t, filepath.Join(dir, "alpha.txt"), "a")
	mustMkdir(t, filepath.Join(dir, "beta"))
	mustWrite(t, filepath.Join(dir, ".hidden"), "h")

	tool := newPickerTool(dir)
	out, err := tool.Handler(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("picker list: %v", err)
	}

	if !strings.Contains(out, "[dir] beta") {
		t.Errorf("expected dir marker for beta, got:\n%s", out)
	}
	if !strings.Contains(out, "  zebra.txt") || !strings.Contains(out, "  alpha.txt") {
		t.Errorf("expected file entries, got:\n%s", out)
	}
	if strings.Contains(out, ".hidden") {
		t.Errorf("hidden file should be skipped, got:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 output lines, got:\n%s", out)
	}
	if !strings.HasPrefix(lines[0], "[dir] beta") {
		t.Errorf("dirs should come first, got first line %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  alpha.txt") {
		t.Errorf("files should be sorted case-insensitively, got second line %q", lines[1])
	}

	jsonStart := strings.Index(out, "\n[")
	if jsonStart < 0 {
		t.Fatalf("expected JSON array in output, got:\n%s", out)
	}
	var items []pickerEntry
	if err := json.Unmarshal([]byte(out[jsonStart+1:]), &items); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(items))
	}
	if items[0].Name != "beta" || items[0].Type != "dir" {
		t.Errorf("unexpected first entry: %+v", items[0])
	}
}

func TestPickerSelectFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "target.txt")
	mustWrite(t, p, "hello")

	tool := newPickerTool(dir)
	out, err := tool.Handler(context.Background(), []byte(`{"select":"target.txt"}`))
	if err != nil {
		t.Fatalf("picker select: %v", err)
	}

	var e pickerEntry
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if e.Name != "target.txt" {
		t.Errorf("expected name target.txt, got %q", e.Name)
	}
	if e.Path != p {
		t.Errorf("expected path %q, got %q", p, e.Path)
	}
	if e.Type != "file" {
		t.Errorf("expected type file, got %q", e.Type)
	}
	if e.Size != 5 {
		t.Errorf("expected size 5, got %d", e.Size)
	}
	if e.Modified == 0 {
		t.Errorf("expected non-zero modified time")
	}
}

func TestPickerSelectMissing(t *testing.T) {
	dir := t.TempDir()
	tool := newPickerTool(dir)
	_, err := tool.Handler(context.Background(), []byte(`{"select":"missing.txt"}`))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPickerListMissing(t *testing.T) {
	dir := t.TempDir()
	tool := newPickerTool(dir)
	_, err := tool.Handler(context.Background(), []byte(`{"path":"no-such-dir"}`))
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
}
