// Package shell runs bash-like scripts in-process via mvdan.cc/sh, so eonyx
// can execute shell commands cross-platform without a system bash. A Shell
// keeps its working directory and exported environment across calls (so `cd`
// and `export` persist within a session) and refuses a small block-list of
// dangerous commands as a coarse safety net beneath the permission gate.
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Result is the outcome of running a script.
type Result struct {
	// Output is the combined stdout and stderr, interleaved as produced.
	Output string
	// ExitCode is the exit status of the last command (0 on success).
	ExitCode int
}

// Shell is a persistent shell session. It is safe for concurrent use; calls are
// serialized so directory and environment state stay coherent.
type Shell struct {
	mu      sync.Mutex
	runner  *interp.Runner
	parser  *syntax.Parser
	blocked map[string]bool
}

// New creates a shell session rooted at dir (defaulting to the process working
// directory) and seeded with the current environment.
func New(dir string) (*Shell, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("shell: resolve working dir: %w", err)
		}
		dir = wd
	}

	sh := &Shell{
		parser:  syntax.NewParser(),
		blocked: defaultBlocked(),
	}
	runner, err := interp.New(
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.CallHandler(sh.callHandler),
	)
	if err != nil {
		return nil, fmt.Errorf("shell: init interpreter: %w", err)
	}
	sh.runner = runner
	return sh, nil
}

// Exec runs script and returns its combined output and exit code. A non-zero
// exit is reported via Result.ExitCode, not as an error; the returned error is
// reserved for parse failures and interpreter faults.
func (s *Shell) Exec(ctx context.Context, script string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.parser.Parse(strings.NewReader(script), "")
	if err != nil {
		return Result{}, fmt.Errorf("shell: parse script: %w", err)
	}

	var buf bytes.Buffer
	if err := interp.StdIO(nil, &buf, &buf)(s.runner); err != nil {
		return Result{}, fmt.Errorf("shell: set io: %w", err)
	}

	runErr := s.runner.Run(ctx, file)
	res := Result{Output: buf.String()}

	var status interp.ExitStatus
	switch {
	case runErr == nil:
		res.ExitCode = 0
	case errors.As(runErr, &status):
		res.ExitCode = int(status)
	default:
		return res, fmt.Errorf("shell: run: %w", runErr)
	}
	return res, nil
}

// Dir returns the session's current working directory.
func (s *Shell) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runner.Dir
}

// Block adds command names to the block-list.
func (s *Shell) Block(names ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range names {
		s.blocked[n] = true
	}
}

// callHandler runs before every command and rejects block-listed ones.
func (s *Shell) callHandler(_ context.Context, args []string) ([]string, error) {
	if len(args) > 0 && s.blocked[args[0]] {
		return nil, fmt.Errorf("command %q is blocked by shell policy", args[0])
	}
	return args, nil
}

// defaultBlocked is the built-in block-list: commands that can take down or
// reformat the host. It is a coarse net; fine-grained control is the job of the
// permission gate.
func defaultBlocked() map[string]bool {
	return map[string]bool{
		"shutdown": true,
		"reboot":   true,
		"halt":     true,
		"poweroff": true,
		"mkfs":     true,
		"init":     true,
	}
}
