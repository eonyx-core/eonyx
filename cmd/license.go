package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/eonyx-core/eonyx/internal/license"
)

func newLicenseCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "license",
		Short: "Manage the EONYX license on this machine",
	}

	activate := &cobra.Command{
		Use:   "activate <key>",
		Short: "Activate a license key on this machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := license.Activate(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, cliHeader("LICENSE"))
			fmt.Fprintf(out, "  %s %s %s\n", okMark(), cliWhite.Render("ACTIVATED"), cliGreenB.Render(st.LicenseKey))
			kvRow(out, "type", st.LicenseType)
			kvRow(out, "valid until", formatValidUntil(st.ValidUntil))
			kvRow(out, "machine", st.MachineID)
			return nil
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show the activation stored on this machine and re-verify it",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := license.Load()
			if err != nil {
				return err
			}
			if st == nil {
				return errors.New("no license is activated on this machine — run: eonyx license activate <key>")
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, cliHeader("LICENSE_STATUS"))
			kvRow(out, "key", st.LicenseKey)
			kvRow(out, "machine", st.MachineID)
			kvRow(out, "type", st.LicenseType)
			kvRow(out, "valid until", formatValidUntil(st.ValidUntil))
			kvRow(out, "server key", license.Fingerprint())
			ok, _, serverMsg, verr := license.VerifyOnline(cmd.Context(), st)
			switch {
			case verr != nil:
				fmt.Fprintf(out, "  %s %s %s\n", failMark(), cliAmber.Render("ONLINE_CHECK: UNREACHABLE"),
					cliDim.Render(fmt.Sprintf("(%v; last verified %s)", verr, time.Unix(st.LastVerified, 0).Format("2006-01-02 15:04"))))
			case ok:
				fmt.Fprintf(out, "  %s %s\n", okMark(), cliGreenB.Render("ONLINE_CHECK: OK"))
			default:
				fmt.Fprintf(out, "  %s %s %s\n", failMark(), cliRed.Render("ONLINE_CHECK: REJECTED"), cliDim.Render("— "+serverMsg))
			}
			return nil
		},
	}

	deactivate := &cobra.Command{
		Use:   "deactivate",
		Short: "Free this machine's binding so the key can be used elsewhere",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := license.Deactivate(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", okMark(), cliWhite.Render("DEACTIVATED"),
				cliDim.Render("— the key can now be activated on another machine"))
			return nil
		},
	}

	root.AddCommand(activate, status, deactivate)
	return root
}

func formatValidUntil(unix int64) string {
	if unix == 0 {
		return "perpetual"
	}
	return time.Unix(unix, 0).Format("2006-01-02")
}
