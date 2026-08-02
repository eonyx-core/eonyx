package session

import (
	"errors"
	"testing"

	"eonyx/internal/llm"
)

func TestFork(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src, _ := store.Create("original")
	if err := store.Append(src.ID, llm.SystemText("sys")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(src.ID, llm.UserText("hello")); err != nil {
		t.Fatal(err)
	}

	fork, err := store.Fork(src.ID, "branch")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if fork.ID == src.ID {
		t.Fatal("fork must get a new id")
	}
	if fork.Title != "branch" {
		t.Errorf("fork title = %q, want branch", fork.Title)
	}

	fm, _ := store.Messages(fork.ID)
	om, _ := store.Messages(src.ID)
	if len(fm) != 2 || len(om) != 2 {
		t.Fatalf("fork copied %d messages, original has %d, want 2 and 2", len(fm), len(om))
	}

	// The fork is independent: appending to it must not touch the original.
	if err := store.Append(fork.ID, llm.UserText("only in fork")); err != nil {
		t.Fatal(err)
	}
	fm2, _ := store.Messages(fork.ID)
	om2, _ := store.Messages(src.ID)
	if len(fm2) != 3 {
		t.Errorf("fork has %d messages after append, want 3", len(fm2))
	}
	if len(om2) != 2 {
		t.Errorf("original changed to %d messages, want it untouched at 2", len(om2))
	}

	if _, err := store.Fork("does-not-exist", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Fork of missing id error = %v, want ErrNotFound", err)
	}
}
