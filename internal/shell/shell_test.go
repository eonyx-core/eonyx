package shell

import (
	"context"
	"strings"
	"testing"
)

func newShell(t *testing.T) *Shell {
	t.Helper()
	sh, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return sh
}

func TestExecOutputAndExitCode(t *testing.T) {
	sh := newShell(t)
	ctx := context.Background()

	res, err := sh.Exec(ctx, "echo hello")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if strings.TrimSpace(res.Output) != "hello" {
		t.Errorf("output = %q, want hello", res.Output)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit = %d, want 0", res.ExitCode)
	}

	res, err = sh.Exec(ctx, "exit 3")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit = %d, want 3", res.ExitCode)
	}
}

func TestEnvPersistsAcrossCalls(t *testing.T) {
	sh := newShell(t)
	ctx := context.Background()

	if _, err := sh.Exec(ctx, "export FOO=bar"); err != nil {
		t.Fatalf("Exec export: %v", err)
	}
	res, err := sh.Exec(ctx, "echo $FOO")
	if err != nil {
		t.Fatalf("Exec echo: %v", err)
	}
	if strings.TrimSpace(res.Output) != "bar" {
		t.Errorf("output = %q, want bar", res.Output)
	}
}

func TestWorkingDirPersists(t *testing.T) {
	sh := newShell(t)
	ctx := context.Background()

	if _, err := sh.Exec(ctx, "mkdir sub && cd sub"); err != nil {
		t.Fatalf("Exec cd: %v", err)
	}
	if !strings.HasSuffix(sh.Dir(), "sub") {
		t.Errorf("Dir = %q, want suffix sub", sh.Dir())
	}
}

func TestSessionContinuesAfterExit(t *testing.T) {
	sh := newShell(t)
	ctx := context.Background()

	if _, err := sh.Exec(ctx, "exit 3"); err != nil {
		t.Fatalf("Exec exit: %v", err)
	}
	res, err := sh.Exec(ctx, "echo still-here")
	if err != nil {
		t.Fatalf("Exec after exit: %v", err)
	}
	if strings.TrimSpace(res.Output) != "still-here" {
		t.Errorf("output after exit = %q, want still-here", res.Output)
	}
}

func TestSessionContinuesAfterBlocked(t *testing.T) {
	sh := newShell(t)
	ctx := context.Background()

	if _, err := sh.Exec(ctx, "shutdown now"); err == nil {
		t.Fatal("expected blocked command to error")
	}
	res, err := sh.Exec(ctx, "echo ok")
	if err != nil {
		t.Fatalf("Exec after blocked: %v", err)
	}
	if strings.TrimSpace(res.Output) != "ok" {
		t.Errorf("output after blocked = %q, want ok", res.Output)
	}
}

func TestBlockedCommand(t *testing.T) {
	sh := newShell(t)
	_, err := sh.Exec(context.Background(), "shutdown now")
	if err == nil {
		t.Fatal("expected blocked command to error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error = %v, want it to mention blocked", err)
	}
}
