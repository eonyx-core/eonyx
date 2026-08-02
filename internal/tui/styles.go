package tui

import "github.com/charmbracelet/lipgloss"

var (
	sageColor  = lipgloss.Color("#81B29A")
	errColor   = lipgloss.Color("#F44336")
	greenColor = lipgloss.Color("#00FF41")

	// Transcript styles: no left gutter — speakers are told apart by a "> "
	// prompt on user turns and rendered markdown on assistant turns, the way
	// Claude Code presents a conversation.
	userStyle       = lipgloss.NewStyle()
	assistantStyle  = lipgloss.NewStyle()
	userPromptStyle = lipgloss.NewStyle().Foreground(dimColor).Bold(true)
	errorStyle      = lipgloss.NewStyle().Foreground(errColor)
	noteStyle       = lipgloss.NewStyle().Foreground(dimColor)
	// reasoningStyle dims a reasoning model's collapsible "thinking" block.
	reasoningStyle = lipgloss.NewStyle().Foreground(dimColor).Italic(true)

	// Badges that mark a transcript line as a system note or error, the way the
	// YOLO badge marks the header.
	noteBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(sageColor).Bold(true)
	errBadgeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(errColor).Bold(true)

	toolStyle = lipgloss.NewStyle().Foreground(sageColor)

	// Markdown styles for finished assistant turns. Inline emphasis, headings,
	// and list bullets are kept close to Claude Code's transcript; code blocks
	// are a filled box drawn by renderCodeBlock.
	mdHeadingStyle    = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	mdBoldStyle       = lipgloss.NewStyle().Bold(true)
	mdItalicStyle     = lipgloss.NewStyle().Italic(true)
	mdInlineCodeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B"))
	mdBulletStyle     = lipgloss.NewStyle().Foreground(sageColor)
	mdStrikeStyle     = lipgloss.NewStyle().Strikethrough(true).Foreground(dimColor)
	mdLinkStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF41")).Underline(true)
	mdHRStyle         = lipgloss.NewStyle().Foreground(dimColor)
	mdQuoteStyle      = lipgloss.NewStyle().Foreground(sageColor)
	codeBorderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5F8787"))
	codeTextStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4D4D4")).Background(lipgloss.Color("#1E1E1E"))

	// Syntax highlighting inside code blocks (chroma tokens → colors), on the
	// code block's dark background. codeLangStyle is the language badge.
	codeBgColor      = lipgloss.Color("#1E1E1E")
	codeFgColor      = lipgloss.Color("#D4D4D4")
	codeBgStyle      = lipgloss.NewStyle().Background(codeBgColor)
	codeLangStyle    = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	codeKeywordColor = lipgloss.Color("#569CD6")
	codeStringColor  = lipgloss.Color("#CE9178")
	codeNumberColor  = lipgloss.Color("#B5CEA8")
	codeCommentColor = lipgloss.Color("#6A9955")
	codeFuncColor    = lipgloss.Color("#DCDCAA")

	// Diff line styles colour the unified diff that write/edit return: additions
	// green, deletions red, hunk headers and context dimmed — like git diff.
	diffAddStyle     = lipgloss.NewStyle().Foreground(greenColor)
	diffDelStyle     = lipgloss.NewStyle().Foreground(errColor)
	diffHunkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5F8787"))
	diffContextStyle = lipgloss.NewStyle().Foreground(dimColor)
	// Word-level emphasis: the changed span within a replaced line gets an
	// inverted highlight, so the eye lands on exactly what differs.
	diffAddWordStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(greenColor).Bold(true)
	diffDelWordStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(errColor).Bold(true)

	headerStyle     = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	headerRuleStyle = lipgloss.NewStyle().Foreground(dimColor)
	dialogStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentColor).Padding(1, 2).Width(60)

	// Header pieces: brand logo and project name.
	logo            = "◆"
	brandStyle      = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	headerLogoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF41")).Bold(true)
	projectStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F4F1DE")).Bold(true)
	yoloStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(errColor).Bold(true)
	safeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#9A9A9A")).Background(lipgloss.Color("#2A2A2A")).Bold(true)
	planStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(lipgloss.Color("#00FF41")).Bold(true)

	// contextFillStyle highlights the estimated context-window usage;
	// contextWarnStyle takes over near the limit, before auto-summarization.
	contextFillStyle = lipgloss.NewStyle().Foreground(dimColor)
	contextWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")).Bold(true)

	// costStyle shows the estimated session cost.
	costStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2CC8F"))

	// workingStyle highlights the live "Working…" status while a turn runs.
	workingStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)

	// fillerStyle draws the "////" hatch filling the header gap, crush-style.
	fillerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A3A"))

	// Scrollbar column drawn to the right of the transcript viewport: a bright
	// thumb over a dim track marks the visible slice of the conversation.
	scrollThumbStyle = lipgloss.NewStyle().Foreground(accentColor)
	scrollTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A3A"))

	// acSelectedStyle highlights the chosen candidate in the autocomplete menu.
	acSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A1A")).Background(accentColor).Bold(true)
)
