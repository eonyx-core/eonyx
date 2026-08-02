package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// runGit runs a git subcommand in workdir and returns its combined output. A
// non-zero exit is turned into an error carrying git's message, so the agent
// sees why the command failed.
func runGit(ctx context.Context, workdir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workdir}, args...)...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(out), "\n")
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("%s", text)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if text == "" {
		return "(no output)", nil
	}
	return text, nil
}

func newGitStatusTool(workdir string) Tool {
	return Tool{
		Name:        "git_status",
		Description: "Show the working tree status of the git repository (git status --short --branch).",
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return runGit(ctx, workdir, "status", "--short", "--branch")
		},
	}
}

func newGitDiffTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "staged": {"type": "boolean", "description": "Show staged (index) changes instead of unstaged working-tree changes."},
    "path": {"type": "string", "description": "Limit the diff to this path."}
  }
}`
	return Tool{
		Name:        "git_diff",
		Description: "Show the git diff of unstaged changes, or staged changes when staged is true. Optionally limit to a path.",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Staged bool   `json:"staged"`
				Path   string `json:"path"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			gitArgs := []string{"diff"}
			if args.Staged {
				gitArgs = append(gitArgs, "--staged")
			}
			if args.Path != "" {
				gitArgs = append(gitArgs, "--", args.Path)
			}
			return runGit(ctx, workdir, gitArgs...)
		},
	}
}

func newGitLogTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "max": {"type": "integer", "description": "Maximum number of commits to show (default 20)."},
    "path": {"type": "string", "description": "Limit the log to commits touching this path."}
  }
}`
	return Tool{
		Name:        "git_log",
		Description: "Show recent commits as a one-line-per-commit log (git log --oneline).",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Max  int    `json:"max"`
				Path string `json:"path"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			max := args.Max
			if max <= 0 {
				max = 20
			}
			if max > 200 {
				max = 200
			}
			gitArgs := []string{"log", "--oneline", "-n", strconv.Itoa(max)}
			if args.Path != "" {
				gitArgs = append(gitArgs, "--", args.Path)
			}
			return runGit(ctx, workdir, gitArgs...)
		},
	}
}

func newGitCommitTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "message": {"type": "string", "description": "The commit message."},
    "all": {"type": "boolean", "description": "Stage all tracked, modified files before committing (git commit -a)."}
  },
  "required": ["message"]
}`
	return Tool{
		Name:        "git_commit",
		Description: "Create a git commit with the given message. Set all to stage modified tracked files first; otherwise commits what is already staged.",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Message string `json:"message"`
				All     bool   `json:"all"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if strings.TrimSpace(args.Message) == "" {
				return "", fmt.Errorf("message is required")
			}
			gitArgs := []string{"commit"}
			if args.All {
				gitArgs = append(gitArgs, "-a")
			}
			gitArgs = append(gitArgs, "-m", args.Message)
			return runGit(ctx, workdir, gitArgs...)
		},
	}
}
