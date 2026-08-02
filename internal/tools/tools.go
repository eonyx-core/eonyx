// Package tools defines eonyx's executable tools and the registry that
// advertises them to the model. A Tool pairs the wire declaration (name,
// description, parameter schema) with a Handler that runs it. Handlers return
// an ordinary error for recoverable failures (e.g. a missing file); the agent
// reports these back to the model as failed tool results rather than aborting.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"eonyx/internal/llm"
)

// Handler executes a tool. input is the raw JSON arguments produced by the
// model, to be decoded against the tool's schema.
type Handler func(ctx context.Context, input json.RawMessage) (string, error)

// Tool is a registered, callable tool.
type Tool struct {
	Name        string
	Description string
	// Schema is the JSON Schema of the tool's parameters.
	Schema  json.RawMessage
	Handler Handler
}

// Registry holds the available tools in registration order. Tools may be
// disabled at runtime (e.g. by the TUI capabilities toggle); a disabled tool is
// neither advertised to the model nor resolvable for execution.
type Registry struct {
	tools    map[string]Tool
	order    []string
	disabled map[string]bool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Replace installs a tool, overwriting an existing one of the same name while
// preserving its position in the advertisement order. It is used to swap a
// tool's behavior at runtime (e.g. rebuilding load_skill for the active set).
func (r *Registry) Replace(t Tool) {
	if t.Name == "" {
		panic("tools: replacing tool with empty name")
	}
	if _, exists := r.tools[t.Name]; !exists {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

// SetDisabled replaces the set of disabled tool names. Pass nil to enable all.
func (r *Registry) SetDisabled(names map[string]bool) {
	r.disabled = names
}

// Names returns every registered tool name in order, including disabled ones.
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// Register adds a tool. It panics on a duplicate or nameless tool, since the
// tool set is fixed at startup and a clash is a programming error.
func (r *Registry) Register(t Tool) {
	if t.Name == "" {
		panic("tools: registering tool with empty name")
	}
	if _, exists := r.tools[t.Name]; exists {
		panic(fmt.Sprintf("tools: duplicate tool %q", t.Name))
	}
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
}

// Get returns the tool with the given name. A disabled tool is reported as
// absent so the agent refuses to execute it.
func (r *Registry) Get(name string) (Tool, bool) {
	if r.disabled[name] {
		return Tool{}, false
	}
	t, ok := r.tools[name]
	return t, ok
}

// Declarations returns the wire declarations for the enabled tools, in
// registration order, for advertising to a provider.
func (r *Registry) Declarations() []llm.Tool {
	out := make([]llm.Tool, 0, len(r.order))
	for _, name := range r.order {
		if r.disabled[name] {
			continue
		}
		t := r.tools[name]
		out = append(out, llm.Tool{
			Name:        t.Name,
			Description: t.Description,
			Schema:      t.Schema,
		})
	}
	return out
}

// decodeInput unmarshals tool arguments into v, wrapping errors with context.
func decodeInput(input json.RawMessage, v any) error {
	if len(input) == 0 {
		return nil
	}
	if err := json.Unmarshal(input, v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
