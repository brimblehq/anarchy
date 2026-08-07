package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"projects"},
		Short:   "Manage projects",
	}

	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectGetCmd())
	cmd.AddCommand(newProjectCreateCmd())
	cmd.AddCommand(newProjectRedeployCmd())
	cmd.AddCommand(newProjectCancelCmd())
	cmd.AddCommand(newProjectDeleteCmd())
	cmd.AddCommand(newProjectLinkCmd())
	cmd.AddCommand(newProjectUnlinkCmd())
	cmd.AddCommand(newProjectStatsCmd())
	return cmd
}

func newProjectListCmd() *cobra.Command {
	var environmentID, serviceType, status, sort string
	var page, limit int
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				fetch := func(ctx context.Context) error {
					var data any
					var err error
					load := func() error {
						data, err = coreClient.ListProjects(ctx, client.ListProjectsOpts{
							TeamID:        cfg.TeamID,
							EnvironmentID: environmentID,
							ServiceType:   serviceType,
							Status:        status,
							Sort:          sort,
							Page:          page,
							Limit:         limit,
						})
						return err
					}
					if watch {
						err = load()
					} else {
						err = ui.WithSpinner("Loading projects…", load)
					}
					if err != nil {
						return err
					}
					if cfg.Output == "json" {
						return printResult(unwrapList(data))
					}
					return printProjectList(unwrapList(data))
				}
				if watch {
					return runWatch(ctx, interval, fetch)
				}
				return fetch(ctx)
			})
		},
	}

	cmd.Flags().StringVar(&environmentID, "environment", "", "Environment filter")
	cmd.Flags().StringVar(&serviceType, "service-type", "", "Service type filter")
	cmd.Flags().StringVar(&status, "status", "", "Status filter")
	cmd.Flags().StringVar(&sort, "sort", "", "Sort order")
	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&limit, "limit", 0, "Page size")
	addWatchFlags(cmd, &watch, &interval)
	return cmd
}

func newProjectGetCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "get [name]",
		Short: "Show a project",
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
				fetch := func(ctx context.Context) error {
					var data any
					var err error
					key := ref.Name
					if key == "" {
						key = ref.APIKey()
					}
					load := func() error {
						data, err = coreClient.GetProject(ctx, key, cfg.TeamID)
						return err
					}
					if watch {
						err = load()
					} else {
						err = ui.WithSpinner("Loading "+ref.Display()+"…", load)
					}
					if err != nil {
						return err
					}
					if cfg.Output == "json" {
						return printResult(data)
					}
					return printProjectView(data)
				}
				if watch {
					return runWatch(ctx, interval, fetch)
				}
				return fetch(ctx)
			})
		},
	}

	addWatchFlags(cmd, &watch, &interval)
	return cmd
}

func newProjectCreateCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create and deploy a project",
		Long: `Create a project interactively (name, source, service type, commands, region).

In a TTY you get a questionnaire matching the dashboard create flow.
For scripts, pass a full Core JSON body with --file.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var body any
				var err error
				if file != "" {
					body, err = readJSONFile(file)
					if err != nil {
						return err
					}
				} else {
					body, err = runCreateQuestionnaire(ctx, cfg.TeamID)
					if err != nil {
						return err
					}
				}

				var data any
				err = ui.WithSpinner("Creating project…", func() error {
					var err error
					data, err = coreClient.CreateProject(ctx, body, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if cfg.Output == "json" {
					return printResult(data)
				}
				if name := projectNameFromAny(data); name != "" {
					ui.Success("Created %s", name)
					return nil
				}
				if m, ok := body.(map[string]any); ok {
					if n, _ := m["name"].(string); n != "" {
						ui.Success("Created %s", n)
						return nil
					}
				}
				ui.Success("Project created")
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON body file for non-interactive create (or - for stdin)")
	return cmd
}

func newProjectRedeployCmd() *cobra.Command {
	var file string
	var yes bool

	cmd := &cobra.Command{
		Use:   "redeploy [name]",
		Short: "Redeploy a project",
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
				if !yes && ui.IsInteractive() {
					ok, err := ui.Confirm(fmt.Sprintf("Redeploy %s?", ref.Display()), true)
					if err != nil {
						return err
					}
					if !ok {
						ui.Muted("Aborted")
						return nil
					}
				}
				var body any
				if file != "" {
					body, err = readJSONFile(file)
					if err != nil {
						return err
					}
				}
				err = ui.WithSpinner("Redeploying "+ref.Display()+"…", func() error {
					_, err := coreClient.RedeployProject(ctx, ref.APIKey(), body, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				ui.Success("Redeploy started for %s", ref.Display())
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Optional JSON body file")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel [name] [logId]",
		Short: "Cancel a deployment",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				projectArg := ""
				logID := ""
				if len(args) > 0 {
					projectArg = args[0]
				}
				if len(args) > 1 {
					logID = args[1]
				}
				ref, err := pickProject(ctx, projectArg)
				if err != nil {
					return err
				}
				if logID == "" {
					if !ui.IsInteractive() {
						return fmt.Errorf("deploy log id is required")
					}
					logID, err = ui.Text("Deploy log id", "", "")
					if err != nil {
						return err
					}
					if logID == "" {
						return fmt.Errorf("deploy log id is required")
					}
				}
				err = ui.WithSpinner("Cancelling deploy…", func() error {
					_, err := coreClient.CancelDeploy(ctx, ref.APIKey(), logID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				ui.Success("Cancel requested for %s", ref.Display())
				return nil
			})
		},
	}
}

func newProjectDeleteCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a project",
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
				if !yes {
					if !ui.IsInteractive() {
						return fmt.Errorf("refusing to delete without --yes in non-interactive mode")
					}
					ok, err := ui.Confirm(fmt.Sprintf("Delete %s? This cannot be undone", ref.Display()), false)
					if err != nil {
						return err
					}
					if !ok {
						ui.Muted("Aborted")
						return nil
					}
				}
				err = ui.WithSpinner("Deleting "+ref.Display()+"…", func() error {
					_, err := coreClient.DeleteProject(ctx, ref.APIKey(), cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				ui.Success("Deleted %s", ref.Display())
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectLinkCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "link [name]",
		Short: "Link a repo or service to a project",
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
				if file == "" && ui.IsInteractive() {
					file, err = ui.Text("Path to JSON body file", "", "")
					if err != nil {
						return err
					}
				}
				if file == "" {
					return fmt.Errorf("--file is required")
				}
				body, err := readJSONFile(file)
				if err != nil {
					return err
				}
				err = ui.WithSpinner("Linking…", func() error {
					_, err := coreClient.LinkProject(ctx, ref.APIKey(), body, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				ui.Success("Linked to %s", ref.Display())
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON body file")
	return cmd
}

func newProjectUnlinkCmd() *cobra.Command {
	var linkType string
	var file string

	cmd := &cobra.Command{
		Use:   "unlink [name]",
		Short: "Unlink a repo or service from a project",
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
				} else if linkType != "" {
					body = map[string]any{"type": linkType}
				} else if ui.IsInteractive() {
					linkType, err = ui.Text("Link type (repo or service)", "repo", "repo")
					if err != nil {
						return err
					}
					body = map[string]any{"type": linkType}
				} else {
					return fmt.Errorf("provide --type or --file")
				}
				err = ui.WithSpinner("Unlinking…", func() error {
					_, err := coreClient.UnlinkProject(ctx, ref.APIKey(), body, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				ui.Success("Unlinked from %s", ref.Display())
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&linkType, "type", "", "Link type: repo or service")
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON body file")
	return cmd
}

func newProjectStatsCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "stats [name]",
		Short: "Show project stats",
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
				fetch := func(ctx context.Context) error {
					var data any
					var err error
					load := func() error {
						data, err = coreClient.ProjectStats(ctx, ref.APIKey(), cfg.TeamID)
						return err
					}
					if watch {
						err = load()
					} else {
						err = ui.WithSpinner("Loading stats…", load)
					}
					if err != nil {
						return err
					}
					if cfg.Output == "json" {
						return printResult(data)
					}
					return printProjectStats(ref.Display(), data)
				}
				if watch {
					return runWatch(ctx, interval, fetch)
				}
				return fetch(ctx)
			})
		},
	}

	addWatchFlags(cmd, &watch, &interval)
	return cmd
}

func projectNameFromAny(data any) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	if n := firstString(m, "name"); n != "" {
		return n
	}
	if p, ok := m["project"].(map[string]any); ok {
		return firstString(p, "name")
	}
	return ""
}
