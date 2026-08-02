package llm

import "context"

// Provider is the single abstraction over every LLM backend. Stream sends the
// conversation and the available tools to the model and returns a channel of
// events as the response is produced. The channel is closed after a terminal
// event (EventDone or EventError). Implementations must stop producing and
// release resources when ctx is cancelled.
type Provider interface {
	Stream(ctx context.Context, msgs []Message, tools []Tool) (<-chan Event, error)
}

// Event is one item in a streamed model response. It is a closed sum type; the
// implementations below are the only event kinds.
type Event interface {
	isEvent()
}

// EventTextDelta is an incremental chunk of assistant text.
type EventTextDelta struct {
	Text string
}

// EventReasoningDelta is an incremental chunk of a reasoning model's separate
// "thinking" stream, emitted before the answer. Models without reasoning never
// produce it.
type EventReasoningDelta struct {
	Text string
}

// EventToolCall is a fully assembled tool call the model wants executed.
// Providers buffer streamed argument fragments and emit this once complete.
type EventToolCall struct {
	Call ToolCall
}

// EventUsage reports token accounting for the request. It may arrive once at
// the end or be updated as the stream progresses, depending on the provider.
type EventUsage struct {
	InputTokens  int
	OutputTokens int
	// CachedTokens is the portion of InputTokens the provider served from its
	// prompt cache (0 when unknown or unsupported).
	CachedTokens int
}

// EventDone is the terminal event of a successful stream.
type EventDone struct {
	// StopReason is the provider's normalized reason for stopping, e.g.
	// "end_turn", "tool_use", or "max_tokens".
	StopReason string
}

// EventError is the terminal event when streaming fails.
type EventError struct {
	Err error
}

func (EventTextDelta) isEvent()      {}
func (EventReasoningDelta) isEvent() {}
func (EventToolCall) isEvent()       {}
func (EventUsage) isEvent()          {}
func (EventDone) isEvent()           {}
func (EventError) isEvent()          {}
