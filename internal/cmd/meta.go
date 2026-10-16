package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newMetaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "meta",
		Short: "Platform metadata",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "regions",
		Short: "List regions",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var catalog client.RegionCatalog
				err := ui.WithSpinner("Loading regions…", func() error {
					var err error
					catalog, err = coreClient.GetRegions(ctx)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(catalog)
				}
				var rows [][]string
				for _, r := range catalog.Regions() {
					status := ui.Green("available")
					switch {
					case !r.Enabled || !r.Available:
						status = ui.Dim("unavailable")
					case r.Eligible != nil && !*r.Eligible:
						status = ui.Yellow("not in your plan")
					}
					name := r.Name
					if r.Default {
						name += ui.Dim(" (default)")
					}
					rows = append(rows, []string{name, r.Type, r.Country, r.Provider, status, r.ID})
				}
				fmt.Fprintln(os.Stdout)
				ui.Table([]string{"REGION", "FOR", "COUNTRY", "PROVIDER", "STATUS", "ID"}, rows)
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	})

	var features bool
	plans := &cobra.Command{
		Use:   "plans",
		Short: "List plans and prices",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var data json.RawMessage
				err := ui.WithSpinner("Loading plans…", func() error {
					var err error
					data, err = coreClient.GetPlans(ctx)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(data)
				}
				var byKey map[string]planInfo
				if err := json.Unmarshal(data, &byKey); err != nil {
					return printResult(data)
				}
				printPlans(byKey, features)
				return nil
			})
		},
	}
	plans.Flags().BoolVar(&features, "features", false, "List what each plan includes")
	cmd.AddCommand(plans)

	cmd.AddCommand(&cobra.Command{
		Use:   "frameworks",
		Short: "List frameworks",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var frameworks []client.Framework
				err := ui.WithSpinner("Loading frameworks…", func() error {
					var err error
					frameworks, err = coreClient.GetFrameworks(ctx)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(frameworkSlugs(frameworks))
				}
				rows := make([][]string, 0, len(frameworks))
				for _, f := range frameworks {
					if f.Slug != "" {
						rows = append(rows, []string{cmp.Or(f.Name, f.Slug), f.Slug, f.Type})
					}
				}
				fmt.Fprintln(os.Stdout)
				ui.Table([]string{"FRAMEWORK", "SLUG", "TYPE"}, rows)
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	})

	return cmd
}

func frameworkSlugs(frameworks []client.Framework) []string {
	out := make([]string, 0, len(frameworks))
	for _, f := range frameworks {
		if f.Slug != "" {
			out = append(out, f.Slug)
		}
	}
	return out
}

type planInfo struct {
	Title         string   `json:"title"`
	TagLine       string   `json:"tagLine"`
	Amount        float64  `json:"amount"`
	Level         int      `json:"level"`
	IsRecommended bool     `json:"isRecommended"`
	Features      []string `json:"features"`
}

func printPlans(byKey map[string]planInfo, features bool) {
	plans := make([]planInfo, 0, len(byKey))
	for _, p := range byKey {
		if p.Title != "" {
			plans = append(plans, p)
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Level < plans[j].Level })

	if features {
		for _, p := range plans {
			ui.Section(fmt.Sprintf("%s · %s", p.Title, planPrice(p.Amount)))
			for _, f := range p.Features {
				ui.Bullet(f)
			}
		}
		fmt.Fprintln(os.Stdout)
		return
	}

	rows := make([][]string, 0, len(plans))
	for _, p := range plans {
		name := p.Title
		if p.IsRecommended {
			name += ui.Dim(" (recommended)")
		}
		rows = append(rows, []string{name, planPrice(p.Amount), p.TagLine})
	}
	fmt.Fprintln(os.Stdout)
	ui.Table([]string{"PLAN", "PRICE", "FOR"}, rows)
	fmt.Fprintln(os.Stdout)
	ui.Muted("  See what each includes with `brimble meta plans --features`")
}

func planPrice(amount float64) string {
	if amount == 0 {
		return "Free"
	}
	return "$" + strconv.FormatFloat(amount, 'f', -1, 64) + "/mo"
}
