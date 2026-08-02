package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// maxGrepMatches caps grep output so a broad pattern cannot flood the context.
const maxGrepMatches = 200

// maxGrepFileSize skips files larger than this, avoiding reading huge binaries
// or build artifacts into memory.
const maxGrepFileSize = 5 << 20 // 5 MiB

func newGlobTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "Glob pattern, supporting ** for recursive matches (e.g. **/*.go). Uses forward slashes."},
    "path": {"type": "string", "description": "Base directory to match within. Defaults to the working directory."}
  },
  "required": ["pattern"]
}`
	return Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Supports ** for recursive matching. Returns paths relative to the base directory.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			base := workdir
			if args.Path != "" {
				base = resolvePath(workdir, args.Path)
			}

			matches, err := doublestar.Glob(os.DirFS(base), args.Pattern)
			if err != nil {
				return "", fmt.Errorf("invalid pattern: %w", err)
			}
			if len(matches) == 0 {
				return "(no matches)", nil
			}
			sort.Strings(matches)
			return strings.Join(matches, "\n"), nil
		},
	}
}

func newGrepTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "Regular expression to search for (RE2 syntax)."},
    "path": {"type": "string", "description": "Base directory to search. Defaults to the working directory."},
    "glob": {"type": "string", "description": "Only search files whose name matches this glob (e.g. *.go)."},
    "ignore_case": {"type": "boolean", "description": "Case-insensitive matching."}
  },
  "required": ["pattern"]
}`
	return Tool{
		Name:        "grep",
		Description: "Search file contents for a regular expression, returning matching lines as path:line:text. Skips binary files and .git.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Pattern    string `json:"pattern"`
				Path       string `json:"path"`
				Glob       string `json:"glob"`
				IgnoreCase bool   `json:"ignore_case"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			if args.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}

			expr := args.Pattern
			if args.IgnoreCase {
				expr = "(?i)" + expr
			}
			re, err := regexp.Compile(expr)
			if err != nil {
				return "", fmt.Errorf("invalid pattern: %w", err)
			}

			base := workdir
			if args.Path != "" {
				base = resolvePath(workdir, args.Path)
			}

			var matches []string
			truncated := false
			walkErr := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // skip unreadable entries
				}
				if d.IsDir() {
					if d.Name() == ".git" {
						return fs.SkipDir
					}
					return nil
				}
				if args.Glob != "" {
					if ok, _ := doublestar.Match(args.Glob, d.Name()); !ok {
						return nil
					}
				}
				if len(matches) >= maxGrepMatches {
					truncated = true
					return fs.SkipAll
				}
				if info, statErr := d.Info(); statErr == nil && info.Size() > maxGrepFileSize {
					return nil
				}

				data, err := os.ReadFile(path)
				if err != nil || isBinary(data) {
					return nil
				}
				rel, relErr := filepath.Rel(base, path)
				if relErr != nil {
					rel = path
				}
				rel = filepath.ToSlash(rel)
				for i, line := range strings.Split(string(data), "\n") {
					if re.MatchString(line) {
						matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, i+1, line))
						if len(matches) >= maxGrepMatches {
							truncated = true
							return fs.SkipAll
						}
					}
				}
				return nil
			})
			if walkErr != nil {
				return "", walkErr
			}
			if len(matches) == 0 {
				return "(no matches)", nil
			}
			out := strings.Join(matches, "\n")
			if truncated {
				out += fmt.Sprintf("\n... (truncated at %d matches)", maxGrepMatches)
			}
			return out, nil
		},
	}
}

// isBinary reports whether data looks like a binary file (contains a NUL byte
// in the inspected prefix).
func isBinary(data []byte) bool {
	const sniff = 8000
	if len(data) > sniff {
		data = data[:sniff]
	}
	return bytes.IndexByte(data, 0) >= 0
}
