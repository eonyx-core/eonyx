package tools

import (
	"slices"
	"testing"
)

func TestReadOnlySetExcludesMutatingTools(t *testing.T) {
	names := ReadOnlySet(t.TempDir(), "", nil).Names()

	for _, want := range []string{"read", "ls", "glob", "grep", "fetch"} {
		if !slices.Contains(names, want) {
			t.Errorf("read-only set missing %q", want)
		}
	}
	// No writing, command execution, or further delegation.
	for _, bad := range []string{"write", "edit", "multi_edit", "bash", "git_commit", "task"} {
		if slices.Contains(names, bad) {
			t.Errorf("read-only set must not include %q", bad)
		}
	}
}
