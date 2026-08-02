package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, tool Tool, input string) (string, error) {
	t.Helper()
	return tool.Handler(context.Background(), json.RawMessage(input))
}

func TestWriteReadEdit(t *testing.T) {
	dir := t.TempDir()

	if _, err := run(t, newWriteTool(dir), `{"path":"a/b.txt","content":"hello\nworld\n"}`); err != nil {
		t.Fatalf("write: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a", "b.txt")); string(data) != "hello\nworld\n" {
		t.Fatalf("file content = %q", data)
	}

	out, err := run(t, newReadTool(dir), `{"path":"a/b.txt"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "hello\nworld\n" {
		t.Errorf("read = %q", out)
	}

	out, err = run(t, newReadTool(dir), `{"path":"a/b.txt","offset":2,"limit":1}`)
	if err != nil {
		t.Fatalf("read offset: %v", err)
	}
	if out != "world" {
		t.Errorf("read offset = %q, want world", out)
	}

	if _, err := run(t, newEditTool(dir), `{"path":"a/b.txt","old_string":"world","new_string":"there"}`); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, _ = run(t, newReadTool(dir), `{"path":"a/b.txt"}`)
	if out != "hello\nthere\n" {
		t.Errorf("after edit = %q", out)
	}
}

func TestWriteEditReportDiff(t *testing.T) {
	dir := t.TempDir()

	out, err := run(t, newWriteTool(dir), `{"path":"f.txt","content":"a\nb\n"}`)
	if err != nil {
		t.Fatalf("write new: %v", err)
	}
	if !strings.Contains(out, "new file") {
		t.Errorf("new-file write summary = %q", out)
	}

	out, err = run(t, newWriteTool(dir), `{"path":"f.txt","content":"a\nB\n"}`)
	if err != nil {
		t.Fatalf("write change: %v", err)
	}
	if !strings.Contains(out, "+1 -1") || !strings.Contains(out, "-b") || !strings.Contains(out, "+B") {
		t.Errorf("overwrite summary/diff = %q", out)
	}

	out, _ = run(t, newWriteTool(dir), `{"path":"f.txt","content":"a\nB\n"}`)
	if !strings.Contains(out, "no changes") {
		t.Errorf("no-op write summary = %q", out)
	}

	out, err = run(t, newEditTool(dir), `{"path":"f.txt","old_string":"B","new_string":"c"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "+1 -1") || !strings.Contains(out, "@@") {
		t.Errorf("edit summary/diff = %q", out)
	}
}

func TestEditAmbiguous(t *testing.T) {
	dir := t.TempDir()
	_, _ = run(t, newWriteTool(dir), `{"path":"f.txt","content":"x x x"}`)

	if _, err := run(t, newEditTool(dir), `{"path":"f.txt","old_string":"x","new_string":"y"}`); err == nil {
		t.Fatal("expected ambiguous edit to error")
	}
	if _, err := run(t, newEditTool(dir), `{"path":"f.txt","old_string":"x","new_string":"y","replace_all":true}`); err != nil {
		t.Fatalf("replace_all: %v", err)
	}
	out, _ := run(t, newReadTool(dir), `{"path":"f.txt"}`)
	if out != "y y y" {
		t.Errorf("after replace_all = %q", out)
	}
}

func TestLS(t *testing.T) {
	dir := t.TempDir()
	_, _ = run(t, newWriteTool(dir), `{"path":"sub/x.txt","content":"1"}`)
	_, _ = run(t, newWriteTool(dir), `{"path":"y.txt","content":"2"}`)

	out, err := run(t, newLSTool(dir), `{}`)
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(out, "sub/") || !strings.Contains(out, "y.txt") {
		t.Errorf("ls = %q, want sub/ and y.txt", out)
	}
}
