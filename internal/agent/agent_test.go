package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"eonyx/internal/llm"
	"eonyx/internal/tools"
)

// scriptedProvider replays a fixed sequence of event batches, one per Stream call.
type scriptedProvider struct {
	turns [][]llm.Event
	call  int
	seen  [][]llm.Message
}

func (p *scriptedProvider) Stream(_ context.Context, msgs []llm.Message, _ []llm.Tool) (<-chan llm.Event, error) {
	snapshot := make([]llm.Message, len(msgs))
	copy(snapshot, msgs)
	p.seen = append(p.seen, snapshot)

	evs := p.turns[p.call]
	p.call++
	ch := make(chan llm.Event)
	go func() {
		for _, e := range evs {
			ch <- e
		}
		close(ch)
	}()
	return ch, nil
}

func TestLoopExecutesToolThenFinishes(t *testing.T) {
	reg := tools.NewRegistry()
	var gotInput string
	reg.Register(tools.Tool{
		Name:   "ls",
		Schema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			gotInput = string(input)
			return "file.txt", nil
		},
	})

	prov := &scriptedProvider{turns: [][]llm.Event{
		{
			llm.EventTextDelta{Text: "listing"},
			llm.EventToolCall{Call: llm.ToolCall{ID: "c1", Name: "ls", Input: json.RawMessage(`{"path":"."}`)}},
			llm.EventUsage{InputTokens: 5, OutputTokens: 3},
			llm.EventDone{StopReason: "tool_use"},
		},
		{
			llm.EventTextDelta{Text: "all done"},
			llm.EventUsage{InputTokens: 7, OutputTokens: 2},
			llm.EventDone{StopReason: "end_turn"},
		},
	}}

	a := New(prov, reg, Options{History: []llm.Message{llm.SystemText("be brief")}})
	out, err := a.Send(context.Background(), "list files")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if out != "all done" {
		t.Errorf("final text = %q, want %q", out, "all done")
	}
	if gotInput != `{"path":"."}` {
		t.Errorf("tool input = %q", gotInput)
	}
	if in, out := a.Usage(); in != 12 || out != 5 {
		t.Errorf("usage = (%d,%d), want (12,5)", in, out)
	}

	// The second request must include the tool result so the model can continue.
	last := prov.seen[len(prov.seen)-1]
	foundResult := false
	for _, m := range last {
		for _, part := range m.Content {
			if tr, ok := part.(llm.ToolResult); ok && tr.ToolCallID == "c1" && tr.Content == "file.txt" {
				foundResult = true
			}
		}
	}
	if !foundResult {
		t.Error("tool result not present in follow-up request history")
	}
}

func TestReadOnlyToolsRunConcurrently(t *testing.T) {
	reg := tools.NewRegistry()
	var active, maxActive atomic.Int32
	slow := func(_ context.Context, input json.RawMessage) (string, error) {
		n := active.Add(1)
		for {
			m := maxActive.Load()
			if n <= m || maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		active.Add(-1)
		return string(input), nil
	}
	reg.Register(tools.Tool{Name: "read", Schema: json.RawMessage(`{"type":"object"}`), Handler: slow})
	reg.Register(tools.Tool{Name: "glob", Schema: json.RawMessage(`{"type":"object"}`), Handler: slow})

	prov := &scriptedProvider{turns: [][]llm.Event{
		{
			llm.EventToolCall{Call: llm.ToolCall{ID: "a", Name: "read", Input: json.RawMessage(`{"path":"1"}`)}},
			llm.EventToolCall{Call: llm.ToolCall{ID: "b", Name: "glob", Input: json.RawMessage(`{"path":"2"}`)}},
			llm.EventToolCall{Call: llm.ToolCall{ID: "c", Name: "read", Input: json.RawMessage(`{"path":"3"}`)}},
			llm.EventDone{StopReason: "tool_use"},
		},
		{
			llm.EventTextDelta{Text: "done"},
			llm.EventDone{StopReason: "end_turn"},
		},
	}}

	a := New(prov, reg, Options{})
	if _, err := a.Send(context.Background(), "go"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := maxActive.Load(); got < 2 {
		t.Errorf("max concurrent read-only tools = %d, want >= 2", got)
	}

	// Results must appear in input order despite concurrent execution.
	var ids []string
	for _, m := range prov.seen[len(prov.seen)-1] {
		for _, part := range m.Content {
			if tr, ok := part.(llm.ToolResult); ok {
				ids = append(ids, tr.ToolCallID)
			}
		}
	}
	if got := strings.Join(ids, ","); got != "a,b,c" {
		t.Errorf("tool result order = %q, want a,b,c", got)
	}
}

func TestCancelledContextReturnsError(t *testing.T) {
	reg := tools.NewRegistry()
	prov := &scriptedProvider{turns: [][]llm.Event{
		{llm.EventDone{StopReason: "end_turn"}},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := New(prov, reg, Options{})
	if _, err := a.Send(ctx, "hi"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
}

func TestAuthorizeDeniesToolWithoutRunning(t *testing.T) {
	reg := tools.NewRegistry()
	executed := false
	reg.Register(tools.Tool{
		Name:   "write",
		Schema: json.RawMessage(`{"type":"object"}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			executed = true
			return "ok", nil
		},
	})

	prov := &scriptedProvider{turns: [][]llm.Event{
		{
			llm.EventToolCall{Call: llm.ToolCall{ID: "c1", Name: "write", Input: json.RawMessage(`{}`)}},
			llm.EventDone{StopReason: "tool_use"},
		},
		{llm.EventTextDelta{Text: "ok then"}, llm.EventDone{StopReason: "end_turn"}},
	}}

	var endResult string
	var endErr bool
	a := New(prov, reg, Options{
		Authorize: func(name string, _ []byte) error {
			return errors.New("permission denied for tool " + name)
		},
		Events: Events{ToolEnd: func(_, result string, isError bool) { endResult, endErr = result, isError }},
	})
	if _, err := a.Send(context.Background(), "write a file"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if executed {
		t.Error("handler ran despite denial")
	}
	if !endErr || !strings.Contains(endResult, "permission denied") {
		t.Errorf("tool end = (%q,%v), want permission-denied error", endResult, endErr)
	}
}

func TestToolErrorIsReportedNotFatal(t *testing.T) {
	reg := tools.NewRegistry()
	prov := &scriptedProvider{turns: [][]llm.Event{
		{
			llm.EventToolCall{Call: llm.ToolCall{ID: "c1", Name: "missing", Input: json.RawMessage(`{}`)}},
			llm.EventDone{StopReason: "tool_use"},
		},
		{
			llm.EventTextDelta{Text: "recovered"},
			llm.EventDone{StopReason: "end_turn"},
		},
	}}

	var endResult string
	var endErr bool
	a := New(prov, reg, Options{Events: Events{
		ToolEnd: func(_, result string, isError bool) { endResult, endErr = result, isError },
	}})
	out, err := a.Send(context.Background(), "go")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if out != "recovered" {
		t.Errorf("final = %q, want recovered", out)
	}
	if !endErr || !strings.Contains(endResult, "unknown tool") {
		t.Errorf("tool end = (%q,%v), want unknown-tool error", endResult, endErr)
	}
}
