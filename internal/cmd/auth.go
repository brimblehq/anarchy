package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/auth"
	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/config"
	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

const dashboardAPIKeysURL = "https://app.brimble.io/settings/api-keys"

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication",
	}

	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthWhoamiCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var apiKey string
	var teamID string
	var openBrowser bool
	var skipValidate bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with a Brimble API key",
		Long: `Log in interactively with a Brimble API key.

Opens a form to paste your key, validates it against Auth (GET /user/me),
and saves credentials to your config file.

In CI or non-interactive shells, pass --api-key (or BRIMBLE_API_KEY).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := loadConfigOnly(); err != nil {
				return err
			}

			if apiKey == "" {
				apiKey = flagAPIKey
			}
			if teamID == "" {
				teamID = flagTeam
			}

			if openBrowser {
				_ = openURL(dashboardAPIKeysURL)
			}

			if apiKey == "" {
				if !ui.IsInteractive() {
					return fmt.Errorf("not a terminal; pass --api-key or set BRIMBLE_API_KEY")
				}

				if !openBrowser {
					open, err := ui.Confirm("Open the API keys page in your browser?", true)
					if err != nil {
						return err
					}
					if open {
						if err := openURL(dashboardAPIKeysURL); err != nil {
							ui.Warn("Could not open browser — visit %s", dashboardAPIKeysURL)
						} else {
							ui.Info("Opened %s", dashboardAPIKeysURL)
						}
					}
				}

				if err := ui.LoginForm(&apiKey, &teamID); err != nil {
					return err
				}
			}

			apiKey = strings.TrimSpace(apiKey)
			teamID = strings.TrimSpace(teamID)
			if apiKey == "" {
				return fmt.Errorf("API key cannot be empty")
			}

			if !skipValidate {
				var me any
				err := ui.WithSpinner("Verifying credentials…", func() error {
					var err error
					me, err = validateAPIKey(apiKey)
					return err
				})
				if err != nil {
					return fmt.Errorf("login failed: %w", err)
				}
				if name := userDisplayName(me); name != "" {
					ui.Success("Logged in as %s", name)
				} else {
					ui.Success("API key is valid")
				}
			}

			if err := config.SaveCredentials(cfg.ConfigPath, apiKey, teamID); err != nil {
				return err
			}
			ui.Success("Credentials saved to %s", cfg.ConfigPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (skips interactive prompt)")
	cmd.Flags().StringVar(&teamID, "team", "", "Default team ID")
	cmd.Flags().BoolVar(&openBrowser, "browser", false, "Open the API keys page in the browser")
	cmd.Flags().BoolVar(&skipValidate, "skip-validate", false, "Save key without calling Auth")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out and remove saved API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := loadConfigOnly(); err != nil {
				return err
			}

			if !yes && ui.IsInteractive() {
				ok, err := ui.Confirm("Remove saved API key from this machine?", true)
				if err != nil {
					return err
				}
				if !ok {
					ui.Muted("Aborted")
					return nil
				}
			}

			if err := config.ClearCredentials(cfg.ConfigPath); err != nil {
				return err
			}
			ui.Success("Logged out")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newAuthWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				var data any
				err := ui.WithSpinner("Fetching account…", func() error {
					var err error
					data, err = authClient.Me(ctx)
					return err
				})
				if err != nil {
					return err
				}

				summary := whoamiSummary(data, cfg.TeamID)
				if cfg.Output == "json" {
					return printResult(summary)
				}

				ui.Title("Account")
				order := []string{"name", "email", "username", "company", "team", "auth"}
				for _, key := range order {
					v, ok := summary[key]
					if !ok {
						continue
					}
					s, _ := v.(string)
					if s == "" {
						continue
					}
					ui.KeyValue(key, s)
				}
				fmt.Fprintln(os.Stdout)
				return nil
			})
		},
	}
}

func validateAPIKey(apiKey string) (any, error) {
	creds := auth.Credentials{APIKey: apiKey}
	ua := "brimble-cli/" + version
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	a := client.NewAuth(cfg.AuthBaseURL, timeout, creds, ua)
	return a.Me(context.Background())
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
