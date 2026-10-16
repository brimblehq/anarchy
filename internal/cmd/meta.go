package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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

	cmd.AddCommand(&cobra.Command{
		Use:   "plans",
		Short: "List plans",
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
				return printResult(data)
			})
		},
	})

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
				slugs := frameworkSlugs(frameworks)
				if cfg.Output == "json" {
					return printResult(slugs)
				}
				for _, slug := range slugs {
					fmt.Fprintln(os.Stdout, slug)
				}
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
