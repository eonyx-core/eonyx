package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// acKind is what the inline autocomplete is completing.
type acKind int

const (
	acNone acKind = iota
	acSlash
	acFile
)

// acItem is one completion candidate: display is shown in the bar, insert is
// written into the input, and isDir marks a directory so completion can descend.
type acItem struct {
	display string
	insert  string
	isDir   bool
}

// slashCommands are the completable "/" commands, in display order.
var slashCommands = []string{
	"/add", "/clear", "/export", "/fork", "/help", "/ip",
	"/model", "/new", "/rename", "/sessions", "/timestamps", "/yolo", "/quit",
}

// updateAutocomplete recomputes the completion state from the current input: a
// leading "/" with no space completes commands, and a trailing "@token"
// completes file paths. Anything else clears it.
func (m *Model) updateAutocomplete() {
	val := m.textarea.Value()
	switch {
	case strings.HasPrefix(val, "/") && !strings.ContainsAny(val, " \t\n"):
		m.setAutocomplete(acSlash, matchSlashCommands(val))
	case strings.HasPrefix(lastToken(val), "@"):
		m.setAutocomplete(acFile, m.matchFiles(strings.TrimPrefix(lastToken(val), "@")))
	default:
		m.acActive, m.acItems, m.acKind = false, nil, acNone
	}
}

func (m *Model) setAutocomplete(kind acKind, items []acItem) {
	m.acKind = kind
	m.acItems = items
	m.acActive = len(items) > 0
	m.acCursor = 0
}

// lastToken returns the text after the last whitespace, or the whole string.
func lastToken(val string) string {
	if i := strings.LastIndexAny(val, " \t\n"); i >= 0 {
		return val[i+1:]
	}
	return val
}

// matchSlashCommands returns the commands that start with prefix.
func matchSlashCommands(prefix string) []acItem {
	var items []acItem
	for _, c := range slashCommands {
		if strings.HasPrefix(c, prefix) {
			items = append(items, acItem{display: c, insert: c})
		}
	}
	return items
}

// matchFiles returns entries under the directory implied by prefix whose names
// start with the trailing component, directories first.
func (m *Model) matchFiles(prefix string) []acItem {
	dirPart, base := "", prefix
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		dirPart, base = prefix[:i+1], prefix[i+1:]
	}
	dir := m.workDir
	if dirPart != "" {
		dir = m.resolvePath(dirPart)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs, files []acItem
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, acItem{display: name + "/", insert: dirPart + name + "/", isDir: true})
		} else {
			files = append(files, acItem{display: name, insert: dirPart + name})
		}
	}
	items := append(dirs, files...)
	const maxItems = 25
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	return items
}

// handleAutocompleteKey processes keys while the completion menu is open:
// arrows move the selection (wrapping at either end), Tab accepts, Esc
// dismisses. Other keys fall through so typing continues and the candidates
// refresh.
func (m *Model) handleAutocompleteKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.acActive = false
		return true, nil
	case "up":
		m.acCursor = (m.acCursor - 1 + len(m.acItems)) % len(m.acItems)
		return true, nil
	case "down":
		m.acCursor = (m.acCursor + 1) % len(m.acItems)
		return true, nil
	case "tab":
		m.acceptAutocomplete()
		return true, nil
	}
	return false, nil
}

// acceptAutocomplete writes the highlighted candidate into the input, then
// refreshes (so completing a directory shows its contents).
func (m *Model) acceptAutocomplete() {
	if m.acCursor < 0 || m.acCursor >= len(m.acItems) {
		return
	}
	it := m.acItems[m.acCursor]
	val := m.textarea.Value()
	switch m.acKind {
	case acSlash:
		val = it.insert + " "
	case acFile:
		if at := strings.LastIndex(val, "@"); at >= 0 {
			val = val[:at+1] + it.insert
			if !it.isDir {
				val += " "
			}
		}
	}
	m.textarea.SetValue(val)
	m.textarea.CursorEnd()
	m.updateAutocomplete()
}

// acMaxShown is the most completion rows shown at once; the menu windows
// around the selection when there are more candidates.
const acMaxShown = 8

// acExtraRows is how many rows beyond the footer's single status line the open
// completion menu needs, so the viewport can shrink to make room for it.
func (m *Model) acExtraRows() int {
	if !m.acActive {
		return 0
	}
	return min(len(m.acItems), acMaxShown)
}

// renderAutocompleteMenu draws the completion candidates as a vertical menu in
// place of the footer hint: one row per candidate, windowed around the
// selection, with a key-hint line below.
func (m *Model) renderAutocompleteMenu() string {
	start := 0
	if m.acCursor >= acMaxShown {
		start = m.acCursor - acMaxShown + 1
	}
	end := min(len(m.acItems), start+acMaxShown)

	dim := lipgloss.NewStyle().Foreground(dimColor)
	w := max(1, m.width)
	var rows []string
	for i := start; i < end; i++ {
		var row string
		if i == m.acCursor {
			row = "› " + acSelectedStyle.Render(" "+m.acItems[i].display+" ")
		} else {
			row = "  " + dim.Render(m.acItems[i].display)
		}
		rows = append(rows, ansi.Truncate(row, w, "…"))
	}
	hint := "↑/↓ select • Tab accept • Esc dismiss"
	if len(m.acItems) > acMaxShown {
		hint = fmt.Sprintf("%d/%d • %s", m.acCursor+1, len(m.acItems), hint)
	}
	rows = append(rows, ansi.Truncate(dim.Render(hint), w, "…"))
	return strings.Join(rows, "\n")
}
