package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

var wellKnownServicePorts = map[string]int{
	"redis":      6379,
	"postgres":   5432,
	"postgresql": 5432,
	"mysql":      3306,
	"mariadb":    3306,
	"mongo":      27017,
	"mongodb":    27017,
	"memcached":  11211,
	"smtp":       1025,
	"mailpit":    1025,
	"minio":      9000,
	"kafka":      9092,
	"rabbitmq":   5672,
	"amqp":       5672,
	"valkey":     6379,
}

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage services of a linked project (any protocol)",
	}
	cmd.AddCommand(newServiceAddCmd())
	cmd.AddCommand(newServiceRemoveCmd())
	cmd.AddCommand(newServiceListCmd())
	return cmd
}

type serviceAddOpts struct {
	svcType   string
	command   string
	host      string
	port      int
	transport string
	dns       bool
	noDNS     bool
}

func newServiceAddCmd() *cobra.Command {
	var o serviceAddOpts
	cmd := &cobra.Command{
		Use:   "add <domain> <name>",
		Short: "Declare a service — managed (command) or external (host)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			domain, err := config.NormalizeDomain(args[0])
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			name := strings.ToLower(args[1])
			if _, err := config.NormalizeLabel(name); err != nil {
				return fmt.Errorf("service name: %w", err)
			}
			svc := config.Service{
				Name:      name,
				Type:      strings.ToLower(o.svcType),
				Command:   strings.TrimSpace(o.command),
				Host:      strings.ToLower(strings.TrimSpace(o.host)),
				Port:      o.port,
				Transport: strings.ToLower(o.transport),
				DNS:       !o.noDNS,
			}
			if containsFlag(cmd, "dns") {
				svc.DNS = o.dns
			}
			switch svc.Transport {
			case "", "tcp", "udp":
			default:
				return fmt.Errorf("--transport must be tcp or udp")
			}
			switch {
			case svc.Command != "" && svc.Host != "":
				return fmt.Errorf("--command and --host are mutually exclusive")
			case svc.Command == "" && svc.Host == "":
				return fmt.Errorf("provide --command (managed) or --host (external)")
			case svc.Host != "" && svc.Port == 0:
				if def, ok := wellKnownServicePorts[svc.Type]; ok {
					svc.Port = def
					fmt.Fprintf(out, "  note: using default %s port %d\n", svc.Type, def)
				} else {
					return fmt.Errorf("external service needs --port")
				}
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != domain {
						continue
					}
					p := &c.Projects[i]
					replaced := false
					for j := range p.Services {
						if p.Services[j].Name == name {
							p.Services[j] = svc
							replaced = true
							break
						}
					}
					if !replaced {
						p.Services = append(p.Services, svc)
					}
					return
				}
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ service %s on %s\n", domain, name)
			printService(out, svc)
			return nil
		},
	}
	cmd.Flags().StringVar(&o.svcType, "type", "", "service label (redis, postgres, smtp, http, ...)")
	cmd.Flags().StringVar(&o.command, "command", "", "supervised command ({port}/{port:<name>} placeholders supported)")
	cmd.Flags().StringVar(&o.host, "host", "", "external endpoint host")
	cmd.Flags().IntVar(&o.port, "port", 0, "port (managed: local bind port, 0 = allocate; external: remote port)")
	cmd.Flags().StringVar(&o.transport, "transport", "tcp", "transport protocol: tcp or udp")
	cmd.Flags().BoolVar(&o.dns, "dns", true, "publish <name>.<domain> in DNS")
	cmd.Flags().BoolVar(&o.noDNS, "no-dns", false, "do not publish a DNS record for this service")
	return cmd
}

func newServiceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <domain> <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a declared service",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			domain, err := config.NormalizeDomain(args[0])
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			name := strings.ToLower(args[1])
			store, err := openStore()
			if err != nil {
				return err
			}
			var removed bool
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != domain {
						continue
					}
					p := &c.Projects[i]
					kept := p.Services[:0]
					for _, s := range p.Services {
						if s.Name == name {
							removed = true
							continue
						}
						kept = append(kept, s)
					}
					p.Services = kept
					return
				}
			})
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("service %q not declared on %s", name, domain)
			}
			fmt.Fprintf(out, "✓ removed service %s from %s\n", name, domain)
			return nil
		},
	}
}

func newServiceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list <domain>",
		Aliases: []string{"ls"},
		Short:   "List declared services",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			domain, err := config.NormalizeDomain(args[0])
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			p, ok := store.FindProject(domain)
			if !ok {
				return fmt.Errorf("project %q is not linked", domain)
			}
			if len(p.Services) == 0 {
				fmt.Fprintf(out, "no services declared on %s — add one with `dnser service add`\n", domain)
				return nil
			}
			tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tMODE\tPORT\tENDPOINT\tDNS")
			for _, svc := range p.Services {
				mode := "external"
				if svc.Managed() {
					mode = "managed"
				}
				dns := "-"
				if svc.DNS {
					dns = svc.Name + "." + domain
				}
				endpoint := svc.Endpoint()
				if svc.Managed() && svc.Port == 0 {
					endpoint = "127.0.0.1:<auto>"
				}
				port := strconv.Itoa(svc.Port)
				if svc.Managed() && svc.Port == 0 {
					port = "<auto>"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", svc.Name, orDash(svc.Type), mode, port, endpoint, dns)
			}
			_ = tw.Flush()
			return nil
		},
	}
}

func printService(out io.Writer, svc config.Service) {
	mode := "external"
	if svc.Managed() {
		mode = "managed"
	}
	if svc.Managed() {
		fmt.Fprintf(out, "  type:   %s (%s)\n", orDash(svc.Type), mode)
		fmt.Fprintf(out, "  runs:   %s\n", svc.Command)
		port := strconv.Itoa(svc.Port)
		if svc.Port == 0 {
			port = "<auto>"
		}
		fmt.Fprintf(out, "  port:   %s\n", port)
	} else {
		fmt.Fprintf(out, "  type:   %s (%s)\n", orDash(svc.Type), mode)
		fmt.Fprintf(out, "  target: %s\n", svc.Endpoint())
	}
	transport := svc.Transport
	if transport == "" {
		transport = "tcp"
	}
	fmt.Fprintf(out, "  transport: %s\n", transport)
	if svc.DNS {
		fmt.Fprintf(out, "  dns:    <name>.<domain> record published when the daemon reloads\n")
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
