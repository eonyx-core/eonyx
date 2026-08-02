package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDefaultConfigCreatesAndLoads(t *testing.T) {
	globalDir := t.TempDir()
	t.Setenv("EONYX_GLOBAL_CONFIG", filepath.Join(globalDir, "eonyx.json"))
	t.Setenv("EONYX_GLOBAL_DATA", filepath.Join(t.TempDir(), "data.json"))

	startDir := t.TempDir()
	path, created, err := EnsureDefaultConfig(startDir)
	if err != nil {
		t.Fatalf("EnsureDefaultConfig: %v", err)
	}
	if !created {
		t.Fatal("expected a default config to be created")
	}
	if !fileExists(path) {
		t.Fatalf("default config not written at %s", path)
	}

	cfg, err := Load(startDir)
	if err != nil {
		t.Fatalf("Load default config: %v", err)
	}
	if cfg.DefaultModel != "qwen2.5-coder" {
		t.Errorf("default_model = %q, want qwen2.5-coder", cfg.DefaultModel)
	}
	for _, id := range []string{"deepseek-v4-flash", "deepseek-v4-pro", "qwen2.5-coder"} {
		if _, ok := cfg.Model(id); !ok {
			t.Errorf("model %q missing from default config", id)
		}
	}
}

func TestEnsureDefaultConfigSkipsWhenProjectExists(t *testing.T) {
	t.Setenv("EONYX_GLOBAL_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("EONYX_GLOBAL_DATA", filepath.Join(t.TempDir(), "missing.json"))

	startDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(startDir, "eonyx.json"), []byte(`{"providers":{},"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, created, err := EnsureDefaultConfig(startDir); err != nil || created {
		t.Fatalf("EnsureDefaultConfig created=%v err=%v, want created=false", created, err)
	}
}

func TestEnsureDefaultConfigSkipsWhenGlobalExists(t *testing.T) {
	globalPath := filepath.Join(t.TempDir(), "eonyx.json")
	if err := os.WriteFile(globalPath, []byte(`{"providers":{},"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EONYX_GLOBAL_CONFIG", globalPath)
	t.Setenv("EONYX_GLOBAL_DATA", filepath.Join(t.TempDir(), "missing.json"))

	if _, created, err := EnsureDefaultConfig(t.TempDir()); err != nil || created {
		t.Fatalf("EnsureDefaultConfig created=%v err=%v, want created=false", created, err)
	}
}
