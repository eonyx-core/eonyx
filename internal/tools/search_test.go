package tools

import (
	"strings"
	"testing"
)

func TestGlob(t *testing.T) {
	dir := t.TempDir()
	_, _ = run(t, newWriteTool(dir), `{"path":"a/x.go","content":"package a"}`)
	_, _ = run(t, newWriteTool(dir), `{"path":"a/y.txt","content":"text"}`)
	_, _ = run(t, newWriteTool(dir), `{"path":"b/z.go","content":"package b"}`)

	out, err := run(t, newGlobTool(dir), `{"pattern":"**/*.go"}`)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "a/x.go") || !strings.Contains(out, "b/z.go") {
		t.Errorf("glob = %q, want a/x.go and b/z.go", out)
	}
	if strings.Contains(out, "y.txt") {
		t.Errorf("glob matched non-.go file: %q", out)
	}
}

func TestGrep(t *testing.T) {
	dir := t.TempDir()
	_, _ = run(t, newWriteTool(dir), `{"path":"a.go","content":"package main\nfunc Foo() {}\n"}`)
	_, _ = run(t, newWriteTool(dir), `{"path":"b.txt","content":"nothing here"}`)

	out, err := run(t, newGrepTool(dir), `{"pattern":"func \\w+","glob":"*.go"}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(out, "a.go:2:func Foo() {}") {
		t.Errorf("grep = %q, want a.go:2 match", out)
	}
	if strings.Contains(out, "b.txt") {
		t.Errorf("grep matched filtered-out file: %q", out)
	}
}

func TestGrepNoMatches(t *testing.T) {
	dir := t.TempDir()
	_, _ = run(t, newWriteTool(dir), `{"path":"a.txt","content":"hello"}`)
	out, err := run(t, newGrepTool(dir), `{"pattern":"zzz"}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if out != "(no matches)" {
		t.Errorf("grep = %q, want (no matches)", out)
	}
}
