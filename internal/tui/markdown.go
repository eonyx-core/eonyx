package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// This is a deliberately small markdown renderer tuned to look like Claude
// Code's transcript. It runs once per finished assistant turn — the result is
// cached on the item (see assistantBody) and never recomputed on streaming
// deltas. In-flight text is shown raw and word-wrapped instead, so the
// transcript never reflows mid-stream the way a per-delta markdown render would.

var (
	mdInlineCodeRe = regexp.MustCompile("`([^`]+)`")
	mdBoldRe       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// Italic is single-asterisk only; underscores are left alone so snake_case
	// identifiers are never mangled.
	mdItalicRe  = regexp.MustCompile(`\*([^*]+)\*`)
	mdHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdListRe    = regexp.MustCompile(`^(\s*)([-*+]|\d+\.)\s+(.*)$`)
	// Strikethrough ~~text~~; a markdown link [text](url); a blockquote line.
	// (A horizontal rule is matched by isHorizontalRule, since RE2 has no
	// backreference to require the same rule character repeated.)
	mdStrikeRe = regexp.MustCompile(`~~([^~]+)~~`)
	mdLinkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	mdQuoteRe  = regexp.MustCompile(`^(\s*)>\s?(.*)$`)
	// A GFM table delimiter cell: dashes with optional alignment colons.
	mdTableDelimCellRe = regexp.MustCompile(`^:?-+:?$`)
)

// align is a table column's horizontal alignment, taken from the delimiter row.
type align int

const (
	alignLeft align = iota
	alignRight
	alignCenter
)

// renderMarkdown renders src to ANSI-styled text wrapped to width. It preserves
// the source's line breaks so the rendered block keeps the same vertical rhythm
// as the raw streamed text: the one-time swap from raw to rendered barely moves
// anything on screen.
func renderMarkdown(src string, width int) string {
	if width < 1 {
		width = 1
	}
	lines := strings.Split(src, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if lang, body, next, ok := fenceAt(lines, i); ok {
			out = append(out, renderCodeBlock(body, lang, width))
			i = next
			continue
		}

		if block, next, ok := tableAt(lines, i); ok {
			out = append(out, renderTable(block, width))
			i = next
			continue
		}

		if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
			out = append(out, styleLines(ansi.Wrap(renderInline(m[2]), width, ""), mdHeadingStyle))
			continue
		}

		if isHorizontalRule(line) {
			out = append(out, mdHRStyle.Render(strings.Repeat("─", width)))
			continue
		}

		if m := mdQuoteRe.FindStringSubmatch(line); m != nil {
			out = append(out, renderQuote(m[2], width))
			continue
		}

		if m := mdListRe.FindStringSubmatch(line); m != nil {
			out = append(out, renderListItem(m[1], m[2], m[3], width))
			continue
		}

		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}

		out = append(out, ansi.Wrap(renderInline(line), width, ""))
	}
	return strings.Join(out, "\n")
}

// fenceAt reports whether lines[i] opens a fenced code block (``` or ~~~). On a
// match it returns the info-string language, the code body, and the index of the
// closing fence; an unterminated fence consumes the rest of the input.
func fenceAt(lines []string, i int) (lang string, body []string, next int, ok bool) {
	open := strings.TrimSpace(lines[i])
	if !strings.HasPrefix(open, "```") && !strings.HasPrefix(open, "~~~") {
		return "", nil, i, false
	}
	marker := open[:3]
	if f := strings.Fields(open[3:]); len(f) > 0 {
		lang = f[0]
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == marker {
			return lang, lines[i+1 : j], j, true
		}
	}
	return lang, lines[i+1:], len(lines) - 1, true
}

// renderCodeBlock frames code lines in a filled box that spans the body width,
// syntax-highlighted for lang (falling back to plain text when the language is
// unknown) and badged with the language on the top border. Code is never
// word-wrapped — long lines are truncated with an ellipsis — so a block can
// never reflow.
func renderCodeBlock(lines []string, lang string, width int) string {
	if width < 4 {
		width = 4
	}
	inner := width - 4 // a "│" border and one padding space on each side
	if inner < 1 {
		inner = 1
	}
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, codeTopBorder(lang, width))

	segLines := highlightLines(strings.Join(lines, "\n"), lang)
	for idx, ln := range lines {
		var fill string
		if segLines != nil && idx < len(segLines) {
			fill = highlightedFill(segLines[idx], inner)
		} else {
			fill = plainFill(ln, inner)
		}
		rows = append(rows, codeBorderStyle.Render("│")+fill+codeBorderStyle.Render("│"))
	}
	rows = append(rows, codeBorderStyle.Render("└"+strings.Repeat("─", width-2)+"┘"))
	return strings.Join(rows, "\n")
}

// codeTopBorder draws the code block's top border with a language badge, or a
// plain border when there is no language or no room for the badge.
func codeTopBorder(lang string, width int) string {
	dashes := width - 5 - lipgloss.Width(lang)
	if lang == "" || dashes < 1 {
		return codeBorderStyle.Render("┌" + strings.Repeat("─", width-2) + "┐")
	}
	return codeBorderStyle.Render("┌─ ") + codeLangStyle.Render(lang) + codeBorderStyle.Render(" "+strings.Repeat("─", dashes)+"┐")
}

// plainFill renders one uncolored code line padded to the inner width.
func plainFill(ln string, inner int) string {
	ln = strings.ReplaceAll(ln, "\t", "    ")
	if ansi.StringWidth(ln) > inner {
		ln = ansi.Truncate(ln, inner, "…")
	}
	pad := inner - ansi.StringWidth(ln)
	if pad < 0 {
		pad = 0
	}
	return codeTextStyle.Render(" " + ln + strings.Repeat(" ", pad) + " ")
}

// highlightedFill renders one line of colored segments padded to the inner
// width, keeping the code background across the whole line.
func highlightedFill(segs []segment, inner int) string {
	var b strings.Builder
	b.WriteString(codeBgStyle.Render(" "))
	used := 0
	for _, s := range segs {
		if used >= inner {
			break
		}
		text := strings.ReplaceAll(s.text, "\t", "    ")
		w := ansi.StringWidth(text)
		if used+w > inner {
			text = ansi.Truncate(text, inner-used, "…")
			w = ansi.StringWidth(text)
		}
		st := codeBgStyle.Foreground(codeFgColor)
		if s.hasColor {
			st = codeBgStyle.Foreground(s.color)
		}
		b.WriteString(st.Render(text))
		used += w
	}
	if used < inner {
		b.WriteString(codeBgStyle.Render(strings.Repeat(" ", inner-used)))
	}
	b.WriteString(codeBgStyle.Render(" "))
	return b.String()
}

// tableAt reports whether lines[i] opens a GFM table: a row containing "|"
// immediately followed by a delimiter row (|---|:--:|). On a match it returns the
// block (header, delimiter, body rows) and the index of its last line.
func tableAt(lines []string, i int) (block []string, next int, ok bool) {
	if i+1 >= len(lines) || !strings.Contains(lines[i], "|") {
		return nil, i, false
	}
	if !isTableDelimiter(lines[i+1]) {
		return nil, i, false
	}
	j := i + 2
	for j < len(lines) && strings.Contains(lines[j], "|") && strings.TrimSpace(lines[j]) != "" {
		j++
	}
	return lines[i:j], j - 1, true
}

// isTableDelimiter reports whether a line is a table delimiter row: every
// "|"-separated cell is dashes with optional alignment colons.
func isTableDelimiter(line string) bool {
	cells := splitTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !mdTableDelimCellRe.MatchString(c) {
			return false
		}
	}
	return true
}

// splitTableRow splits a table row into trimmed cells, dropping the optional
// leading and trailing pipes.
func splitTableRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	cells := strings.Split(s, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// renderTable draws a GFM table as an aligned, box-framed grid — column widths
// measured from the visible cell text so wide (multi-byte) content lines up, then
// shrunk to fit width. It reuses the code block's border style.
func renderTable(block []string, width int) string {
	n := len(splitTableRow(block[0]))
	if n == 0 {
		return ""
	}
	aligns := parseTableAligns(block[1], n)

	fit := func(row []string) []string {
		cells := make([]string, n)
		copy(cells, row)
		return cells
	}
	head := fit(splitTableRow(block[0]))
	body := make([][]string, 0, len(block)-2)
	for _, ln := range block[2:] {
		row := fit(splitTableRow(ln))
		for i := range row {
			row[i] = renderInline(row[i])
		}
		body = append(body, row)
	}

	colw := make([]int, n)
	for i, c := range head {
		colw[i] = ansi.StringWidth(c)
	}
	for _, row := range body {
		for i, c := range row {
			if w := ansi.StringWidth(c); w > colw[i] {
				colw[i] = w
			}
		}
	}
	fitTableColumns(colw, width)

	var b strings.Builder
	b.WriteString(tableBorder(colw, "┌", "┬", "┐"))
	b.WriteByte('\n')
	b.WriteString(tableRow(head, colw, aligns, true))
	b.WriteByte('\n')
	b.WriteString(tableBorder(colw, "├", "┼", "┤"))
	for _, row := range body {
		b.WriteByte('\n')
		b.WriteString(tableRow(row, colw, aligns, false))
	}
	b.WriteByte('\n')
	b.WriteString(tableBorder(colw, "└", "┴", "┘"))
	return b.String()
}

// parseTableAligns reads column alignments from the delimiter row, padded to n
// columns (default left).
func parseTableAligns(delim string, n int) []align {
	cells := splitTableRow(delim)
	aligns := make([]align, n)
	for i := 0; i < n; i++ {
		if i >= len(cells) {
			break
		}
		c := cells[i]
		left, right := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":")
		switch {
		case left && right:
			aligns[i] = alignCenter
		case right:
			aligns[i] = alignRight
		}
	}
	return aligns
}

// fitTableColumns shrinks the widest columns in place until the whole table fits
// width, keeping every column at least one cell wide.
func fitTableColumns(colw []int, width int) {
	n := len(colw)
	avail := width - (n + 1) - n*2 // borders + one padding space each side
	if avail < n {
		avail = n
	}
	sum := 0
	for _, w := range colw {
		sum += w
	}
	for sum > avail {
		maxW, idx := 0, -1
		for i, w := range colw {
			if w > maxW {
				maxW, idx = w, i
			}
		}
		if idx < 0 || colw[idx] <= 1 {
			break
		}
		colw[idx]--
		sum--
	}
}

// tableBorder draws a horizontal rule with the given corner/junction runes.
func tableBorder(colw []int, left, mid, right string) string {
	parts := make([]string, len(colw))
	for i, w := range colw {
		parts[i] = strings.Repeat("─", w+2)
	}
	return codeBorderStyle.Render(left + strings.Join(parts, mid) + right)
}

// tableRow renders one row, padding each cell to its column width per alignment;
// header cells are bold.
func tableRow(cells []string, colw []int, aligns []align, header bool) string {
	bar := codeBorderStyle.Render("│")
	var b strings.Builder
	b.WriteString(bar)
	for i, w := range colw {
		c := ""
		if i < len(cells) {
			c = cells[i]
		}
		c = fmtTableCell(c, w, aligns[i])
		if header {
			c = mdBoldStyle.Render(c)
		}
		b.WriteString(" " + c + " ")
		b.WriteString(bar)
	}
	return b.String()
}

// fmtTableCell truncates a cell to w visible columns and pads it per alignment.
func fmtTableCell(s string, w int, a align) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	pad := w - ansi.StringWidth(s)
	if pad < 0 {
		pad = 0
	}
	switch a {
	case alignRight:
		return strings.Repeat(" ", pad) + s
	case alignCenter:
		l := pad / 2
		return strings.Repeat(" ", l) + s + strings.Repeat(" ", pad-l)
	default:
		return s + strings.Repeat(" ", pad)
	}
}

// renderListItem renders one list item with a hanging indent: continuation lines
// align under the text, not under the bullet.
func renderListItem(indent, marker, content string, width int) string {
	bullet := "• "
	if marker != "-" && marker != "*" && marker != "+" {
		bullet = marker + " "
	}
	hang := ansi.StringWidth(bullet)
	wrapW := width - ansi.StringWidth(indent) - hang
	if wrapW < 1 {
		wrapW = 1
	}
	wlines := strings.Split(ansi.Wrap(renderInline(content), wrapW, ""), "\n")
	for i, wl := range wlines {
		if i == 0 {
			wlines[i] = indent + mdBulletStyle.Render(bullet) + wl
		} else {
			wlines[i] = indent + strings.Repeat(" ", hang) + wl
		}
	}
	return strings.Join(wlines, "\n")
}

// isHorizontalRule reports whether a line is a markdown thematic break: three or
// more of the same -, *, or _ character, ignoring surrounding spaces.
func isHorizontalRule(line string) bool {
	s := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

// renderQuote renders a blockquote line as a coloured bar plus the wrapped,
// inline-styled content, so quotes stand apart from body text.
func renderQuote(content string, width int) string {
	prefix := mdQuoteStyle.Render("▌ ")
	wrapW := width - 2
	if wrapW < 1 {
		wrapW = 1
	}
	lines := strings.Split(ansi.Wrap(renderInline(content), wrapW, ""), "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}

// renderInline applies inline markdown — `code`, [links](url), **bold**,
// *italic*, ~~strikethrough~~ — to a line. Code spans are styled first so
// markup inside them is left literal; links next so their URLs are not touched
// by the emphasis passes.
func renderInline(s string) string {
	s = mdInlineCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		return mdInlineCodeStyle.Render(m[1 : len(m)-1])
	})
	s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdLinkRe.FindStringSubmatch(m)
		return osc8(sub[2], mdLinkStyle.Render(sub[1]))
	})
	s = mdBoldRe.ReplaceAllStringFunc(s, func(m string) string {
		return mdBoldStyle.Render(m[2 : len(m)-2])
	})
	s = mdItalicRe.ReplaceAllStringFunc(s, func(m string) string {
		return mdItalicStyle.Render(m[1 : len(m)-1])
	})
	s = mdStrikeRe.ReplaceAllStringFunc(s, func(m string) string {
		return mdStrikeStyle.Render(m[2 : len(m)-2])
	})
	return s
}

// osc8 wraps text in an OSC 8 hyperlink escape so terminals that support it make
// text clickable, linking to url; other terminals show text unchanged.
func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// styleLines applies style to each line of s independently, so a multi-line
// (wrapped) string keeps the style on every visual row.
func styleLines(s string, style lipgloss.Style) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = style.Render(ln)
	}
	return strings.Join(lines, "\n")
}
