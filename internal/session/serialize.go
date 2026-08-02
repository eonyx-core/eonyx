package session

import (
	"encoding/json"
	"fmt"

	"eonyx/internal/llm"
)

// messageDTO is the on-disk form of an llm.Message. The content sum type
// (Text/ToolCall/ToolResult) is encoded as a tagged list so it round-trips
// through plain JSON.
type messageDTO struct {
	Role    string    `json:"role"`
	Content []partDTO `json:"content"`
}

type partDTO struct {
	Type string `json:"type"` // "text" | "tool_call" | "tool_result"

	Text string `json:"text,omitempty"` // type=text

	ID    string          `json:"id,omitempty"`    // type=tool_call
	Name  string          `json:"name,omitempty"`  // type=tool_call
	Input json.RawMessage `json:"input,omitempty"` // type=tool_call

	ToolCallID string `json:"tool_call_id,omitempty"` // type=tool_result
	Result     string `json:"result,omitempty"`       // type=tool_result
	IsError    bool   `json:"is_error,omitempty"`     // type=tool_result
}

const (
	partText       = "text"
	partToolCall   = "tool_call"
	partToolResult = "tool_result"
)

// encodeMessage marshals a message to a single JSONL line (without newline).
func encodeMessage(m llm.Message) ([]byte, error) {
	dto := messageDTO{Role: string(m.Role)}
	for _, part := range m.Content {
		switch p := part.(type) {
		case llm.Text:
			dto.Content = append(dto.Content, partDTO{Type: partText, Text: p.Text})
		case llm.ToolCall:
			dto.Content = append(dto.Content, partDTO{Type: partToolCall, ID: p.ID, Name: p.Name, Input: p.Input})
		case llm.ToolResult:
			dto.Content = append(dto.Content, partDTO{Type: partToolResult, ToolCallID: p.ToolCallID, Result: p.Content, IsError: p.IsError})
		default:
			return nil, fmt.Errorf("session: cannot encode content part of type %T", part)
		}
	}
	return json.Marshal(dto)
}

// decodeMessage parses one JSONL line back into a message.
func decodeMessage(line []byte) (llm.Message, error) {
	var dto messageDTO
	if err := json.Unmarshal(line, &dto); err != nil {
		return llm.Message{}, fmt.Errorf("session: decode message: %w", err)
	}

	m := llm.Message{Role: llm.Role(dto.Role)}
	for _, p := range dto.Content {
		switch p.Type {
		case partText:
			m.Content = append(m.Content, llm.Text{Text: p.Text})
		case partToolCall:
			m.Content = append(m.Content, llm.ToolCall{ID: p.ID, Name: p.Name, Input: p.Input})
		case partToolResult:
			m.Content = append(m.Content, llm.ToolResult{ToolCallID: p.ToolCallID, Content: p.Result, IsError: p.IsError})
		default:
			return llm.Message{}, fmt.Errorf("session: unknown content part type %q", p.Type)
		}
	}
	return m, nil
}
