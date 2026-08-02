package provider

import (
	"net/http"
	"testing"

	"eonyx/internal/config"
	"eonyx/internal/llm/openai"
)

func TestForEmptyModelIDUsesDefault(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"p1": {Type: "openai", BaseURL: "http://example.com"},
		},
		Models:       []config.Model{{ID: "m1", Provider: "p1"}},
		DefaultModel: "m1",
	}

	prov, err := For(cfg, "", http.DefaultClient)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if prov == nil {
		t.Fatal("provider is nil")
	}
	if _, ok := prov.(*openai.Client); !ok {
		t.Errorf("got %T, want *openai.Client", prov)
	}
}

func TestForNoDefaultReturnsError(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"p1": {Type: "openai", BaseURL: "http://example.com"},
		},
		Models: []config.Model{{ID: "m1", Provider: "p1"}},
	}

	if _, err := For(cfg, "", http.DefaultClient); err == nil {
		t.Fatal("expected error when modelID and default_model are empty")
	}
}

func TestForUnknownModel(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"p1": {Type: "openai", BaseURL: "http://example.com"},
		},
		Models: []config.Model{{ID: "m1", Provider: "p1"}},
	}

	if _, err := For(cfg, "missing", http.DefaultClient); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestForUnknownProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"p1": {Type: "openai", BaseURL: "http://example.com"},
		},
		Models: []config.Model{{ID: "m1", Provider: "missing"}},
	}

	if _, err := For(cfg, "m1", http.DefaultClient); err == nil {
		t.Fatal("expected error for model referencing unknown provider")
	}
}

func TestForUnsupportedProviderType(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"p1": {Type: "unknown", BaseURL: "http://example.com"},
		},
		Models: []config.Model{{ID: "m1", Provider: "p1"}},
	}

	if _, err := For(cfg, "m1", http.DefaultClient); err == nil {
		t.Fatal("expected error for unsupported provider type")
	}
}

func TestForExplicitModelID(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.Provider{
			"p1": {Type: "openai", BaseURL: "http://example.com"},
		},
		Models:       []config.Model{{ID: "m1", Provider: "p1"}, {ID: "m2", Provider: "p1"}},
		DefaultModel: "m1",
	}

	prov, err := For(cfg, "m2", http.DefaultClient)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if prov == nil {
		t.Fatal("provider is nil")
	}
}
