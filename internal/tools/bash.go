package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eonyx-core/eonyx/internal/shell"
)

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
)

func newBashTool(sh *shell.Shell) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "Shell command to run. The working directory and exported variables persist between calls."},
    "timeout_seconds": {"type": "integer", "description": "Maximum run time in seconds (default 120, max 600)."}
  },
  "required": ["command"]
}`
	return Tool{
		Name:        "bash",
		Description: "Run a shell command in a persistent session. Returns combined stdout and stderr; a non-zero exit is reported in the output.",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Command        string `json:"command"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Command == "" {
				return "", fmt.Errorf("command is required")
			}

			timeout := defaultBashTimeout
			if args.TimeoutSeconds > 0 {
				timeout = time.Duration(args.TimeoutSeconds) * time.Second
				timeout = min(timeout, maxBashTimeout)
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			res, err := sh.Exec(ctx, args.Command)
			if err != nil {
				return "", err
			}
			if res.ExitCode != 0 {
				if res.Output == "" {
					return fmt.Sprintf("(exit code %d)", res.ExitCode), nil
				}
				return fmt.Sprintf("%s\n(exit code %d)", res.Output, res.ExitCode), nil
			}
			if res.Output == "" {
				return "(no output)", nil
			}
			return res.Output, nil
		},
	}
}
