package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"eonyx/internal/shell"
)

// Hook describes a before/after shell command for a named tool.
type Hook struct {
	// Before runs before the tool. A non-zero exit code blocks the tool.
	Before string `json:"before,omitempty"`
	// After runs after the tool. Its output is appended to the tool result.
	After string `json:"after,omitempty"`
}

// Hooks maps tool names to their hook configuration.
type Hooks map[string]Hook

// Engine runs configured hook scripts via the shared shell.
type Engine struct {
	sh    *shell.Shell
	hooks Hooks
}

// New creates a hook engine backed by the shared shell session.
func New(sh *shell.Shell, hooks Hooks) *Engine {
	if hooks == nil {
		hooks = Hooks{}
	}
	return &Engine{sh: sh, hooks: hooks}
}

// Before runs the configured pre-hook for a tool. A non-zero exit code blocks
// the tool and the error is returned. The tool name, phase, working directory,
// and raw JSON input are exposed as EONYX_TOOL, EONYX_PHASE,
// EONYX_CWD, and EONYX_INPUT.
func (e *Engine) Before(ctx context.Context, name string, input []byte) error {
	h, ok := e.hooks[name]
	if !ok || h.Before == "" {
		return nil
	}
	return e.run(ctx, name, "before", h.Before, input, "")
}

// After runs the configured post-hook for a tool. Its output is appended to the
// result when it succeeds. Errors are ignored so the tool result is preserved.
// In addition to the variables Before exposes, the tool result is available as
// EONYX_RESULT.
func (e *Engine) After(ctx context.Context, name string, input []byte, result string) string {
	h, ok := e.hooks[name]
	if !ok || h.After == "" {
		return result
	}
	out, err := e.exec(ctx, name, "after", h.After, input, result)
	if err != nil {
		return result + fmt.Sprintf("\n(hook after %s failed: %s)", name, err)
	}
	if out == "" {
		return result
	}
	return result + "\n" + out
}

func (e *Engine) run(ctx context.Context, name, phase, script string, input []byte, result string) error {
	_, err := e.exec(ctx, name, phase, script, input, result)
	return err
}

func (e *Engine) exec(ctx context.Context, name, phase, script string, input []byte, result string) (string, error) {
	if e.sh == nil {
		return "", fmt.Errorf("no shell available for hooks")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	wrapped := e.wrapScript(name, phase, script, input, result)
	res, err := e.sh.Exec(ctx, wrapped)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		msg := res.Output
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return "", fmt.Errorf("hook %s %s refused: %s", phase, name, msg)
	}
	return res.Output, nil
}

// wrapScript exposes the tool context to the hook as environment variables and
// then runs the script. Values are passed as data via the environment (the tool
// input as EONYX_INPUT, the result as EONYX_RESULT), never interpolated
// into the script text, so tool arguments and results cannot inject commands.
func (e *Engine) wrapScript(name, phase, script string, input []byte, result string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "export EONYX_TOOL=%s\n", shellQuote(name))
	fmt.Fprintf(&b, "export EONYX_PHASE=%s\n", shellQuote(phase))
	fmt.Fprintf(&b, "export EONYX_CWD=%s\n", shellQuote(filepath.ToSlash(e.sh.Dir())))
	if len(input) > 0 {
		fmt.Fprintf(&b, "export EONYX_INPUT=%s\n", shellQuote(string(input)))
	}
	if result != "" {
		fmt.Fprintf(&b, "export EONYX_RESULT=%s\n", shellQuote(result))
	}
	b.WriteString(script)
	b.WriteString("\n")
	return b.String()
}

// shellQuote returns s quoted as a single shell word, safe for arbitrary
// content so tool input and results cannot inject commands. It uses the shell
// parser's own quoter; the rare unquotable input (e.g. an embedded NUL) falls
// back to an empty word.
func shellQuote(s string) string {
	q, err := syntax.Quote(s, syntax.LangBash)
	if err != nil {
		return "''"
	}
	return q
}

// MarshalJSON serializes hooks to JSON for config display.
func (h Hooks) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]Hook(h))
}

// UnmarshalJSON parses hooks from JSON.
func (h *Hooks) UnmarshalJSON(data []byte) error {
	var m map[string]Hook
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*h = m
	return nil
}
