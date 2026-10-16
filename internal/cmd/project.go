package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"projects", "p"},
		Short:   "Create, inspect and manage projects",
	}

	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectGetCmd())
	cmd.AddCommand(newProjectCreateCmd())
	cmd.AddCommand(newProjectRedeployCmd())
	cmd.AddCommand(newProjectRollbackCmd())
	cmd.AddCommand(newProjectRestartCmd())
	cmd.AddCommand(newProjectCancelCmd())
	cmd.AddCommand(newProjectStartStopCmd("start"))
	cmd.AddCommand(newProjectStartStopCmd("stop"))
	cmd.AddCommand(newProjectToggleCmd("maintenance", "Turn maintenance mode on or off",
		"Visitors see a maintenance page instead of the app while it is on."))
	cmd.AddCommand(newProjectToggleCmd("password", "Turn password protection on or off",
		"Visitors must enter a password to view the project while it is on."))
	cmd.AddCommand(newProjectToggleCmd("previews", "Turn preview deployments on or off",
		"Pull requests and non-production branches get their own deployments while it is on."))
	cmd.AddCommand(newProjectTransferCmd())
	cmd.AddCommand(newProjectDeleteCmd())
	cmd.AddCommand(newProjectLinkCmd())
	cmd.AddCommand(newProjectUnlinkCmd())
	cmd.AddCommand(deprecatedAlias(newProjectLinkCmd(), "link", "connect-repo"))
	cmd.AddCommand(deprecatedAlias(newProjectUnlinkCmd(), "unlink", "disconnect-repo"))
	cmd.AddCommand(newProjectStatsCmd())
	return cmd
}

func newProjectListCmd() *cobra.Command {
	var environment, serviceType, status, sort string
	var page, limit int
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List projects",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				environmentID := ""
				if environment != "" {
					env, err := pickEnvironment(ctx, environment)
					if err != nil {
						return err
					}
					environmentID = env.ID
				}
				fetch := func(ctx context.Context) error {
					var list client.ProjectList
					var err error
					load := func() error {
						list, err = coreClient.ListProjects(ctx, client.ListProjectsOpts{
							TeamID:        cfg.TeamID,
							EnvironmentID: environmentID,
							ServiceType:   serviceType,
							Status:        strings.ToUpper(status),
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
					if jsonOutput() {
						return printResult(list)
					}
					return printProjectList(list)
				}
				if watch {
					return runWatch(ctx, interval, fetch)
				}
				return fetch(ctx)
			})
		},
	}

	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Only projects in this environment (name or ID)")
	cmd.Flags().StringVar(&serviceType, "type", "", "Filter by type: web-service, static, worker, mcp, database")
	cmd.Flags().StringVar(&status, "status", "", "Filter by latest deployment status, e.g. ACTIVE or FAILED")
	cmd.Flags().StringVar(&sort, "sort", "updatedAt", "Sort field, newest first: updatedAt or createdAt")
	cmd.Flags().IntVar(&page, "page", 0, "Page number")
	cmd.Flags().IntVar(&limit, "limit", 50, "Projects per page")
	addWatchFlags(cmd, &watch, &interval)
	return cmd
}

func newProjectGetCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:     "get [project]",
		Aliases: []string{"view", "show"},
		Short:   "Show a project's details",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				fetch := func(ctx context.Context) error {
					var project client.Project
					var err error
					load := func() error {
						project, err = coreClient.GetProject(ctx, ref.ID, cfg.TeamID)
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
					if jsonOutput() {
						return printResult(project)
					}
					return printProjectView(project)
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
	var f createFlags
	var detach, noLogs bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project and deploy it",
		Long: `Create a project from a Git repository, a Docker image or a local folder.

In a terminal you're walked through the same steps as the dashboard. In
scripts, pass --repo, --image or --dir.

Build, start, resources and the rest of a project's settings live in
brimble.toml in your repository; Brimble applies it on each deploy. Start
one from an existing project with "brimble config pull".`,
		Example: `  brimble project create
  brimble project create --repo acme/web --branch main --region eu-central
  brimble project create --image nginx:1.27 --name edge
  brimble project create --dir ./dist --name docs-site`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var draft *createDraft
				if f.given() {
					var err error
					if draft, err = draftFromFlags(ctx, f); err != nil {
						return err
					}
				}
				return runCreate(ctx, draft, createOptions{detach: detach, noLogs: noLogs})
			})
		},
	}

	cmd.Flags().StringVar(&f.repo, "repo", "", "Git repository to deploy, as owner/name")
	cmd.Flags().StringVar(&f.git, "git", "github", "Git provider for --repo: github, gitlab or bitbucket")
	cmd.Flags().StringVar(&f.branch, "branch", "", "Production branch (default: the repository's default branch)")
	cmd.Flags().StringVar(&f.image, "image", "", "Docker image to deploy, e.g. nginx:1.27")
	cmd.Flags().StringVar(&f.dir, "dir", "", "Local folder to upload as a static site")
	cmd.Flags().StringVar(&f.name, "name", "", "Project name (default: from the repo, image or folder)")
	cmd.Flags().StringVar(&f.serviceType, "type", "", "Service type: web-service (default), static, worker or mcp")
	cmd.Flags().StringVar(&f.region, "region", "", "Region name, e.g. eu-central (default: your plan's default)")
	cmd.Flags().BoolVar(&f.noDeploy, "no-deploy", false, "Create without deploying")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Don't follow the first deployment")
	cmd.Flags().BoolVar(&noLogs, "no-logs", false, "Follow the deployment without printing build output")
	return cmd
}

func newProjectRedeployCmd() *cobra.Command {
	var detach, noLogs bool

	cmd := &cobra.Command{
		Use:   "redeploy [project]",
		Short: "Rebuild and deploy the latest commit",
		Long: `Rebuild and deploy the latest commit of the production branch.

To change build or runtime settings, edit brimble.toml in your repository
and push; preview a change first with "brimble config plan".`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				logID, err := redeploy(ctx, ref, nil, false)
				if err != nil {
					return err
				}
				return afterDeployStarted(ctx, ref, logID, detach, !noLogs)
			})
		},
	}

	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Don't follow the deployment")
	cmd.Flags().BoolVar(&noLogs, "no-logs", false, "Follow the deployment without printing build output")
	return cmd
}

func newProjectRollbackCmd() *cobra.Command {
	var detach, yes bool

	cmd := &cobra.Command{
		Use:   "rollback [project] [deploymentId]",
		Short: "Redeploy the commit of an earlier deployment",
		Long: `Redeploy the commit of an earlier successful deployment.

Without a deployment ID you pick one from recent successful deployments.`,
		Args: cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				target := client.DeployLog{ID: argOr(args, 1)}
				if target.ID == "" {
					if !ui.IsInteractive() {
						return fmt.Errorf("deployment ID is required — see `brimble logs list %s`", ref.Name)
					}
					target, err = pickDeployment(ctx, ref, "Roll back to", func(d client.DeployLog) bool {
						return deploySucceeded(d.Status) || d.Status == "SUPERSEDED"
					})
					if err != nil {
						return err
					}
				}
				label := shortID(target.ID)
				if target.Message != "" {
					label += " (" + truncate(target.Message, 50) + ")"
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Roll %s back to %s?", ref.Display(), label), false)
				if err != nil || !ok {
					return err
				}
				body, err := json.Marshal(client.RedeployRequest{LogID: target.ID})
				if err != nil {
					return err
				}
				logID, err := redeploy(ctx, ref, body, false)
				if err != nil {
					return err
				}
				return afterDeployStarted(ctx, ref, logID, detach, true)
			})
		},
	}

	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Don't follow the deployment")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectRestartCmd() *cobra.Command {
	var detach bool
	cmd := &cobra.Command{
		Use:   "restart [project]",
		Short: "Restart the running deployment without rebuilding",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				body, err := json.Marshal(client.RedeployRequest{StartOnly: true})
				if err != nil {
					return err
				}
				logID, err := redeploy(ctx, ref, body, true)
				if err != nil {
					return err
				}
				return afterDeployStarted(ctx, ref, logID, detach, false)
			})
		},
	}
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Don't wait for the restart to finish")
	return cmd
}

func newProjectCancelCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "cancel [project] [deploymentId]",
		Short: "Cancel an in-progress deployment",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				logID := argOr(args, 1)
				if logID == "" {
					d, err := pickDeployment(ctx, ref, "Cancel which deployment?", func(d client.DeployLog) bool {
						return !deployFinished(d.Status)
					})
					if err != nil {
						return fmt.Errorf("%s has no deployment in progress", ref.Display())
					}
					logID = d.ID
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Cancel deployment %s of %s?", shortID(logID), ref.Display()), false)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Cancelling deployment…", func() error {
					return coreClient.CancelDeploy(ctx, ref.ID, logID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Cancelled deployment %s of %s", shortID(logID), ref.Display())
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectStartStopCmd(action string) *cobra.Command {
	var yes bool
	short := "Start a stopped project"
	if action == "stop" {
		short = "Stop a project (it stops serving traffic)"
	}
	cmd := &cobra.Command{
		Use:   action + " [project]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				if action == "stop" {
					ok, err := confirmOrAbort(yes, fmt.Sprintf("Stop %s? It will stop serving traffic.", ref.Display()), true)
					if err != nil || !ok {
						return err
					}
				}
				var data json.RawMessage
				verb := map[string]string{"start": "Starting", "stop": "Stopping"}[action]
				err = ui.WithSpinner(verb+" "+ref.Display()+"…", func() error {
					var err error
					if action == "start" {
						data, err = coreClient.StartProject(ctx, ref.ID, cfg.TeamID)
					} else {
						data, err = coreClient.StopProject(ctx, ref.ID, cfg.TeamID)
					}
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(data)
				}
				done := map[string]string{"start": "Started", "stop": "Stopped"}[action]
				ui.Success("%s %s", done, ref.Display())
				return nil
			})
		},
	}
	if action == "stop" {
		cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	}
	return cmd
}

// newProjectToggleCmd builds `project <setting> [project] on|off`.
func newProjectToggleCmd(setting, short, long string) *cobra.Command {
	var password string
	cmd := &cobra.Command{
		Use:       setting + " [project] on|off",
		Short:     short,
		Long:      short + ".\n\n" + long,
		Args:      cobra.RangeArgs(1, 2),
		ValidArgs: []string{"on", "off"},
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				state := args[len(args)-1]
				projectArg := ""
				if len(args) == 2 {
					projectArg = args[0]
				}
				var enabled bool
				switch strings.ToLower(state) {
				case "on", "true", "enable":
					enabled = true
				case "off", "false", "disable":
				default:
					return fmt.Errorf("expected on or off, got %q", state)
				}
				ref, err := pickProject(ctx, projectArg)
				if err != nil {
					return err
				}

				if setting == "password" && enabled && password == "" {
					if !ui.IsInteractive() {
						return fmt.Errorf("--password is required to turn password protection on")
					}
					if password, err = ui.Password("Password visitors must enter", ""); err != nil {
						return err
					}
					if password == "" {
						return fmt.Errorf("password cannot be empty")
					}
				}

				err = ui.WithSpinner("Updating "+ref.Display()+"…", func() error {
					switch setting {
					case "maintenance":
						return coreClient.SetMaintenance(ctx, ref.ID, enabled, cfg.TeamID)
					case "password":
						return coreClient.SetPasswordProtection(ctx, ref.ID, enabled, password, cfg.TeamID)
					case "previews":
						return coreClient.SetPreviewDeployments(ctx, ref.ID, enabled, cfg.TeamID)
					}
					return nil
				})
				if err != nil {
					return err
				}
				label := map[string]string{"maintenance": "Maintenance mode", "password": "Password protection", "previews": "Preview deployments"}[setting]
				if enabled {
					ui.Success("%s is on for %s", label, ref.Display())
				} else {
					ui.Success("%s is off for %s", label, ref.Display())
				}
				return nil
			})
		},
	}
	if setting == "password" {
		cmd.Flags().StringVar(&password, "password", "", "Password to require (prompted when omitted)")
	}
	return cmd
}

func newProjectTransferCmd() *cobra.Command {
	var toTeam string
	var yes bool
	cmd := &cobra.Command{
		Use:     "transfer [project] --to <team>",
		Short:   "Move a project to another team",
		Example: "  brimble project transfer my-api --to acme",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				team, err := pickTeam(ctx, toTeam)
				if err != nil {
					return err
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Transfer %s to %s?", ref.Display(), team.Name), true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Transferring…", func() error {
					return coreClient.TransferProject(ctx, ref.ID, team.ID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Transferred %s to %s", ref.Display(), team.Name)
				if linked != nil && linked.ProjectID == ref.ID {
					ui.Hint("Run `brimble link %s --team %s` to update this directory's link", ref.Name, team.Slug)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&toTeam, "to", "", "Destination team (ID, name or slug)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectDeleteCmd() *cobra.Command {
	var yes, deleteVolume bool

	cmd := &cobra.Command{
		Use:     "delete [project]",
		Aliases: []string{"rm"},
		Short:   "Delete a project",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				if !yes {
					if !ui.IsInteractive() {
						return fmt.Errorf("refusing to delete without --yes in a non-interactive shell")
					}
					typed, err := ui.Text(fmt.Sprintf("This permanently deletes %s. Type the project name to confirm", ref.Display()), ref.Name, "")
					if err != nil {
						return err
					}
					if typed != ref.Name {
						ui.Muted("Name did not match — aborted")
						return nil
					}
				}
				err = ui.WithSpinner("Deleting "+ref.Display()+"…", func() error {
					return coreClient.DeleteProject(ctx, ref.ID, deleteVolume, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Deleted %s", ref.Display())
				if linked != nil && linked.ProjectID == ref.ID {
					ui.Hint("This directory is still linked to it — run `brimble unlink`")
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	cmd.Flags().BoolVar(&deleteVolume, "delete-volume", false, "Also delete the attached persistent volume")
	return cmd
}

func newProjectLinkCmd() *cobra.Command {
	var repo, branch, git string

	cmd := &cobra.Command{
		Use:   "connect-repo [project]",
		Short: "Connect a Git repository to a project",
		Long: `Connect a Git repository to a project, so pushes to its branch deploy it.

In a terminal you pick the provider, account, repository and branch; in
scripts pass --repo. To link this directory to a project instead, use
"brimble link".`,
		Example: `  brimble project connect-repo
  brimble project connect-repo my-api --repo acme/api --branch main`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var req client.LinkRepoRequest
				if repo != "" {
					source := strings.ToLower(git)
					found, installationID, err := findRepo(ctx, source, repo)
					if err != nil {
						return err
					}
					req = client.LinkRepoRequest{Type: "repo", Repo: client.LinkRepoSpec{
						Name: found.FullName, Branch: cmp.Or(branch, found.Branch, "main"),
						Git: strings.ToUpper(source), InstallationID: installationID,
					}}
				} else if req, err = repoLinkQuestionnaire(ctx); err != nil {
					return err
				}
				body, err := json.Marshal(req)
				if err != nil {
					return err
				}
				err = ui.WithSpinner("Connecting repository…", func() error {
					return coreClient.LinkProject(ctx, ref.ID, body, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Connected %s to %s", req.Repo.Name, ref.Display())
				ui.Hint("Deploy it with `brimble deploy %s`", ref.Name)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository as owner/name (skips the prompts)")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch to deploy (default: the repository's default branch)")
	cmd.Flags().StringVar(&git, "git", "github", "Git provider: github, gitlab or bitbucket")
	return cmd
}

func repoLinkQuestionnaire(ctx context.Context) (client.LinkRepoRequest, error) {
	if !ui.IsInteractive() {
		return client.LinkRepoRequest{}, fmt.Errorf("not a terminal; pass --repo owner/name")
	}
	source, err := ui.Select("Git provider", []ui.Option{
		{Label: "GitHub", Value: "github"},
		{Label: "GitLab", Value: "gitlab"},
		{Label: "Bitbucket", Value: "bitbucket"},
	})
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	d := &createDraft{Source: source, TeamID: cfg.TeamID}
	accounts, err := loadGitAccounts(ctx, source, cfg.TeamID)
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	if len(accounts) == 0 {
		return client.LinkRepoRequest{}, fmt.Errorf("%s is not connected to Brimble — connect it at https://app.brimble.io", gitLabel(source))
	}
	account, err := pickGitAccount(accounts)
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	search, err := ui.Text("Search repositories (leave empty to list recent)", "", "")
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	repos, err := loadGitRepos(ctx, source, account.InstallationID, strings.TrimSpace(search), cfg.TeamID)
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	if len(repos) == 0 {
		return client.LinkRepoRequest{}, fmt.Errorf("no repositories found")
	}
	repo, err := pickGitRepo(repos)
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	branch, err := ui.Text("Branch", cmp.Or(repo.Branch, "main"), cmp.Or(repo.Branch, "main"))
	if err != nil {
		return client.LinkRepoRequest{}, err
	}
	d.InstallationID = account.InstallationID
	if repo.InstallationID > 0 {
		d.InstallationID = repo.InstallationID
	}
	spec := client.LinkRepoSpec{Name: repo.FullName, Branch: branch, Git: strings.ToUpper(source)}
	if d.InstallationID > 0 {
		spec.InstallationID = d.InstallationID
	}
	return client.LinkRepoRequest{Type: "repo", Repo: spec}, nil
}

func newProjectUnlinkCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "disconnect-repo [project]",
		Short: "Disconnect the Git repository from a project",
		Long:  "Disconnect the Git repository from a project. To unlink this directory instead, use \"brimble unlink\".",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Disconnect the repository from %s? Pushes will no longer deploy it.", ref.Display()), true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Disconnecting…", func() error {
					return coreClient.UnlinkProject(ctx, ref.ID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Disconnected the repository from %s", ref.Display())
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProjectStatsCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:     "stats [project]",
		Aliases: []string{"metrics"},
		Short:   "Show CPU, memory, traffic and latency",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				fetch := func(ctx context.Context) error {
					var stats client.ProjectStats
					var err error
					load := func() error {
						stats, err = coreClient.ProjectStats(ctx, ref.ID, cfg.TeamID)
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
					if jsonOutput() {
						return printResult(stats)
					}
					return printProjectStats(ref.Display(), stats)
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

// deprecatedAlias keeps an old command name working (hidden from help) while
// pointing users at its replacement.
func deprecatedAlias(cmd *cobra.Command, oldName, newName string) *cobra.Command {
	cmd.Use = oldName + strings.TrimPrefix(cmd.Use, newName)
	cmd.Deprecated = fmt.Sprintf("use `brimble project %s` instead", newName)
	return cmd
}
