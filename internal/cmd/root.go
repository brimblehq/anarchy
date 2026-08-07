package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/auth"
	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/config"
	"github.com/brimbleapp/brimble-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	flagAPIKey  string
	flagTeam    string
	flagBaseURL string
	flagAuthURL string
	flagOutput  string
	flagTimeout time.Duration
)

var cfg config.Config
var coreClient *client.Core
var authClient *client.Auth

func formatVersion() string {
	if commit == "none" || commit == "" {
		return version
	}
	if date == "unknown" || date == "" {
		return fmt.Sprintf("%s (%s)", version, commit)
	}
	return fmt.Sprintf("%s (%s, %s)", version, commit, date)
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "brimble",
		Short:         "Brimble CLI",
		Long:          "Command-line interface for the Brimble platform. Authenticate with an API key (x-brimble-key).",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       formatVersion(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if isAuthLocalCommand(cmd) {
				return loadConfigOnly()
			}
			return initClients(true)
		},
	}

	root.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "Brimble API key (or BRIMBLE_API_KEY)")
	root.PersistentFlags().StringVar(&flagTeam, "team", "", "Team ID (or BRIMBLE_TEAM_ID)")
	root.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", "Core API base URL (or BRIMBLE_CORE_BASE_URL)")
	root.PersistentFlags().StringVar(&flagAuthURL, "auth-url", "", "Auth API base URL (or BRIMBLE_AUTH_BASE_URL)")
	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table|json")
	root.PersistentFlags().DurationVar(&flagTimeout, "timeout", 30*time.Second, "HTTP request timeout")

	root.AddCommand(newAuthCmd())
	root.AddCommand(newMetaCmd())
	root.AddCommand(newProjectCmd())
	root.AddCommand(newEnvCmd())
	root.AddCommand(newLogCmd())
	root.AddCommand(newTeamCmd())

	return root
}

func isAuthLocalCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	name := cmd.Name()
	parent := ""
	if cmd.Parent() != nil {
		parent = cmd.Parent().Name()
	}
	if parent == "auth" && (name == "login" || name == "logout") {
		return true
	}
	if name == "help" || name == "completion" || name == "brimble" {
		return true
	}
	return false
}

func loadConfigOnly() error {
	var err error
	cfg, err = config.Load(config.Overrides{
		APIKey:      flagAPIKey,
		TeamID:      flagTeam,
		CoreBaseURL: flagBaseURL,
		AuthBaseURL: flagAuthURL,
		Output:      flagOutput,
		Timeout:     flagTimeout,
	})
	return err
}

func initClients(requireKey bool) error {
	if err := loadConfigOnly(); err != nil {
		return err
	}
	if requireKey && cfg.APIKey == "" {
		return fmt.Errorf("not logged in — run: brimble auth login")
	}

	creds := auth.Credentials{APIKey: cfg.APIKey, TeamID: cfg.TeamID}
	ua := "brimble-cli/" + version
	coreClient = client.NewCore(cfg.CoreBaseURL, cfg.Timeout, creds, ua)
	authClient = client.NewAuth(cfg.AuthBaseURL, cfg.Timeout, creds, ua)
	return nil
}

func printResult(data any) error {
	return output.Print(os.Stdout, cfg.Output, data)
}

func run(fn func(ctx context.Context) error) {
	if err := fn(context.Background()); err != nil {
		output.Fail(err)
	}
}

func readJSONFile(path string) (any, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var body any
	if err := jsonUnmarshal(b, &body); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return body, nil
}
