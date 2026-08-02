package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"eonyx/internal/llm"
	"eonyx/internal/tools"
)

// fakeProvider replies with a fixed final answer and no tool calls.
type fakeProvider struct{ reply string }

func (f fakeProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.Tool) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 2)
	ch <- llm.EventTextDelta{Text: f.reply}
	ch <- llm.EventDone{StopReason: "end_turn"}
	close(ch)
	return ch, nil
}

func TestTaskToolReturnsChildAnswer(t *testing.T) {
	tool := NewTaskTool(fakeProvider{reply: "the answer is 42"}, tools.NewRegistry(), "system", 0, 5)
	if tool.Name != "task" {
		t.Errorf("name = %q, want task", tool.Name)
	}
	out, err := tool.Handler(context.Background(), json.RawMessage(`{"description":"compute the answer"}`))
	if err != nil {
		t.Fatalf("task handler: %v", err)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("result = %q, want the child's answer", out)
	}
}

func TestTaskToolValidatesInput(t *testing.T) {
	tool := NewTaskTool(fakeProvider{}, tools.NewRegistry(), "system", 0, 5)
	if _, err := tool.Handler(context.Background(), json.RawMessage(`{"description":""}`)); err == nil {
		t.Error("empty description should be rejected")
	}
	if _, err := tool.Handler(context.Background(), json.RawMessage(`{bad`)); err == nil {
		t.Error("invalid JSON should be rejected")
	}
}
