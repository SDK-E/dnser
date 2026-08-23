package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/certs"
	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/service"
	"github.com/SDK-E/dnser/internal/setup"
)

func newSetupCmd() *cobra.Command {
	var skipCA bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "One-time OS configuration: system resolver + CA trust",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			cfg := store.Settings()
			dir, err := config.DefaultDir()
			if err != nil {
				return err
			}
			state, err := setup.LoadState(dir)
			if err != nil {
				return err
			}
			r := setup.SystemRunner()

			if !state.DNSApplied {
				fmt.Fprintf(out, "Pointing system DNS at %s...\n", cfg.Bind)
				saved, err := setup.ConfigureDNS(r, cfg.Bind)
				if err != nil {
					return fmt.Errorf("configure DNS resolver: %w", err)
				}
				state.DNSServices = saved
				state.DNSApplied = true
				fmt.Fprintln(out, "  done")
			} else {
				fmt.Fprintln(out, "System resolver already configured")
			}

			if !skipCA && !state.CATrusted {
				ca, err := certs.NewCA(dir + "/certs")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, "Trusting DNSer local CA (admin authorization may be requested)...")
				path, err := setup.TrustCA(r, ca.CertificatePEM(), dir)
				if err != nil {
					fmt.Fprintf(out, "  warning: CA trust failed: %v\n", err)
					fmt.Fprintln(out, "  HTTPS domains will show certificate warnings until trusted.")
					fmt.Fprintln(out, "  Re-run `dnser setup` to try again.")
				} else {
					state.CATrusted = true
					state.CAInstallPath = path
					fmt.Fprintf(out, "  CA installed (%s)\n", path)
				}
			} else if state.CATrusted {
				fmt.Fprintln(out, "Local CA already trusted")
			}

			if !yes && !state.ServiceInstalled {
				answer := prompt(out, "Start DNSer automatically at login? [Y/n] ")
				if strings.EqualFold(answer, "") || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
					mgr := service.NewManager()
					exe, err := os.Executable()
					if err == nil {
						if err := mgr.Install(exe); err != nil {
							fmt.Fprintf(out, "  warning: could not install %s service: %v\n", mgr.Name(), err)
						} else {
							state.ServiceInstalled = true
							fmt.Fprintf(out, "  installed (%s)\n", mgr.Name())
						}
					}
				}
			}

			if err := setup.SaveState(dir, state); err != nil {
				return err
			}
			fmt.Fprintln(out, "\nSetup complete. Link your first project:")
			fmt.Fprintf(out, "  dnser link --domain=myproject --port=3000\n")
			fmt.Fprintf(out, "\nDashboard: https://%s\n", config.DashboardDomain(cfg.TLD))
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipCA, "skip-ca", false, "do not trust the local CA")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "accept defaults non-interactively")
	return cmd
}

func newUnsetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unsetup",
		Short: "Revert exactly what dnser setup changed",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			dir, err := config.DefaultDir()
			if err != nil {
				return err
			}
			state, err := setup.LoadState(dir)
			if err != nil {
				return err
			}
			r := setup.SystemRunner()

			if state.ServiceInstalled {
				mgr := service.NewManager()
				fmt.Fprintf(out, "Removing %s service...\n", mgr.Name())
				if err := mgr.Uninstall(); err != nil {
					fmt.Fprintf(out, "  warning: %v\n", err)
				} else {
					fmt.Fprintln(out, "  done")
				}
			}
			if state.CATrusted {
				fmt.Fprintln(out, "Removing trusted CA...")
				if err := setup.UntrustCA(r, state.CAInstallPath); err != nil {
					fmt.Fprintf(out, "  warning: %v\n", err)
				} else {
					fmt.Fprintln(out, "  done")
				}
			}
			if state.DNSApplied {
				fmt.Fprintln(out, "Restoring previous DNS settings...")
				if err := setup.RestoreDNS(r, state.DNSServices); err != nil {
					return fmt.Errorf("restore DNS: %w", err)
				}
				fmt.Fprintln(out, "  done")
			}
			if err := setup.ClearState(dir); err != nil {
				return err
			}
			fmt.Fprintln(out, "Unsetup complete.")
			return nil
		},
	}
}

func prompt(out interface{ Write([]byte) (int, error) }, q string) string {
	fmt.Fprint(out, q)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
