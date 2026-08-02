// Package openai implements the llm.Provider interface against any
// OpenAI-compatible Chat Completions endpoint (OpenAI, OpenRouter, DeepSeek,
// Moonshot, and local servers such as Ollama or LM Studio). It speaks the wire
// protocol directly over net/http with server-sent-event streaming, so it
// pulls in no vendor SDK.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eonyx/internal/llm"
)

// maxStreamLine bounds a single SSE data line. Individual completion chunks are
// small, but tool-call argument fragments can occasionally be large.
const maxStreamLine = 1 << 20

// Client is an OpenAI-compatible provider bound to one model.
type Client struct {
	baseURL        string
	apiKey         string
	model          string
	http           *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

// Option customizes a Client at construction.
type Option func(*Client)

// WithRetry overrides the retry policy: maxRetries extra attempts after a
// transient failure, and baseDelay as the first backoff step. A negative
// maxRetries or a non-positive baseDelay leaves the corresponding default in
// place, so callers can tune one without the other.
func WithRetry(maxRetries int, baseDelay time.Duration) Option {
	return func(c *Client) {
		if maxRetries >= 0 {
			c.maxRetries = maxRetries
		}
		if baseDelay > 0 {
			c.retryBaseDelay = baseDelay
		}
	}
}

// New builds a client for the given OpenAI-compatible endpoint and model. A nil
// httpClient uses http.DefaultClient. apiKey may be empty for local servers
// that do not require authentication.
func New(baseURL, apiKey, model string, httpClient *http.Client, opts ...Option) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	c := &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		model:          model,
		http:           httpClient,
		maxRetries:     maxRetries,
		retryBaseDelay: retryBaseDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

var _ llm.Provider = (*Client)(nil)

// Stream sends the conversation and returns a channel of response events. Setup
// failures (request construction, transport, non-2xx status) are returned
// directly; failures mid-stream arrive as a final llm.EventError on the channel.
func (c *Client) Stream(ctx context.Context, msgs []llm.Message, tools []llm.Tool) (<-chan llm.Event, error) {
	body := chatRequest{
		Model:         c.model,
		Messages:      toChatMessages(msgs),
		Tools:         toChatTools(tools),
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	resp, err := c.do(ctx, buf)
	if err != nil {
		return nil, err
	}

	ch := make(chan llm.Event)
	go c.readStream(ctx, resp, ch)
	return ch, nil
}

// maxRetries is the default number of extra attempts a transient failure (a
// transport error or a 429/5xx status) gets before Stream gives up. A Client
// copies it at construction; WithRetry overrides the per-client value.
const maxRetries = 3

// retryBaseDelay is the default first backoff step; each further retry doubles
// it. It is a var so tests can shrink it, and a Client copies it at construction.
var retryBaseDelay = 500 * time.Millisecond

// do sends the request with bounded retries on transient failures, rebuilding
// the body each attempt (the previous one's reader is consumed). A 2xx response
// is returned for streaming; a non-retryable status or exhausted retries returns
// an error.
func (c *Client) do(ctx context.Context, buf []byte) (*http.Response, error) {
	var lastErr error
	var wait time.Duration // Retry-After honored from the previous attempt, if any
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, backoff(attempt-1, wait, c.retryBaseDelay)); err != nil {
				return nil, err
			}
			wait = 0
		}
		req, err := c.newRequest(ctx, buf)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("openai: request failed: %w", err)
			}
			lastErr = fmt.Errorf("openai: request failed: %w", err)
			slog.Warn("openai: transport error, will retry", "attempt", attempt+1, "err", err)
			continue
		}
		if resp.StatusCode/100 == 2 {
			return resp, nil
		}
		msg := readAPIError(resp)
		if !retryable(resp.StatusCode) {
			resp.Body.Close()
			return nil, fmt.Errorf("openai: %s: %s", resp.Status, msg)
		}
		wait = retryAfter(resp.Header)
		resp.Body.Close()
		lastErr = fmt.Errorf("openai: %s: %s", resp.Status, msg)
		if attempt < c.maxRetries {
			slog.Warn("openai: retryable status, will retry", "status", resp.StatusCode, "attempt", attempt+1, "retryAfter", wait)
		}
	}
	return nil, lastErr
}

// newRequest builds the chat-completions POST for the given body.
func (c *Client) newRequest(ctx context.Context, buf []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}

// retryable reports whether an HTTP status is worth retrying: rate limiting and
// transient server errors, but not client errors that would fail identically.
func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// backoff returns the delay before the given zero-based retry attempt: the
// server's Retry-After when present, else exponential from base, capped at 30s.
func backoff(attempt int, retryAfter, base time.Duration) time.Duration {
	const maxDelay = 30 * time.Second
	if retryAfter > 0 {
		return min(retryAfter, maxDelay)
	}
	return min(base<<attempt, maxDelay)
}

// retryAfter parses a Retry-After header (delta-seconds or an HTTP date),
// returning 0 when it is absent or unparseable.
func retryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// readStream parses the SSE body, normalizes events, and sends them on ch until
// a terminal event. It owns resp.Body and ch.
func (c *Client) readStream(ctx context.Context, resp *http.Response, ch chan<- llm.Event) {
	defer resp.Body.Close()
	defer close(ch)

	emit := func(ev llm.Event) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Tool calls stream as fragments keyed by index; accumulate them in arrival
	// order so they can be emitted whole once the stream ends.
	type pendingCall struct {
		id, name string
		args     strings.Builder
	}
	calls := map[int]*pendingCall{}
	var order []int
	var usage *llm.EventUsage
	stopReason := "end_turn"

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLine)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[len("data:"):])
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			emit(llm.EventError{Err: fmt.Errorf("openai: decode chunk: %w", err)})
			return
		}
		if chunk.Usage != nil {
			cached := chunk.Usage.PromptCacheHitTokens
			if d := chunk.Usage.PromptTokensDetails; d != nil && d.CachedTokens > cached {
				cached = d.CachedTokens
			}
			usage = &llm.EventUsage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
				CachedTokens: cached,
			}
		}
		for _, choice := range chunk.Choices {
			if r := choice.Delta.ReasoningContent; r != "" {
				if !emit(llm.EventReasoningDelta{Text: r}) {
					return
				}
			} else if r := choice.Delta.Reasoning; r != "" {
				if !emit(llm.EventReasoningDelta{Text: r}) {
					return
				}
			}
			if choice.Delta.Content != "" {
				if !emit(llm.EventTextDelta{Text: choice.Delta.Content}) {
					return
				}
			}
			for _, frag := range choice.Delta.ToolCalls {
				pc := calls[frag.Index]
				if pc == nil {
					pc = &pendingCall{}
					calls[frag.Index] = pc
					order = append(order, frag.Index)
				}
				if frag.ID != "" {
					pc.id = frag.ID
				}
				if frag.Function.Name != "" {
					pc.name = frag.Function.Name
				}
				pc.args.WriteString(frag.Function.Arguments)
			}
			if choice.FinishReason != nil {
				stopReason = normalizeFinish(*choice.FinishReason)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		emit(llm.EventError{Err: fmt.Errorf("openai: read stream: %w", err)})
		return
	}

	for _, idx := range order {
		pc := calls[idx]
		id := pc.id
		if id == "" {
			id = fmt.Sprintf("call_%d", idx)
		}
		args := pc.args.String()
		if args == "" {
			args = "{}"
		}
		if !emit(llm.EventToolCall{Call: llm.ToolCall{ID: id, Name: pc.name, Input: json.RawMessage(args)}}) {
			return
		}
	}
	if usage != nil {
		if !emit(*usage) {
			return
		}
	}
	emit(llm.EventDone{StopReason: stopReason})
}

// readAPIError extracts a human-readable message from an error response,
// falling back to the raw body.
func readAPIError(resp *http.Response) string {
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	raw := strings.TrimSpace(buf.String())
	var e apiError
	if json.Unmarshal(buf.Bytes(), &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	if raw == "" {
		return "no response body"
	}
	return raw
}
