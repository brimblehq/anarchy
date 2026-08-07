package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/brimbleapp/brimble-cli/internal/ui"
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
				var data any
				err := ui.WithSpinner("Loading regions…", func() error {
					var err error
					data, err = coreClient.GetRegions(ctx)
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
		Use:   "plans",
		Short: "List plans",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var data any
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
				var data any
				err := ui.WithSpinner("Loading frameworks…", func() error {
					var err error
					data, err = coreClient.GetFrameworks(ctx)
					return err
				})
				if err != nil {
					return err
				}
				if cfg.Output == "json" {
					return printResult(slugsOnly(data))
				}
				return printSlugs(data)
			})
		},
	})

	return cmd
}

func slugsOnly(data any) []string {
	items := asSlice(unwrapList(data))
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch t := item.(type) {
		case string:
			if t != "" {
				out = append(out, t)
			}
		case map[string]any:
			if slug := firstString(t, "slug"); slug != "" {
				out = append(out, slug)
			}
		}
	}
	return out
}

func printSlugs(data any) error {
	for _, slug := range slugsOnly(data) {
		fmt.Fprintln(os.Stdout, slug)
	}
	return nil
}
