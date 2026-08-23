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
)

func newStartCmd() *cobra.Command {
	var foreground bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the DNSer daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if foreground {
				return runForeground(cmd)
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
	return cmd
}

func runForeground(cmd *cobra.Command) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	rt, err := daemon.New(daemon.Options{Store: store, Version: version})
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "DNSer %s listening\n", version)
	fmt.Fprintf(out, "  DNS      127.0.0.1:%d\n", rt.DNSPort())
	st := store.Settings()
	fmt.Fprintf(out, "  Proxy    %s:%d / %s:%d\n", st.Bind, st.Ports.HTTP, st.Bind, st.Ports.HTTPS)
	fmt.Fprintf(out, "  Dashboard %s\n", rt.DashboardURL())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Fprintln(out, "\nshutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*1000*1000*1000)
	defer cancel()
	return rt.Shutdown(shutdownCtx)
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the DNSer daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
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
