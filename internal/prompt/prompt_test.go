package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSystemIncludesContextFilesNearestLast(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main")
	write(t, filepath.Join(root, "AGENTS.md"), "ROOT RULES")
	sub := filepath.Join(root, "pkg")
	write(t, filepath.Join(sub, "AGENTS.md"), "SUB RULES")

	out := System(sub, "", nil)

	if !strings.Contains(out, "Working directory: "+sub) {
		t.Errorf("missing working directory line in:\n%s", out)
	}
	iRoot := strings.Index(out, "ROOT RULES")
	iSub := strings.Index(out, "SUB RULES")
	if iRoot < 0 || iSub < 0 {
		t.Fatalf("missing context files: root=%d sub=%d", iRoot, iSub)
	}
	if iRoot > iSub {
		t.Error("expected outer AGENTS.md before nearer one")
	}
}

func TestSystemWithoutContextFiles(t *testing.T) {
	dir := t.TempDir()
	out := System(dir, dir, nil)
	if !strings.Contains(out, "You are eonyx") {
		t.Errorf("base prompt missing:\n%s", out)
	}
	if strings.Contains(out, "Project context:") {
		t.Errorf("unexpected context section:\n%s", out)
	}
}
