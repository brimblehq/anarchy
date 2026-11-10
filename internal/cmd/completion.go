package cmd

import (
	"context"
	"time"

	"github.com/brimblehq/anarchy/internal/auth"
	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/config"
	"github.com/spf13/cobra"
)

const completionTimeout = 5 * time.Second

// completionClient builds a Core client for shell completion. Completion runs
// without the root command's PersistentPreRunE, so the package-level client and
// config aren't initialized; load them here. Returns nil when there is no API
// key to complete against.
func completionClient() *client.Core {
	cfg, err := config.Load(config.Overrides{})
	if err != nil || cfg.APIKey == "" {
		return nil
	}
	return client.NewCore(cfg.CoreBaseURL, completionTimeout, auth.Credentials{APIKey: cfg.APIKey, TeamID: cfg.TeamID}, "brimble-cli/"+version)
}

// completeNames fetches completion candidates; a missing key or any error
// yields no completions (and disables file fallback).
func completeNames(fetch func(ctx context.Context, c *client.Core) ([]string, error)) ([]string, cobra.ShellCompDirective) {
	c := completionClient()
	if c == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names, err := fetch(context.Background(), c)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeRunnerArg completes the positional runner argument of ci
// subcommands. For commands with a second argument (jobs/logs/detach) it only
// completes the first, leaving the trailing slot empty.
func completeRunnerArg(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeNames(func(ctx context.Context, c *client.Core) ([]string, error) {
		list, err := c.ListCiRunners(ctx, 1, 100, "")
		if err != nil {
			return nil, err
		}
		var names []string
		for _, r := range list.Data {
			if r.Name != "" {
				names = append(names, r.Name)
			}
		}
		return names, nil
	})
}

func completeProjectNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func(ctx context.Context, c *client.Core) ([]string, error) {
		list, err := c.ListProjects(ctx, client.ListProjectsOpts{Limit: 100})
		if err != nil {
			return nil, err
		}
		var names []string
		for _, p := range list.Projects {
			if p.Name != "" {
				names = append(names, p.Name)
			}
		}
		return names, nil
	})
}

func completeSandboxNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func(ctx context.Context, c *client.Core) ([]string, error) {
		sandboxes, err := c.ListSandboxes(ctx, "", "")
		if err != nil {
			return nil, err
		}
		var names []string
		for _, s := range sandboxes {
			if s.Name != "" {
				names = append(names, s.Name)
			}
		}
		return names, nil
	})
}

func completeGitAccountNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func(ctx context.Context, c *client.Core) ([]string, error) {
		accounts, err := c.ListGitAccounts(ctx, "github", "")
		if err != nil {
			return nil, err
		}
		var names []string
		for _, a := range accounts.Accounts {
			switch {
			case a.Username != "":
				names = append(names, a.Username)
			case a.Name != "":
				names = append(names, a.Name)
			}
		}
		return names, nil
	})
}

func completeSandboxRegionNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func(ctx context.Context, c *client.Core) ([]string, error) {
		regions, err := c.ListSandboxRegions(ctx, "")
		if err != nil {
			return nil, err
		}
		var names []string
		for _, r := range regions {
			if r.Name != "" {
				names = append(names, r.Name)
			}
		}
		return names, nil
	})
}

func completeSandboxSizeNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func(ctx context.Context, c *client.Core) ([]string, error) {
		sizes, err := c.ListSandboxSizes(ctx, "", "")
		if err != nil {
			return nil, err
		}
		var names []string
		for _, s := range sizes {
			if s.Name != "" {
				names = append(names, s.Name)
			}
		}
		return names, nil
	})
}

func completeModes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"per_job", "reuse"}, cobra.ShellCompDirectiveNoFileComp
}
