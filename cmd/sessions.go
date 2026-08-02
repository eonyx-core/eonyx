package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"eonyx/internal/config"
	"eonyx/internal/session"
)

func newSessionsCmd() *cobra.Command {
	var search string
	cmd := &cobra.Command{
		Use:          "sessions",
		Short:        "List saved sessions",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if search != "" {
				hits, err := session.Search(store, search)
				if err != nil {
					return err
				}
				if len(hits) == 0 {
					fmt.Fprintln(out, cliDim.Render(fmt.Sprintf("No sessions match %q.", search)))
					return nil
				}
				fmt.Fprintln(out, cliHeader("SESSIONS"))
				for _, h := range hits {
					fmt.Fprintf(out, "%s %s  %s  %s\n     %s\n",
						cliGreenB.Render("❯"), cliGreen.Render(h.Session.ID),
						cliDim.Render(h.Session.UpdatedAt.Local().Format("2006-01-02 15:04")),
						cliWhite.Render(h.Session.Title), cliDim.Render(h.Snippet))
				}
				return nil
			}

			list, err := store.List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Fprintln(out, cliDim.Render("No sessions yet."))
				return nil
			}
			fmt.Fprintln(out, cliHeader("SESSIONS"))
			for _, s := range list {
				fmt.Fprintf(out, "%s %s  %s  %s  %s\n",
					cliGreenB.Render("❯"), cliGreen.Render(s.ID),
					cliDim.Render(s.UpdatedAt.Local().Format("2006-01-02 15:04")),
					cliDim.Render(fmt.Sprintf("%3d msg", s.Messages)),
					cliWhite.Render(s.Title))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "list only sessions whose title or messages contain this text")
	cmd.AddCommand(newSessionsDeleteCmd())
	return cmd
}

func newSessionsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "delete <id>",
		Short:        "Delete a saved session",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", okMark(), cliWhite.Render("DELETED"), cliGreen.Render(args[0]))
			return nil
		},
	}
}

// openStore loads config and opens the session store for the working directory.
func openStore() (session.Store, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(wd)
	if err != nil {
		return nil, err
	}
	return session.NewJSONLStore(sessionDir(cfg, wd))
}
