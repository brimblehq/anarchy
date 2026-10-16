package cmd

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

const configFileName = "brimble.toml"

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"toml"},
		Short:   "Manage brimble.toml (config as code)",
		Long: `Manage brimble.toml, the file in your repository that describes how a
project builds, runs and scales. Brimble applies it on every push: safe
changes apply on their own, new resources and risky changes wait for your
approval, and blocked changes stop the deploy with a reason.

  brimble config pull       write brimble.toml from the project's current settings
  brimble config validate   check the file before you commit it
  brimble config plan       preview what the file would change
  brimble config approve    approve changes a deploy is waiting on

Reference: https://paper.brimble.io/config-as-code/reference`,
	}
	cmd.AddCommand(newConfigPullCmd(), newConfigValidateCmd(), newConfigPlanCmd(),
		newConfigShowCmd(), newConfigDecideCmd("approve"), newConfigDecideCmd("reject"))
	return cmd
}

func newConfigPullCmd() *cobra.Command {
	var file string
	var yes bool
	cmd := &cobra.Command{
		Use:     "pull [project]",
		Aliases: []string{"export", "generate"},
		Short:   "Write brimble.toml from a project's current settings",
		Long: `Write brimble.toml from a project's current settings. Commit it unchanged
and the next push changes nothing; edit it and push to change the project.

Environment variable values are left out, so nothing secret is written.`,
		Example: `  brimble config pull
  brimble config pull my-api -f apps/api/brimble.toml
  brimble config pull -f -          # print to stdout`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var content string
				err = ui.WithSpinner("Generating brimble.toml…", func() error {
					content, err = coreClient.ExportConfig(ctx, ref.ID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if file == "-" {
					_, err := os.Stdout.WriteString(content)
					return err
				}
				if file == "" {
					file = defaultConfigPath()
				}
				if _, err := os.Stat(file); err == nil {
					ok, err := confirmOrAbort(yes, fmt.Sprintf("Overwrite %s?", file), true)
					if err != nil || !ok {
						return err
					}
				}
				if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
					return err
				}
				ui.Success("Wrote %s from %s's current settings", file, ref.Display())
				// Catch a generated file that wouldn't pass on push.
				if issues, err := coreClient.ValidateConfig(ctx, content, cfg.TeamID); err == nil && len(issues) > 0 {
					ui.Warn("%s", configIssuesError(file, issues))
					ui.Hint("Fix these lines before you commit, or the deploy will fail. Check again with `brimble config validate`.")
					return nil
				}
				ui.Hint("Commit it and push. Edit it to change the project; env var values were left out.")
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Where to write (default: brimble.toml in the linked folder, - for stdout)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Overwrite without asking")
	return cmd
}

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [file]",
		Short: "Check a brimble.toml for errors",
		Long:  "Check a brimble.toml for syntax errors, unknown fields and out-of-range values. Exits non-zero when it's invalid.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				path, content, err := readConfigFile(argOr(args, 0))
				if err != nil {
					return err
				}
				var issues []client.ConfigIssue
				err = ui.WithSpinner("Validating "+path+"…", func() error {
					issues, err = coreClient.ValidateConfig(ctx, content, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(map[string]any{"file": path, "valid": len(issues) == 0, "errors": issues})
				}
				if len(issues) > 0 {
					return configIssuesError(path, issues)
				}
				ui.Success("%s is valid", path)
				return nil
			})
		},
	}
}

func newConfigPlanCmd() *cobra.Command {
	var file string
	var yes bool
	cmd := &cobra.Command{
		Use:     "plan [project]",
		Aliases: []string{"preview", "diff"},
		Short:   "Preview what a brimble.toml would change",
		Long: `Compare a brimble.toml with what's running and show what would change.
Nothing changes unless you approve the plan.

Usually you just commit the file and push; plan is for checking a change
first. If you apply from here, commit the same file so the next push keeps it.`,
		Example: `  brimble config plan
  brimble config plan my-api -f apps/api/brimble.toml`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				path, content, err := readConfigFile(file)
				if err != nil {
					return err
				}
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var plan client.ConfigPlan
				err = ui.WithSpinner("Planning "+path+" against "+ref.Display()+"…", func() error {
					plan, err = coreClient.PlanConfig(ctx, ref.ID, content, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(plan)
				}
				if len(plan.Errors) > 0 {
					return configIssuesError(path, plan.Errors)
				}
				printConfigPlan(ref, plan)
				if !planHasChanges(plan) {
					ui.Success("%s matches %s — nothing to change", path, ref.Display())
					return nil
				}
				if planBlocked(plan) {
					return fmt.Errorf("the plan has blocked changes — fix them in %s first", path)
				}
				ok, err := confirmOrAbort(yes, "Apply these changes now?", false)
				if err != nil {
					return err
				}
				if !ok || (!yes && !ui.IsInteractive()) {
					ui.Hint("Nothing changed. Commit %s and push to apply it with your next deploy.", path)
					return nil
				}
				if err := decideConfigPlan(ctx, ref, plan, "approve"); err != nil {
					return err
				}
				ui.Hint("Commit %s so your next push keeps these settings.", path)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "brimble.toml to plan (default: brimble.toml in the linked folder)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Apply without asking")
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [project]",
		Short: "Show the project's latest brimble.toml plan",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				plan, err := latestConfigPlan(ctx, ref)
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(plan)
				}
				if plan == nil {
					ui.Muted("%s has no brimble.toml plans yet — push a commit with brimble.toml, or run `brimble config plan`", ref.Display())
					return nil
				}
				printConfigPlan(ref, *plan)
				if plan.Status == "awaiting_approval" {
					ui.Hint("Approve with `brimble config approve %s`, or reject with `brimble config reject %s`", ref.Name, ref.Name)
				}
				return nil
			})
		},
	}
}

func newConfigDecideCmd(decision string) *cobra.Command {
	var yes bool
	short := "Approve the brimble.toml changes a project is waiting on"
	if decision == "reject" {
		short = "Reject the brimble.toml changes a project is waiting on"
	}
	cmd := &cobra.Command{
		Use:   decision + " [project]",
		Short: short,
		Long: short + `.

A deploy that needs approval resumes after you approve, and is cancelled
when you reject.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				plan, err := latestConfigPlan(ctx, ref)
				if err != nil {
					return err
				}
				if plan == nil || plan.Status != "awaiting_approval" {
					ui.Muted("%s has no brimble.toml changes waiting for approval", ref.Display())
					return nil
				}
				printConfigPlan(ref, *plan)
				verb := map[string]string{"approve": "Approve and apply these changes?", "reject": "Reject these changes?"}[decision]
				if plan.Log != "" && decision == "reject" {
					verb = "Reject these changes? The waiting deploy will be cancelled."
				}
				ok, err := confirmOrAbort(yes, verb, decision == "reject")
				if err != nil || !ok {
					return err
				}
				if err := decideConfigPlan(ctx, ref, *plan, decision); err != nil {
					return err
				}
				if decision == "approve" && plan.Log != "" {
					return followDeploy(ctx, ref, plan.Log, true, false)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func latestConfigPlan(ctx context.Context, ref ProjectRef) (*client.ConfigPlan, error) {
	var plan *client.ConfigPlan
	err := ui.WithSpinner("Loading the latest plan…", func() error {
		var err error
		plan, err = coreClient.LatestConfigPlan(ctx, ref.ID, cfg.TeamID)
		return err
	})
	return plan, err
}

func decideConfigPlan(ctx context.Context, ref ProjectRef, plan client.ConfigPlan, decision string) error {
	err := ui.WithSpinner(map[string]string{"approve": "Applying…", "reject": "Rejecting…"}[decision], func() error {
		if decision == "approve" {
			return coreClient.ApproveConfigPlan(ctx, plan.ID, ref.ID, cfg.TeamID)
		}
		return coreClient.RejectConfigPlan(ctx, plan.ID, ref.ID, cfg.TeamID)
	})
	if err != nil {
		return err
	}
	if decision == "approve" {
		ui.Success("Approved — applying the changes to %s", ref.Display())
	} else {
		ui.Success("Rejected the changes for %s", ref.Display())
	}
	return nil
}

// defaultConfigPath is brimble.toml in the linked folder, else the current one.
func defaultConfigPath() string {
	if linkRoot != "" {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, filepath.Join(linkRoot, configFileName)); err == nil {
				return rel
			}
		}
	}
	return configFileName
}

// readConfigFile reads path, or brimble.toml in the current or linked folder.
func readConfigFile(path string) (string, string, error) {
	candidates := []string{path}
	if path == "" {
		candidates = []string{configFileName, defaultConfigPath()}
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil {
			return p, string(b), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
	}
	if path != "" {
		return "", "", fmt.Errorf("%s not found", path)
	}
	return "", "", fmt.Errorf("no brimble.toml here — create one with `brimble config pull`, or pass --file")
}

func configIssuesError(path string, issues []client.ConfigIssue) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is invalid:", path)
	for _, is := range issues {
		loc := is.Path
		if is.Line > 0 {
			loc = fmt.Sprintf("line %d", is.Line)
			if is.Path != "" {
				loc += " (" + is.Path + ")"
			}
		}
		fmt.Fprintf(&b, "\n  %s: %s", loc, is.Message)
	}
	return errors.New(b.String())
}

func planHasChanges(plan client.ConfigPlan) bool {
	for _, s := range plan.Plan.Steps {
		if s.Kind != "noop" {
			return true
		}
	}
	return false
}

func planBlocked(plan client.ConfigPlan) bool {
	for _, s := range plan.Plan.Steps {
		if s.Kind == "blocked" {
			return true
		}
	}
	return false
}

// printConfigPlan prints a plan grouped like the dashboard's review screen.
func printConfigPlan(ref ProjectRef, plan client.ConfigPlan) {
	source := cmp.Or(plan.Path, configFileName)
	if plan.Commit != "" && plan.Commit != "dashboard" {
		source += " @ " + shortCommit(plan.Commit)
	}
	ui.Title(ref.Display() + " · brimble.toml plan")
	printKV("Source", source)
	printKV("Status", configPlanStatus(plan.Status))
	if plan.Error != "" {
		printKV("Error", ui.Red(plan.Error))
	}

	groups := []struct{ kinds, title string }{
		{"blocked", ui.Red("Blocked")},
		{"create", ui.Yellow("Needs approval: creates")},
		{"confirm", ui.Yellow("Needs approval: risky")},
		{"safe", ui.Green("Applies automatically")},
	}
	for _, g := range groups {
		var steps []client.ConfigStep
		for _, s := range plan.Plan.Steps {
			if s.Kind == g.kinds {
				steps = append(steps, s)
			}
		}
		if len(steps) == 0 {
			continue
		}
		ui.Section(g.title)
		for _, s := range steps {
			fmt.Fprintf(os.Stdout, "  %s %s\n", ui.Dim(s.Resource), s.Name)
			shown := 0
			for _, c := range s.Changes {
				if c.Kind == "noop" {
					continue
				}
				if shown == 8 {
					ui.Muted("      … and %d more fields", len(s.Changes)-shown)
					break
				}
				shown++
				line := "      " + c.Path
				before, after := planValue(c.Before), planValue(c.After)
				switch {
				case before != "" && after != "":
					line += "  " + ui.Dim(before) + " → " + after
				case after != "":
					line += "  " + after
				case before != "":
					line += "  " + ui.Dim(before+" → (removed)")
				}
				fmt.Fprintln(os.Stdout, line)
				if c.Kind == "blocked" && c.Reason != "" {
					fmt.Fprintf(os.Stdout, "        %s\n", ui.Red(c.Reason))
				}
			}
			if s.Kind == "blocked" && shown == 0 && s.Reason != "" {
				fmt.Fprintf(os.Stdout, "      %s\n", ui.Red(s.Reason))
			}
		}
	}
	if len(plan.Plan.Orphaned) > 0 {
		ui.Section("Not in brimble.toml " + ui.Dim("(left as they are)"))
		for _, o := range plan.Plan.Orphaned {
			fmt.Fprintf(os.Stdout, "  %s %s\n", ui.Dim(o.Resource), o.Name)
		}
	}
	fmt.Fprintln(os.Stdout)
}

func configPlanStatus(s string) string {
	switch s {
	case "awaiting_approval":
		return ui.Yellow("Needs approval")
	case "applying":
		return ui.Yellow("Applying")
	case "applied":
		return ui.Green("Applied")
	case "failed":
		return ui.Red("Failed")
	case "rejected":
		return ui.Dim("Rejected")
	case "planned":
		return "Planned"
	}
	return s
}

// planValue renders a before/after value compactly; "" when absent.
func planValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return truncate(string(raw), 60)
}

func shortCommit(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
