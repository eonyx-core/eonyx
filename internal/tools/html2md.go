package tools

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var wsRun = regexp.MustCompile(`[ \t\r\f\v]*\n[ \t\r\f\v]*`)
var blankRun = regexp.MustCompile(`\n{3,}`)
var spaceRun = regexp.MustCompile(`[ \t]+`)

// htmlToMarkdown renders an HTML document into a readable markdown-ish text.
// It is intentionally lightweight: it captures structure (headings, paragraphs,
// lists, links, emphasis, code) and drops non-content elements, rather than
// being a faithful converter.
func htmlToMarkdown(root *html.Node) string {
	var sb strings.Builder
	renderNode(root, &sb)
	return cleanup(sb.String())
}

func renderNode(n *html.Node, sb *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		sb.WriteString(spaceRun.ReplaceAllString(strings.ReplaceAll(n.Data, "\n", " "), " "))
		return
	case html.ElementNode:
		switch n.Data {
		case "script", "style", "head", "noscript", "svg", "template", "iframe":
			return
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(n.Data[1] - '0')
			sb.WriteString("\n\n")
			sb.WriteString(strings.Repeat("#", level))
			sb.WriteString(" ")
			renderChildren(n, sb)
			sb.WriteString("\n\n")
			return
		case "p", "div", "section", "article", "header", "footer", "main", "ul", "ol", "table", "tr":
			sb.WriteString("\n\n")
			renderChildren(n, sb)
			sb.WriteString("\n\n")
			return
		case "br":
			sb.WriteString("\n")
			return
		case "hr":
			sb.WriteString("\n\n---\n\n")
			return
		case "li":
			sb.WriteString("\n- ")
			renderChildren(n, sb)
			return
		case "a":
			sb.WriteString("[")
			renderChildren(n, sb)
			sb.WriteString("]")
			if href := attr(n, "href"); href != "" {
				sb.WriteString("(")
				sb.WriteString(href)
				sb.WriteString(")")
			}
			return
		case "strong", "b":
			sb.WriteString("**")
			renderChildren(n, sb)
			sb.WriteString("**")
			return
		case "em", "i":
			sb.WriteString("*")
			renderChildren(n, sb)
			sb.WriteString("*")
			return
		case "code":
			sb.WriteString("`")
			renderChildren(n, sb)
			sb.WriteString("`")
			return
		case "pre":
			sb.WriteString("\n\n```\n")
			sb.WriteString(rawText(n))
			sb.WriteString("\n```\n\n")
			return
		}
	}
	renderChildren(n, sb)
}

func renderChildren(n *html.Node, sb *strings.Builder) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(c, sb)
	}
}

func rawText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// cleanup normalizes whitespace: trims each line, collapses runs of blank
// lines, and trims the result.
func cleanup(s string) string {
	s = wsRun.ReplaceAllString(s, "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	s = strings.Join(lines, "\n")
	s = blankRun.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
