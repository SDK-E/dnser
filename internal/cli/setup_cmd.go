package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/certs"
	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/dnscore"
	"github.com/SDK-E/dnser/internal/service"
	"github.com/SDK-E/dnser/internal/setup"
)

func newSetupCmd() *cobra.Command {
	var skipCA bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "One-time OS configuration: start daemon, verify :53, then switch system resolver",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
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
			dashDomain := config.DashboardDomain(cfg.TLD)

			if _, err := certs.NewCA(dir + "/certs"); err != nil {
				return fmt.Errorf("init CA: %w", err)
			}

			fmt.Fprintln(out, "1. Starting the DNSer daemon...")
			if !state.ServiceInstalled {
				mgr := service.NewManager()
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("resolve binary path: %w", err)
				}
				mode := "user"
				if root, ok := mgr.(service.RootInstaller); ok {
					if err := root.InstallRoot(exe); err == nil {
						mode = "root"
					} else {
						fmt.Fprintf(errOut, "   note: privileged install declined (%v)\n", firstLine(err.Error()))
					}
				}
				if mode != "root" {
					if err := mgr.Install(exe); err != nil {
						fmt.Fprintf(errOut, "   warning: could not install %s service: %v\n", mgr.Name(), err)
						fmt.Fprintln(out, "   attempting to serve in the background anyway...")
					}
				}
				state.ServiceMode = mode
				state.ServiceInstalled = true
				time.Sleep(1200 * time.Millisecond)
			} else {
				fmt.Fprintln(out, "   already installed (mode: "+state.ServiceMode+")")
			}

			fmt.Fprintln(out, "2. Verifying dnser answers on 127.0.0.1:53 ...")
			probeErr := dnscore.ProbeLocal(cfg.Bind, 53, dashDomain)
			if probeErr != nil {
				fmt.Fprintf(errOut, "\n   ✗ NOT SERVING on port 53 (%v)\n", probeErr)
				fmt.Fprintln(errOut, "   macOS requires elevated privileges to bind port 53.")
				fmt.Fprintln(errOut, "   Fix without touching your system DNS:")
				fmt.Fprintln(errOut, "     sudo dnser start --bind-port 53")
				fmt.Fprintln(errOut, "   then re-run: dnser setup")
				fmt.Fprintln(errOut, "\n   System DNS was left untouched — your network is unaffected.")
				_ = setup.SaveState(dir, state)
				return nil
			}
			fmt.Fprintln(out, "   ✓ answering authoritatively")

			if state.DNSApplied {
				fmt.Fprintln(out, "3. System resolver already points at dnser")
			} else {
				fmt.Fprintln(out, "3. Pointing system DNS at "+cfg.Bind+" ...")
				saved, err := setup.ConfigureDNS(r, cfg.Bind)
				if err != nil {
					return fmt.Errorf("configure DNS resolver: %w", err)
				}
				state.DNSServices = saved
				state.DNSApplied = true
				fmt.Fprintln(out, "   done")
			}

			if !skipCA && !state.CATrusted {
				ca, _ := certs.NewCA(dir + "/certs")
				fmt.Fprintln(out, "4. Trusting DNSer local CA (admin authorization may be requested)...")
				path, err := setup.TrustCA(r, ca.CertificatePEM(), dir)
				if err != nil {
					fmt.Fprintf(errOut, "   warning: CA trust failed: %v\n", err)
					fmt.Fprintln(errOut, "   https:// domains will show certificate warnings until trusted; re-run `dnser setup`.")
				} else {
					state.CATrusted = true
					state.CAInstallPath = path
					fmt.Fprintf(out, "   CA installed (%s)\n", path)
				}
			} else if state.CATrusted {
				fmt.Fprintln(out, "4. Local CA already trusted")
			}

			if err := setup.SaveState(dir, state); err != nil {
				return err
			}

			fmt.Fprintln(out, "\nSetup complete.")
			fmt.Fprintf(out, "  Dashboard   http://%s:%d  (also https://%s)\n", cfg.Bind, cfg.Ports.UI, dashDomain)
			fmt.Fprintln(out, "  Next step   dnser link --domain=myproject --port=3000")
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

			mgr0 := service.NewManager()
			rootOK := false
			if ri, ok := mgr0.(service.RootInstaller); ok {
				rootOK = ri.HasRootService()
			}
			if state.ServiceInstalled || rootOK {
				mgr := service.NewManager()
				fmt.Fprintln(out, "Removing dnser service...")
				if root, ri := mgr.(service.RootInstaller); ri && root.HasRootService() {
					if err := root.UninstallRoot(); err != nil {
						fmt.Fprintf(out, "  warning: %v\n", err)
					} else {
						fmt.Fprintln(out, "  removed privileged daemon")
					}
				}
				if state.ServiceMode != "root" {
					if err := mgr.Uninstall(); err != nil {
						fmt.Fprintf(out, "  warning: %v\n", err)
					}
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
