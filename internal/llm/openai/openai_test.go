package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"eonyx/internal/llm"
)

func collect(t *testing.T, ch <-chan llm.Event) []llm.Event {
	t.Helper()
	var evs []llm.Event
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func TestStreamTextToolCallAndUsage(t *testing.T) {
	const body = `data: {"choices":[{"delta":{"content":"Hello"}}]}

data: {"choices":[{"delta":{"content":" world"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"ls","arguments":"{\"path\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\".\"}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", "gpt-test", srv.Client())
	ch, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	evs := collect(t, ch)

	var text string
	var toolCalls []llm.ToolCall
	var usage llm.EventUsage
	var done *llm.EventDone
	for _, ev := range evs {
		switch e := ev.(type) {
		case llm.EventTextDelta:
			text += e.Text
		case llm.EventToolCall:
			toolCalls = append(toolCalls, e.Call)
		case llm.EventUsage:
			usage = e
		case llm.EventDone:
			d := e
			done = &d
		case llm.EventError:
			t.Fatalf("unexpected error event: %v", e.Err)
		}
	}

	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(toolCalls))
	}
	if tc := toolCalls[0]; tc.ID != "call_1" || tc.Name != "ls" || string(tc.Input) != `{"path":"."}` {
		t.Errorf("tool call = %+v, want id=call_1 name=ls input={\"path\":\".\"}", tc)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {10 5}", usage)
	}
	if done == nil || done.StopReason != "tool_use" {
		t.Errorf("done = %+v, want stop reason tool_use", done)
	}
}

func TestStreamParsesCachedTokens(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "openai",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":5,\"prompt_tokens_details\":{\"cached_tokens\":64}}}\n\n" +
				"data: [DONE]\n\n",
			want: 64,
		},
		{
			name: "deepseek",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":5,\"prompt_cache_hit_tokens\":80}}\n\n" +
				"data: [DONE]\n\n",
			want: 80,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := New(srv.URL, "", "m", srv.Client())
			ch, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil)
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			var usage llm.EventUsage
			for _, ev := range collect(t, ch) {
				if u, ok := ev.(llm.EventUsage); ok {
					usage = u
				}
			}
			if usage.CachedTokens != tc.want {
				t.Errorf("CachedTokens = %d, want %d", usage.CachedTokens, tc.want)
			}
			if usage.InputTokens != 100 {
				t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
			}
		})
	}
}

func TestStreamParsesReasoning(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"let me \"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"the answer\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(srv.URL, "", "m", srv.Client())
	ch, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var reasoning, text string
	for _, ev := range collect(t, ch) {
		switch e := ev.(type) {
		case llm.EventReasoningDelta:
			reasoning += e.Text
		case llm.EventTextDelta:
			text += e.Text
		}
	}
	if reasoning != "let me think" {
		t.Errorf("reasoning = %q, want %q", reasoning, "let me think")
	}
	if text != "the answer" {
		t.Errorf("text = %q, want %q", text, "the answer")
	}
}

func TestStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"auth"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "", "m", srv.Client())
	_, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStreamRetriesOnTransientStatus(t *testing.T) {
	old := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = old }()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, "", "m", srv.Client())
	ch, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil)
	if err != nil {
		t.Fatalf("Stream after retries: %v", err)
	}
	var text string
	for _, ev := range collect(t, ch) {
		if e, ok := ev.(llm.EventTextDelta); ok {
			text += e.Text
		}
	}
	if text != "ok" {
		t.Errorf("text = %q, want ok", text)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("server calls = %d, want 3 (two 503s then 200)", n)
	}
}

func TestStreamGivesUpAfterMaxRetries(t *testing.T) {
	old := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = old }()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "", "m", srv.Client())
	if _, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if n := calls.Load(); n != int32(maxRetries+1) {
		t.Errorf("server calls = %d, want %d", n, maxRetries+1)
	}
}

func TestStreamDoesNotRetryClientError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "", "m", srv.Client())
	if _, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil); err == nil {
		t.Fatal("expected error on 400")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("server calls = %d, want 1 (no retry on 4xx)", n)
	}
}

func TestWithRetryOverridesAttempts(t *testing.T) {
	old := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = old }()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// One extra retry means two total attempts before giving up.
	c := New(srv.URL, "", "m", srv.Client(), WithRetry(1, 0))
	if _, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil); err == nil {
		t.Fatal("expected error after exhausting the configured retries")
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("server calls = %d, want 2 (WithRetry(1) → 1 retry)", n)
	}
}

func TestWithRetryZeroDisablesRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(srv.URL, "", "m", srv.Client(), WithRetry(0, 0))
	if _, err := c.Stream(context.Background(), []llm.Message{llm.UserText("hi")}, nil); err == nil {
		t.Fatal("expected error with retries disabled")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("server calls = %d, want 1 (retries disabled)", n)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	if got := retryAfter(http.Header{"Retry-After": {"2"}}); got != 2*time.Second {
		t.Errorf("retryAfter(2) = %v, want 2s", got)
	}
	if got := retryAfter(http.Header{}); got != 0 {
		t.Errorf("retryAfter(absent) = %v, want 0", got)
	}
	if got := retryAfter(http.Header{"Retry-After": {"-5"}}); got != 0 {
		t.Errorf("retryAfter(-5) = %v, want 0", got)
	}
}
