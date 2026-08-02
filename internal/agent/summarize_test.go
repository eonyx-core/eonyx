package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"eonyx/internal/llm"
	"eonyx/internal/tools"
)

func TestSummarizeCompactsOldHistory(t *testing.T) {
	seed := []llm.Message{llm.SystemText("base instructions here, keep me")}
	for i := 0; i < 5; i++ {
		seed = append(seed, llm.UserText(fmt.Sprintf("user message number %d with padding text", i)))
		seed = append(seed, llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{
			llm.Text{Text: fmt.Sprintf("assistant reply %d with some padding", i)},
		}})
	}

	// Turn 0 answers the summary request; turn 1 is the real reply.
	prov := &scriptedProvider{turns: [][]llm.Event{
		{llm.EventTextDelta{Text: "SUMMARY: earlier stuff"}, llm.EventDone{StopReason: "end_turn"}},
		{llm.EventTextDelta{Text: "final answer"}, llm.EventDone{StopReason: "end_turn"}},
	}}

	a := New(prov, tools.NewRegistry(), Options{History: seed, ContextTokens: 50})
	out, err := a.Send(context.Background(), "new question")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if out != "final answer" {
		t.Errorf("final = %q, want final answer", out)
	}
	if prov.call != 2 {
		t.Fatalf("provider called %d times, want 2 (summary + reply)", prov.call)
	}

	// The summary request is a single user message.
	if len(prov.seen[0]) != 1 || prov.seen[0][0].Role != llm.RoleUser {
		t.Errorf("summary request = %+v, want single user message", prov.seen[0])
	}

	// The reply request must be compacted: system (with folded summary) + the
	// last 3 user turns and their replies + the new user message = 6 messages.
	reply := prov.seen[1]
	if len(reply) != 6 {
		t.Fatalf("compacted request had %d messages, want 6", len(reply))
	}
	if reply[0].Role != llm.RoleSystem {
		t.Fatalf("compacted request must start with system, got %s", reply[0].Role)
	}
	sysText := renderMessage(reply[0])
	if !strings.Contains(sysText, "base instructions here") {
		t.Errorf("base instructions lost from system: %q", sysText)
	}
	if !strings.Contains(sysText, "SUMMARY: earlier stuff") {
		t.Errorf("summary not folded into system: %q", sysText)
	}
	if reply[1].Role != llm.RoleUser {
		t.Errorf("message after system should be a user turn, got %s", reply[1].Role)
	}
}

func TestNoSummarizeWhenDisabled(t *testing.T) {
	prov := &scriptedProvider{turns: [][]llm.Event{
		{llm.EventTextDelta{Text: "ok"}, llm.EventDone{StopReason: "end_turn"}},
	}}
	a := New(prov, tools.NewRegistry(), Options{History: []llm.Message{llm.SystemText("sys")}})
	if _, err := a.Send(context.Background(), "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if prov.call != 1 {
		t.Errorf("provider called %d times, want 1 (no summarization)", prov.call)
	}
}
