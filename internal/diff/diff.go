// Package diff produces unified, line-based text diffs between two versions of
// a file. It is small and dependency-free, used to show what the edit and write
// tools changed without dumping whole files.
package diff

import (
	"fmt"
	"strings"
)

// context is the number of unchanged lines shown around each change.
const context = 3

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

// op is one line of the edit script, annotated with its 1-based line numbers in
// the old and new files (a side that does not contain the line carries 0).
type op struct {
	kind             opKind
	text             string
	oldLine, newLine int
}

// Unified returns a unified diff of old vs new for a file at path, in the same
// shape as `diff -u`: a `--- a/path` / `+++ b/path` header followed by @@ hunks
// with up to three lines of context. It returns an empty string when the two
// versions are identical.
func Unified(path, old, new string) string {
	if old == new {
		return ""
	}
	ops := diffLines(splitLines(old), splitLines(new))
	ranges := hunkRanges(ops, context)
	if len(ranges) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n", path)
	fmt.Fprintf(&b, "+++ b/%s\n", path)
	for _, r := range ranges {
		seg := ops[r[0]:r[1]]
		oldStart, oldCount, newStart, newCount := hunkBounds(seg)
		fmt.Fprintf(&b, "@@ -%s +%s @@\n", rangeStr(oldStart, oldCount), rangeStr(newStart, newCount))
		for _, o := range seg {
			switch o.kind {
			case opEqual:
				b.WriteString(" " + o.text + "\n")
			case opDelete:
				b.WriteString("-" + o.text + "\n")
			case opInsert:
				b.WriteString("+" + o.text + "\n")
			}
		}
	}
	return b.String()
}

// Stat returns how many lines were added and removed between old and new.
func Stat(old, new string) (added, removed int) {
	for _, o := range diffLines(splitLines(old), splitLines(new)) {
		switch o.kind {
		case opInsert:
			added++
		case opDelete:
			removed++
		}
	}
	return added, removed
}

// diffLines computes a line-level edit script transforming old into new via a
// longest-common-subsequence walk.
func diffLines(old, new []string) []op {
	n, m := len(old), len(new)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if old[i] == new[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case old[i] == new[j]:
			ops = append(ops, op{opEqual, old[i], i + 1, j + 1})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{opDelete, old[i], i + 1, 0})
			i++
		default:
			ops = append(ops, op{opInsert, new[j], 0, j + 1})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{opDelete, old[i], i + 1, 0})
	}
	for ; j < m; j++ {
		ops = append(ops, op{opInsert, new[j], 0, j + 1})
	}
	return ops
}

// hunkRanges groups changed ops into [start,end) ranges over ops, merging
// changes closer than 2*ctx unchanged lines and padding each range with ctx
// lines of surrounding context.
func hunkRanges(ops []op, ctx int) [][2]int {
	var changes []int
	for i, o := range ops {
		if o.kind != opEqual {
			changes = append(changes, i)
		}
	}
	if len(changes) == 0 {
		return nil
	}

	var ranges [][2]int
	start, prev := changes[0], changes[0]
	for _, c := range changes[1:] {
		if c-prev > 2*ctx+1 {
			ranges = append(ranges, [2]int{start, prev + 1})
			start = c
		}
		prev = c
	}
	ranges = append(ranges, [2]int{start, prev + 1})

	for i := range ranges {
		ranges[i][0] = max(0, ranges[i][0]-ctx)
		ranges[i][1] = min(len(ops), ranges[i][1]+ctx)
	}
	return ranges
}

// hunkBounds computes the old/new starting line and line count for one hunk.
func hunkBounds(seg []op) (oldStart, oldCount, newStart, newCount int) {
	for _, o := range seg {
		if o.kind == opEqual || o.kind == opDelete {
			if oldStart == 0 {
				oldStart = o.oldLine
			}
			oldCount++
		}
		if o.kind == opEqual || o.kind == opInsert {
			if newStart == 0 {
				newStart = o.newLine
			}
			newCount++
		}
	}
	return oldStart, oldCount, newStart, newCount
}

// rangeStr renders a unified-diff range as "start" or "start,count".
func rangeStr(start, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

// splitLines splits s into lines, dropping the trailing empty element produced
// by a final newline so files differing only in content diff cleanly.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
