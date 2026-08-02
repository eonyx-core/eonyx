package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectConfig(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(tmp, "project", "parent")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(project, "eonyx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"providers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	path, root, found := FindProjectConfig(child)
	if !found {
		t.Fatal("expected to find project config")
	}
	if root != project {
		t.Errorf("root = %q, want %q", root, project)
	}
	if path != cfgPath {
		t.Errorf("path = %q, want %q", path, cfgPath)
	}
}

func TestFindProjectConfigHiddenName(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".eonyx.json")
	if err := os.WriteFile(cfgPath, []byte(`{"providers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	path, root, found := FindProjectConfig(tmp)
	if !found {
		t.Fatal("expected to find hidden project config")
	}
	if root != tmp {
		t.Errorf("root = %q, want %q", root, tmp)
	}
	if path != cfgPath {
		t.Errorf("path = %q, want %q", path, cfgPath)
	}
}

func TestFindProjectConfigIgnoresDirectory(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "eonyx.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, found := FindProjectConfig(tmp); found {
		t.Error("expected directory named eonyx.json to be ignored")
	}
}

func TestFindProjectConfigNotFound(t *testing.T) {
	tmp := t.TempDir()
	if _, _, found := FindProjectConfig(tmp); found {
		t.Error("expected no config in empty tree")
	}
}

func TestFindProjectConfigStopsAtRoot(t *testing.T) {
	tmp := t.TempDir()
	// No config anywhere; should walk up to tmp and stop, not error.
	_, _, found := FindProjectConfig(tmp)
	if found {
		t.Error("expected no config")
	}
}
