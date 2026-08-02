// Package agent runs the core loop: it streams a model response, executes any
// tools the model calls, feeds the results back, and repeats until the model
// stops calling tools. It holds the running conversation so the same Agent can
// be reused across turns.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"eonyx/internal/hooks"
	"eonyx/internal/llm"
	"eonyx/internal/tools"
)

// ErrMaxSteps is returned (wrapped) by Send when the loop hits the tool
// round-trip limit without the model producing a final answer. Callers can test
// for it with errors.Is to distinguish this from a provider or persistence
// failure.
var ErrMaxSteps = errors.New("agent: step limit reached")

// readOnlyTools are side-effect-free tools that may run concurrently within a
// turn. It mirrors the permission package's always-allowed set; keep them in
// sync. Anything not listed here (bash, write, edit, and MCP tools) runs
// sequentially so shared state and permission prompts stay ordered.
var readOnlyTools = map[string]bool{
	"read":       true,
	"ls":         true,
	"glob":       true,
	"grep":       true,
	"git_status": true,
	"git_diff":   true,
	"git_log":    true,
}

func isReadOnly(name string) bool { return readOnlyTools[name] }

// DefaultMaxSteps is the tool-calling round-trip limit per turn when a config
// or Options value is not set.
const DefaultMaxSteps = 50

// Events is a set of optional callbacks the agent invokes as work progresses.
// Any nil callback is skipped.
type Events struct {
	// Text receives assistant text as it streams.
	Text func(delta string)
	// Reasoning receives a reasoning model's separate thinking stream as it
	// streams (only for models that emit one). It is not persisted.
	Reasoning func(delta string)
	// ToolStart fires before a tool runs.
	ToolStart func(name string, input []byte)
	// ToolEnd fires after a tool runs, with its result and error flag.
	ToolEnd func(name, result string, isError bool)
	// StepStart fires at the start of each loop iteration with the 1-based step
	// number and the configured limit, so callers can show progress toward it.
	StepStart func(step, maxSteps int)
}

// Options configures an Agent.
type Options struct {
	// History seeds the conversation, e.g. a system message for a fresh chat or
	// a resumed conversation loaded from a store. These messages are not passed
	// to Persist, since they already exist.
	History []llm.Message
	// MaxSteps bounds tool-calling round-trips per Send (default 50).
	MaxSteps int
	// Events receives best-effort progress callbacks (streaming text, tools).
	Events Events
	// Persist, if set, is called for every new message committed to the
	// conversation (user, assistant, tool). An error from Persist aborts the
	// turn, so eonyx never continues a conversation it failed to record.
	Persist func(llm.Message) error
	// Authorize, if set, gates each tool call. Returning an error refuses the
	// call; the refusal is reported to the model as a failed tool result, so
	// the conversation stays consistent and the model can adapt.
	Authorize func(name string, input []byte) error
	// Hooks, if set, runs configured before/after shell scripts around each
	// tool call. Before-hook errors refuse the tool; after-hook output is
	// appended to the tool result.
	Hooks *hooks.Engine
	// ContextTokens is the model's usable context window in tokens. When > 0,
	// older history is summarized as the conversation approaches the limit.
	ContextTokens int
}

// Agent drives the conversation with one provider and tool set.
type Agent struct {
	provider      llm.Provider
	tools         *tools.Registry
	maxSteps      int
	events        Events
	persist       func(llm.Message) error
	authorize     func(name string, input []byte) error
	hooks         *hooks.Engine
	contextTokens int
	baseSystem    string

	history      []llm.Message
	inTokens     int
	outTokens    int
	cachedTokens int
}

// New builds an Agent.
func New(provider llm.Provider, registry *tools.Registry, opts Options) *Agent {
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	a := &Agent{
		provider:      provider,
		tools:         registry,
		maxSteps:      maxSteps,
		events:        opts.Events,
		persist:       opts.Persist,
		authorize:     opts.Authorize,
		hooks:         opts.Hooks,
		contextTokens: opts.ContextTokens,
	}
	a.history = append(a.history, opts.History...)
	a.baseSystem = baseSystemText(a.history)
	return a
}

// baseSystemText returns the text of the leading system message, if any. It is
// the stable instruction text that summarization preserves while folding the
// running conversation summary into the system prompt.
func baseSystemText(history []llm.Message) string {
	if len(history) == 0 || history[0].Role != llm.RoleSystem {
		return ""
	}
	var b strings.Builder
	for _, part := range history[0].Content {
		if t, ok := part.(llm.Text); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// Send adds the user's message and runs the loop until the model produces a
// final answer (no more tool calls), returning that answer text.
func (a *Agent) Send(ctx context.Context, userText string) (string, error) {
	slog.Info("agent turn start", "chars", len(userText), "history", len(a.history))
	if err := a.commit(llm.UserText(userText)); err != nil {
		return "", err
	}

	for step := 0; step < a.maxSteps; step++ {
		if a.events.StepStart != nil {
			a.events.StepStart(step+1, a.maxSteps)
		}
		if err := a.maybeSummarize(ctx); err != nil {
			return "", err
		}
		text, toolCalls, err := a.stream(ctx)
		if err != nil {
			slog.Error("agent stream failed", "step", step+1, "err", err)
			return "", err
		}

		assistant := llm.Message{Role: llm.RoleAssistant}
		if text != "" {
			assistant.Content = append(assistant.Content, llm.Text{Text: text})
		}
		for _, tc := range toolCalls {
			assistant.Content = append(assistant.Content, tc)
		}
		if err := a.commit(assistant); err != nil {
			return "", err
		}

		if len(toolCalls) == 0 {
			return text, nil
		}

		results := llm.Message{Role: llm.RoleTool, Content: a.executeTools(ctx, toolCalls)}
		if err := a.commit(results); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("stopped after the step limit of %d tool round-trips without a final answer — raise \"max_steps\" in your config if the task needs more: %w", a.maxSteps, ErrMaxSteps)
}

// commit appends a message to the in-memory history and durably records it via
// Persist (if configured). A persistence failure is returned so the caller can
// stop the turn.
func (a *Agent) commit(msg llm.Message) error {
	a.history = append(a.history, msg)
	if a.persist != nil {
		if err := a.persist(msg); err != nil {
			return fmt.Errorf("agent: persist message: %w", err)
		}
	}
	return nil
}

// Usage returns the accumulated input and output token counts.
func (a *Agent) Usage() (in, out int) { return a.inTokens, a.outTokens }

// Cached returns the accumulated count of input tokens the provider served from
// its prompt cache.
func (a *Agent) Cached() int { return a.cachedTokens }

// History returns the current in-memory conversation history. Callers must not
// modify the returned slice.
func (a *Agent) History() []llm.Message { return a.history }

// stream consumes one provider response, returning the assistant text and any
// tool calls. It forwards text deltas to the Text callback.
func (a *Agent) stream(ctx context.Context) (string, []llm.ToolCall, error) {
	ch, err := a.provider.Stream(ctx, a.history, a.tools.Declarations())
	if err != nil {
		return "", nil, err
	}

	var text strings.Builder
	var toolCalls []llm.ToolCall
	for ev := range ch {
		switch e := ev.(type) {
		case llm.EventTextDelta:
			text.WriteString(e.Text)
			if a.events.Text != nil {
				a.events.Text(e.Text)
			}
		case llm.EventReasoningDelta:
			// Reasoning is display-only: forwarded to the UI but never added to
			// the persisted assistant message.
			if a.events.Reasoning != nil {
				a.events.Reasoning(e.Text)
			}
		case llm.EventToolCall:
			toolCalls = append(toolCalls, e.Call)
		case llm.EventUsage:
			a.inTokens += e.InputTokens
			a.outTokens += e.OutputTokens
			a.cachedTokens += e.CachedTokens
		case llm.EventError:
			return "", nil, e.Err
		case llm.EventDone:
		}
	}
	// A cancelled context makes a provider close its stream early; surface that
	// as an error rather than treating the partial response as a final answer.
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	return text.String(), toolCalls, nil
}

// execTool runs one tool call, returning its result content and whether it
// failed. Failures are returned as content (not Go errors) so the loop can feed
// them back to the model.
func (a *Agent) execTool(ctx context.Context, tc llm.ToolCall) (string, bool) {
	if a.events.ToolStart != nil {
		a.events.ToolStart(tc.Name, tc.Input)
	}
	content, isError := a.execToolBody(ctx, tc)
	if a.events.ToolEnd != nil {
		a.events.ToolEnd(tc.Name, content, isError)
	}
	return content, isError
}

// execToolBody runs the before/after hooks and the tool itself without emitting
// any events, so a batch of read-only calls can run concurrently while the
// caller emits the ordered ToolStart/ToolEnd events.
func (a *Agent) execToolBody(ctx context.Context, tc llm.ToolCall) (string, bool) {
	start := time.Now()
	if a.hooks != nil {
		if err := a.hooks.Before(ctx, tc.Name, tc.Input); err != nil {
			slog.Warn("tool blocked by before-hook", "tool", tc.Name, "err", err)
			return err.Error(), true
		}
	}
	content, isError := a.runTool(ctx, tc)
	if a.hooks != nil && !isError {
		content = a.hooks.After(ctx, tc.Name, tc.Input, content)
	}
	slog.Info("tool done", "tool", tc.Name, "error", isError, "ms", time.Since(start).Milliseconds())
	return content, isError
}

// executeTools runs a turn's tool calls and returns their results in input
// order. Consecutive read-only calls are run concurrently; mutating calls (and
// any singleton read-only call) run sequentially, so ordering relative to
// side-effecting tools is preserved.
func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolCall) []llm.ContentPart {
	results := make([]llm.ContentPart, len(calls))
	for i := 0; i < len(calls); {
		if !isReadOnly(calls[i].Name) {
			results[i] = a.toolResult(ctx, calls[i])
			i++
			continue
		}
		j := i + 1
		for j < len(calls) && isReadOnly(calls[j].Name) {
			j++
		}
		if j-i == 1 {
			results[i] = a.toolResult(ctx, calls[i])
		} else {
			a.execConcurrent(ctx, calls[i:j], results[i:j])
		}
		i = j
	}
	return results
}

// toolResult runs one tool call (emitting its events) and wraps the outcome.
func (a *Agent) toolResult(ctx context.Context, tc llm.ToolCall) llm.ToolResult {
	content, isErr := a.execTool(ctx, tc)
	return llm.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
}

// execConcurrent runs a batch of read-only calls in parallel but emits their
// ToolStart/ToolEnd events and writes their results in input order, so event
// consumers (the TUI) still see a stable, correlated sequence.
func (a *Agent) execConcurrent(ctx context.Context, calls []llm.ToolCall, out []llm.ContentPart) {
	if a.events.ToolStart != nil {
		for _, tc := range calls {
			a.events.ToolStart(tc.Name, tc.Input)
		}
	}
	type outcome struct {
		content string
		isErr   bool
	}
	outcomes := make([]outcome, len(calls))
	var wg sync.WaitGroup
	for k := range calls {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			outcomes[k].content, outcomes[k].isErr = a.execToolBody(ctx, calls[k])
		}(k)
	}
	wg.Wait()
	for k, tc := range calls {
		if a.events.ToolEnd != nil {
			a.events.ToolEnd(tc.Name, outcomes[k].content, outcomes[k].isErr)
		}
		out[k] = llm.ToolResult{ToolCallID: tc.ID, Content: outcomes[k].content, IsError: outcomes[k].isErr}
	}
}

// runTool resolves, authorizes, and executes a tool call. A missing tool, a
// permission refusal, or a handler error all return an error message with the
// error flag set, so the model is told what happened.
func (a *Agent) runTool(ctx context.Context, tc llm.ToolCall) (string, bool) {
	tool, ok := a.tools.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("unknown tool %q", tc.Name), true
	}
	if a.authorize != nil {
		if err := a.authorize(tc.Name, tc.Input); err != nil {
			return err.Error(), true
		}
	}
	out, err := tool.Handler(ctx, tc.Input)
	if err != nil {
		return err.Error(), true
	}
	return out, false
}
