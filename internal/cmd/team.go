package cmd

import (
	"context"

	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"teams"},
		Short:   "Manage teams",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List teams",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var data any
				err := ui.WithSpinner("Loading teams…", func() error {
					var err error
					data, err = coreClient.ListTeams(ctx)
					return err
				})
				if err != nil {
					return err
				}
				return printResult(unwrapList(data))
			})
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "get [name]",
		Short: "Get a team",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				name, err := pickTeamName(ctx, arg)
				if err != nil {
					return err
				}
				var data any
				err = ui.WithSpinner("Loading team…", func() error {
					var err error
					data, err = coreClient.GetTeam(ctx, name)
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
		Use:   "members [name]",
		Short: "List team members",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				name, err := pickTeamName(ctx, arg)
				if err != nil {
					return err
				}
				var data any
				err = ui.WithSpinner("Loading members…", func() error {
					var err error
					data, err = coreClient.TeamMembers(ctx, name)
					return err
				})
				if err != nil {
					return err
				}
				return printResult(unwrapList(data))
			})
		},
	})

	return cmd
}
