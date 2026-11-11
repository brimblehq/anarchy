package cmd

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ci",
		Aliases: []string{"ci-runner", "runners"},
		Short:   "Run GitHub Actions jobs on Brimble (CI runners)",
		Long: `Manage CI runners: Brimble sandboxes that run your GitHub organization's
Actions jobs instead of GitHub-hosted runners.

Two modes:
  per_job  a fresh sandbox runs each queued job, then is discarded
  reuse    persistent sandboxes you attach act as self-hosted runners

CI runners need a paid plan and a GitHub organization (not a personal
account).`,
	}

	cmd.AddCommand(newCIRunnerListCmd())
	cmd.AddCommand(newCIRunnerCreateCmd())
	cmd.AddCommand(newCIRunnerGetCmd())
	cmd.AddCommand(newCIRunnerUpdateCmd())
	cmd.AddCommand(newCIRunnerDeleteCmd())
	cmd.AddCommand(newCIRunnerJobsCmd())
	cmd.AddCommand(newCIRunnerLogsCmd())
	cmd.AddCommand(newCIRunnerSandboxCmd())
	return cmd
}

func loadCiRunners(ctx context.Context) ([]client.CiRunner, error) {
	var list client.CiRunnerList
	err := ui.WithSpinner("Loading CI runners…", func() error {
		var err error
		list, err = coreClient.ListCiRunners(ctx, 1, 100, cfg.TeamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return list.Data, nil
}

func pickCiRunner(ctx context.Context, arg string) (client.CiRunner, error) {
	runners, err := loadCiRunners(ctx)
	if err != nil {
		return client.CiRunner{}, err
	}
	if arg != "" {
		for _, r := range runners {
			if r.ID == arg || strings.EqualFold(r.Name, arg) {
				return r, nil
			}
		}
		return client.CiRunner{}, fmt.Errorf("CI runner %q not found%s — see `brimble ci list`", arg, teamSuffix())
	}
	if len(runners) == 0 {
		return client.CiRunner{}, fmt.Errorf("no CI runners%s — create one with `brimble ci create`", teamSuffix())
	}
	if !ui.IsInteractive() {
		return client.CiRunner{}, fmt.Errorf("a CI runner is required — pass its name")
	}
	opts := make([]ui.Option, 0, len(runners))
	byID := map[string]client.CiRunner{}
	for _, r := range runners {
		label := r.Name
		if target := runnerTarget(r); target != "" {
			label += "  " + ui.Dim(target)
		}
		opts = append(opts, ui.Option{Label: label, Value: r.ID})
		byID[r.ID] = r
	}
	id, err := ui.Select("Select a CI runner", opts)
	if err != nil {
		return client.CiRunner{}, err
	}
	return byID[id], nil
}

func newCIRunnerListCmd() *cobra.Command {
	var page, limit int
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List CI runners",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var list client.CiRunnerList
				err := ui.WithSpinner("Loading CI runners…", func() error {
					var err error
					list, err = coreClient.ListCiRunners(ctx, page, limit, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(list)
				}
				printCiRunnerList(list)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&limit, "limit", 20, "Entries per page (max 100)")
	return cmd
}

func printCiRunnerList(list client.CiRunnerList) {
	ui.Title("CI runners")
	if len(list.Data) == 0 {
		ui.Muted("  No CI runners yet — create one with `brimble ci create`")
		fmt.Fprintln(os.Stdout)
		return
	}
	rows := make([][]string, 0, len(list.Data))
	for _, r := range list.Data {
		rows = append(rows, []string{
			shortID(r.ID),
			r.Name,
			runnerProvider(r.Provider),
			runnerTarget(r),
			runnerMode(r.Mode),
			runnerStatus(r.Enabled),
			formatAgo(r.CreatedAt),
		})
	}
	ui.Table([]string{"ID", "NAME", "PROVIDER", "TARGET", "MODE", "STATUS", "CREATED"}, rows)
	if list.TotalPages > 1 {
		fmt.Fprintln(os.Stdout)
		ui.Muted("  Page %d of %d · --page to see more", max(1, list.CurrentPage), list.TotalPages)
	}
	fmt.Fprintln(os.Stdout)
}

func newCIRunnerCreateCmd() *cobra.Command {
	var name, account, mode, size, region, provider, resourceClass string
	var maxConcurrency int
	var yes, tokenStdin bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a CI runner for GitHub Actions or CircleCI",
		Long: `Create a CI runner so a GitHub organization's Actions jobs, or the jobs of a
CircleCI resource class, run on Brimble sandboxes.

GitHub needs an organization connected to your account. In per_job mode a
size and region are required.

CircleCI needs a resource class and its token. The token is read from a
prompt, or from stdin with --token-stdin; it is never a flag. CircleCI
runners always use long-lived sandboxes.`,
		Example: `  brimble ci create --account acme --name ci --mode per_job --size sbx.micro --region <region>
  brimble ci create --account acme --mode reuse
  brimble ci create --provider circleci --resource-class acme/brimble --name circle
  cat token.txt | brimble ci create --provider circleci --resource-class acme/brimble --name circle --token-stdin -y`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				chosen, err := pickRunnerProvider(provider, resourceClass)
				if err != nil {
					return err
				}
				if chosen == providerCircleCI {
					return createCircleCiRunner(ctx, name, resourceClass, tokenStdin, yes)
				}

				acct, err := resolveGitAccount(ctx, account)
				if err != nil {
					return err
				}

				if name == "" && ui.IsInteractive() {
					name, err = ui.Text("Runner name", "runs GitHub Actions jobs", acct.Username+"-runner")
					if err != nil {
						return err
					}
				}
				name = strings.TrimSpace(name)
				if name == "" {
					return fmt.Errorf("--name is required")
				}

				if mode == "" {
					if ui.IsInteractive() {
						mode, err = ui.Select("Runner mode", []ui.Option{
							{Label: "Per-job sandbox — a fresh sandbox runs each job", Value: "per_job"},
							{Label: "Reuse sandboxes — attach persistent sandboxes as self-hosted runners", Value: "reuse"},
						})
						if err != nil {
							return err
						}
					} else {
						mode = "per_job"
					}
				}
				if mode != "per_job" && mode != "reuse" {
					return fmt.Errorf("--mode must be per_job or reuse")
				}

				if mode == "per_job" {
					region, err = resolveSandboxRegion(ctx, region)
					if err != nil {
						return err
					}
					if size == "" {
						size, err = pickSandboxSize(ctx, region)
						if err != nil {
							return err
						}
					}
				}

				prompt := fmt.Sprintf("Create CI runner %q for the %s GitHub organization (%s mode)?", name, acct.Username, runnerMode(mode))
				ok, err := confirmOrAbort(yes, prompt, false)
				if err != nil || !ok {
					return err
				}

				var runner client.CiRunner
				err = ui.WithSpinner("Creating CI runner…", func() error {
					var err error
					runner, err = coreClient.CreateCiRunner(ctx, client.CreateCiRunnerRequest{
						Name:           name,
						InstallationID: acct.InstallationID,
						Mode:           mode,
						Size:           size,
						Region:         region,
						MaxConcurrency: maxConcurrency,
					}, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(runner)
				}
				ui.Success("Created CI runner %s", runner.Name)
				printCiRunnerDetail(runner)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Runner name (default: <org>-runner)")
	cmd.Flags().StringVar(&provider, "provider", "", "CI provider: github or circleci (default: github)")
	cmd.Flags().StringVar(&resourceClass, "resource-class", "", "CircleCI resource class, as namespace/name")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Read the CircleCI resource class token from stdin")
	cmd.Flags().StringVar(&account, "account", "", "GitHub organization (username or name)")
	cmd.Flags().StringVar(&mode, "mode", "", "Mode: per_job or reuse")
	cmd.Flags().StringVar(&size, "size", "", "Sandbox size for per_job mode (e.g. sbx.micro)")
	cmd.Flags().StringVar(&region, "region", "", "Sandbox region for per_job mode")
	cmd.Flags().IntVar(&maxConcurrency, "max-concurrency", 0, "Maximum concurrent jobs")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	cmd.RegisterFlagCompletionFunc("account", completeGitAccountNames)
	cmd.RegisterFlagCompletionFunc("mode", completeModes)
	cmd.RegisterFlagCompletionFunc("size", completeSandboxSizeNames)
	cmd.RegisterFlagCompletionFunc("region", completeSandboxRegionNames)
	return cmd
}

// pickRunnerProvider resolves the provider from --provider, from a CircleCI
// flag being set, or by asking.
func pickRunnerProvider(provider, resourceClass string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return providerGitHub, nil
	case "circleci":
		return providerCircleCI, nil
	case "":
	default:
		return "", fmt.Errorf("--provider must be github or circleci")
	}
	if resourceClass != "" {
		return providerCircleCI, nil
	}
	if !ui.IsInteractive() {
		return providerGitHub, nil
	}
	return ui.Select("CI provider", []ui.Option{
		{Label: "GitHub Actions", Value: providerGitHub},
		{Label: "CircleCI", Value: providerCircleCI},
	})
}

func createCircleCiRunner(ctx context.Context, name, resourceClass string, tokenStdin, yes bool) error {
	var err error
	resourceClass = strings.TrimSpace(resourceClass)
	if resourceClass == "" && ui.IsInteractive() && !tokenStdin {
		resourceClass, err = ui.Text("Resource class", "namespace/name", "")
		if err != nil {
			return err
		}
		resourceClass = strings.TrimSpace(resourceClass)
	}
	if resourceClass == "" {
		return fmt.Errorf("--resource-class is required for CircleCI")
	}

	if name == "" && ui.IsInteractive() && !tokenStdin {
		name, err = ui.Text("Runner name", "runs CircleCI jobs", "circleci-runner")
		if err != nil {
			return err
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("--name is required")
	}

	token, err := readRunnerToken(tokenStdin)
	if err != nil {
		return err
	}

	// stdin carried the token, so there is no terminal left to confirm on.
	ok, err := confirmOrAbort(yes || tokenStdin, fmt.Sprintf("Create CI runner %q for the CircleCI resource class %s?", name, resourceClass), false)
	if err != nil || !ok {
		return err
	}

	var runner client.CiRunner
	err = ui.WithSpinner("Creating CI runner…", func() error {
		var err error
		runner, err = coreClient.CreateCiRunner(ctx, client.CreateCiRunnerRequest{
			Name:          name,
			Provider:      providerCircleCI,
			ResourceClass: resourceClass,
			Token:         token,
			Mode:          "reuse",
		}, cfg.TeamID)
		return err
	})
	if err != nil {
		return err
	}
	if jsonOutput() {
		return printResult(runner)
	}
	ui.Success("Created CI runner %s", runner.Name)
	printCiRunnerDetail(runner)
	ui.Muted("  Attach a sandbox to start running jobs: brimble ci sandbox attach %s", runner.Name)
	fmt.Fprintln(os.Stdout)
	return nil
}

func newCIRunnerGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "get [runner]",
		Aliases:           []string{"show", "info"},
		Short:             "Show a CI runner's details",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(runner)
				}
				printCiRunnerDetail(runner)
				return nil
			})
		},
	}
}

func newCIRunnerUpdateCmd() *cobra.Command {
	var name, mode, size, region string
	var maxConcurrency int
	var enable, disable, tokenStdin bool

	cmd := &cobra.Command{
		Use:   "update [runner]",
		Short: "Update a CI runner",
		Long: `Update a CI runner's name, mode, size, region or concurrency, or pause it
with --disable. With no flags you can change fields interactively.

For a CircleCI runner, --token-stdin replaces the resource class token and
restarts the runner in its attached sandboxes.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}

				var body client.UpdateCiRunnerRequest
				if tokenStdin {
					body.Token, err = readRunnerToken(true)
				} else {
					body, err = buildUpdateRequest(ctx, runner, name, mode, size, region, maxConcurrency, enable, disable)
				}
				if err != nil {
					return err
				}

				var updated client.CiRunner
				err = ui.WithSpinner("Updating CI runner…", func() error {
					var err error
					updated, err = coreClient.UpdateCiRunner(ctx, runner.ID, body, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(updated)
				}
				ui.Success("Updated CI runner %s", updated.Name)
				printCiRunnerDetail(updated)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "New runner name")
	cmd.Flags().StringVar(&mode, "mode", "", "Mode: per_job or reuse")
	cmd.Flags().StringVar(&size, "size", "", "Sandbox size (per_job)")
	cmd.Flags().StringVar(&region, "region", "", "Sandbox region (per_job)")
	cmd.Flags().IntVar(&maxConcurrency, "max-concurrency", 0, "Maximum concurrent jobs")
	cmd.Flags().BoolVar(&enable, "enable", false, "Enable the runner")
	cmd.Flags().BoolVar(&disable, "disable", false, "Disable the runner")
	cmd.Flags().BoolVar(&tokenStdin, "token-stdin", false, "Replace a CircleCI runner's token, read from stdin")
	cmd.MarkFlagsMutuallyExclusive("enable", "disable")
	cmd.RegisterFlagCompletionFunc("mode", completeModes)
	cmd.RegisterFlagCompletionFunc("size", completeSandboxSizeNames)
	cmd.RegisterFlagCompletionFunc("region", completeSandboxRegionNames)
	return cmd
}

func buildUpdateRequest(ctx context.Context, runner client.CiRunner, name, mode, size, region string, maxConcurrency int, enable, disable bool) (client.UpdateCiRunnerRequest, error) {
	body := client.UpdateCiRunnerRequest{}
	enableSet := enable || disable
	hasFlags := name != "" || mode != "" || size != "" || region != "" || maxConcurrency > 0 || enableSet

	if hasFlags {
		if name != "" {
			body.Name = name
		}
		if mode != "" {
			if mode != "per_job" && mode != "reuse" {
				return body, fmt.Errorf("--mode must be per_job or reuse")
			}
			body.Mode = mode
		}
		if size != "" {
			body.Size = size
		}
		if region != "" {
			regionID, err := resolveSandboxRegion(ctx, region)
			if err != nil {
				return body, err
			}
			body.Region = regionID
		}
		if maxConcurrency > 0 {
			body.MaxConcurrency = maxConcurrency
		}
		if enableSet {
			body.Enabled = &enable
		}
		return body, nil
	}

	if !ui.IsInteractive() {
		return body, fmt.Errorf("nothing to update — pass --name, --mode, --size, --region, --max-concurrency, --enable or --disable")
	}

	enableLabel := "Enable runner (currently paused)"
	if runner.Enabled {
		enableLabel = "Pause runner (currently active)"
	}
	field, err := ui.Select("What do you want to change?", []ui.Option{
		{Label: "Name", Value: "name"},
		{Label: "Mode", Value: "mode"},
		{Label: "Maximum concurrency", Value: "concurrency"},
		{Label: enableLabel, Value: "enabled"},
	})
	if err != nil {
		return body, err
	}

	switch field {
	case "name":
		name, err := ui.Text("New name", runner.Name, runner.Name)
		if err != nil {
			return body, err
		}
		body.Name = name
	case "mode":
		mode, err := ui.Select("New mode", []ui.Option{
			{Label: "Per-job sandbox — a fresh sandbox runs each job", Value: "per_job"},
			{Label: "Reuse sandboxes — attach persistent sandboxes as self-hosted runners", Value: "reuse"},
		})
		if err != nil {
			return body, err
		}
		body.Mode = mode
	case "concurrency":
		text, err := ui.Text("Maximum concurrent jobs", "1", strconv.Itoa(runner.MaxConcurrency))
		if err != nil {
			return body, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 1 {
			return body, fmt.Errorf("concurrency must be a positive number")
		}
		body.MaxConcurrency = n
	case "enabled":
		target := !runner.Enabled
		body.Enabled = &target
	}
	return body, nil
}

func newCIRunnerDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:               "delete [runner]",
		Aliases:           []string{"rm", "remove"},
		Short:             "Delete a CI runner",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				prompt := fmt.Sprintf("Delete CI runner %s? Attached sandboxes will be detached.", runner.Name)
				ok, err := confirmOrAbort(yes, prompt, true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Deleting CI runner…", func() error {
					return coreClient.DeleteCiRunner(ctx, runner.ID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Deleted CI runner %s", runner.Name)
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newCIRunnerJobsCmd() *cobra.Command {
	var page, limit int
	cmd := &cobra.Command{
		Use:               "jobs [runner]",
		Short:             "List a CI runner's job history",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var list client.CiRunnerJobList
				err = ui.WithSpinner("Loading jobs…", func() error {
					var err error
					list, err = coreClient.ListCiRunnerJobs(ctx, runner.ID, page, limit, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(list)
				}
				printCiRunnerJobs(runner, list)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&limit, "limit", 20, "Entries per page (max 100)")
	return cmd
}

func printCiRunnerJobs(runner client.CiRunner, list client.CiRunnerJobList) {
	ui.Title(runner.Name + " · jobs")
	if len(list.Data) == 0 {
		ui.Muted("  No jobs have run yet")
		fmt.Fprintln(os.Stdout)
		return
	}
	rows := make([][]string, 0, len(list.Data))
	for _, j := range list.Data {
		name := j.JobName
		if name == "" {
			name = j.RepoFullName
		} else if j.RepoFullName != "" {
			name = j.RepoFullName + " · " + name
		}
		rows = append(rows, []string{
			shortID(j.ID),
			name,
			ciJobStatus(j.Status),
			formatAgo(cmp.Or(j.CompletedAt, cmp.Or(j.StartedAt, j.QueuedAt))),
		})
	}
	ui.Table([]string{"ID", "JOB", "STATUS", "RAN"}, rows)
	if list.TotalPages > 1 {
		fmt.Fprintln(os.Stdout)
		ui.Muted("  Page %d of %d · --page to see more", max(1, list.CurrentPage), list.TotalPages)
	}
	fmt.Fprintln(os.Stdout)
}

func newCIRunnerLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <runner> [job]",
		Short: "Print a CI job's logs",
		Long: `Print the logs of one of a CI runner's jobs. With no job argument you can
pick a job from the list.`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, args[0])
				if err != nil {
					return err
				}

				jobID := argOr(args, 1)
				if jobID == "" {
					jobID, err = pickCiRunnerJob(ctx, runner)
					if err != nil {
						return err
					}
				}

				var logs client.CiRunnerJobLogs
				err = ui.WithSpinner("Loading job logs…", func() error {
					var err error
					logs, err = coreClient.GetCiRunnerJobLogs(ctx, runner.ID, jobID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(logs)
				}
				fmt.Fprint(os.Stdout, logs.Logs)
				if logs.Truncated {
					ui.Muted("  … logs truncated")
				}
				return nil
			})
		},
	}
	return cmd
}

func pickCiRunnerJob(ctx context.Context, runner client.CiRunner) (string, error) {
	var list client.CiRunnerJobList
	err := ui.WithSpinner("Loading jobs…", func() error {
		var err error
		list, err = coreClient.ListCiRunnerJobs(ctx, runner.ID, 1, 20, cfg.TeamID)
		return err
	})
	if err != nil {
		return "", err
	}
	if len(list.Data) == 0 {
		return "", fmt.Errorf("%s has no jobs yet", runner.Name)
	}
	if !ui.IsInteractive() {
		return "", fmt.Errorf("a job is required — pass its ID")
	}
	opts := make([]ui.Option, 0, len(list.Data))
	byID := map[string]client.CiRunnerJob{}
	for _, j := range list.Data {
		name := j.JobName
		if j.RepoFullName != "" {
			name = j.RepoFullName + " · " + name
		}
		opts = append(opts, ui.Option{Label: name + "  " + ciJobStatus(j.Status), Value: j.ID})
		byID[j.ID] = j
	}
	id, err := ui.Select("Select a job", opts)
	if err != nil {
		return "", err
	}
	return byID[id].ID, nil
}

func newCIRunnerSandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sandbox",
		Aliases: []string{"sandboxes"},
		Short:   "Attach sandboxes to a reuse-mode runner",
	}
	cmd.AddCommand(newCIRunnerSandboxListCmd())
	cmd.AddCommand(newCIRunnerSandboxAttachCmd())
	cmd.AddCommand(newCIRunnerSandboxDetachCmd())
	return cmd
}

func newCIRunnerSandboxListCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "list [runner]",
		Aliases:           []string{"ls"},
		Short:             "List a runner's attached sandboxes",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var sandboxes []client.CiRunnerSandbox
				err = ui.WithSpinner("Loading sandboxes…", func() error {
					var err error
					sandboxes, err = coreClient.ListCiRunnerSandboxes(ctx, runner.ID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(sandboxes)
				}
				ui.Title(runner.Name + " · sandboxes")
				if len(sandboxes) == 0 {
					ui.Muted("  No sandboxes attached — attach one with `brimble ci sandbox attach %s`", runner.Name)
					fmt.Fprintln(os.Stdout)
					return nil
				}
				rows := make([][]string, 0, len(sandboxes))
				for _, s := range sandboxes {
					rows = append(rows, []string{shortID(s.ID), s.RunnerName, ciSandboxStatus(s.Status), formatAgo(s.CreatedAt)})
				}
				ui.Table([]string{"ID", "RUNNER", "STATUS", "ATTACHED"}, rows)
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	}
}

func newCIRunnerSandboxAttachCmd() *cobra.Command {
	var sandboxArg, size, region string
	var createNew bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "attach [runner]",
		Short: "Attach a sandbox to a reuse-mode runner",
		Long: `Attach a sandbox to a reuse-mode CI runner so it acts as a self-hosted
runner. Attach an existing ready sandbox, or create a fresh one.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}

				if !createNew && sandboxArg == "" && ui.IsInteractive() {
					kind, err := ui.Select("Attach", []ui.Option{
						{Label: "An existing sandbox", Value: "existing"},
						{Label: "A new sandbox", Value: "new"},
					})
					if err != nil {
						return err
					}
					createNew = kind == "new"
				}

				var body client.AttachCiRunnerSandboxRequest
				if createNew {
					region, err = resolveSandboxRegion(ctx, region)
					if err != nil {
						return err
					}
					if size == "" {
						size, err = pickSandboxSize(ctx, region)
						if err != nil {
							return err
						}
					}
					body.Sandbox = &client.NewCiRunnerSandboxInput{Size: size, Region: region}
				} else {
					if sandboxArg == "" && !ui.IsInteractive() {
						return fmt.Errorf("--sandbox or --new is required in non-interactive mode")
					}
					sb, err := pickSandbox(ctx, sandboxArg)
					if err != nil {
						return err
					}
					body.SandboxID = sb.ID
				}

				prompt := fmt.Sprintf("Attach a sandbox to CI runner %s?", runner.Name)
				ok, err := confirmOrAbort(yes, prompt, false)
				if err != nil || !ok {
					return err
				}

				var attached client.CiRunnerSandbox
				err = ui.WithSpinner("Attaching sandbox…", func() error {
					var err error
					attached, err = coreClient.AttachCiRunnerSandbox(ctx, runner.ID, body, cfg.TeamID)
					if err != nil {
						return err
					}
					return waitForAttach(ctx, runner.ID, attached.SandboxID)
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(attached)
				}
				ui.Success("Attached sandbox to CI runner %s", runner.Name)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&sandboxArg, "sandbox", "", "Existing sandbox to attach (name or ID)")
	cmd.Flags().BoolVar(&createNew, "new", false, "Create and attach a new sandbox")
	cmd.Flags().StringVar(&size, "size", "", "Size for a new sandbox (e.g. sbx.micro)")
	cmd.Flags().StringVar(&region, "region", "", "Region for a new sandbox")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	cmd.RegisterFlagCompletionFunc("sandbox", completeSandboxNames)
	cmd.RegisterFlagCompletionFunc("size", completeSandboxSizeNames)
	cmd.RegisterFlagCompletionFunc("region", completeSandboxRegionNames)
	return cmd
}

func newCIRunnerSandboxDetachCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:               "detach [runner] [sandbox]",
		Short:             "Detach a sandbox from a runner",
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeRunnerArg,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				runner, err := pickCiRunner(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				sandboxID, err := pickAttachedSandbox(ctx, runner, argOr(args, 1))
				if err != nil {
					return err
				}
				prompt := fmt.Sprintf("Detach this sandbox from CI runner %s?", runner.Name)
				ok, err := confirmOrAbort(yes, prompt, true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Detaching sandbox…", func() error {
					return coreClient.DetachCiRunnerSandbox(ctx, runner.ID, sandboxID, cfg.TeamID)
				})
				if err != nil {
					return err
				}
				ui.Success("Detached sandbox from CI runner %s", runner.Name)
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func pickAttachedSandbox(ctx context.Context, runner client.CiRunner, arg string) (string, error) {
	var sandboxes []client.CiRunnerSandbox
	err := ui.WithSpinner("Loading sandboxes…", func() error {
		var err error
		sandboxes, err = coreClient.ListCiRunnerSandboxes(ctx, runner.ID, cfg.TeamID)
		return err
	})
	if err != nil {
		return "", err
	}
	if len(sandboxes) == 0 {
		return "", fmt.Errorf("%s has no attached sandboxes", runner.Name)
	}
	if arg != "" {
		for _, s := range sandboxes {
			if s.SandboxID == arg || strings.HasSuffix(s.SandboxID, arg) || strings.EqualFold(s.RunnerName, arg) {
				return s.SandboxID, nil
			}
		}
		return "", fmt.Errorf("sandbox %q is not attached to %s", arg, runner.Name)
	}
	if !ui.IsInteractive() {
		return "", fmt.Errorf("a sandbox is required — pass its ID")
	}
	opts := make([]ui.Option, 0, len(sandboxes))
	byID := map[string]client.CiRunnerSandbox{}
	for _, s := range sandboxes {
		opts = append(opts, ui.Option{Label: s.RunnerName + "  " + ciSandboxStatus(s.Status), Value: s.SandboxID})
		byID[s.SandboxID] = s
	}
	id, err := ui.Select("Detach which sandbox?", opts)
	if err != nil {
		return "", err
	}
	return byID[id].SandboxID, nil
}

// resolveGitAccount resolves a GitHub account (org) by username or name, or
// prompts for one.
func resolveGitAccount(ctx context.Context, arg string) (client.GitAccount, error) {
	accounts, err := loadGitAccounts(ctx, "github", cfg.TeamID)
	if err != nil {
		return client.GitAccount{}, err
	}
	if len(accounts) == 0 {
		return client.GitAccount{}, fmt.Errorf("no GitHub organization is connected — connect GitHub in the dashboard, then try again")
	}
	if arg != "" {
		for _, a := range accounts {
			if strings.EqualFold(a.Username, arg) || strings.EqualFold(a.Name, arg) {
				return a, nil
			}
		}
		return client.GitAccount{}, fmt.Errorf("GitHub account %q not found", arg)
	}
	if !ui.IsInteractive() {
		return client.GitAccount{}, fmt.Errorf("--account is required (your GitHub organization)")
	}
	return pickGitAccount(accounts)
}

// resolveSandboxRegion returns the ID of the sandbox region named by the
// --region flag (a name or an ID). With no flag it asks, when interactive.
func resolveSandboxRegion(ctx context.Context, requested string) (string, error) {
	var regions []client.SandboxRegion
	err := ui.WithSpinner("Loading regions…", func() error {
		var err error
		regions, err = coreClient.ListSandboxRegions(ctx, cfg.TeamID)
		return err
	})
	if err != nil {
		return "", err
	}
	if len(regions) == 0 {
		return "", fmt.Errorf("no sandbox regions are available on your account")
	}
	if requested != "" {
		names := make([]string, 0, len(regions))
		for _, r := range regions {
			if r.ID == requested || strings.EqualFold(r.Name, requested) {
				return r.ID, nil
			}
			names = append(names, r.Name)
		}
		return "", fmt.Errorf("unknown sandbox region %q — available: %s", requested, strings.Join(names, ", "))
	}
	if !ui.IsInteractive() {
		return "", fmt.Errorf("--region is required")
	}
	opts := make([]ui.Option, 0, len(regions))
	for _, r := range regions {
		label := r.Name
		if r.Country != "" {
			label += " · " + r.Country
		}
		opts = append(opts, ui.Option{Label: label, Value: r.ID})
	}
	return ui.Select("Sandbox region", opts)
}

func pickSandboxSize(ctx context.Context, region string) (string, error) {
	var sizes []client.SandboxSize
	err := ui.WithSpinner("Loading sandbox sizes…", func() error {
		var err error
		sizes, err = coreClient.ListSandboxSizes(ctx, region, cfg.TeamID)
		return err
	})
	if err != nil {
		return "", err
	}
	if len(sizes) == 0 {
		return "", fmt.Errorf("no sandbox sizes are available")
	}
	if !ui.IsInteractive() {
		for _, s := range sizes {
			if s.Available {
				return s.Name, nil
			}
		}
		return "", fmt.Errorf("--size is required")
	}
	opts := make([]ui.Option, 0, len(sizes))
	for _, s := range sizes {
		label := s.Name
		if s.VCPU > 0 || s.MemoryGB > 0 {
			label = fmt.Sprintf("%s · %d vCPU · %d GB RAM", s.Name, s.VCPU, s.MemoryGB)
		}
		opts = append(opts, ui.Option{Label: label, Value: s.Name})
	}
	return ui.Select("Sandbox size", opts)
}

func printCiRunnerDetail(r client.CiRunner) {
	ui.Title(r.Name)
	printKV("Provider", runnerProvider(r.Provider))
	if r.Provider == providerCircleCI {
		printKV("Resource class", r.ResourceClass)
	} else {
		printKV("GitHub org", r.AccountLogin)
	}
	printKV("Mode", runnerMode(r.Mode))
	printKV("Size", r.Size)
	printKV("Max concurrency", strconv.Itoa(r.MaxConcurrency))
	printKV("Status", runnerStatus(r.Enabled))
	printKV("Created", formatAgo(r.CreatedAt))
	fmt.Fprintln(os.Stdout)
}

const (
	providerGitHub   = "GITHUB"
	providerCircleCI = "CIRCLECI"
	attachPollEvery  = 3 * time.Second
	attachWaitLimit  = 3 * time.Minute
)

func runnerProvider(provider string) string {
	if provider == providerCircleCI {
		return "CircleCI"
	}
	return "GitHub"
}

// runnerTarget is what a runner serves: a GitHub organization or a CircleCI
// resource class.
func runnerTarget(r client.CiRunner) string {
	if r.Provider == providerCircleCI {
		return r.ResourceClass
	}
	if r.AccountLogin == "" {
		return ""
	}
	return "@" + r.AccountLogin
}

// readRunnerToken reads a CircleCI resource class token from stdin when asked,
// or prompts for it, so it never has to be passed as a flag.
func readRunnerToken(fromStdin bool) (string, error) {
	if fromStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", fmt.Errorf("no token on stdin")
		}
		return token, nil
	}
	if !ui.IsInteractive() {
		return "", fmt.Errorf("pass the resource class token on stdin with --token-stdin")
	}
	token, err := ui.Password("Resource class token", "from `circleci runner resource-class create --generate-token`")
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("a resource class token is required")
	}
	return token, nil
}

// waitForAttach polls until the runner in a sandbox is online. Attaching
// finishes in the background after the API call returns.
func waitForAttach(ctx context.Context, runnerID, sandboxID string) error {
	deadline := time.Now().Add(attachWaitLimit)
	for time.Now().Before(deadline) {
		sandboxes, err := coreClient.ListCiRunnerSandboxes(ctx, runnerID, cfg.TeamID)
		if err != nil {
			return err
		}
		for _, sandbox := range sandboxes {
			if sandbox.SandboxID != sandboxID {
				continue
			}
			if sandbox.Status == "online" {
				return nil
			}
			if sandbox.Status == "offline" {
				return fmt.Errorf("the runner did not start: %s\n  Detach it with `brimble ci sandbox detach` and try again", strings.TrimSpace(sandbox.Error))
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(attachPollEvery):
		}
	}
	return fmt.Errorf("the runner is still starting — check `brimble ci sandbox list`")
}

func runnerMode(mode string) string {
	if mode == "per_job" {
		return "per-job"
	}
	return mode
}

func runnerStatus(enabled bool) string {
	if enabled {
		return ui.Green("● active")
	}
	return ui.Dim("● paused")
}

func ciJobStatus(status string) string {
	label := "● " + status
	switch status {
	case "completed":
		return ui.Green(label)
	case "failed", "abandoned":
		return ui.Red(label)
	case "queued", "provisioning", "provisioned", "running":
		return ui.Yellow(label)
	}
	return ui.Dim(label)
}

func ciSandboxStatus(status string) string {
	label := "● " + status
	switch status {
	case "online":
		return ui.Green(label)
	case "attaching":
		return ui.Yellow(label)
	}
	return ui.Dim(label)
}
