package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// pickerEntry is one item returned by the file-picker tool.
type pickerEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Size     int64  `json:"size,omitempty"`
	Modified int64  `json:"modified_at,omitempty"`
}

func newPickerTool(workdir string) Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Directory to list, absolute or relative to the working directory. Defaults to the working directory."},
    "select": {"type": "string", "description": "If provided, return the resolved absolute path of this file or directory instead of listing."}
  }
}`
	return Tool{
		Name:        "picker",
		Description: "Browse the project tree. List entries in a directory with metadata, or resolve a selected path. Call repeatedly to navigate and select files.",
		Schema:      json.RawMessage(schema),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path   string `json:"path"`
				Select string `json:"select"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}

			if args.Select != "" {
				p := resolvePath(workdir, args.Select)
				fi, err := os.Stat(p)
				if err != nil {
					return "", err
				}
				entry := pickerEntry{
					Name: filepath.Base(p),
					Path: p,
					Type: entryType(fi),
				}
				if !fi.IsDir() {
					entry.Size = fi.Size()
					entry.Modified = fi.ModTime().Unix()
				}
				return formatEntry(entry), nil
			}

			dir := workdir
			if args.Path != "" {
				dir = resolvePath(workdir, args.Path)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return "", err
			}

			var items []pickerEntry
			for _, e := range entries {
				name := e.Name()
				if isHidden(name) {
					continue
				}
				fi, err := e.Info()
				if err != nil {
					continue
				}
				item := pickerEntry{
					Name: name,
					Path: filepath.Join(dir, name),
					Type: entryType(fi),
				}
				if !fi.IsDir() {
					item.Size = fi.Size()
					item.Modified = fi.ModTime().Unix()
				}
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].Type != items[j].Type {
					return items[i].Type == "dir"
				}
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			})
			return formatList(items), nil
		},
	}
}

func entryType(fi os.FileInfo) string {
	if fi.IsDir() {
		return "dir"
	}
	return "file"
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".") && name != "."
}

func formatEntry(e pickerEntry) string {
	data, _ := json.Marshal(e)
	return string(data)
}

func formatList(items []pickerEntry) string {
	var b strings.Builder
	for _, it := range items {
		marker := "  "
		if it.Type == "dir" {
			marker = "[dir] "
		}
		fmt.Fprintf(&b, "%s%s\n", marker, it.Name)
	}
	data, _ := json.Marshal(items)
	b.WriteString("\n")
	b.Write(data)
	return b.String()
}
