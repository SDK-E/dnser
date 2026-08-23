package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

func newLogsCmd() *cobra.Command {
	var follow bool
	var last int
	cmd := &cobra.Command{
		Use:   "logs [-f]",
		Short: "Show or stream the live DNS query log",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			base := apiBase(store.Settings())
			if !follow {
				return printRecent(out, base, last)
			}
			return streamLogs(out, base)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new queries continuously")
	cmd.Flags().IntVar(&last, "last", 30, "number of recent entries to show")
	return cmd
}

func apiBase(st config.Settings) string {
	return fmt.Sprintf("http://%s:%d/api/v1", st.Bind, st.Ports.UI)
}

func printRecent(out interface{ Write([]byte) (int, error) }, base string, n int) error {
	resp, err := http.Get(base + "/logs?limit=" + fmt.Sprint(n))
	if err != nil {
		return fmt.Errorf("daemon unreachable at %s (is `dnser start` running?)", base)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %d", resp.StatusCode)
	}
	var payload struct {
		Events []logEvent `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode logs: %w", err)
	}
	for _, ev := range payload.Events {
		fmt.Fprintln(out, formatEvent(ev))
	}
	return nil
}

func streamLogs(out interface{ Write([]byte) (int, error) }, base string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/logs/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("daemon unreachable at %s (is `dnser start` running?)", base)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev logEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		fmt.Fprintln(out, formatEvent(ev))
	}
	return ctx.Err()
}

type logEvent struct {
	Time    time.Time `json:"time"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Source  string    `json:"source"`
	Answer  string    `json:"answer"`
	Latency int64     `json:"latency_ns"`
}

func formatEvent(ev logEvent) string {
	ts := ev.Time.Format("15:04:05")
	return fmt.Sprintf("%s  %-22s %-5s %-8s %s (%.1fms)", ts, ev.Name, ev.Type, ev.Source, ev.Answer, float64(ev.Latency)/1e6)
}
