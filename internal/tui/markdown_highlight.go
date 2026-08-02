package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

// segment is a run of code with one syntax color.
type segment struct {
	text     string
	color    lipgloss.Color
	hasColor bool
}

// highlightLines tokenizes code for lang with chroma and returns the colored
// segments per source line. It returns nil when the language is unknown or
// tokenizing fails, so the caller falls back to plain rendering.
func highlightLines(code, lang string) [][]segment {
	lexer := lexers.Get(lang)
	if lexer == nil {
		return nil
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return nil
	}

	var lines [][]segment
	var cur []segment
	for _, tok := range iterator.Tokens() {
		color, ok := tokenColor(tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				lines = append(lines, cur)
				cur = nil
			}
			if p != "" {
				cur = append(cur, segment{text: p, color: color, hasColor: ok})
			}
		}
	}
	lines = append(lines, cur)
	return lines
}

// tokenColor maps a chroma token type to a color, or reports false to leave it
// the default code foreground.
func tokenColor(t chroma.TokenType) (lipgloss.Color, bool) {
	switch {
	case t.InCategory(chroma.Comment):
		return codeCommentColor, true
	case t.InCategory(chroma.Keyword):
		return codeKeywordColor, true
	case t.InSubCategory(chroma.LiteralString):
		return codeStringColor, true
	case t.InSubCategory(chroma.LiteralNumber):
		return codeNumberColor, true
	case t.InSubCategory(chroma.NameFunction):
		return codeFuncColor, true
	}
	return "", false
}
