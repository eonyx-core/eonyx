package session

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"eonyx/internal/llm"
)

func TestMessageRoundTrip(t *testing.T) {
	msgs := []llm.Message{
		llm.SystemText("be brief"),
		llm.UserText("list files"),
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{
			llm.Text{Text: "sure"},
			llm.ToolCall{ID: "c1", Name: "ls", Input: json.RawMessage(`{"path":"."}`)},
		}},
		{Role: llm.RoleTool, Content: []llm.ContentPart{
			llm.ToolResult{ToolCallID: "c1", Content: "a.txt\nb.txt", IsError: false},
			llm.ToolResult{ToolCallID: "c2", Content: "boom", IsError: true},
		}},
	}

	for _, m := range msgs {
		line, err := encodeMessage(m)
		if err != nil {
			t.Fatalf("encode %v: %v", m.Role, err)
		}
		got, err := decodeMessage(line)
		if err != nil {
			t.Fatalf("decode %v: %v", m.Role, err)
		}
		if !reflect.DeepEqual(got, m) {
			t.Errorf("round-trip mismatch:\n got %#v\nwant %#v", got, m)
		}
	}
}

func TestStoreRename(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}
	sess, err := store.Create("old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Rename(sess.ID, "new title"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "new title" {
		t.Errorf("title = %q, want %q", got.Title, "new title")
	}
	if err := store.Rename("missing", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Rename(missing) = %v, want ErrNotFound", err)
	}
}

func TestStoreLifecycle(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONLStore: %v", err)
	}

	sess, err := store.Create("first task")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Append(sess.ID, llm.UserText("hello")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(sess.ID, llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text{Text: "hi"}}}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	msgs, err := store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Messages != 2 || list[0].Title != "first task" {
		t.Errorf("list = %+v, want one session with 2 messages", list)
	}

	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Messages(sess.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete, Messages err = %v, want ErrNotFound", err)
	}
}

func TestResumeWithNewStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("task")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sess.ID, llm.UserText("remember this")); err != nil {
		t.Fatal(err)
	}

	// A fresh store over the same dir must see the persisted session.
	reopened, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := reopened.Messages(sess.ID)
	if err != nil {
		t.Fatalf("Messages after reopen: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
}

func TestAppendUnknownSession(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append("ghost", llm.UserText("hi")); !errors.Is(err, ErrNotFound) {
		t.Errorf("Append to unknown session err = %v, want ErrNotFound", err)
	}
}

func TestReset(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("keep me")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sess.ID, llm.UserText("old message")); err != nil {
		t.Fatal(err)
	}

	if err := store.Reset(sess.ID); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	msgs, err := store.Messages(sess.ID)
	if err != nil {
		t.Fatalf("Messages after reset: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages after reset, want 0", len(msgs))
	}

	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("Get after reset: %v", err)
	}
	if got.Title != "keep me" {
		t.Errorf("title = %q, want %q (reset must keep id and title)", got.Title, "keep me")
	}
	if got.Messages != 0 {
		t.Errorf("index message count = %d, want 0", got.Messages)
	}

	// The session must remain appendable after a reset.
	if err := store.Append(sess.ID, llm.UserText("new message")); err != nil {
		t.Errorf("Append after reset: %v", err)
	}
}

func TestResetUnknownSession(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Reset("ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Reset unknown session err = %v, want ErrNotFound", err)
	}
}
