package cmd

import (
	"context"
	"fmt"
	"os"

	"encoding/json"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "environment",
		Aliases: []string{"environments"},
		Short:   "Manage environments (production, staging, …)",
		Long: `Manage environments, which group projects into production, staging,
previews and so on. For a project's variables, use "brimble env".`,
	}
	cmd.AddCommand(newEnvironmentListCmd())
	cmd.AddCommand(newEnvironmentCreateCmd())
	cmd.AddCommand(newEnvironmentDeleteCmd())
	cmd.AddCommand(newEnvironmentDefaultCmd())
	return cmd
}

func loadEnvironments(ctx context.Context) ([]client.Environment, error) {
	var envs []client.Environment
	err := ui.WithSpinner("Loading environments…", func() error {
		var err error
		envs, err = coreClient.ListEnvironments(ctx, cfg.TeamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]client.Environment, 0, len(envs))
	for _, e := range envs {
		if e.Name != "" {
			out = append(out, e)
		}
	}
	return out, nil
}

// pickEnvironment resolves an environment by ID, name or slug, or prompts.
func pickEnvironment(ctx context.Context, arg string) (client.Environment, error) {
	envs, err := loadEnvironments(ctx)
	if err != nil {
		return client.Environment{}, err
	}
	if arg != "" {
		for _, e := range envs {
			if e.ID == arg || e.Name == arg || e.Slug == arg {
				return e, nil
			}
		}
		return client.Environment{}, fmt.Errorf("environment %q not found — see `brimble environment list`", arg)
	}
	if !ui.IsInteractive() {
		return client.Environment{}, fmt.Errorf("environment name is required")
	}
	opts := make([]ui.Option, 0, len(envs))
	for _, e := range envs {
		opts = append(opts, ui.Option{Label: e.Name, Value: e.ID})
	}
	id, err := ui.Select("Select an environment", opts)
	if err != nil {
		return client.Environment{}, err
	}
	for _, e := range envs {
		if e.ID == id {
			return e, nil
		}
	}
	return client.Environment{}, fmt.Errorf("environment not found")
}

func newEnvironmentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List environments",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				envs, err := loadEnvironments(ctx)
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(envs)
				}
				rows := make([][]string, 0, len(envs))
				for _, e := range envs {
					def := " "
					if e.IsDefault {
						def = ui.Green("default")
					}
					rows = append(rows, []string{e.Name, e.Slug, def})
				}
				fmt.Fprintln(os.Stdout)
				ui.Table([]string{"NAME", "SLUG", ""}, rows)
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	}
}

func newEnvironmentCreateCmd() *cobra.Command {
	var inheritFrom string
	cmd := &cobra.Command{
		Use:     "create <name>",
		Short:   "Create an environment",
		Example: "  brimble environment create staging --inherit-from production",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				parentID := ""
				if inheritFrom != "" {
					parent, err := pickEnvironment(ctx, inheritFrom)
					if err != nil {
						return err
					}
					parentID = parent.ID
				}
				var data json.RawMessage
				err := ui.WithSpinner("Creating "+args[0]+"…", func() error {
					var err error
					data, err = coreClient.CreateEnvironment(ctx, args[0], parentID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(data)
				}
				ui.Success("Created environment %s", args[0])
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&inheritFrom, "inherit-from", "", "Environment to inherit shared variables from")
	return cmd
}

func newEnvironmentDeleteCmd() *cobra.Command {
	var moveTo string
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <name> --move-to <name>",
		Aliases: []string{"rm"},
		Short:   "Delete an environment, moving its projects elsewhere",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				env, err := pickEnvironment(ctx, args[0])
				if err != nil {
					return err
				}
				if moveTo == "" && !ui.IsInteractive() {
					return fmt.Errorf("--move-to is required: the environment that receives its projects")
				}
				target, err := pickEnvironment(ctx, moveTo)
				if err != nil {
					return err
				}
				if target.ID == env.ID {
					return fmt.Errorf("--move-to must be a different environment")
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Delete %s and move its projects to %s?", env.Name, target.Name), true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Deleting "+env.Name+"…", func() error {
					return coreClient.DeleteEnvironment(ctx, env.ID, target.ID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Deleted %s; its projects moved to %s", env.Name, target.Name)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&moveTo, "move-to", "", "Environment that receives the deleted environment's projects")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newEnvironmentDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <name>",
		Short: "Make an environment the default for new projects",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				env, err := pickEnvironment(ctx, args[0])
				if err != nil {
					return err
				}
				err = ui.WithSpinner("Updating…", func() error {
					return coreClient.SetDefaultEnvironment(ctx, env.ID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("%s is now the default environment", env.Name)
				return nil
			})
		},
	}
}
