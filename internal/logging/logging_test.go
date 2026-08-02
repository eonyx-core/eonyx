package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesLogFile(t *testing.T) {
	dir := t.TempDir()
	closer := Init(dir)
	slog.Info("hello-entry", "k", "v")
	closer()

	data, err := os.ReadFile(filepath.Join(dir, "eonyx.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello-entry") {
		t.Errorf("log missing entry, got:\n%s", data)
	}
}

func TestInitFileWritesToExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "custom.log")
	closer := InitFile(path)
	slog.Warn("boom-entry")
	closer()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "boom-entry") {
		t.Errorf("log missing entry, got:\n%s", data)
	}
}

func TestInitDiscardsOnBadDir(t *testing.T) {
	// A path whose parent is a file cannot be created; Init must fall back to a
	// no-op closer rather than panic.
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	closer := Init(filepath.Join(file, "sub"))
	slog.Info("ignored")
	closer()
}

func TestLevelFromEnv(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo,
		"bogus":   slog.LevelInfo,
	}
	for val, want := range cases {
		t.Setenv("EONYX_LOG_LEVEL", val)
		if got := levelFromEnv(); got != want {
			t.Errorf("levelFromEnv(%q) = %v, want %v", val, got, want)
		}
	}
}
