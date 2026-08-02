package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eonyx-core/eonyx/internal/agent"
	"github.com/eonyx-core/eonyx/internal/config"
	"github.com/eonyx-core/eonyx/internal/skills"
)

func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the resolved configuration",
		Long: "config loads and validates the merged configuration and prints a\n" +
			"summary. API keys are redacted.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.Load(wd)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			header := cliGreenB
			name := cliWhite
			dim := cliDim
			ok := cliGreen
			bullet := dim.Render("»")

			names := make([]string, 0, len(cfg.Providers))
			for n := range cfg.Providers {
				names = append(names, n)
			}
			sort.Strings(names)

			fmt.Fprintln(out, cliHeader("CONFIG"))
			fmt.Fprintln(out, header.Render("Providers"))
			for _, n := range names {
				p := cfg.Providers[n]
				line := fmt.Sprintf("  %s  %s", name.Render(fmt.Sprintf("%-12s", n)), dim.Render(p.Type))
				if p.BaseURL != "" {
					line += "  " + p.BaseURL
				}
				if p.APIKey != "" {
					line += "  " + ok.Render("[api_key set]")
				}
				fmt.Fprintln(out, line)
			}

			fmt.Fprintln(out, "\n"+header.Render("Models"))
			for _, m := range cfg.Models {
				marker := "  "
				suffix := ""
				if m.ID == cfg.DefaultModel {
					marker = header.Render("●") + " "
					suffix = "  " + dim.Render("(default)")
				}
				fmt.Fprintf(out, "%s%s  %s%s\n", marker, name.Render(fmt.Sprintf("%-20s", m.ID)), dim.Render(m.Provider), suffix)
			}

			maxSteps := cfg.MaxSteps
			if maxSteps == 0 {
				maxSteps = agent.DefaultMaxSteps
			}
			fmt.Fprintf(out, "\n%s  %d %s\n", header.Render("Max steps"), maxSteps, dim.Render("tool round-trips per turn"))

			if len(cfg.Permissions.AllowedTools) > 0 {
				fmt.Fprintln(out, "\n"+header.Render("Allowed tools"))
				for _, t := range cfg.Permissions.AllowedTools {
					fmt.Fprintf(out, "  %s %s\n", bullet, t)
				}
			}

			skillDirs := skills.DefaultDirs(cfg.ProjectRoot())
			skillDirs = append(skillDirs, cfg.Skills...)
			discovered := skills.Discover(skillDirs)
			if len(cfg.Skills) > 0 || len(discovered) > 0 {
				fmt.Fprintln(out, "\n"+header.Render("Skills"))
				for _, d := range cfg.Skills {
					fmt.Fprintf(out, "  %s %s\n", dim.Render("dir:"), d)
				}
				for _, s := range discovered {
					fmt.Fprintf(out, "  %s %s  %s\n", bullet, name.Render(s.Meta.Name), dim.Render(truncate(s.Meta.Description, 60)))
				}
			}

			if len(cfg.MCP) > 0 {
				fmt.Fprintln(out, "\n"+header.Render("MCP servers"))
				for _, s := range cfg.MCP {
					label := name.Render(fmt.Sprintf("%-12s", s.Name))
					if s.URL != "" {
						fmt.Fprintf(out, "  %s  %s  %s\n", label, dim.Render("http "), s.URL)
						continue
					}
					cmdline := s.Command
					if len(s.Args) > 0 {
						cmdline += " " + strings.Join(s.Args, " ")
					}
					fmt.Fprintf(out, "  %s  %s  %s\n", label, dim.Render("stdio"), cmdline)
				}
			}

			if len(cfg.Hooks) > 0 {
				fmt.Fprintln(out, "\n"+header.Render("Hooks"))
				hookNames := make([]string, 0, len(cfg.Hooks))
				for n := range cfg.Hooks {
					hookNames = append(hookNames, n)
				}
				sort.Strings(hookNames)
				for _, n := range hookNames {
					h := cfg.Hooks[n]
					label := name.Render(fmt.Sprintf("%-8s", n))
					if h.Before != "" {
						fmt.Fprintf(out, "  %s %s %s\n", label, dim.Render("before:"), truncate(h.Before, 60))
					}
					if h.After != "" {
						fmt.Fprintf(out, "  %s %s %s\n", label, dim.Render("after: "), truncate(h.After, 60))
					}
				}
			}
			return nil
		},
	}
}
