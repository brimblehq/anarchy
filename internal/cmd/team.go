package cmd

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/config"
	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "team",
		Aliases: []string{"teams"},
		Short:   "Switch workspaces and manage team members",
	}

	cmd.AddCommand(newTeamListCmd())
	cmd.AddCommand(newTeamSwitchCmd())
	cmd.AddCommand(newTeamGetCmd())
	cmd.AddCommand(newTeamMembersCmd())
	cmd.AddCommand(newTeamInviteCmd())
	cmd.AddCommand(newTeamRemoveCmd())
	return cmd
}

func newTeamListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your teams",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var teams []client.Team
				err := ui.WithSpinner("Loading teams…", func() error {
					var err error
					teams, err = coreClient.ListTeams(ctx)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(teams)
				}
				return printTeamList(teams)
			})
		},
	}
}

func newTeamSwitchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch [team|personal]",
		Short: "Set the default workspace for commands",
		Long: `Set the default workspace commands act in: a team, or "personal".

A project linked to a directory keeps acting in its own team; --team and
BRIMBLE_TEAM_ID override everything for a single command.`,
		Example: `  brimble team switch acme
  brimble team switch personal`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := argOr(args, 0)
				if arg == "" && ui.IsInteractive() {
					teams, err := loadTeams(ctx)
					if err != nil {
						return err
					}
					opts := []ui.Option{{Label: "Personal workspace", Value: personalWorkspace}}
					for _, t := range teams {
						label := t.Name
						if t.ID == cfg.TeamID {
							label += ui.Dim("  (current)")
						}
						opts = append(opts, ui.Option{Label: label, Value: t.ID})
					}
					if arg, err = ui.Select("Switch to", opts); err != nil {
						return err
					}
				}
				if arg == "" {
					return fmt.Errorf("pass a team name, or \"personal\"")
				}
				if arg == personalWorkspace || strings.EqualFold(arg, "personal") {
					if err := config.SaveTeam(cfg.ConfigPath, "", ""); err != nil {
						return err
					}
					ui.Success("Switched to your personal workspace")
					return nil
				}
				t, err := pickTeam(ctx, arg)
				if err != nil {
					return err
				}
				if err := config.SaveTeam(cfg.ConfigPath, t.ID, t.Name); err != nil {
					return err
				}
				ui.Success("Switched to %s", t.Name)
				return nil
			})
		},
	}
	return cmd
}

func newTeamGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [team]",
		Short: "Show a team",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				t, err := pickTeam(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var team client.Team
				err = ui.WithSpinner("Loading team…", func() error {
					var err error
					team, err = coreClient.GetTeam(ctx, cmp.Or(t.Slug, t.Name))
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(team)
				}
				return printTeamView(team)
			})
		},
	}
}

func newTeamMembersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "members [team]",
		Short: "List a team's members",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				t, err := pickTeam(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				members, err := loadMembers(ctx, t)
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(members)
				}
				printTeamMembers(t.Name, members)
				return nil
			})
		},
	}
}

func newTeamInviteCmd() *cobra.Command {
	var team, role string
	cmd := &cobra.Command{
		Use:     "invite <email>...",
		Short:   "Invite people to a team",
		Example: "  brimble team invite ada@example.com grace@example.com --role MEMBER",
		Args:    cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				role = strings.ToUpper(role)
				switch role {
				case "ADMINISTRATOR", "MEMBER", "VIEWER":
				default:
					return fmt.Errorf("--role must be ADMINISTRATOR, MEMBER or VIEWER")
				}
				t, err := pickTeam(ctx, team)
				if err != nil {
					return err
				}
				err = ui.WithSpinner("Sending invitations…", func() error {
					return coreClient.InviteMembers(ctx, t.ID, args, role)
				})
				if err != nil {
					return err
				}
				ui.Success("Invited %s to %s as %s", strings.Join(args, ", "), t.Name, strings.ToLower(role))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&team, "to", "", "Team to invite to (default: current team)")
	cmd.Flags().StringVar(&role, "role", "MEMBER", "Role: ADMINISTRATOR, MEMBER or VIEWER")
	return cmd
}

func newTeamRemoveCmd() *cobra.Command {
	var team string
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <email>",
		Short: "Remove a member from a team",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				t, err := pickTeam(ctx, team)
				if err != nil {
					return err
				}
				members, err := loadMembers(ctx, t)
				if err != nil {
					return err
				}
				var target *client.TeamMember
				for i := range members {
					if strings.EqualFold(members[i].Email, args[0]) || members[i].Username == args[0] || members[i].ID == args[0] {
						target = &members[i]
					}
				}
				if target == nil {
					return fmt.Errorf("%s is not a member of %s", args[0], t.Name)
				}
				ok, err := confirmOrAbort(yes, fmt.Sprintf("Remove %s from %s?", target.Email, t.Name), true)
				if err != nil || !ok {
					return err
				}
				err = ui.WithSpinner("Removing…", func() error {
					return coreClient.RemoveMember(ctx, t.ID, target.ID)
				})
				if err != nil {
					return err
				}
				ui.Success("Removed %s from %s", target.Email, t.Name)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&team, "from", "", "Team to remove from (default: current team)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func loadMembers(ctx context.Context, t teamRef) ([]client.TeamMember, error) {
	var members []client.TeamMember
	err := ui.WithSpinner("Loading members…", func() error {
		var err error
		members, err = coreClient.TeamMembers(ctx, cmp.Or(t.Slug, t.Name))
		return err
	})
	if err != nil {
		return nil, err
	}
	return members, nil
}

func printTeamList(teams []client.Team) error {
	refs := teamRefs(teams)
	if len(refs) == 0 {
		ui.Muted("You are not a member of any team")
		return nil
	}
	rows := make([][]string, 0, len(refs)+1)
	current := func(active bool) string {
		if active {
			return ui.Green("● current")
		}
		return " "
	}
	rows = append(rows, []string{"Personal workspace", "", current(cfg.TeamID == "")})
	for _, t := range refs {
		rows = append(rows, []string{t.Name, t.Slug, current(t.ID == cfg.TeamID)})
	}
	fmt.Fprintln(os.Stdout)
	ui.Table([]string{"WORKSPACE", "SLUG", ""}, rows)
	fmt.Fprintln(os.Stdout)
	ui.Muted("  Switch with `brimble team switch <slug>`")
	return nil
}

func printTeamView(t client.Team) error {
	name := t.Name
	if name == "" {
		name = "team"
	}
	ui.Title(name)
	printKV("Slug", t.Slug)
	printKV("Description", t.Description)
	if t.TotalMembers != 0 {
		printKV("Members", formatFloat(t.TotalMembers))
	}
	printKV("Created", formatTime(t.CreatedAt))
	fmt.Fprintln(os.Stdout)
	return nil
}

func printTeamMembers(team string, members []client.TeamMember) {
	ui.Title(team + " · members")
	if len(members) == 0 {
		ui.Muted("  (no members)")
		return
	}
	rows := make([][]string, 0, len(members))
	for _, m := range members {
		email := m.Email
		if m.IsMe {
			email += ui.Dim(" (you)")
		}
		state := ui.Green("active")
		if !m.Accepted {
			state = ui.Yellow("invited")
		}
		rows = append(rows, []string{email, m.DisplayName(), strings.ToLower(m.Role), state})
	}
	ui.Table([]string{"EMAIL", "NAME", "ROLE", "STATUS"}, rows)
	fmt.Fprintln(os.Stdout)
}
