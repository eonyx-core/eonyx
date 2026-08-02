package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"eonyx/internal/config"
)

func newCleanCmd() *cobra.Command {
	var assumeYes bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete eonyx configuration files (global and local)",
		Long: "clean removes eonyx's own configuration files: the global config, the\n" +
			"global data config, and the project (local) config discovered from the\n" +
			"working directory. Other files are left untouched. Prompts for confirmation\n" +
			"unless --yes is given.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}

			seen := map[string]bool{}
			var targets []string
			add := func(p string) {
				if p == "" || seen[p] {
					return
				}
				seen[p] = true
				if fi, statErr := os.Stat(p); statErr == nil && !fi.IsDir() {
					targets = append(targets, p)
				}
			}
			add(config.GlobalConfigPath())
			add(config.GlobalDataPath())
			if path, _, found := config.FindProjectConfig(wd); found {
				add(path)
			}

			out := cmd.OutOrStdout()
			if len(targets) == 0 {
				fmt.Fprintln(out, cliDim.Render("No eonyx configuration files found."))
				return nil
			}

			fmt.Fprintln(out, cliHeader("CLEAN"))
			fmt.Fprintln(out, cliAmber.Render("The following configuration files will be deleted:"))
			for _, p := range targets {
				fmt.Fprintf(out, "  %s %s\n", cliDim.Render("»"), cliGreen.Render(p))
			}
			if !assumeYes && !confirm(cmd) {
				fmt.Fprintln(out, cliDim.Render("Aborted."))
				return nil
			}

			for _, p := range targets {
				if err := os.Remove(p); err != nil {
					return fmt.Errorf("remove %s: %w", p, err)
				}
				fmt.Fprintf(out, "%s %s %s\n", okMark(), cliWhite.Render("REMOVED"), cliGreen.Render(p))
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// confirm asks for a yes/no answer on the command's input stream.
func confirm(cmd *cobra.Command) bool {
	fmt.Fprint(cmd.OutOrStdout(), cliAmber.Render("⚠ Delete these files? [y/N]: "))
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
