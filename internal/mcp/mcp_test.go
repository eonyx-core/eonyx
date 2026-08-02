package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"eonyx/internal/tools"
)

func TestFormatResources(t *testing.T) {
	out := formatResources([]*mcp.Resource{
		{URI: "file://a.txt", Name: "a", Description: "the a file"},
		{URI: "mem://b", Title: "B"},
	})
	if !strings.Contains(out, "file://a.txt") || !strings.Contains(out, "the a file") {
		t.Errorf("resource listing missing fields:\n%s", out)
	}
	if !strings.Contains(out, "B") {
		t.Errorf("title not shown:\n%s", out)
	}
	if formatResources(nil) != "(no resources)" {
		t.Error("empty listing should say (no resources)")
	}
}

func TestFormatResourceContents(t *testing.T) {
	out := formatResourceContents([]*mcp.ResourceContents{{URI: "file://a", Text: "hello"}})
	if out != "hello" {
		t.Errorf("text content = %q, want hello", out)
	}
	blob := formatResourceContents([]*mcp.ResourceContents{{URI: "file://b", Blob: []byte{1, 2, 3}}})
	if !strings.Contains(blob, "binary") || !strings.Contains(blob, "3 bytes") {
		t.Errorf("blob content = %q, want a binary note", blob)
	}
}

func TestFormatPrompts(t *testing.T) {
	out := formatPrompts([]*mcp.Prompt{{
		Name:        "greet",
		Description: "say hi",
		Arguments:   []*mcp.PromptArgument{{Name: "who", Required: true}, {Name: "tone"}},
	}})
	for _, want := range []string{"greet", "who*", "tone", "say hi"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt listing missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatPromptMessages(t *testing.T) {
	out := formatPromptMessages(&mcp.GetPromptResult{
		Description: "a greeting",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: "hello there"}},
		},
	})
	if !strings.Contains(out, "a greeting") || !strings.Contains(out, "[user]") || !strings.Contains(out, "hello there") {
		t.Errorf("prompt messages missing parts:\n%s", out)
	}
}

func TestRegisterFromClientRegistersResourcesAndPrompts(t *testing.T) {
	fake := &fakeClient{tools: []*mcp.Tool{{Name: "t", InputSchema: map[string]any{"type": "object"}}}}
	reg := tools.NewRegistry()
	if err := RegisterFromClient(context.Background(), reg, fake); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fake_t", "fake_read_resource", "fake_get_prompt"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("expected %s to be registered", name)
		}
	}
}

func TestRegisterFromClientSkipsUnsupportedCapabilities(t *testing.T) {
	fake := &fakeClient{
		tools:     []*mcp.Tool{{Name: "t", InputSchema: map[string]any{"type": "object"}}},
		resErr:    errors.New("method not found"),
		promptErr: errors.New("method not found"),
	}
	reg := tools.NewRegistry()
	if err := RegisterFromClient(context.Background(), reg, fake); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("fake_t"); !ok {
		t.Error("the tool should still register when resources/prompts are unsupported")
	}
	if _, ok := reg.Get("fake_read_resource"); ok {
		t.Error("resource tools should be skipped when unsupported")
	}
	if _, ok := reg.Get("fake_get_prompt"); ok {
		t.Error("prompt tools should be skipped when unsupported")
	}
}

func TestToolNamePrefixing(t *testing.T) {
	if got := toolName("server", "read"); got != "server_read" {
		t.Errorf("toolName = %q, want server_read", got)
	}
}

func TestFormatResult(t *testing.T) {
	res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "hello"}}}
	if got := formatResult(res); got != "hello" {
		t.Errorf("formatResult = %q, want hello", got)
	}
}

func TestConvertTool(t *testing.T) {
	mcpTool := &mcp.Tool{
		Name:        "read",
		Description: "read a file",
		InputSchema: map[string]any{"type": "object"},
	}
	called := false
	handler := func(context.Context, json.RawMessage) (string, error) {
		called = true
		return "ran", nil
	}
	converted := toolFromMCP("server", mcpTool, handler)
	if converted.Name != "server_read" {
		t.Errorf("name = %q, want server_read", converted.Name)
	}
	if converted.Description != "read a file" {
		t.Errorf("description = %q", converted.Description)
	}
	if len(converted.Schema) == 0 {
		t.Error("expected schema")
	}
	// The handler must be the one passed in, not a discarded stub.
	if converted.Handler == nil {
		t.Fatal("expected a handler to be wired")
	}
	out, err := converted.Handler(context.Background(), nil)
	if err != nil || out != "ran" || !called {
		t.Errorf("handler not wired: out=%q err=%v called=%v", out, err, called)
	}
}

func TestRegisterFromClientConvertsTools(t *testing.T) {
	fake := &fakeClient{
		tools: []*mcp.Tool{{
			Name:        "greet",
			Description: "hi",
			InputSchema: map[string]any{"type": "object"},
		}},
	}
	reg := tools.NewRegistry()
	if err := RegisterFromClient(context.Background(), reg, fake); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := reg.Get("fake_greet"); !ok {
		t.Error("expected fake_greet to be registered")
	}
}

type fakeClient struct {
	tools     []*mcp.Tool
	resErr    error
	promptErr error
}

func (f *fakeClient) DiscoverTools(ctx context.Context) ([]tools.Tool, error) {
	var out []tools.Tool
	for _, t := range f.tools {
		name := t.Name
		out = append(out, toolFromMCP("fake", t, func(context.Context, json.RawMessage) (string, error) {
			return "called " + name, nil
		}))
	}
	return out, nil
}

func noopTool(name string) tools.Tool {
	return tools.Tool{Name: name, Schema: json.RawMessage(`{}`),
		Handler: func(context.Context, json.RawMessage) (string, error) { return "", nil }}
}

func (f *fakeClient) DiscoverResourceTools(context.Context) ([]tools.Tool, error) {
	if f.resErr != nil {
		return nil, f.resErr
	}
	return []tools.Tool{noopTool("fake_read_resource")}, nil
}

func (f *fakeClient) DiscoverPromptTools(context.Context) ([]tools.Tool, error) {
	if f.promptErr != nil {
		return nil, f.promptErr
	}
	return []tools.Tool{noopTool("fake_get_prompt")}, nil
}

func (f *fakeClient) Close() error { return nil }

// ensure fakeClient implements the client interface used by RegisterFromClient.
var _ interface {
	DiscoverTools(context.Context) ([]tools.Tool, error)
	Close() error
} = (*fakeClient)(nil)
