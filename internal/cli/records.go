package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SDK-E/dnser/internal/config"
)

func newAddRecordCmd() *cobra.Command {
	var (
		domain   string
		rtype    string
		name     string
		value    string
		ttl      uint32
		priority uint16
		weight   uint16
		port     uint16
	)
	cmd := &cobra.Command{
		Use:   "add-record --domain=myproject.test --type=TXT --name=_verify --value=token123",
		Short: "Add a DNS record to a linked project",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			norm, err := config.NormalizeDomain(domain)
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			rec := config.Record{Type: rtype, Name: name, Value: value, TTL: ttl, Priority: priority, Weight: weight, Port: port}
			if err := config.ValidateRecord(rec); err != nil {
				return err
			}
			var found bool
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != norm {
						continue
					}
					found = true
					c.Projects[i].Records = append(c.Projects[i].Records, rec)
				}
			})
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("project %q not linked; run `dnser link` first", norm)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s %s%s %s\n", rtype, recLabel(name), norm, value)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "linked project domain")
	cmd.Flags().StringVar(&rtype, "type", "", "record type (A AAAA CNAME TXT MX SRV NS)")
	cmd.Flags().StringVar(&name, "name", "@", "record name relative to domain (@, sub, *.sub)")
	cmd.Flags().StringVar(&value, "value", "", "record value")
	cmd.Flags().Uint32Var(&ttl, "ttl", 120, "TTL seconds")
	cmd.Flags().Uint16Var(&priority, "priority", 0, "priority (MX/SRV)")
	cmd.Flags().Uint16Var(&weight, "weight", 0, "weight (SRV)")
	cmd.Flags().Uint16Var(&port, "srv-port", 0, "port (SRV)")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("value")
	return cmd
}

func recLabel(name string) string {
	if name == "@" {
		return ""
	}
	return name + "."
}

func newRemoveRecordCmd() *cobra.Command {
	var domain, name, rtype, value string
	cmd := &cobra.Command{
		Use:   "remove-record --domain=myproject.test --name=x [--type=TXT]",
		Short: "Remove matching records from a linked project",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			norm, err := config.NormalizeDomain(domain)
			if err != nil {
				return fmt.Errorf("domain: %w", err)
			}
			label, err := config.NormalizeLabel(name)
			if err != nil {
				return fmt.Errorf("name: %w", err)
			}
			removed := 0
			err = store.Update(func(c *config.Config) {
				for i := range c.Projects {
					if c.Projects[i].Domain != norm {
						continue
					}
					kept := c.Projects[i].Records[:0]
					for _, r := range c.Projects[i].Records {
						matchName := r.Name == label
						matchType := rtype == "" || r.Type == rtype
						matchValue := value == "" || r.Value == value
						if matchName && matchType && matchValue {
							removed++
							continue
						}
						kept = append(kept, r)
					}
					c.Projects[i].Records = kept
				}
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ removed %d record(s)\n", removed)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "linked project domain")
	cmd.Flags().StringVar(&name, "name", "@", "record name")
	cmd.Flags().StringVar(&rtype, "type", "", "filter by type")
	cmd.Flags().StringVar(&value, "value", "", "filter by value")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func newListRecordsCmd() *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "list-records [domain]",
		Short: "List records for one or all linked projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			filter := domain
			if filter == "" && len(args) > 0 {
				filter = args[0]
			}
			if filter != "" {
				norm, err := config.NormalizeDomain(filter)
				if err != nil {
					return fmt.Errorf("domain: %w", err)
				}
				p, ok := store.FindProject(norm)
				if !ok {
					return fmt.Errorf("project %q not linked", norm)
				}
				printProjectRecords(out, p)
				return nil
			}
			projects := store.Projects()
			if len(projects) == 0 {
				fmt.Fprintln(out, "No linked projects.")
				return nil
			}
			for _, p := range projects {
				printProjectRecords(out, p)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "limit to one project")
	return cmd
}

func printProjectRecords(out interface{ Write([]byte) (int, error) }, p config.Project) {
	targets := []string{}
	for _, r := range p.Routes {
		if r.Host == "@" {
			targets = append(targets, r.Backends...)
		}
	}
	summary := strings.Join(targets, ", ")
	if summary == "" {
		summary = "dns-only"
	}
	fmt.Fprintf(out, "%s (%s)\n", p.Domain, summary)
	if len(p.Records) == 0 {
		fmt.Fprintln(out, "  (implicit A records only)")
		return
	}
	for _, r := range p.Records {
		extra := ""
		switch r.Type {
		case "MX":
			extra = strconv.Itoa(int(r.Priority)) + " "
		case "SRV":
			extra = fmt.Sprintf("%d %d %d ", r.Priority, r.Weight, r.Port)
		}
		fmt.Fprintf(out, "  %-5s %-18s %s%s\n", r.Type, r.Name, extra, r.Value)
	}
}
