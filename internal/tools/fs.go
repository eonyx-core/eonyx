package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eonyx-core/eonyx/internal/diff"
)

// resolvePath resolves p against workdir. Absolute paths are used as-is.
func resolvePath(workdir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(workdir, p)
}

func newReadTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "File path, absolute or relative to the working directory."},
    "offset": {"type": "integer", "description": "1-based line number to start reading from."},
    "limit": {"type": "integer", "description": "Maximum number of lines to return."}
  },
  "required": ["path"]
}`
	return Tool{
		Name:        "read",
		Description: "Read a text file and return its contents. Optionally start at a line offset and cap the number of lines.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			data, err := os.ReadFile(resolvePath(workdir, args.Path))
			if err != nil {
				return "", err
			}
			if args.Offset <= 0 && args.Limit <= 0 {
				return string(data), nil
			}

			lines := strings.Split(string(data), "\n")
			start := 0
			if args.Offset > 0 {
				start = args.Offset - 1
			}
			if start > len(lines) {
				start = len(lines)
			}
			end := len(lines)
			if args.Limit > 0 && start+args.Limit < end {
				end = start + args.Limit
			}
			return strings.Join(lines[start:end], "\n"), nil
		},
	}
}

func newWriteTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "File path, absolute or relative to the working directory."},
    "content": {"type": "string", "description": "Full file content to write."},
    "dry_run": {"type": "boolean", "description": "Preview the change (return the diff) without writing to disk."}
  },
  "required": ["path", "content"]
}`
	return Tool{
		Name:        "write",
		Description: "Write content to a file, creating parent directories and overwriting any existing file. Set dry_run to preview the diff without writing.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path    string `json:"path"`
				Content string `json:"content"`
				DryRun  bool   `json:"dry_run"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			full := resolvePath(workdir, args.Path)
			old, existed := readExisting(full)
			if args.DryRun {
				if !existed {
					return fmt.Sprintf("[dry run] would create %s (%d bytes, %d lines)", args.Path, len(args.Content), countLines(args.Content)), nil
				}
				d := diff.Unified(args.Path, old, args.Content)
				if d == "" {
					return fmt.Sprintf("[dry run] %s would be unchanged", args.Path), nil
				}
				added, removed := diff.Stat(old, args.Content)
				return fmt.Sprintf("[dry run] would write %s (+%d -%d)\n\n%s", args.Path, added, removed, d), nil
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(full, []byte(args.Content), 0o644); err != nil {
				return "", err
			}
			if !existed {
				return fmt.Sprintf("wrote %d bytes to %s (new file, %d lines)", len(args.Content), args.Path, countLines(args.Content)), nil
			}
			d := diff.Unified(args.Path, old, args.Content)
			if d == "" {
				return fmt.Sprintf("wrote %d bytes to %s (no changes)", len(args.Content), args.Path), nil
			}
			added, removed := diff.Stat(old, args.Content)
			return fmt.Sprintf("wrote %s (+%d -%d)\n\n%s", args.Path, added, removed, d), nil
		},
	}
}

// readExisting returns the current contents of path and whether it existed.
func readExisting(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// countLines counts the number of lines in s, ignoring a single trailing newline.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

func newEditTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "File path, absolute or relative to the working directory."},
    "old_string": {"type": "string", "description": "Exact text to replace. Must occur exactly once unless replace_all is set."},
    "new_string": {"type": "string", "description": "Replacement text."},
    "replace_all": {"type": "boolean", "description": "Replace every occurrence instead of requiring a unique match."},
    "dry_run": {"type": "boolean", "description": "Preview the change (return the diff) without writing to disk."}
  },
  "required": ["path", "old_string", "new_string"]
}`
	return Tool{
		Name:        "edit",
		Description: "Replace an exact substring in a file. By default old_string must occur exactly once; set replace_all to replace every occurrence, or dry_run to preview the diff without writing.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path       string `json:"path"`
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
				DryRun     bool   `json:"dry_run"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			if args.OldString == "" {
				return "", fmt.Errorf("old_string is required")
			}
			if args.OldString == args.NewString {
				return "", fmt.Errorf("old_string and new_string are identical")
			}

			full := resolvePath(workdir, args.Path)
			data, err := os.ReadFile(full)
			if err != nil {
				return "", err
			}
			content := string(data)
			count := strings.Count(content, args.OldString)
			if count == 0 {
				return "", fmt.Errorf("old_string not found in %s", args.Path)
			}
			if count > 1 && !args.ReplaceAll {
				return "", fmt.Errorf("old_string occurs %d times in %s; add context to make it unique or set replace_all", count, args.Path)
			}

			var updated string
			if args.ReplaceAll {
				updated = strings.ReplaceAll(content, args.OldString, args.NewString)
			} else {
				updated = strings.Replace(content, args.OldString, args.NewString, 1)
			}
			if args.DryRun {
				added, removed := diff.Stat(content, updated)
				d := diff.Unified(args.Path, content, updated)
				return fmt.Sprintf("[dry run] would edit %s (%d replacement(s), +%d -%d)\n\n%s", args.Path, count, added, removed, d), nil
			}
			if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
				return "", err
			}
			added, removed := diff.Stat(content, updated)
			d := diff.Unified(args.Path, content, updated)
			return fmt.Sprintf("edited %s (%d replacement(s), +%d -%d)\n\n%s", args.Path, count, added, removed, d), nil
		},
	}
}

func newMultiEditTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "File path, absolute or relative to the working directory."},
    "edits": {
      "type": "array",
      "description": "Replacements applied in order; if any fails the file is left unchanged.",
      "items": {
        "type": "object",
        "properties": {
          "old_string": {"type": "string", "description": "Exact text to replace. Must occur exactly once unless replace_all is set."},
          "new_string": {"type": "string", "description": "Replacement text."},
          "replace_all": {"type": "boolean", "description": "Replace every occurrence instead of requiring a unique match."}
        },
        "required": ["old_string", "new_string"]
      }
    },
    "dry_run": {"type": "boolean", "description": "Preview the change (return the diff) without writing to disk."}
  },
  "required": ["path", "edits"]
}`
	return Tool{
		Name:        "multi_edit",
		Description: "Apply several exact-substring replacements to one file in a single atomic call. Edits are applied in order and all must succeed, or the file is left unchanged. Cheaper than repeated edit calls. Set dry_run to preview the diff without writing.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path  string `json:"path"`
				Edits []struct {
					OldString  string `json:"old_string"`
					NewString  string `json:"new_string"`
					ReplaceAll bool   `json:"replace_all"`
				} `json:"edits"`
				DryRun bool `json:"dry_run"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Path == "" {
				return "", fmt.Errorf("path is required")
			}
			if len(args.Edits) == 0 {
				return "", fmt.Errorf("edits is required and must be non-empty")
			}

			full := resolvePath(workdir, args.Path)
			data, err := os.ReadFile(full)
			if err != nil {
				return "", err
			}
			original := string(data)
			content := original
			replacements := 0
			for i, e := range args.Edits {
				if e.OldString == "" {
					return "", fmt.Errorf("edit %d: old_string is required", i+1)
				}
				if e.OldString == e.NewString {
					return "", fmt.Errorf("edit %d: old_string and new_string are identical", i+1)
				}
				count := strings.Count(content, e.OldString)
				if count == 0 {
					return "", fmt.Errorf("edit %d: old_string not found in %s", i+1, args.Path)
				}
				if count > 1 && !e.ReplaceAll {
					return "", fmt.Errorf("edit %d: old_string occurs %d times in %s; add context to make it unique or set replace_all", i+1, count, args.Path)
				}
				if e.ReplaceAll {
					content = strings.ReplaceAll(content, e.OldString, e.NewString)
					replacements += count
				} else {
					content = strings.Replace(content, e.OldString, e.NewString, 1)
					replacements++
				}
			}
			if content == original {
				return fmt.Sprintf("multi_edit %s: no changes", args.Path), nil
			}
			added, removed := diff.Stat(original, content)
			d := diff.Unified(args.Path, original, content)
			if args.DryRun {
				return fmt.Sprintf("[dry run] would edit %s (%d edits, %d replacement(s), +%d -%d)\n\n%s", args.Path, len(args.Edits), replacements, added, removed, d), nil
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("edited %s (%d edits, %d replacement(s), +%d -%d)\n\n%s", args.Path, len(args.Edits), replacements, added, removed, d), nil
		},
	}
}

func newLSTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Directory path, absolute or relative to the working directory. Defaults to the working directory."}
  }
}`
	return Tool{
		Name:        "ls",
		Description: "List the entries of a directory. Directories are shown with a trailing slash.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			dir := workdir
			if args.Path != "" {
				dir = resolvePath(workdir, args.Path)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return "", err
			}
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				names = append(names, name)
			}
			sort.Strings(names)
			if len(names) == 0 {
				return "(empty directory)", nil
			}
			return strings.Join(names, "\n"), nil
		},
	}
}
