package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/eonyx-core/eonyx/internal/config"
	"github.com/eonyx-core/eonyx/internal/session"
)

// execute runs cmd with the given args and captures stdout/stderr.
func execute(t *testing.T, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader(""))
	cErr := cmd.Execute()
	return outBuf.String(), errBuf.String(), cErr
}

func setupConfig(t *testing.T, dir string) {
	t.Helper()
	cfgJSON := `{
  "providers": {"openai": {"type": "openai", "base_url": "https://api.example.com/v1", "api_key": "sk-test"}},
  "models": [{"id": "gpt-x", "provider": "openai"}],
  "default_model": "gpt-x"
}`
	if err := os.WriteFile(filepath.Join(dir, "eonyx.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openTestStore(t *testing.T, dir string) (session.Store, error) {
	t.Helper()
	dataDir := config.ProjectDataDir(dir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	return session.NewJSONLStore(dataDir)
}

func TestRootCmdRequiresConfig(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	cmd := newRootCmd()
	if _, _, err := execute(t, cmd); err == nil {
		t.Fatal("expected root command to fail without configuration")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	// Isolate tests from the developer's real global config so config.Load only
	// sees the temp project; otherwise a machine-local config leaks in.
	t.Setenv("EONYX_GLOBAL_CONFIG", filepath.Join(dir, "no-global-config.json"))
	t.Setenv("EONYX_GLOBAL_DATA", filepath.Join(dir, "no-global-data.json"))
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
}

func TestConfigCmd(t *testing.T) {
	dir := t.TempDir()
	setupConfig(t, dir)
	chdir(t, dir)

	cmd := newConfigCmd()
	out, _, err := execute(t, cmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "openai") {
		t.Errorf("output = %q, want provider openai", out)
	}
	if !strings.Contains(out, "gpt-x") {
		t.Errorf("output = %q, want model gpt-x", out)
	}
	if !strings.Contains(out, "● gpt-x") || !strings.Contains(out, "(default)") {
		t.Errorf("output = %q, want default model marker", out)
	}
}

func TestSessionsCmdEmpty(t *testing.T) {
	dir := t.TempDir()
	setupConfig(t, dir)
	chdir(t, dir)

	cmd := newSessionsCmd()
	out, _, err := execute(t, cmd)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "No sessions yet") {
		t.Errorf("output = %q, want empty-session message", out)
	}
}

func TestCleanCmdNoFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cmd := newCleanCmd()
	out, _, err := execute(t, cmd, "-y")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "No eonyx configuration files found") {
		t.Errorf("output = %q, want no-files message", out)
	}
}

func TestCleanCmdDeletesFiles(t *testing.T) {
	dir := t.TempDir()
	setupConfig(t, dir)
	chdir(t, dir)
	cmd := newCleanCmd()
	out, _, err := execute(t, cmd, "-y")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "REMOVED") {
		t.Errorf("output = %q, want removed files", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "eonyx.json")); !os.IsNotExist(err) {
		t.Errorf("expected project config to be deleted")
	}
}

func TestRunCmdRequiresArgs(t *testing.T) {
	cmd := newRunCmd()
	_, _, err := execute(t, cmd)
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestResolveSessionNew(t *testing.T) {
	dir := t.TempDir()
	store, err := openTestStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, history, err := resolveSession(store, "", false, "hello", "sys")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if len(history) != 1 || history[0].Role != "system" {
		t.Errorf("history = %+v, want system message", history)
	}
	if sess.Title != "hello" {
		t.Errorf("title = %q, want hello", sess.Title)
	}
}

func TestResolveSessionContinue(t *testing.T) {
	dir := t.TempDir()
	store, err := openTestStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	sess1, _, err := resolveSession(store, "", false, "one", "sys")
	if err != nil {
		t.Fatal(err)
	}
	sess2, history, err := resolveSession(store, "", true, "two", "sys")
	if err != nil {
		t.Fatalf("resolveSession continue: %v", err)
	}
	if sess2.ID != sess1.ID {
		t.Errorf("continued wrong session: got %s, want %s", sess2.ID, sess1.ID)
	}
	if len(history) == 0 {
		t.Error("expected history from continued session")
	}
}

func TestResolveSessionByID(t *testing.T) {
	dir := t.TempDir()
	store, err := openTestStore(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	sess1, _, err := resolveSession(store, "", false, "one", "sys")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = resolveSession(store, "", false, "two", "sys")
	if err != nil {
		t.Fatal(err)
	}

	sess, history, err := resolveSession(store, sess1.ID, false, "three", "sys")
	if err != nil {
		t.Fatalf("resolveSession by id: %v", err)
	}
	if sess.ID != sess1.ID {
		t.Errorf("got session %s, want %s", sess.ID, sess1.ID)
	}
	if len(history) == 0 {
		t.Error("expected history from selected session")
	}
}

func TestContextWindow(t *testing.T) {
	cfg := &config.Config{
		Models:       []config.Model{{ID: "a", Provider: "p", ContextWindow: 1000}},
		DefaultModel: "a",
	}
	if got := contextWindow(cfg, "a"); got != 1000 {
		t.Errorf("contextWindow(a) = %d, want 1000", got)
	}
	if got := contextWindow(cfg, ""); got != 1000 {
		t.Errorf("contextWindow(default) = %d, want 1000", got)
	}
	if got := contextWindow(cfg, "missing"); got != 0 {
		t.Errorf("contextWindow(missing) = %d, want 0", got)
	}
}

func TestSessionDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	if got := sessionDir(cfg, dir); got != config.GlobalSessionsDir() {
		t.Errorf("sessionDir = %q, want %q", got, config.GlobalSessionsDir())
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate(short) = %q, want short", got)
	}
	long := strings.Repeat("a", 20)
	want := strings.Repeat("a", 10) + "…"
	if got := truncate(long, 10); got != want {
		t.Errorf("truncate(long) = %q, want %q", got, want)
	}
	if got := truncate("a\nb", 10); got != "a b" {
		t.Errorf("truncate(newline) = %q, want a b", got)
	}
}
