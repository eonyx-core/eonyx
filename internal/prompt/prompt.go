// Package prompt assembles the agent's system prompt: the base instructions
// plus any project context files (AGENTS.md) discovered near the working
// directory.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eonyx/internal/skills"
)

const baseTemplate = `You are eonyx, a terminal-based AI coding assistant working in the user's project.

You have tools to read, write, and edit files, list directories, glob and grep the codebase, run shell commands, and fetch web pages. Inspect the project with these tools before making changes, and prefer small, verifiable steps. Keep prose concise; when the task is done, briefly state what you changed.

Working directory: %s`

// contextFileNames are the project context files searched for at each directory.
var contextFileNames = []string{"AGENTS.md"}

// System builds the system prompt for workdir. It appends the contents of any
// context files found from the outermost search boundary down to workdir, so
// that more specific (closer) instructions come last. stopDir bounds the
// upward search (typically the project root); the search also stops at a
// directory containing .git or at the filesystem root.
func System(workdir, stopDir string, skillSet []*skills.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, baseTemplate, workdir)

	for _, f := range discover(workdir, stopDir) {
		b.WriteString("\n\n# Project context: ")
		b.WriteString(f.name)
		b.WriteString("\n\n")
		b.WriteString(f.content)
	}
	b.WriteString(skills.Catalog(skillSet))
	return b.String()
}

type contextFile struct {
	name    string // path relative to workdir, slash-separated
	content string
}

func discover(workdir, stopDir string) []contextFile {
	var dirs []string
	dir := workdir
	for {
		dirs = append(dirs, dir)
		if dir == stopDir || hasGit(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// dirs runs workdir -> outermost; walk it in reverse so outer files are
	// added first and the nearest file last.
	var files []contextFile
	for i := len(dirs) - 1; i >= 0; i-- {
		for _, name := range contextFileNames {
			path := filepath.Join(dirs[i], name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			rel, relErr := filepath.Rel(workdir, path)
			if relErr != nil {
				rel = path
			}
			files = append(files, contextFile{
				name:    filepath.ToSlash(rel),
				content: strings.TrimSpace(string(data)),
			})
		}
	}
	return files
}

func hasGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}
