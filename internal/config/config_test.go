package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the global config/data env overrides at non-existent files so
// tests only see the project config they create.
func isolate(t *testing.T) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "none.json")
	t.Setenv("EONYX_GLOBAL_CONFIG", missing)
	t.Setenv("EONYX_GLOBAL_DATA", missing)
}

func TestLoadProjectConfigWithEnvExpansion(t *testing.T) {
	isolate(t)
	t.Setenv("MY_KEY", "sk-123")

	dir := t.TempDir()
	cfgJSON := `{
  "providers": {"openai": {"type": "openai", "base_url": "https://api.example.com/v1", "api_key": "$MY_KEY"}},
  "models": [{"id": "gpt-x", "provider": "openai"}],
  "default_model": "gpt-x"
}`
	if err := os.WriteFile(filepath.Join(dir, "eonyx.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Providers["openai"].APIKey; got != "sk-123" {
		t.Errorf("api_key = %q, want expanded sk-123", got)
	}
	if _, ok := cfg.Model("gpt-x"); !ok {
		t.Error("model gpt-x not found")
	}
	if cfg.ProjectRoot() != dir {
		t.Errorf("project root = %q, want %q", cfg.ProjectRoot(), dir)
	}
}

func TestLoadFailsOnIncompatibleLayer(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	// "models" as an object is incompatible with our schema (it expects an array).
	bad := `{"models": {"foo": "bar"}}`
	if err := os.WriteFile(filepath.Join(dir, "eonyx.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("expected Load to fail on an incompatible config")
	}
}

func TestLoadFailsOnUnknownField(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	// An unknown top-level key must be rejected, not silently ignored, and the
	// error must name the field, point to its line, and hint at how to fix it.
	cfgJSON := "{\n  \"providers\": {},\n  \"theme\": \"dark\",\n  \"models\": []\n}"
	if err := os.WriteFile(filepath.Join(dir, "eonyx.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected Load to fail on an unknown field")
	}
	msg := err.Error()
	for _, want := range []string{`unknown field "theme"`, "line 3", "eonyx.example.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestValidateRejectsDanglingDefault(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	bad := `{"default_model": "ghost"}`
	if err := os.WriteFile(filepath.Join(dir, "eonyx.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected validation error for dangling default_model")
	}
}
