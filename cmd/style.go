package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared CLI output styling: the EONYX terminal-green brand. lipgloss detects
// non-TTY stdout and degrades to plain text, so piped output and tests stay
// uncolored.
var (
	cliGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF41"))
	cliGreenB = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF41")).Bold(true)
	cliDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C6C6C"))
	cliWhite  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E8E8E8")).Bold(true)
	cliRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F44336")).Bold(true)
	cliAmber  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B"))
)

// cliHeader renders the "◆ EONYX // SECTION" banner line.
func cliHeader(section string) string {
	return cliGreenB.Render("◆ EONYX") + cliDim.Render(" // ") + cliWhite.Render(section)
}

func okMark() string   { return cliGreenB.Render("✓") }
func failMark() string { return cliRed.Render("✗") }

// kvRow prints an aligned "  key  value" row: dim key, bright value.
func kvRow(out io.Writer, key, value string) {
	fmt.Fprintf(out, "  %s %s\n", cliDim.Render(fmt.Sprintf("%-13s", key)), cliGreen.Render(value))
}

// renderCLIError formats a returned command error for stderr: ✗ prefix on the
// first line, activation commands highlighted in brand green.
func renderCLIError(err error) string {
	lines := strings.Split(err.Error(), "\n")
	var b strings.Builder
	for i, ln := range lines {
		if i == 0 {
			b.WriteString(failMark() + " " + cliRed.Render(ln))
		} else if strings.Contains(ln, "eonyx license activate") {
			b.WriteString(cliGreenB.Render(ln))
		} else {
			b.WriteString(ln)
		}
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
