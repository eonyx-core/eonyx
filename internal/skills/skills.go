package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eonyx/internal/tools"
)

// SkillMeta is the frontmatter of a SKILL.md file.
type SkillMeta struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

// Skill represents a local skill loaded from a SKILL.md file.
type Skill struct {
	Dir      string
	FilePath string
	Meta     SkillMeta
	Body     string
}

// Discover scans the given directories for subdirectories containing SKILL.md.
// It returns skills in the order discovered; duplicate names keep the first.
func Discover(dirs []string) []*Skill {
	seen := map[string]bool{}
	var out []*Skill
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name(), "SKILL.md")
			fi, err := os.Stat(path)
			if err != nil || fi.IsDir() {
				continue
			}
			s, err := parseSkill(path, filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			if seen[s.Meta.Name] {
				continue
			}
			seen[s.Meta.Name] = true
			out = append(out, s)
		}
	}
	return out
}

// DefaultDirs returns the standard skill directories, most specific first, so
// that on a name clash Discover keeps the project skill over a global one.
// projectRoot may be empty if no project config was found. EONYX_SKILLS_DIR
// overrides the entire list.
func DefaultDirs(projectRoot string) []string {
	if d := os.Getenv("EONYX_SKILLS_DIR"); d != "" {
		return strings.Split(d, string(os.PathListSeparator))
	}
	var dirs []string
	if projectRoot != "" {
		dirs = append(dirs, filepath.Join(projectRoot, ".eonyx", "skills"))
	}
	if home, _ := os.UserHomeDir(); home != "" {
		dirs = append(dirs, filepath.Join(home, ".config", "eonyx", "skills"))
		dirs = append(dirs, filepath.Join(home, ".config", "agents", "skills"))
	}
	return dirs
}

// Catalog renders a short list of skill names and descriptions for the system
// prompt. This is the "progressive disclosure" layer: only metadata is shown.
func Catalog(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Available Skills\n\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- **%s**: %s\n", s.Meta.Name, s.Meta.Description)
		if len(s.Meta.Tags) > 0 {
			fmt.Fprintf(&b, "  tags: %s\n", strings.Join(s.Meta.Tags, ", "))
		}
	}
	b.WriteString("\nTo load a skill, the model can call the `load_skill` tool with the skill name.")
	return b.String()
}

// Find returns the skill with the given name, or nil if not found.
func Find(skills []*Skill, name string) *Skill {
	for _, s := range skills {
		if s.Meta.Name == name {
			return s
		}
	}
	return nil
}

// parseSkill reads a SKILL.md file and extracts its frontmatter and body.
// Frontmatter sits between triple-dash fences. The standard form is YAML, e.g.:
//
//	---
//	name: go
//	description: Go best practices
//	tags: [style, idioms]
//	---
//
// A JSON object is also accepted for compatibility.
func parseSkill(path, dir string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	body := string(data)
	var meta SkillMeta
	if strings.HasPrefix(body, "---") {
		end := strings.Index(body[3:], "---")
		if end >= 0 {
			front := strings.TrimSpace(body[3 : end+3])
			body = strings.TrimSpace(body[end+6:])
			if err := parseFrontmatter(front, &meta); err != nil {
				return nil, fmt.Errorf("parse frontmatter: %w", err)
			}
		}
	}
	if meta.Name == "" {
		meta.Name = filepath.Base(dir)
	}
	if meta.Description == "" {
		meta.Description = "No description provided"
	}

	return &Skill{
		Dir:      dir,
		FilePath: path,
		Meta:     meta,
		Body:     body,
	}, nil
}

// parseFrontmatter fills meta from frontmatter that is either a JSON object or
// simple `key: value` YAML lines (the keys eonyx uses: name, description,
// tags). It is intentionally minimal rather than a full YAML implementation.
func parseFrontmatter(front string, meta *SkillMeta) error {
	if strings.HasPrefix(front, "{") {
		return json.Unmarshal([]byte(front), meta)
	}
	for _, line := range strings.Split(front, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		switch key {
		case "name":
			meta.Name = val
		case "description":
			meta.Description = val
		case "tags":
			meta.Tags = parseTagList(val)
		}
	}
	return nil
}

// parseTagList parses an inline tag list like "[a, b]" or "a, b" into tags.
func parseTagList(val string) []string {
	val = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(val, "["), "]"))
	if val == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(val, ",") {
		if t = strings.Trim(strings.TrimSpace(t), `"'`); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// NewLoadTool creates a tool that the model can call to load a full skill body.
func NewLoadTool(skills []*Skill) tools.Tool {
	const schema = `{
  "type": "object",
  "properties": {
    "name": {"type": "string", "description": "Name of the skill to load"}
  },
  "required": ["name"]
}`
	return tools.Tool{
		Name:        "load_skill",
		Description: "Load the full contents of a skill by name. Use when a task matches one of the available skills.",
		Schema:      json.RawMessage(schema),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Name string `json:"name"`
			}
			if err := decodeInput(input, &args); err != nil {
				return "", err
			}
			s := Find(skills, args.Name)
			if s == nil {
				return "", fmt.Errorf("skill %q not found", args.Name)
			}
			return s.Body, nil
		},
	}
}

// decodeInput mirrors tools.decodeInput to avoid a circular import.
func decodeInput(input json.RawMessage, v any) error {
	if len(input) == 0 {
		return nil
	}
	if err := json.Unmarshal(input, v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
