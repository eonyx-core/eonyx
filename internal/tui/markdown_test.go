package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdownWrapsToWidth(t *testing.T) {
	const width = 30
	src := strings.Repeat("word ", 40)
	out := renderMarkdown(src, width)
	for _, ln := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(ln); w > width {
			t.Fatalf("line width %d exceeds %d: %q", w, width, ln)
		}
	}
}

func TestRenderInlineStripsMarkers(t *testing.T) {
	cases := map[string]string{
		"**bold**":   "bold",
		"*italic*":   "italic",
		"`code`":     "code",
		"plain text": "plain text",
	}
	for src, want := range cases {
		got := ansi.Strip(renderInline(src))
		if got != want {
			t.Errorf("renderInline(%q) stripped = %q, want %q", src, got, want)
		}
	}
}

func TestRenderMarkdownHeading(t *testing.T) {
	out := ansi.Strip(renderMarkdown("# Hello", 40))
	if strings.Contains(out, "#") {
		t.Errorf("heading marker not stripped: %q", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("heading text missing: %q", out)
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	src := "```go\nfunc main() {}\n```"
	out := renderMarkdown(src, 40)
	if !strings.Contains(out, "┌") || !strings.Contains(out, "└") {
		t.Errorf("code block not framed: %q", out)
	}
	if !strings.Contains(ansi.Strip(out), "func main() {}") {
		t.Errorf("code body missing: %q", ansi.Strip(out))
	}
}

func TestRenderMarkdownUnterminatedFence(t *testing.T) {
	// A code fence that never closes (as happens mid-stream, though we only
	// render finished turns) must not panic and must still frame the body.
	out := renderMarkdown("```\nint x = 1;", 40)
	if !strings.Contains(ansi.Strip(out), "int x = 1;") {
		t.Errorf("unterminated fence body missing: %q", ansi.Strip(out))
	}
}

func TestRenderMarkdownTable(t *testing.T) {
	src := "| Name | Age |\n|------|----:|\n| Alice | 30 |\n| Bob | 5 |"
	out := renderMarkdown(src, 40)
	stripped := ansi.Strip(out)

	if !strings.Contains(stripped, "┌") || !strings.Contains(stripped, "┼") || !strings.Contains(stripped, "└") {
		t.Errorf("table not framed: %q", stripped)
	}
	if strings.Contains(stripped, "---") || strings.Contains(stripped, "|") {
		t.Errorf("raw table markup leaked: %q", stripped)
	}
	for _, want := range []string{"Name", "Age", "Alice", "Bob", "30"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("table cell %q missing: %q", want, stripped)
		}
	}
	for _, ln := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(ln); w > 40 {
			t.Errorf("table line width %d exceeds 40: %q", w, ansi.Strip(ln))
		}
	}
}

func TestRenderMarkdownTableRowsAligned(t *testing.T) {
	src := "| A | B |\n|---|---|\n| longvalue | x |\n| y | z |"
	out := ansi.Strip(renderMarkdown(src, 60))
	lines := strings.Split(out, "\n")
	width := ansi.StringWidth(lines[0])
	for _, ln := range lines {
		if w := ansi.StringWidth(ln); w != width {
			t.Errorf("table rows not aligned: %d vs %d (%q)", w, width, ln)
		}
	}
}

func TestNonTablePipeLineNotFramed(t *testing.T) {
	// A line with a pipe but no delimiter row must stay plain text.
	out := ansi.Strip(renderMarkdown("a | b | c", 40))
	if strings.Contains(out, "┌") {
		t.Errorf("plain pipe line wrongly framed as table: %q", out)
	}
}

func TestRenderListItemHangingIndent(t *testing.T) {
	out := renderListItem("", "-", strings.Repeat("x ", 30), 20)
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped list item, got %d line(s)", len(lines))
	}
	if !strings.HasPrefix(ansi.Strip(lines[1]), "  ") {
		t.Errorf("continuation line not hang-indented: %q", ansi.Strip(lines[1]))
	}
}
