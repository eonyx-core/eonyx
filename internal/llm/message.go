// Package llm defines the provider-agnostic conversation model and the
// streaming interface that every backend implements. Backends are
// OpenAI-compatible (OpenAI, OpenRouter, DeepSeek, Moonshot, and local servers
// like Ollama, llama.cpp, or LM Studio); wire-level differences are normalized
// behind these types so the rest of eonyx only ever sees them.
package llm

import "encoding/json"

// Role identifies the author of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	// RoleTool carries the result of a tool execution back to the model.
	RoleTool Role = "tool"
)

// Message is one turn in a conversation. Its content is a sequence of typed
// parts: a single assistant turn may interleave text and tool calls, and a
// tool turn carries one or more tool results.
type Message struct {
	Role    Role
	Content []ContentPart
}

// ContentPart is a single block within a message. It is a closed sum type:
// the only implementations are Text, ToolCall, and ToolResult, all defined in
// this package.
type ContentPart interface {
	isContentPart()
}

// Text is a plain-text content block.
type Text struct {
	Text string
}

// ToolCall is the model's request to invoke a tool. Input holds the raw JSON
// arguments as produced by the model; it is decoded against the tool's schema
// by the executor, not here.
type ToolCall struct {
	// ID correlates this call with its ToolResult. Providers that do not
	// supply an ID (some OpenAI-compatible servers) get one synthesized.
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult carries the output of an executed tool back to the model.
type ToolResult struct {
	// ToolCallID matches the ID of the ToolCall this result answers.
	ToolCallID string
	Content    string
	// IsError marks the tool execution as failed so the model can react.
	IsError bool
}

func (Text) isContentPart()       {}
func (ToolCall) isContentPart()   {}
func (ToolResult) isContentPart() {}

// Helpers for the common single-part messages.

// UserText builds a user message containing a single text block.
func UserText(text string) Message {
	return Message{Role: RoleUser, Content: []ContentPart{Text{Text: text}}}
}

// SystemText builds a system message containing a single text block.
func SystemText(text string) Message {
	return Message{Role: RoleSystem, Content: []ContentPart{Text{Text: text}}}
}
