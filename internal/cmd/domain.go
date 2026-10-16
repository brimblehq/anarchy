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
		Short: "Check a domain's DNS setup",
		Long:  "Check whether a domain's DNS points at Brimble, and show the records to create if it doesn't yet.",
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
				var st domainStatus
				if err := json.Unmarshal(data, &st); err != nil {
					return printResult(data)
				}
				printDomainStatus(name, st)
				return nil
			})
		},
	}
}

// domainStatus is GET /v1/domains/{domain}/status.
type domainStatus struct {
	Active  bool `json:"active"`
	Primary bool `json:"primary"`
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
	Records []struct {
		Type          string `json:"type"`
		Name          string `json:"name"`
		Value         string `json:"value"`
		IsRecommended bool   `json:"isRecommended"`
	} `json:"records"`
	Nameserver struct {
		Expected []string `json:"expected"`
		Actual   []string `json:"actual"`
	} `json:"nameserver"`
	Redirect struct {
		URL string `json:"url"`
	} `json:"redirect"`
}

func printDomainStatus(name string, st domainStatus) {
	ui.Title(name)
	if st.Active {
		printKV("Status", ui.Status("ACTIVE")+ui.Dim("  DNS points at Brimble"))
	} else {
		printKV("Status", ui.Yellow("● Pending DNS"))
	}
	printKV("Project", st.Project.Name)
	if st.Primary {
		printKV("Primary", "yes")
	}
	printKV("Redirects to", st.Redirect.URL)

	if len(st.Records) > 0 {
		title := "DNS record"
		if !st.Active {
			title = "Add this record at your DNS provider"
		}
		ui.Section(title)
		rows := make([][]string, 0, len(st.Records))
		for _, r := range st.Records {
			rows = append(rows, []string{r.Type, r.Name, r.Value})
		}
		ui.Table([]string{"TYPE", "NAME", "VALUE"}, rows)
	}

	// Root domains can instead move to Brimble's nameservers.
	if isRootDomain(name) && len(st.Nameserver.Expected) > 0 {
		ui.Section("Or use Brimble's nameservers")
		printKV("Brimble", strings.Join(st.Nameserver.Expected, ", "))
		current := strings.Join(st.Nameserver.Actual, ", ")
		if current == "" {
			current = ui.Dim("none found")
		}
		printKV("Current", current)
	}

	fmt.Fprintln(os.Stdout)
	if !st.Active {
		ui.Hint("DNS changes can take a few minutes to an hour. Run `brimble domain status %s` again to check.", name)
	}
}

func isRootDomain(name string) bool {
	return strings.Count(name, ".") == 1
}

func normalizeDomain(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimSuffix(s, "/")
}
