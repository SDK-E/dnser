package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/daemon"
	"github.com/SDK-E/dnser/internal/service"
	"github.com/SDK-E/dnser/internal/setup"
)

func mustConfigDir() string {
	dir, err := config.DefaultDir()
	if err != nil {
		return ""
	}
	return dir
}

func newStartCmd() *cobra.Command {
	var (
		foreground bool
		bindPort   int
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the DNSer daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if foreground {
				return runForeground(cmd, bindPort)
			}
			mgr := service.NewManager()
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve binary path: %w", err)
			}
			if err := mgr.Install(exe); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "DNSer started via", mgr.Name())
			return nil
		},
	}
	cmd.Flags().BoolVar(&foreground, "foreground", false, "run in foreground (used by launchd/systemd)")
	cmd.Flags().IntVar(&bindPort, "bind-port", 0, "override preferred DNS port (e.g. 53 with sudo)")
	return cmd
}

func runForeground(cmd *cobra.Command, bindPort int) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	rt, err := daemon.New(daemon.Options{Store: store, Version: version, DNSBindPort: bindPort})
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	st := store.Settings()
	fmt.Fprintf(out, "DNSer %s listening\n", version)
	fmt.Fprintf(out, "  DNS      %s:%d\n", st.Bind, rt.DNSPort())
	fmt.Fprintf(out, "  Proxy    %s / %s\n", rt.Proxy().HTTPAddr(), rt.Proxy().HTTPSAddr())
	fmt.Fprintf(out, "  Dashboard http://%s:%d  |  https://%s\n", st.Bind, st.Ports.UI, config.DashboardDomain(st.TLD))
	if state, _ := setup.LoadState(mustConfigDir()); state.DNSApplied && rt.DNSPort() != 53 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  ⚠ SYSTEM DNS POINTS AT 127.0.0.1:53 BUT DNSER SERVES ON ANOTHER PORT")
		fmt.Fprintln(out, "  ⚠ name resolution will fail. Fix with: sudo dnser start --bind-port 53")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Fprintln(out, "\nshutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*1000*1000*1000)
	defer cancel()
	if err := rt.Shutdown(shutdownCtx); err != nil {
		restoreDNSIfApplied(out)
		return err
	}
	restoreDNSIfApplied(out)
	return nil
}

func newStopCmd() *cobra.Command {
	var keepDNS bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the DNSer daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !keepDNS {
				restoreDNSIfApplied(cmd.OutOrStdout())
			}
			mgr := service.NewManager()
			if err := mgr.Stop(); err != nil {
				return err
			}
			if err := mgr.Uninstall(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "DNSer stopped")
			return nil
		},
	}
	cmd.Flags().BoolVar(&keepDNS, "keep-dns", false, "leave system resolver pointing at 127.0.0.1 (used internally by restart)")
	return cmd
}

func restoreDNSIfApplied(out interface{ Write([]byte) (int, error) }) {
	dir := mustConfigDir()
	state, err := setup.LoadState(dir)
	if err != nil || !state.DNSApplied || len(state.DNSServices) == 0 {
		return
	}
	if err := setup.RestoreDNS(setup.SystemRunner(), state.DNSServices); err != nil {
		fmt.Fprintf(out, "warning: could not restore system DNS: %v\n", err)
		return
	}
	state.DNSApplied = false
	state.DNSServices = nil
	_ = setup.SaveState(dir, state)
	fmt.Fprintln(out, "system DNS restored to your network's defaults")
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the DNSer daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			stopCmd := newStopCmd()
			stopCmd.SetOut(cmd.OutOrStdout())
			if err := stopCmd.RunE(cmd, args); err != nil {
				return err
			}
			startCmd := newStartCmd()
			startCmd.SetOut(cmd.OutOrStdout())
			return startCmd.RunE(cmd, args)
		},
	}
}

var _ = config.DefaultTLD
