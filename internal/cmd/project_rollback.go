package cmd

import (
	"cmp"
	"context"
	"fmt"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newProjectRollbackCmd() *cobra.Command {
	var detach, yes bool

	cmd := &cobra.Command{
		Use:   "rollback [project] [deploymentId]",
		Short: "Put an earlier deployment back live",
		Long: `Put an earlier deployment back live, with the image and environment
variables it ran with. Nothing is rebuilt.

Pushes stop deploying until you resume auto-deploy with
"brimble project auto-deploy resume" or deploy manually.

Without a deployment ID you pick from the deployments that can be rolled
back. Static sites and Docker image projects can't be rolled back.`,
		Example: `  brimble project rollback
  brimble project rollback my-api 6ad8e1c0cd318e60aec7ea04 -y`,
		Args: cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var target client.DeployLog
				if id := argOr(args, 1); id != "" {
					if target, err = findDeployment(ctx, ref, id); err != nil {
						return err
					}
				} else {
					if !ui.IsInteractive() {
						return fmt.Errorf("deployment ID is required — see `brimble logs list %s`", ref.Name)
					}
					target, err = pickDeployment(ctx, ref, "Roll back to", func(d client.DeployLog) bool {
						return d.RollbackEligible
					})
					if err != nil {
						return err
					}
				}
				label := shortID(target.ID)
				if target.Message != "" {
					label += " (" + truncate(firstLine(target.Message), 50) + ")"
				}
				prompt := fmt.Sprintf("Roll %s back to %s? Pushes won't deploy until you resume auto-deploy.", ref.Display(), label)
				ok, err := confirmOrAbort(yes, prompt, false)
				if err != nil || !ok {
					return err
				}

				var result client.RollbackResult
				err = ui.WithSpinner("Rolling back "+ref.Display()+"…", func() error {
					var err error
					result, err = coreClient.Rollback(ctx, ref.ID, target.ID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if err := afterDeployStarted(ctx, ref, result.LogID, detach, false); err != nil {
					return err
				}
				if !jsonOutput() {
					ui.Hint("Auto-deploy is paused. Resume it with `brimble project auto-deploy resume %s`", ref.Name)
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Don't follow the deployment")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectAutoDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto-deploy",
		Short: "Resume deploying on push after a rollback",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "resume [project]",
		Short: "Deploy on push again",
		Long: `A rollback pauses deploying on push, so the next commit doesn't replace
the deployment you rolled back to. This turns it back on. Deploying
manually turns it back on too.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				err = ui.WithSpinner("Resuming auto-deploy…", func() error {
					return coreClient.ResumeAutoDeploy(ctx, ref.ID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(map[string]any{"projectId": ref.ID, "autoDeployPaused": false})
				}
				ui.Success("Pushes to %s deploy again", ref.Display())
				return nil
			})
		},
	})
	return cmd
}

func newProjectDuplicateCmd() *cobra.Command {
	var envName, domainSuffix string

	cmd := &cobra.Command{
		Use:     "duplicate [project]",
		Aliases: []string{"clone"},
		Short:   "Copy a project into another environment",
		Long: `Copy a project into another environment, with its settings and environment
variables, then deploy the copy.

Its brimble.app domains are copied with the environment's slug added
(my-api.brimble.app becomes my-api-staging.brimble.app); --domain-suffix
picks a different suffix. Custom domains aren't copied.`,
		Example: `  brimble project duplicate my-api --env staging
  brimble project duplicate my-api --env staging --domain-suffix qa`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				env, err := pickEnvironment(ctx, envName)
				if err != nil {
					return err
				}
				var result client.DuplicateProjectResult
				err = ui.WithSpinner(fmt.Sprintf("Duplicating %s into %s…", ref.Display(), env.Name), func() error {
					var err error
					result, err = coreClient.DuplicateProject(ctx, ref.ID, client.DuplicateProjectRequest{
						EnvironmentID: env.ID,
						DomainSuffix:  domainSuffix,
					}, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(result)
				}
				return printDuplicateResult(ref, env, result)
			})
		},
	}

	cmd.Flags().StringVarP(&envName, "env", "e", "", "Environment to copy into (name, slug or ID)")
	cmd.Flags().StringVar(&domainSuffix, "domain-suffix", "", "Added to copied domains (default: the environment's slug)")
	return cmd
}

func printDuplicateResult(ref ProjectRef, env client.Environment, r client.DuplicateProjectResult) error {
	for _, f := range r.CapacityFailures {
		ui.Warn("%s wasn't copied: %s", cmp.Or(f.ProjectName, ref.Display()), f.Reason)
	}
	if r.ProjectID == "" {
		return fmt.Errorf("%s wasn't duplicated into %s", ref.Display(), env.Name)
	}
	ui.Success("Duplicated %s into %s — its first deploy is starting", ref.Display(), env.Name)
	printKV("Project ID", r.ProjectID)
	if r.DomainsCloned > 0 {
		printKV("Domains", fmt.Sprintf("%d copied", r.DomainsCloned))
	}
	for _, d := range r.SkippedDomains {
		if d.Reason == "purchased/custom" {
			ui.Muted("  %s wasn't copied: custom domains stay with the original", d.Name)
			continue
		}
		ui.Warn("%s wasn't copied: %s", d.Name, d.Reason)
	}
	ui.Hint("Follow it with `brimble logs list %s --watch`", r.ProjectID)
	return nil
}
