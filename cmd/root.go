// Package cmd defines eonyx's command-line interface. The root command's
// default action is the interactive TUI (added in a later phase); subcommands
// provide headless and inspection entry points.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"eonyx/internal/config"
	"eonyx/internal/license"
	"eonyx/internal/logging"
	"eonyx/internal/tui"
	"eonyx/internal/version"
)

// logFileFlag is bound to the root's persistent --log-file flag, so both the
// TUI and the run subcommand can direct the log to an explicit path.
var logFileFlag string

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "eonyx",
		Short: "Terminal-first AI coding agent",
		Long: "eonyx is a terminal-first AI coding agent that talks directly to\n" +
			"model APIs and local models — no external services, no telemetry.",
		Version:      version.String(),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The TUI needs a real terminal; refuse to launch it when piped or
			// in tests so it never blocks waiting for input that cannot arrive.
			if !isTTY(os.Stdin) || !isTTY(os.Stdout) {
				return errors.New(`the interactive TUI requires a terminal; use "eonyx run <prompt>" for non-interactive use`)
			}
			if err := license.Ensure(cmd.Context()); err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			if path, created, err := config.EnsureDefaultConfig(wd); err != nil {
				return err
			} else if created {
				fmt.Fprintf(cmd.ErrOrStderr(), "Created default config at %s — runs against a local Ollama model by default; edit it to switch providers.\n", path)
			}
			cfg, err := config.Load(wd)
			if err != nil {
				return err
			}
			defer initLogging(cfg, wd)()
			return tui.Program(cmd.Context(), cfg, wd)
		},
	}
	root.SetVersionTemplate(cliHeader("VERSION") + " " + cliGreen.Render("{{.Version}}") + "\n")
	root.PersistentFlags().StringVar(&logFileFlag, "log-file", "", "write the log to this file instead of the per-project default")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return coded(exitUsage, err) })
	root.AddCommand(newRunCmd(), newConfigCmd(), newInfoCmd(), newCleanCmd(), newSessionsCmd(), newLicenseCmd())
	return root
}

// initLogging points the logger at the right destination, honoring the
// --log-file flag first, then the log_file config field, then the per-project
// default directory. The returned closer must be deferred by the caller.
func initLogging(cfg *config.Config, wd string) func() {
	if logFileFlag != "" {
		return logging.InitFile(logFileFlag)
	}
	if cfg.LogFile != "" {
		return logging.InitFile(cfg.LogFile)
	}
	return logging.Init(logDir(cfg, wd))
}

// logDir is the directory eonyx writes its log file to: the per-project
// .eonyx/logs, falling back to the working directory when there is no
// project config.
func logDir(cfg *config.Config, wd string) string {
	root := cfg.ProjectRoot()
	if root == "" {
		root = wd
	}
	return filepath.Join(config.ProjectDataDir(root), "logs")
}

// Execute builds and runs the eonyx command tree, printing errors itself in
// the brand style; the returned error signals the process should exit non-zero.
func Execute() error {
	root := newRootCmd()
	root.SilenceErrors = true
	err := root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, renderCLIError(err))
	}
	return err
}

// isTTY reports whether f is connected to a terminal, used to switch between
// human-readable and pipe-friendly output.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
