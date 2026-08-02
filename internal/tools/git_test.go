package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := runGit(ctx, dir, args...); err != nil {
			t.Skipf("git not usable in this environment: %v", err)
		}
	}
	return dir
}

func TestGitStatusDiffLogCommit(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := newGitStatusTool(dir).Handler(ctx, nil)
	if err != nil {
		t.Fatalf("git_status: %v", err)
	}
	if !strings.Contains(status, "a.txt") {
		t.Errorf("status should mention the untracked file, got:\n%s", status)
	}

	if _, err := runGit(ctx, dir, "add", "a.txt"); err != nil {
		t.Fatal(err)
	}

	staged, err := newGitDiffTool(dir).Handler(ctx, json.RawMessage(`{"staged":true}`))
	if err != nil {
		t.Fatalf("git_diff staged: %v", err)
	}
	if !strings.Contains(staged, "a.txt") || !strings.Contains(staged, "hello") {
		t.Errorf("staged diff should show the new file, got:\n%s", staged)
	}

	if _, err := newGitCommitTool(dir).Handler(ctx, json.RawMessage(`{"message":"add a"}`)); err != nil {
		t.Fatalf("git_commit: %v", err)
	}

	logOut, err := newGitLogTool(dir).Handler(ctx, json.RawMessage(`{"max":5}`))
	if err != nil {
		t.Fatalf("git_log: %v", err)
	}
	if !strings.Contains(logOut, "add a") {
		t.Errorf("log should contain the commit subject, got:\n%s", logOut)
	}
}

func TestGitCommitRequiresMessage(t *testing.T) {
	dir := initRepo(t)
	if _, err := newGitCommitTool(dir).Handler(context.Background(), json.RawMessage(`{"message":"  "}`)); err == nil {
		t.Error("git_commit with a blank message should fail")
	}
}
