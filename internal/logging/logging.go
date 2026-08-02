// Package logging configures eonyx's file logger. All packages log through
// log/slog's default logger; Init points that default at a per-project log file
// so TUI and agent failures leave a trace that can be inspected after the fact,
// rather than vanishing when the terminal clears.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Init points slog's default logger at eonyx.log inside dir, creating the
// directory as needed. The level comes from EONYX_LOG_LEVEL (debug, info,
// warn, error; default info). Any setup failure falls back to a discard logger
// so logging never prevents eonyx from running. The returned func closes the
// log file and should be deferred by the caller.
func Init(dir string) func() {
	return InitFile(filepath.Join(dir, "eonyx.log"))
}

// InitFile is like Init but writes to an explicit file path, creating its parent
// directory as needed. It is used by the --log-file flag and the log_file config
// field to override the per-project default location.
func InitFile(path string) func() {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return discard()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return discard()
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelFromEnv()})))
	return func() { _ = f.Close() }
}

// discard silences logging and returns a no-op closer.
func discard() func() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return func() {}
}

func levelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("EONYX_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
