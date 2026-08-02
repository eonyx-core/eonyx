package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eonyx/internal/shell"
)

func TestHookBeforeAllowsTool(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hk := New(sh, Hooks{"read": {Before: "true"}})
	if err := hk.Before(context.Background(), "read", []byte(`{}`)); err != nil {
		t.Fatalf("before hook should allow: %v", err)
	}
}

func TestHookBeforeBlocksTool(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hk := New(sh, Hooks{"write": {Before: "echo nope; exit 1"}})
	err = hk.Before(context.Background(), "write", []byte(`{}`))
	if err == nil {
		t.Fatal("expected before hook to block")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("expected error to contain hook output, got %q", err.Error())
	}
}

func TestHookAfterAppendsOutput(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hk := New(sh, Hooks{"read": {After: "echo appended"}})
	out := hk.After(context.Background(), "read", []byte(`{}`), "result")
	if !strings.Contains(out, "result") {
		t.Errorf("expected original result, got %q", out)
	}
	if !strings.Contains(out, "appended") {
		t.Errorf("expected appended output, got %q", out)
	}
}

func TestHookNoConfig(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hk := New(sh, Hooks{})
	if err := hk.Before(context.Background(), "read", []byte(`{}`)); err != nil {
		t.Fatalf("expected no error without hook: %v", err)
	}
	out := hk.After(context.Background(), "read", []byte(`{}`), "result")
	if out != "result" {
		t.Errorf("expected unchanged result, got %q", out)
	}
}

func TestHookAfterErrorAppends(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hk := New(sh, Hooks{"read": {After: "exit 1"}})
	out := hk.After(context.Background(), "read", []byte(`{}`), "result")
	if !strings.Contains(out, "result") {
		t.Errorf("expected original result preserved, got %q", out)
	}
	if !strings.Contains(out, "hook after read failed") {
		t.Errorf("expected failure note, got %q", out)
	}
}

func TestHookEnvironmentVariables(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hk := New(sh, Hooks{"read": {Before: "echo $EONYX_TOOL $EONYX_PHASE"}})
	out, err := hk.exec(context.Background(), "read", "before", "echo $EONYX_TOOL $EONYX_PHASE", []byte(`{}`), "")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "read before") {
		t.Errorf("expected env vars in output, got %q", out)
	}
}

// TestHookInputExposedAsData verifies the tool input reaches the hook intact via
// EONYX_INPUT, including a value containing a single quote.
func TestHookInputExposedAsData(t *testing.T) {
	sh, err := shell.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"path":"a'b"}`)
	hk := New(sh, Hooks{"read": {Before: `printf '%s' "$EONYX_INPUT"`}})
	out, err := hk.exec(context.Background(), "read", "before", `printf '%s' "$EONYX_INPUT"`, input, "")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out != string(input) {
		t.Errorf("EONYX_INPUT = %q, want %q", out, string(input))
	}
}

// TestHookInputDoesNotInject verifies that shell metacharacters in the tool
// input cannot execute commands in the hook shell.
func TestHookInputDoesNotInject(t *testing.T) {
	dir := t.TempDir()
	sh, err := shell.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "pwned")
	input := []byte(`{"x":"$(touch ` + marker + `)"}`)
	hk := New(sh, Hooks{"read": {Before: `echo "$EONYX_INPUT" > /dev/null`}})
	if err := hk.Before(context.Background(), "read", input); err != nil {
		t.Fatalf("before: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command substitution in input executed; injection not prevented")
	}
}
