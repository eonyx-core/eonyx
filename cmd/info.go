package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/eonyx-core/eonyx/internal/config"
)

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Print configuration and data directories",
		Long: "info prints the global config, global data, cache, and project data\n" +
			"locations. Config layers merge from top to bottom.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}

			configPath := config.GlobalConfigPath()
			dataPath := config.GlobalDataPath()
			cachePath := config.CacheDir()
			var projectPath string
			if _, root, found := config.FindProjectConfig(wd); found {
				projectPath = config.ProjectDataDir(root)
			}

			out := cmd.OutOrStdout()
			if isTTY(os.Stdout) {
				fmt.Fprintln(out, cliHeader("PATHS"))
				row := func(name, value string) {
					fmt.Fprintf(out, "  %s %s\n", cliDim.Render(fmt.Sprintf("%-8s", name)), cliGreen.Render(value))
				}
				row("config", configPath)
				row("data", dataPath)
				row("cache", cachePath)
				if projectPath != "" {
					row("project", projectPath)
				}
				fmt.Fprintln(out, "\n"+cliDim.Render("Configs merge from top to bottom."))
				return nil
			}

			fmt.Fprintln(out, configPath)
			fmt.Fprintln(out, dataPath)
			fmt.Fprintln(out, cachePath)
			if projectPath != "" {
				fmt.Fprintln(out, projectPath)
			}
			return nil
		},
	}
}
