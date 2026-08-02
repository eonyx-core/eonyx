package session

import (
	"strings"
	"testing"

	"eonyx/internal/llm"
)

func TestSearch(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := store.Create("Refactor parser")
	if err := store.Append(s1.ID, llm.UserText("please fix the tokenizer bug")); err != nil {
		t.Fatal(err)
	}
	s2, _ := store.Create("Unrelated chat")
	if err := store.Append(s2.ID, llm.UserText("hello world")); err != nil {
		t.Fatal(err)
	}

	// Match on message content, with a snippet showing the term.
	hits, err := Search(store, "tokenizer")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Session.ID != s1.ID {
		t.Fatalf("content search hits = %+v, want only s1", hits)
	}
	if !strings.Contains(hits[0].Snippet, "tokenizer") {
		t.Errorf("snippet = %q, want it to contain the match", hits[0].Snippet)
	}

	// Match on title (case-insensitive).
	if hits, _ := Search(store, "unrelated"); len(hits) != 1 || hits[0].Session.ID != s2.ID {
		t.Errorf("title search hits = %+v, want only s2", hits)
	}

	// No match.
	if hits, _ := Search(store, "zzz-none"); len(hits) != 0 {
		t.Errorf("expected no hits, got %+v", hits)
	}

	// Empty query matches nothing.
	if hits, _ := Search(store, "   "); hits != nil {
		t.Errorf("empty query should match nothing, got %+v", hits)
	}
}
