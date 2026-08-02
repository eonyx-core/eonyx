package diff

import "testing"

func TestUnifiedIdentical(t *testing.T) {
	if got := Unified("f.txt", "a\nb\n", "a\nb\n"); got != "" {
		t.Fatalf("expected empty diff, got %q", got)
	}
}

func TestUnifiedReplaceLine(t *testing.T) {
	old := "alpha\nbeta\ngamma\n"
	new := "alpha\nBETA\ngamma\n"
	want := "--- a/f.txt\n" +
		"+++ b/f.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" alpha\n" +
		"-beta\n" +
		"+BETA\n" +
		" gamma\n"
	if got := Unified("f.txt", old, new); got != want {
		t.Fatalf("diff mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestUnifiedPureInsertion(t *testing.T) {
	old := "a\nb\n"
	new := "a\nx\nb\n"
	want := "--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1,2 +1,3 @@\n" +
		" a\n" +
		"+x\n" +
		" b\n"
	if got := Unified("f", old, new); got != want {
		t.Fatalf("diff mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestUnifiedNewFile(t *testing.T) {
	new := "one\ntwo\n"
	want := "--- a/f\n" +
		"+++ b/f\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+one\n" +
		"+two\n"
	if got := Unified("f", "", new); got != want {
		t.Fatalf("diff mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestUnifiedSeparateHunks(t *testing.T) {
	old := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	new := "X\n2\n3\n4\n5\n6\n7\n8\n9\nY\n"
	got := Unified("f", old, new)
	// Two distant changes must produce two hunks, not one big block.
	if c := countHunks(got); c != 2 {
		t.Fatalf("expected 2 hunks, got %d:\n%s", c, got)
	}
}

func TestStat(t *testing.T) {
	added, removed := Stat("a\nb\nc\n", "a\nB\nc\nd\n")
	if added != 2 || removed != 1 {
		t.Fatalf("Stat = (%d, %d), want (2, 1)", added, removed)
	}
}

func countHunks(diff string) int {
	n := 0
	for _, line := range splitLines(diff) {
		if len(line) >= 2 && line[:2] == "@@" {
			n++
		}
	}
	return n
}
