package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/config"
	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newLinkCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "link [project]",
		Short: "Link this directory to a project",
		Long: `Link the current directory to a Brimble project.

Commands run in this directory (or below it) then default to that project,
so "brimble deploy", "brimble logs" and "brimble env list" need no arguments.
The link is stored in .brimble/project.json; it holds IDs, not secrets.`,
		Example: `  brimble link
  brimble link my-api --team acme`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				if linked != nil && linkRoot == wd && !yes {
					ok, err := confirmOrAbort(false, fmt.Sprintf("This directory is linked to %s. Link it to another project?", linked.ProjectName), false)
					if err != nil || !ok {
						return err
					}
				}

				// Ignore the existing link while choosing a new project.
				previous := linked
				linked = nil
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					linked = previous
					return err
				}
				return writeLink(wd, ref)
			})
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Replace an existing link without asking")
	return cmd
}

func newUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove this directory's project link",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if linked == nil {
				ui.Muted("This directory is not linked to a project")
				return nil
			}
			if err := config.RemoveLink(linkRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			ui.Success("Unlinked %s from %s", ui.Dim(linkRoot), linked.ProjectName)
			return nil
		},
	}
}

type statusView struct {
	Project     client.Project     `json:"project"`
	Deployments []client.DeployLog `json:"deployments"`
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [project]",
		Short: "Show a project's status and recent deployments",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				project, err := projectData(ctx, &ref)
				if err != nil {
					return err
				}
				var logs client.DeployLogList
				err = ui.WithSpinner("Loading deployments…", func() error {
					var err error
					logs, err = coreClient.ListDeployLogs(ctx, ref.ID, url.Values{"limit": {"5"}}, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(statusView{Project: project, Deployments: logs.Logs})
				}
				if err := printProjectView(project); err != nil {
					return err
				}
				ui.Section("Recent deployments")
				list := deployments(logs)
				if len(list) == 0 {
					ui.Muted("  None yet — run `brimble deploy`")
				} else {
					rows := make([][]string, 0, len(list))
					for _, d := range list {
						rows = append(rows, []string{shortID(d.ID), ui.Status(d.Status), truncate(d.Message, 48), formatAgo(d.CreatedAt)})
					}
					ui.Table([]string{"ID", "STATUS", "COMMIT", "CREATED"}, rows)
				}
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	}
}

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [project]",
		Short: "Open a project's URL in your browser",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				project, err := projectData(ctx, &ref)
				if err != nil {
					return err
				}
				domain := project.Domain
				if domain == "" {
					return fmt.Errorf("%s has no public URL yet", ref.Display())
				}
				if err := openURL(domain); err != nil {
					fmt.Fprintln(os.Stdout, domain)
					return nil
				}
				ui.Info("Opened %s", domain)
				return nil
			})
		},
	}
}
