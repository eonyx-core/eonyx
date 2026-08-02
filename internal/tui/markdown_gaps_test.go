package tui

import (
	"strings"
	"testing"
)

func TestMarkdownHorizontalRule(t *testing.T) {
	for _, rule := range []string{"---", "***", "___", "- - -"} {
		out := renderMarkdown("above\n"+rule+"\nbelow", 20)
		if !strings.Contains(out, strings.Repeat("─", 20)) {
			t.Errorf("rule %q did not render a full-width line:\n%s", rule, out)
		}
	}
	// A bullet list item must not be mistaken for a rule.
	out := renderMarkdown("- an item", 20)
	if strings.Contains(out, strings.Repeat("─", 20)) {
		t.Errorf("list item rendered as a rule:\n%s", out)
	}
}

func TestMarkdownBlockquote(t *testing.T) {
	out := renderMarkdown("> quoted text", 40)
	if !strings.Contains(out, "quoted text") {
		t.Errorf("blockquote lost its text:\n%s", out)
	}
	if !strings.Contains(out, "▌") {
		t.Errorf("blockquote missing its bar marker:\n%s", out)
	}
}

func TestMarkdownStrikethrough(t *testing.T) {
	out := renderInline("this is ~~gone~~ now")
	if !strings.Contains(out, mdStrikeStyle.Render("gone")) {
		t.Errorf("strikethrough not styled:\n%q", out)
	}
	if strings.Contains(out, "~~") {
		t.Errorf("strikethrough markers should be consumed:\n%q", out)
	}
}

func TestMarkdownLinkOSC8(t *testing.T) {
	out := renderInline("see [docs](https://example.com) here")
	if !strings.Contains(out, "\x1b]8;;https://example.com\x1b\\") {
		t.Errorf("link missing OSC 8 hyperlink escape:\n%q", out)
	}
	if !strings.Contains(out, "docs") {
		t.Errorf("link text lost:\n%q", out)
	}
	if strings.Contains(out, "](") {
		t.Errorf("raw link markup should be consumed:\n%q", out)
	}
}
