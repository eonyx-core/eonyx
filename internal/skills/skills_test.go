package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, name, body string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillDir
}

func TestDiscoverParsesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "go", `---
{"name": "go", "description": "Go best practices", "tags": ["backend"]}
---

Use gofmt, keep packages small.`)

	skills := Discover([]string{dir})
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Meta.Name != "go" {
		t.Errorf("name = %q, want go", skills[0].Meta.Name)
	}
	if skills[0].Meta.Description != "Go best practices" {
		t.Errorf("description = %q", skills[0].Meta.Description)
	}
	if !strings.Contains(skills[0].Body, "gofmt") {
		t.Errorf("body missing gofmt: %q", skills[0].Body)
	}
}

func TestDiscoverIgnoresMissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "rust", "Rust safety rules.")

	skills := Discover([]string{dir})
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Meta.Name != "rust" {
		t.Errorf("name = %q, want rust", skills[0].Meta.Name)
	}
	if skills[0].Meta.Description == "" {
		t.Error("expected a default description")
	}
}

func TestDiscoverDuplicatesFirstWins(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeSkill(t, dir1, "dup", `---
{"name": "dup", "description": "first"}
---
body1`)
	writeSkill(t, dir2, "dup", `---
{"name": "dup", "description": "second"}
---
body2`)

	skills := Discover([]string{dir1, dir2})
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Meta.Description != "first" {
		t.Errorf("description = %q, want first", skills[0].Meta.Description)
	}
}

func TestDiscoverSkipsNonDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-a-skill.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if skills := Discover([]string{dir}); len(skills) != 0 {
		t.Errorf("got %d skills, want 0", len(skills))
	}
}

func TestCatalogEmpty(t *testing.T) {
	if got := Catalog(nil); got != "" {
		t.Errorf("Catalog(nil) = %q, want empty", got)
	}
}

func TestCatalogRenders(t *testing.T) {
	s := []*Skill{
		{Meta: SkillMeta{Name: "a", Description: "desc a", Tags: []string{"t1"}}},
		{Meta: SkillMeta{Name: "b", Description: "desc b"}},
	}
	out := Catalog(s)
	if !strings.Contains(out, "a") || !strings.Contains(out, "desc a") {
		t.Errorf("catalog missing skill a: %q", out)
	}
	if !strings.Contains(out, "b") || !strings.Contains(out, "desc b") {
		t.Errorf("catalog missing skill b: %q", out)
	}
	if !strings.Contains(out, "tags: t1") {
		t.Errorf("catalog missing tags: %q", out)
	}
}

func TestFind(t *testing.T) {
	s := []*Skill{{Meta: SkillMeta{Name: "x"}}}
	if got := Find(s, "x"); got == nil {
		t.Error("Find(x) = nil, want skill")
	}
	if got := Find(s, "y"); got != nil {
		t.Error("Find(y) should be nil")
	}
}

func TestLoadTool(t *testing.T) {
	s := []*Skill{{
		Meta: SkillMeta{Name: "go"},
		Body: "body",
	}}
	tool := NewLoadTool(s)
	out, err := tool.Handler(t.Context(), []byte(`{"name":"go"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out != "body" {
		t.Errorf("output = %q, want body", out)
	}

	if _, err := tool.Handler(t.Context(), []byte(`{"name":"missing"}`)); err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestDefaultDirs(t *testing.T) {
	// Without project root, at least global dirs are returned on systems with home.
	dirs := DefaultDirs("")
	home, _ := os.UserHomeDir()
	if home == "" && len(dirs) != 0 {
		t.Errorf("expected no dirs without home, got %v", dirs)
	}

	project := t.TempDir()
	dirs = DefaultDirs(project)
	if len(dirs) == 0 {
		t.Fatal("expected project dir")
	}
	// The project dir comes first so it wins on a name clash with a global skill.
	if got := dirs[0]; got != filepath.Join(project, ".eonyx", "skills") {
		t.Errorf("first dir = %q, want %q", got, filepath.Join(project, ".eonyx", "skills"))
	}
}

func TestDiscoverParsesYAMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "go", `---
name: go
description: Go best practices
tags: [backend, style]
---

Use gofmt.`)

	skills := Discover([]string{dir})
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Meta.Name != "go" {
		t.Errorf("name = %q, want go", skills[0].Meta.Name)
	}
	if skills[0].Meta.Description != "Go best practices" {
		t.Errorf("description = %q", skills[0].Meta.Description)
	}
	if len(skills[0].Meta.Tags) != 2 || skills[0].Meta.Tags[0] != "backend" {
		t.Errorf("tags = %v, want [backend style]", skills[0].Meta.Tags)
	}
	if !strings.Contains(skills[0].Body, "gofmt") {
		t.Errorf("body missing gofmt: %q", skills[0].Body)
	}
}
