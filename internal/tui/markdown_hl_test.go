package tui

import (
	"strings"
	"testing"
)

func TestHighlightColorsKeywords(t *testing.T) {
	segLines := highlightLines("func main() {}", "go")
	if segLines == nil {
		t.Fatal("expected highlighting for a known language")
	}
	var found bool
	for _, line := range segLines {
		for _, s := range line {
			if strings.Contains(s.text, "func") && s.hasColor && s.color == codeKeywordColor {
				found = true
			}
		}
	}
	if !found {
		t.Error("the func keyword should be colored with the keyword color")
	}
}

func TestHighlightUnknownLanguageIsPlain(t *testing.T) {
	if highlightLines("some plain text", "definitely-not-a-language") != nil {
		t.Error("an unknown language should return nil so rendering falls back to plain")
	}
	if highlightLines("x", "") != nil {
		t.Error("an empty language should not highlight")
	}
}

func TestCodeBlockShowsLanguageBadge(t *testing.T) {
	// (Color styling is asserted at the segment level in TestHighlightColorsKeywords;
	// lipgloss emits no ANSI without a terminal, so it is not checked here.)
	out := renderMarkdown("```python\nx = 1\n```", 40)
	if !strings.Contains(out, "python") {
		t.Errorf("code block should badge the language:\n%s", out)
	}
	if !strings.Contains(out, "x = 1") {
		t.Errorf("code content should survive:\n%s", out)
	}
}
