package cmd

import (
	"context"
	"fmt"

	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "env",
		Aliases: []string{"envs"},
		Short:   "Manage project environment variables",
	}

	cmd.AddCommand(newEnvListCmd())
	cmd.AddCommand(newEnvSetCmd())
	cmd.AddCommand(newEnvDeleteCmd())
	return cmd
}

func newEnvListCmd() *cobra.Command {
	var environment string

	cmd := &cobra.Command{
		Use:   "list [project]",
		Short: "List environment variable names",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				ref, err := pickProject(ctx, arg)
				if err != nil {
					return err
				}
				var data any
				err = ui.WithSpinner("Loading env vars…", func() error {
					var err error
					data, err = coreClient.ListEnvVars(ctx, ref.APIKey(), environment, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if cfg.Output == "json" {
					return printResult(unwrapList(data))
				}
				return printEnvList(ref.Display(), unwrapList(data))
			})
		},
	}

	cmd.Flags().StringVar(&environment, "environment", "", "Environment name")
	return cmd
}

func newEnvSetCmd() *cobra.Command {
	var environment, name, value, file string

	cmd := &cobra.Command{
		Use:   "set [project]",
		Short: "Create or update an environment variable",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				ref, err := pickProject(ctx, arg)
				if err != nil {
					return err
				}

				var body any
				if file != "" {
					body, err = readJSONFile(file)
					if err != nil {
						return err
					}
				} else {
					if name == "" && ui.IsInteractive() {
						name, err = ui.Text("Variable name", "", "")
						if err != nil {
							return err
						}
					}
					if name == "" {
						return fmt.Errorf("provide --name/--value or --file")
					}
					if value == "" && ui.IsInteractive() {
						value, err = ui.Password("Variable value", "")
						if err != nil {
							return err
						}
					}
					body = map[string]any{
						"name":  name,
						"value": value,
					}
				}

				err = ui.WithSpinner("Saving…", func() error {
					_, err := coreClient.SetEnvVar(ctx, ref.APIKey(), environment, body, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if name != "" {
					ui.Success("Saved %s on %s", name, ref.Display())
				} else {
					ui.Success("Saved env vars on %s", ref.Display())
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&environment, "environment", "", "Environment name")
	cmd.Flags().StringVar(&name, "name", "", "Variable name")
	cmd.Flags().StringVar(&value, "value", "", "Variable value")
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON body file")
	return cmd
}

func newEnvDeleteCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [project] [name]",
		Short: "Delete an environment variable",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				projectArg := ""
				varName := ""
				if len(args) > 0 {
					projectArg = args[0]
				}
				if len(args) > 1 {
					varName = args[1]
				}
				ref, err := pickProject(ctx, projectArg)
				if err != nil {
					return err
				}

				var list any
				err = ui.WithSpinner("Loading env vars…", func() error {
					var err error
					list, err = coreClient.ListEnvVars(ctx, ref.APIKey(), "", cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				envs := envEntries(unwrapList(list))
				if len(envs) == 0 {
					return fmt.Errorf("no environment variables on %s", ref.Display())
				}

				entry, err := pickEnvEntry(envs, varName)
				if err != nil {
					return err
				}

				if !yes && ui.IsInteractive() {
					ok, err := ui.Confirm(fmt.Sprintf("Delete %s from %s?", entry.Name, ref.Display()), false)
					if err != nil {
						return err
					}
					if !ok {
						ui.Muted("Aborted")
						return nil
					}
				}
				err = ui.WithSpinner("Deleting…", func() error {
					_, err := coreClient.DeleteEnvVar(ctx, ref.APIKey(), entry.ID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				ui.Success("Deleted %s from %s", entry.Name, ref.Display())
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

type envEntry struct {
	ID   string
	Name string
}

func envEntries(data any) []envEntry {
	items := asSlice(data)
	out := make([]envEntry, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := firstString(m, "name", "key")
		if name == "" {
			continue
		}
		out = append(out, envEntry{
			ID:   firstString(m, "_id", "id"),
			Name: name,
		})
	}
	return out
}

func pickEnvEntry(envs []envEntry, name string) (envEntry, error) {
	if name != "" {
		for _, e := range envs {
			if e.Name == name || e.ID == name {
				return e, nil
			}
		}
		return envEntry{}, fmt.Errorf("env var %q not found", name)
	}
	if !ui.IsInteractive() {
		return envEntry{}, fmt.Errorf("env var name is required")
	}
	opts := make([]ui.Option, 0, len(envs))
	byName := map[string]envEntry{}
	for _, e := range envs {
		opts = append(opts, ui.Option{Label: e.Name, Value: e.Name})
		byName[e.Name] = e
	}
	picked, err := ui.Select("Select a variable", opts)
	if err != nil {
		return envEntry{}, err
	}
	return byName[picked], nil
}

func printEnvList(project string, data any) error {
	envs := envEntries(data)
	ui.Title(project + " · env")
	if len(envs) == 0 {
		ui.Muted("  (no variables)")
		return nil
	}
	for _, e := range envs {
		ui.Bullet(e.Name)
	}
	ui.Muted("")
	ui.Muted("  %d variable(s)  ·  values hidden", len(envs))
	return nil
}
