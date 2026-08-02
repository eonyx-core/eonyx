package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBranch(t *testing.T) {
	tmp := t.TempDir()

	if b := Branch(tmp); b != "" {
		t.Fatalf("expected empty branch outside git, got %q", b)
	}

	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = tmp
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	write(t, filepath.Join(tmp, "a.txt"), "a")
	run("git", "add", "a.txt")
	run("git", "commit", "-m", "init")
	run("git", "checkout", "-b", "feature")

	if b := Branch(tmp); b != "feature" {
		t.Fatalf("expected branch feature, got %q", b)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
