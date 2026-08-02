package session

import (
	"testing"

	"github.com/eonyx-core/eonyx/internal/llm"
)

func TestPruneEmpty(t *testing.T) {
	store, err := NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// empty: never appended to (0 messages).
	empty, _ := store.Create("empty")
	// seeded: only the system prompt (1 message) — still "empty" for pruning.
	seeded, _ := store.Create("seeded")
	if err := store.Append(seeded.ID, llm.SystemText("be brief")); err != nil {
		t.Fatal(err)
	}
	// real: a genuine conversation (system + user).
	real, _ := store.Create("real")
	if err := store.Append(real.ID, llm.SystemText("be brief")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(real.ID, llm.UserText("hi")); err != nil {
		t.Fatal(err)
	}
	// current: empty but must be kept because it is the active session.
	current, _ := store.Create("current")

	pruned, err := PruneEmpty(store, current.ID)
	if err != nil {
		t.Fatalf("PruneEmpty: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2 (empty + seeded)", pruned)
	}

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{}
	for _, s := range list {
		kept[s.ID] = true
	}
	if kept[empty.ID] || kept[seeded.ID] {
		t.Errorf("empty/seeded sessions should have been pruned: %v", kept)
	}
	if !kept[real.ID] || !kept[current.ID] {
		t.Errorf("real and current sessions must be kept: %v", kept)
	}
}
