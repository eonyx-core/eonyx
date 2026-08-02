package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"eonyx/internal/config"
	"eonyx/internal/hooks"
	"eonyx/internal/mcp"
	"eonyx/internal/prompt"
	"eonyx/internal/provider"
	"eonyx/internal/report"
	"eonyx/internal/session"
	"eonyx/internal/shell"
	"eonyx/internal/skills"
	"eonyx/internal/subagent"
	"eonyx/internal/tools"
)

// Program is the public entry point for the interactive TUI.
func Program(ctx context.Context, cfg *config.Config, workDir string) error {
	prov, err := provider.For(cfg, "", nil)
	if err != nil {
		return err
	}

	store, err := session.NewJSONLStore(sessionDir(cfg, workDir))
	if err != nil {
		return err
	}

	skillDirs := skills.DefaultDirs(cfg.ProjectRoot())
	skillDirs = append(skillDirs, cfg.Skills...)
	skillSet := skills.Discover(skillDirs)

	mcpConfigs := make([]mcp.ServerConfig, len(cfg.MCP))
	for i, c := range cfg.MCP {
		mcpConfigs[i] = mcp.ServerConfig(c)
	}
	mcpClients, mcpErrs := mcp.DialAll(ctx, mcpConfigs)
	defer mcp.CloseAll(mcpClients)
	for _, e := range mcpErrs {
		fmt.Fprintf(os.Stderr, "warning: mcp: %v\n", e)
	}

	sh, err := shell.New(workDir)
	if err != nil {
		return err
	}
	hk := hooks.New(sh, cfg.Hooks)
	registry := tools.DefaultSet(workDir, cfg.FileSandboxRoot(workDir), sh, nil)
	registry.Register(skills.NewLoadTool(skillSet))
	if cfg.Report.DatabaseURL != "" {
		for _, t := range report.NewTools(cfg.Report.DatabaseURL) {
			registry.Register(t)
		}
	}
	for _, cli := range mcpClients {
		if err := mcp.RegisterFromClient(ctx, registry, cli); err != nil {
			fmt.Fprintf(os.Stderr, "warning: mcp: %v\n", err)
		}
	}

	childTools := tools.ReadOnlySet(workDir, cfg.FileSandboxRoot(workDir), nil)
	registry.Register(subagent.NewTaskTool(prov, childTools, systemPrompt(workDir, cfg.ProjectRoot(), skillSet), contextWindow(cfg, ""), cfg.MaxSteps))

	mcpServers := make([]string, 0, len(mcpClients))
	for _, c := range mcpClients {
		mcpServers = append(mcpServers, c.Name())
	}

	m, err := newModel(ctx, cfg, prov, store, registry, workDir, skillSet, hk, mcpServers)
	if err != nil {
		return err
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.SetProgram(p)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	// On a clean exit, drop the session if the user left it empty (including a
	// fresh chat they never sent a message to) so nothing accumulates.
	if n, err := session.PruneEmpty(store, ""); err != nil {
		slog.Warn("prune empty sessions on exit", "err", err)
	} else if n > 0 {
		slog.Info("pruned empty sessions on exit", "count", n)
	}
	return nil
}

func sessionDir(_ *config.Config, _ string) string {
	return config.GlobalSessionsDir()
}

func contextWindow(cfg *config.Config, modelID string) int {
	if modelID == "" {
		modelID = cfg.DefaultModel
	}
	if m, ok := cfg.Model(modelID); ok {
		return m.ContextWindow
	}
	return 0
}

func systemPrompt(workDir, stopDir string, skillSet []*skills.Skill) string {
	return prompt.System(workDir, stopDir, skillSet)
}

// truncate truncates a string to max displayable runes, appending an ellipsis
// when it exceeds the limit.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

var (
	accentColor = lipgloss.Color("#00FF41")
	dimColor    = lipgloss.Color("#6C6C6C")
)

func statusHint() string {
	return lipgloss.NewStyle().Foreground(dimColor).Render("Enter send • / commands • ? help • ! shell • ↑/↓ history • Esc stop • Ctrl+C quit")
}
