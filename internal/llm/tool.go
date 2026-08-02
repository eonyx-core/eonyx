package llm

import "encoding/json"

// Tool is the declaration of a callable tool as advertised to the model. It is
// intentionally just the wire description — name, human-readable purpose, and
// the JSON Schema of its parameters. The executable side (the function that
// runs the tool and the permission gate around it) lives in internal/tools.
type Tool struct {
	Name        string
	Description string
	// Schema is a JSON Schema object describing the tool's parameters. It is
	// passed through to the provider verbatim, so it must be valid JSON.
	Schema json.RawMessage
}
