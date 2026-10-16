package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/auth"
	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/config"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

const dashboardAPIKeysURL = "https://app.brimble.io/settings/api-keys"

const personalWorkspace = "__personal__"

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}

	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())
	cmd.AddCommand(newWhoamiCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	var openBrowser bool
	var skipValidate bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with a Brimble API key",
		Long: `Log in with a Brimble API key and choose a default workspace.

Paste your key into the hidden prompt; it is verified and saved to your
config file. If you belong to teams, you pick the workspace commands act in
(change it later with "brimble team switch").

In CI, skip login and set BRIMBLE_API_KEY, or pass --api-key.`,
		Example: `  brimble login
  brimble login --api-key br_xxx --team acme`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey := strings.TrimSpace(flagAPIKey)

			if apiKey == "" {
				if !ui.IsInteractive() {
					return fmt.Errorf("not a terminal; pass --api-key or set BRIMBLE_API_KEY")
				}
				open := openBrowser
				if !open {
					var err error
					open, err = ui.Confirm("Open the API keys page in your browser?", true)
					if err != nil {
						return err
					}
				}
				if open {
					if err := openURL(dashboardAPIKeysURL); err != nil {
						ui.Warn("Could not open a browser — visit %s", dashboardAPIKeysURL)
					} else {
						ui.Info("Opened %s", dashboardAPIKeysURL)
					}
				}
				if err := ui.LoginForm(&apiKey); err != nil {
					return err
				}
				apiKey = strings.TrimSpace(apiKey)
			}
			if apiKey == "" {
				return fmt.Errorf("API key cannot be empty")
			}

			if !skipValidate {
				var me client.User
				err := ui.WithSpinner("Verifying API key…", func() error {
					var err error
					me, err = newAuthClient(apiKey).Me(context.Background())
					return err
				})
				if err != nil {
					return fmt.Errorf("login failed: %w", err)
				}
				if name := me.DisplayName(); name != "" {
					ui.Success("Logged in as %s", name)
				} else {
					ui.Success("API key is valid")
				}
			}

			if err := config.SaveCredentials(cfg.ConfigPath, apiKey, ""); err != nil {
				return err
			}

			workspace, canSwitch := "Personal", false
			if skipValidate {
				// Can't list teams without a verified key; trust --team as given.
				team := strings.TrimSpace(flagTeam)
				if err := config.SaveTeam(cfg.ConfigPath, team, ""); err != nil {
					return err
				}
				if team != "" {
					workspace, canSwitch = team, true
				}
			} else {
				var err error
				workspace, canSwitch, err = chooseLoginTeam(apiKey)
				if err != nil {
					ui.Warn("Could not set a default team (using your personal workspace): %s", err)
				}
			}

			ui.Success("Saved to %s", cfg.ConfigPath)
			ui.Success("Workspace: %s", workspace)
			if canSwitch {
				ui.Hint("Change workspace anytime with `brimble team switch`")
			}
			ui.Hint("Next: `brimble link` in a project folder, or `brimble project list`")
			return nil
		},
	}

	cmd.Flags().BoolVar(&openBrowser, "browser", false, "Open the API keys page without asking")
	cmd.Flags().BoolVar(&skipValidate, "skip-validate", false, "Save the key without verifying it")
	return cmd
}

// chooseLoginTeam saves the default workspace after login: --team when given,
// otherwise a picker when the user belongs to any team, otherwise personal.
// It always overwrites the saved team, which may belong to another account.
// It returns the workspace name and whether the user has teams to switch to.
func chooseLoginTeam(apiKey string) (workspace string, canSwitch bool, err error) {
	workspace = "Personal"
	if err := config.SaveTeam(cfg.ConfigPath, "", ""); err != nil {
		return workspace, false, err
	}
	buildClients(auth.Credentials{APIKey: apiKey})
	ctx := context.Background()

	if strings.TrimSpace(flagTeam) != "" {
		t, err := pickTeam(ctx, strings.TrimSpace(flagTeam))
		if err != nil {
			return workspace, false, err
		}
		return t.Name, true, config.SaveTeam(cfg.ConfigPath, t.ID, t.Name)
	}

	teams, err := loadTeams(ctx)
	if err != nil || len(teams) == 0 {
		return workspace, false, err
	}
	if !ui.IsInteractive() {
		return workspace, true, nil
	}
	opts := []ui.Option{{Label: "Personal workspace", Value: personalWorkspace}}
	for _, t := range teams {
		opts = append(opts, ui.Option{Label: t.Name, Value: t.ID})
	}
	picked, err := ui.Select("Default workspace", opts)
	if err != nil {
		return workspace, true, err
	}
	if picked == personalWorkspace {
		return workspace, true, nil
	}
	t, _ := findTeam(teams, picked)
	return t.Name, true, config.SaveTeam(cfg.ConfigPath, t.ID, t.Name)
}

func newLogoutCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove the saved API key from this machine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := confirmOrAbort(yes, "Remove the saved API key from this machine?", false)
			if err != nil || !ok {
				return err
			}
			if err := config.ClearCredentials(cfg.ConfigPath); err != nil {
				return err
			}
			ui.Success("Logged out")
			if os.Getenv("BRIMBLE_API_KEY") != "" {
				ui.Warn("BRIMBLE_API_KEY is still set in your environment")
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the logged-in user and active workspace",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var user client.User
				err := ui.WithSpinner("Fetching account…", func() error {
					var err error
					user, err = authClient.Me(ctx)
					return err
				})
				if err != nil {
					return err
				}

				workspace := "Personal"
				if cfg.TeamID != "" {
					workspace = cfg.TeamName
					if workspace == "" {
						if t, err := pickTeam(ctx, cfg.TeamID); err == nil {
							workspace = t.Name
						} else {
							workspace = cfg.TeamID
						}
					}
				}
				summary := whoamiView{
					Name:     user.FullName(),
					Email:    user.Email,
					Username: user.Username,
					Company:  user.Company,
					Auth:     "api key",
				}
				if workspace != "Personal" {
					summary.Team = workspace
				}
				if jsonOutput() {
					return printResult(summary)
				}

				ui.Title("Account")
				printKV("Name", summary.Name)
				printKV("Email", summary.Email)
				printKV("Username", summary.Username)
				printKV("Company", summary.Company)
				printKV("Workspace", workspace+teamSourceNote())
				if linked != nil {
					printKV("Linked", linked.ProjectName+ui.Dim("  ("+linkRoot+")"))
				}
				printKV("Auth", "API key"+keySourceNote())
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	}
}

func teamSourceNote() string {
	switch cfg.TeamSource {
	case "flag":
		return ui.Dim("  (from --team)")
	case "env":
		return ui.Dim("  (from BRIMBLE_TEAM_ID)")
	case "link":
		return ui.Dim("  (from linked project)")
	}
	return ""
}

func keySourceNote() string {
	switch {
	case flagAPIKey != "":
		return ui.Dim("  (from --api-key)")
	case os.Getenv("BRIMBLE_API_KEY") != "":
		return ui.Dim("  (from BRIMBLE_API_KEY)")
	}
	return ""
}

type whoamiView struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
	Company  string `json:"company,omitempty"`
	Team     string `json:"team,omitempty"`
	Auth     string `json:"auth"`
}

func newAuthClient(apiKey string) *client.Auth {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return client.NewAuth(cfg.AuthBaseURL, timeout, auth.Credentials{APIKey: apiKey}, "brimble-cli/"+version)
}

func openURL(raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Start()
}
