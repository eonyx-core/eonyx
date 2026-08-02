package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/eonyx-core/eonyx/internal/agent"
)

func TestResolvePromptFromArgs(t *testing.T) {
	cmd := newRunCmd()
	got, err := resolvePrompt(cmd, []string{"fix", "the", "bug"})
	if err != nil {
		t.Fatalf("resolvePrompt: %v", err)
	}
	if got != "fix the bug" {
		t.Errorf("prompt = %q, want %q", got, "fix the bug")
	}
}

func TestResolvePromptFromStdin(t *testing.T) {
	cmd := newRunCmd()
	cmd.SetIn(strings.NewReader("  read this from stdin\n"))
	got, err := resolvePrompt(cmd, nil)
	if err != nil {
		t.Fatalf("resolvePrompt: %v", err)
	}
	if got != "read this from stdin" {
		t.Errorf("prompt = %q, want trimmed stdin", got)
	}
}

func TestResolvePromptEmptyStdinIsUsageError(t *testing.T) {
	cmd := newRunCmd()
	cmd.SetIn(strings.NewReader("   \n"))
	_, err := resolvePrompt(cmd, nil)
	if err == nil {
		t.Fatal("expected usage error for empty stdin")
	}
	if got := ExitCode(err); got != exitUsage {
		t.Errorf("exit code = %d, want %d", got, exitUsage)
	}
}

func TestClassifySendError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"canceled", context.Canceled, exitInterrupted},
		{"deadline", context.DeadlineExceeded, exitInterrupted},
		{"max steps", fmt.Errorf("wrap: %w", agent.ErrMaxSteps), exitMaxSteps},
		{"api", errors.New("openai: 500"), exitAPIError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(classifySendError(tc.err)); got != tc.want {
				t.Errorf("ExitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", got)
	}
	if got := ExitCode(errors.New("plain")); got != exitError {
		t.Errorf("ExitCode(plain) = %d, want %d", got, exitError)
	}
	if got := ExitCode(coded(exitAPIError, errors.New("x"))); got != exitAPIError {
		t.Errorf("ExitCode(coded) = %d, want %d", got, exitAPIError)
	}
}

func TestExitCodeOverride(t *testing.T) {
	prev := exitOverride
	defer func() { exitOverride = prev }()
	exitOverride = exitPermissionDenied
	if got := ExitCode(nil); got != exitPermissionDenied {
		t.Errorf("ExitCode(nil) with override = %d, want %d", got, exitPermissionDenied)
	}
}
