package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSandboxConfinesPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "in.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := sandboxed(root, root, newReadTool(root))
	ctx := context.Background()

	if _, err := read.Handler(ctx, json.RawMessage(`{"path":"in.txt"}`)); err != nil {
		t.Errorf("reading inside the sandbox should work: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Handler(ctx, json.RawMessage(fmt.Sprintf(`{"path":%q}`, outside))); err == nil {
		t.Error("reading an absolute path outside the sandbox should be refused")
	}
	if _, err := read.Handler(ctx, json.RawMessage(`{"path":"../escape.txt"}`)); err == nil {
		t.Error("parent-traversal path should be refused")
	}
}

func TestSandboxDisabledIsPassThrough(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "ok.txt")
	if err := os.WriteFile(outside, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// root == "" leaves the tool unwrapped, so an outside read succeeds.
	read := sandboxed("", root, newReadTool(root))
	if _, err := read.Handler(context.Background(), json.RawMessage(fmt.Sprintf(`{"path":%q}`, outside))); err != nil {
		t.Errorf("with sandbox off, outside reads should work: %v", err)
	}
}

func TestTodoWrite(t *testing.T) {
	tool := newTodoTool()
	ctx := context.Background()

	out, err := tool.Handler(ctx, json.RawMessage(`{"todos":[
		{"content":"design","status":"completed"},
		{"content":"build","status":"in_progress"},
		{"content":"test"}
	]}`))
	if err != nil {
		t.Fatalf("todo_write: %v", err)
	}
	for _, want := range []string{"[x] design", "[~] build", "[ ] test"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered list missing %q in:\n%s", want, out)
		}
	}

	// The list is replaced, not merged.
	out2, _ := tool.Handler(ctx, json.RawMessage(`{"todos":[{"content":"only this"}]}`))
	if strings.Contains(out2, "design") {
		t.Errorf("second call should replace the list, got:\n%s", out2)
	}

	if _, err := tool.Handler(ctx, json.RawMessage(`{"todos":[{"content":"x","status":"bogus"}]}`)); err == nil {
		t.Error("invalid status should be rejected")
	}
	if _, err := tool.Handler(ctx, json.RawMessage(`{"todos":[{"content":"  "}]}`)); err == nil {
		t.Error("empty content should be rejected")
	}
}
