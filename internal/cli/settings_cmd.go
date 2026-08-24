package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

func newSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Show or change daemon settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			store, err := openStore()
			if err != nil {
				return err
			}
			s := store.Settings()
			fmt.Fprintf(out, "tld                   %s\n", s.TLD)
			fmt.Fprintf(out, "bind                  %s\n", s.Bind)
			fmt.Fprintf(out, "upstreams             %s\n", strings.Join(s.Upstreams, ", "))
			fmt.Fprintf(out, "autostart             %t\n", s.Autostart)
			fmt.Fprintf(out, "force_https           %t\n", s.ForceHTTPS)
			fmt.Fprintf(out, "path_refresh_minutes  %d\n", s.PathRefresh())
			fmt.Fprintf(out, "ports.dns             %d\n", s.Ports.DNS)
			fmt.Fprintf(out, "ports.http            %d\n", s.Ports.HTTP)
			fmt.Fprintf(out, "ports.https           %d\n", s.Ports.HTTPS)
			fmt.Fprintf(out, "ports.ui              %d\n", s.Ports.UI)
			return nil
		},
	}
	cmd.AddCommand(newSettingsSetCmd())
	return cmd
}

func newSettingsSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Change one setting (hot-reloads the daemon)",
		Long: `Supported keys:
  tld                     development TLD (test)
  bind                    bind IP address
  upstreams               comma-separated resolver list
  autostart               true|false
  force_https             true|false — redirect HTTP to HTTPS on every HTTPS route
  path_refresh_minutes    TTL for the daemon's cached login-shell PATH (0 = default 1440)
  ports.dns|http|https|ui listener ports`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			key := strings.ToLower(args[0])
			value := args[1]
			store, err := openStore()
			if err != nil {
				return err
			}
			setBool := func(dst **bool) error {
				v, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %q is not a boolean", key, value)
				}
				*dst = &v
				return nil
			}
			var (
				newTLD       *string
				newBind      *string
				newUpstreams *[]string
				newAutostart *bool
				newForce     *bool
				newRefresh   *int
				newPorts     map[string]*int
			)
			switch key {
			case "tld":
				if _, err := config.NormalizeDomain(value); err != nil {
					return fmt.Errorf("tld: %w", err)
				}
				newTLD = &value
			case "bind":
				newBind = &value
			case "upstreams":
				parts := strings.Split(value, ",")
				clean := make([]string, 0, len(parts))
				for _, p := range parts {
					if p = strings.TrimSpace(p); p != "" {
						clean = append(clean, p)
					}
				}
				if len(clean) == 0 {
					return fmt.Errorf("upstreams: empty list")
				}
				newUpstreams = &clean
			case "autostart":
				if err := setBool(&newAutostart); err != nil {
					return err
				}
			case "force_https":
				if err := setBool(&newForce); err != nil {
					return err
				}
			case "path_refresh_minutes":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 || n > 525600 {
					return fmt.Errorf("path_refresh_minutes: %q must be an integer 0-525600", value)
				}
				newRefresh = &n
			case "ports.dns", "ports.http", "ports.https", "ports.ui":
				n, err := strconv.Atoi(value)
				if err != nil || n < 1 || n > 65535 {
					return fmt.Errorf("%s: %q must be a port 1-65535", key, value)
				}
				newPorts = map[string]*int{strings.TrimPrefix(key, "ports."): &n}
			default:
				return fmt.Errorf("unknown setting %q — run 'dnser settings set' for keys", key)
			}

			err = store.Update(func(c *config.Config) {
				if newTLD != nil {
					c.Settings.TLD = *newTLD
				}
				if newBind != nil {
					c.Settings.Bind = *newBind
				}
				if newUpstreams != nil {
					c.Settings.Upstreams = *newUpstreams
				}
				if newAutostart != nil {
					c.Settings.Autostart = *newAutostart
				}
				if newForce != nil {
					c.Settings.ForceHTTPS = *newForce
				}
				if newRefresh != nil {
					c.Settings.PathRefreshMins = *newRefresh
				}
				switch {
				case newPorts["dns"] != nil:
					c.Settings.Ports.DNS = *newPorts["dns"]
				case newPorts["http"] != nil:
					c.Settings.Ports.HTTP = *newPorts["http"]
				case newPorts["https"] != nil:
					c.Settings.Ports.HTTPS = *newPorts["https"]
				case newPorts["ui"] != nil:
					c.Settings.Ports.UI = *newPorts["ui"]
				}
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ %s = %s\n", key, value)
			fmt.Fprintln(out, "  a running daemon hot-reloads this change")
			return nil
		},
	}
	return cmd
}
