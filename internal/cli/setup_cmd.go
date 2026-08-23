package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
			if state.ServiceMode == "root" {
				mgr := service.NewManager()
				exe0, _ := os.Executable()
				if root, ok := mgr.(service.RootInstaller); ok {
					fmt.Fprintln(out, "   refreshing privileged daemon...")
					if err := root.InstallRoot(exe0); err != nil {
						fmt.Fprintf(errOut, "   warning: refresh failed (%v); keeping previous\n", firstLine(err.Error()))
					}
					time.Sleep(1200 * time.Millisecond)
					state.ServiceInstalled = true
				}
			}
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

			if state.ServiceMode == "root" {
				bounceIfStale(out, errOut, cfg, dir)
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
				lan := setup.CaptureDNS(r)
				if len(lan) > 0 {
					seen := map[string]bool{}
					merged := make([]string, 0, len(lan)+len(cfg.Upstreams))
					for _, u := range append(append([]string{}, lan...), cfg.Upstreams...) {
						if !seen[u] {
							seen[u] = true
							merged = append(merged, u)
						}
					}
					if len(merged) > 6 {
						merged = merged[:6]
					}
					_ = store.Update(func(c *config.Config) { c.Settings.Upstreams = merged })
					cfg = store.Settings()
					fmt.Fprintf(out, "   forwarding to your network's resolvers first: %s\n", strings.Join(lan, ", "))
				}
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
				fmt.Fprintln(out, "4. Trusting DNSer local CA...")
				path, mode, terr := setup.TrustCA(r, ca.CertificatePEM(), dir)
				if terr != nil {
					fmt.Fprintf(errOut, "   warning: CA trust failed: %v\n", terr)
					fmt.Fprintln(errOut, "   https:// domains will show certificate warnings until trusted; re-run `dnser setup`.")
				} else {
					state.CATrusted = true
					state.CAInstallPath = path
					state.CATrustMode = mode
					where := "your user keychain (silent)"
					if mode == setup.TrustModeAdmin {
						where = "System keychain"
					}
					fmt.Fprintf(out, "   trusted via %s\n", where)
					fmt.Fprintln(out, "   restart your browser to pick up the new trust settings")
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
				if err := setup.UntrustCA(r, state.CAInstallPath, state.CATrustMode); err != nil {
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
			if len(state.ResolverDomains) > 0 || state.CapGranted {
				fmt.Fprintln(out, "Removing desktop routing...")
				if err := setup.RevertDesktopState(r, state); err != nil {
					fmt.Fprintf(out, "  warning: %v\n", err)
				} else {
					fmt.Fprintln(out, "  done")
				}
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

func bounceIfStale(out, errOut interface{ Write([]byte) (int, error) }, st config.Settings, dir string) {
	running := daemonAPIVersion(st)
	if running == "" || running == version {
		return
	}
	fmt.Fprintf(out, "   upgrading running daemon %s -> %s (authorization may be requested)...\n", running, version)
	script := fmt.Sprintf("launchctl kickstart -k system/%s", label)
	o, err := exec.Command("osascript", "-e",
		fmt.Sprintf("do shell script %q with administrator privileges", script)).CombinedOutput()
	if err != nil {
		fmt.Fprintf(errOut, "   warning: could not restart privileged daemon: %v\n%s\n", err, o)
		return
	}
	time.Sleep(1500 * time.Millisecond)
	if state, serr := setup.LoadState(dir); serr == nil && state.DNSApplied && len(state.DNSServices) > 0 {
		if rerr := setup.ReassertDNS(setup.SystemRunner(), st.Bind, state.DNSServices); rerr != nil {
			fmt.Fprintf(errOut, "   warning: could not re-assert system DNS after upgrade: %v\n", rerr)
		}
	}
}

func daemonAPIVersion(st config.Settings) string {
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://%s:%d/api/v1/status", st.Bind, st.Ports.UI))
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	return payload.Version
}

const label = "enterprises.sdk.dnser"

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
