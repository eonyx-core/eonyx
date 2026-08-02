package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// sandboxed wraps a file tool so that, when root is non-empty, it refuses a
// "path" argument that resolves outside root. root == "" returns the tool
// unchanged, so sandboxing is entirely opt-in.
func sandboxed(root, workdir string, t Tool) Tool {
	if root == "" {
		return t
	}
	inner := t.Handler
	t.Handler = func(ctx context.Context, input json.RawMessage) (string, error) {
		var arg struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(input, &arg)
		if arg.Path != "" {
			if err := withinSandbox(root, resolvePath(workdir, arg.Path)); err != nil {
				return "", err
			}
		}
		return inner(ctx, input)
	}
	return t
}

// withinSandbox reports an error when full is not inside root.
func withinSandbox(root, full string) error {
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside the project sandbox (%s); set permissions.sandbox to false to allow paths outside the project", full, root)
	}
	return nil
}
