package agent

import (
	"context"
	"testing"

	"eonyx/internal/llm"
	"eonyx/internal/session"
	"eonyx/internal/tools"
)

// TestPersistAndResume verifies the Phase 2 contract: messages committed by the
// agent are durably stored, and a later agent seeded from that store continues
// the same conversation.
func TestPersistAndResume(t *testing.T) {
	store, err := session.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	sess, err := store.Create("task")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sysMsg := llm.SystemText("be brief")
	if err := store.Append(sess.ID, sysMsg); err != nil {
		t.Fatalf("seed system: %v", err)
	}
	persist := func(m llm.Message) error { return store.Append(sess.ID, m) }
	reg := tools.NewRegistry()

	// First turn.
	prov1 := &scriptedProvider{turns: [][]llm.Event{
		{llm.EventTextDelta{Text: "hi"}, llm.EventDone{StopReason: "end_turn"}},
	}}
	a1 := New(prov1, reg, Options{History: []llm.Message{sysMsg}, Persist: persist})
	if out, err := a1.Send(context.Background(), "hello"); err != nil || out != "hi" {
		t.Fatalf("turn1 = %q, %v", out, err)
	}

	msgs, err := store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 3 { // system, user, assistant
		t.Fatalf("after turn1 got %d messages, want 3", len(msgs))
	}

	// Resume in a fresh agent seeded from the store.
	prov2 := &scriptedProvider{turns: [][]llm.Event{
		{llm.EventTextDelta{Text: "again"}, llm.EventDone{StopReason: "end_turn"}},
	}}
	a2 := New(prov2, reg, Options{History: msgs, Persist: persist})
	if out, err := a2.Send(context.Background(), "more"); err != nil || out != "again" {
		t.Fatalf("turn2 = %q, %v", out, err)
	}

	// The resumed agent must have seen the prior conversation as context.
	first := prov2.seen[0]
	if len(first) != 4 { // system, user, assistant, user(more)
		t.Fatalf("resumed request had %d messages, want 4", len(first))
	}

	msgs, err = store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("after turn2 got %d messages, want 5", len(msgs))
	}
}
