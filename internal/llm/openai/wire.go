package openai

import (
	"encoding/json"
	"strings"

	"eonyx/internal/llm"
)

// Wire types for the OpenAI Chat Completions API. Only the fields eonyx
// uses are modeled.

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	Tools         []chatTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	// Index is present only in streamed responses, identifying which call a
	// fragment belongs to. It is omitted from requests.
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatToolDecl `json:"function"`
}

type chatToolDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// chatChunk is one server-sent event in a streamed completion.
type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			// ReasoningContent (DeepSeek R1) / Reasoning (some gateways) carry the
			// model's separate thinking stream, when it emits one.
			ReasoningContent string         `json:"reasoning_content"`
			Reasoning        string         `json:"reasoning"`
			ToolCalls        []chatToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		// PromptCacheHitTokens is DeepSeek's cached-prefix count; OpenAI reports
		// the same figure under PromptTokensDetails.CachedTokens.
		PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
		PromptTokensDetails  *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// apiError is the standard OpenAI error envelope.
type apiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// toChatMessages converts eonyx messages to the OpenAI wire format. A single
// tool-role message carrying several results expands into one wire message per
// result, as the API requires.
func toChatMessages(msgs []llm.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			for _, part := range m.Content {
				if tr, ok := part.(llm.ToolResult); ok {
					out = append(out, chatMessage{
						Role:       "tool",
						ToolCallID: tr.ToolCallID,
						Content:    tr.Content,
					})
				}
			}
			continue
		}

		cm := chatMessage{Role: string(m.Role)}
		var text strings.Builder
		for _, part := range m.Content {
			switch p := part.(type) {
			case llm.Text:
				text.WriteString(p.Text)
			case llm.ToolCall:
				cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
					ID:       p.ID,
					Type:     "function",
					Function: chatFunction{Name: p.Name, Arguments: string(p.Input)},
				})
			}
		}
		cm.Content = text.String()
		out = append(out, cm)
	}
	return out
}

// toChatTools converts tool declarations to the OpenAI wire format.
func toChatTools(tools []llm.Tool) []chatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]chatTool, len(tools))
	for i, t := range tools {
		out[i] = chatTool{
			Type: "function",
			Function: chatToolDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		}
	}
	return out
}

// normalizeFinish maps OpenAI finish_reason values onto eonyx's normalized
// stop reasons.
func normalizeFinish(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}
