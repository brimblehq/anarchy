package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "domain",
		Aliases: []string{"domains"},
		Short:   "Manage custom domains",
	}
	cmd.AddCommand(newDomainListCmd())
	cmd.AddCommand(newDomainAddCmd())
	cmd.AddCommand(newDomainRemoveCmd())
	cmd.AddCommand(newDomainStatusCmd())
	return cmd
}

type domainRef struct {
	ID          string
	Name        string
	ProjectID   string
	ProjectName string
	Primary     bool
	Active      bool
}

func domainRefs(list client.DomainList) []domainRef {
	var out []domainRef
	for _, d := range list.Domains {
		if d.Name == "" {
			continue
		}
		out = append(out, domainRef{
			ID:        d.ID,
			Name:      d.Name,
			ProjectID: d.Project,
			Primary:   d.Primary,
			Active:    d.Active,
		})
	}
	return out
}

func loadDomains(ctx context.Context) (client.DomainList, []domainRef, error) {
	var list client.DomainList
	err := ui.WithSpinner("Loading domains…", func() error {
		var err error
		list, err = coreClient.ListDomains(ctx, cfg.TeamID)
		return err
	})
	if err != nil {
		return client.DomainList{}, nil, err
	}
	domains := domainRefs(list)
	nameProjects(ctx, domains)
	return list, domains, nil
}

// nameProjects fills in project names; the domain list only carries IDs.
func nameProjects(ctx context.Context, domains []domainRef) {
	needed := false
	for _, d := range domains {
		needed = needed || d.ProjectID != ""
	}
	if !needed {
		return
	}
	projects, err := loadProjects(ctx)
	if err != nil {
		return
	}
	names := map[string]string{}
	for _, p := range projects {
		names[p.ID] = p.Name
	}
	for i := range domains {
		domains[i].ProjectName = names[domains[i].ProjectID]
	}
}

func newDomainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List domains in the workspace",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				data, domains, err := loadDomains(ctx)
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(data.Domains)
				}
				if len(domains) == 0 {
					ui.Muted("No domains yet — add one with `brimble domain add example.com`")
					return nil
				}
				rows := make([][]string, 0, len(domains))
				for _, d := range domains {
					status := ui.Yellow("pending DNS")
					if d.Active {
						status = ui.Green("active")
					}
					primary := " "
					if d.Primary {
						primary = "yes"
					}
					rows = append(rows, []string{d.Name, cmp.Or(d.ProjectName, shortID(d.ProjectID)), status, primary})
				}
				fmt.Fprintln(os.Stdout)
				ui.Table([]string{"DOMAIN", "PROJECT", "STATUS", "PRIMARY"}, rows)
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	}
}

func newDomainAddCmd() *cobra.Command {
	var project string
	var workspaceOnly bool

	cmd := &cobra.Command{
		Use:   "add <domain>",
		Short: "Add a custom domain to a project",
		Long: `Add a custom domain to a project (the linked one by default).

Afterwards, point your DNS at Brimble and check progress with
"brimble domain status <domain>".`,
		Example: `  brimble domain add app.example.com
  brimble domain add api.example.com --project my-api`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				name := normalizeDomain(args[0])
				projectID, target := "", "your workspace"
				if !workspaceOnly {
					ref, err := pickProject(ctx, project)
					if err != nil {
						return err
					}
					projectID, target = ref.ID, ref.Display()
				}
				var data json.RawMessage
				err := ui.WithSpinner("Adding "+name+"…", func() error {
					var err error
					data, err = coreClient.AddDomain(ctx, projectID, name, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(data)
				}
				ui.Success("Added %s to %s", name, target)
				ui.Hint("Point its DNS at Brimble, then run `brimble domain status %s`", name)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", "", "Project to attach the domain to (default: linked project)")
	cmd.Flags().BoolVar(&workspaceOnly, "workspace", false, "Add to the workspace without attaching it to a project")
	return cmd
}

func newDomainRemoveCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:     "remove <domain>",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove a domain",
		Long:    "Remove a domain from its project, or delete it from the workspace when it is not attached to one.",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				name := normalizeDomain(args[0])
				_, domains, err := loadDomains(ctx)
				if err != nil {
					return err
				}
				var target *domainRef
				for i := range domains {
					if domains[i].Name == name {
						target = &domains[i]
						break
					}
				}
				if target == nil {
					return fmt.Errorf("domain %q not found%s — see `brimble domain list`", name, teamSuffix())
				}
				prompt := fmt.Sprintf("Delete %s from your workspace?", name)
				if target.ProjectID != "" {
					prompt = fmt.Sprintf("Remove %s from %s?", name, cmp.Or(target.ProjectName, "its project"))
				}
				ok, err := confirmOrAbort(yes, prompt, true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Removing "+name+"…", func() error {
					return coreClient.DeleteDomain(ctx, target.ID, target.ProjectID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Removed %s", name)
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newDomainStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <domain>",
		Short: "Check a domain's DNS and certificate status",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				name := normalizeDomain(args[0])
				var data json.RawMessage
				err := ui.WithSpinner("Checking "+name+"…", func() error {
					var err error
					data, err = coreClient.DomainStatus(ctx, name, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(data)
				}
				ui.Title(name)
				return printResult(data)
			})
		},
	}
}

func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimSuffix(s, "/")
}
