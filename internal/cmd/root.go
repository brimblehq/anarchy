package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/auth"
	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/config"
	"github.com/brimbleapp/brimble-cli/internal/output"
	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/charmbracelet/huh"
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

// linked is the project linked to the working directory (nil when none);
// linkRoot is the directory holding its .brimble folder.
var linked *config.Link
var linkRoot string

const (
	groupStart  = "start"
	groupDeploy = "deploy"
	groupManage = "manage"
)

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
		Use:   "brimble",
		Short: "Deploy and manage apps on Brimble",
		Long: `Deploy and manage apps on Brimble from your terminal.

Quick start:
  brimble login                 Authenticate with an API key
  brimble link                  Connect this directory to a project
  brimble deploy                Deploy and follow the build
  brimble logs                  Stream live application logs

Inside a linked directory, commands default to the linked project, so
"brimble env list" or "brimble status" need no arguments.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       formatVersion(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if isLocalCommand(cmd) {
				return loadConfigOnly()
			}
			return initClients(cmd.Context())
		},
	}

	root.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "Brimble API key (or BRIMBLE_API_KEY)")
	root.PersistentFlags().StringVar(&flagTeam, "team", "", "Team ID, name or slug (or BRIMBLE_TEAM_ID)")
	root.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", "Core API base URL (or BRIMBLE_CORE_BASE_URL)")
	root.PersistentFlags().StringVar(&flagAuthURL, "auth-url", "", "Auth API base URL (or BRIMBLE_AUTH_BASE_URL)")
	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table|json")
	root.PersistentFlags().DurationVar(&flagTimeout, "timeout", 30*time.Second, "HTTP request timeout")

	root.AddGroup(
		&cobra.Group{ID: groupStart, Title: "Getting started:"},
		&cobra.Group{ID: groupDeploy, Title: "Deploy & observe:"},
		&cobra.Group{ID: groupManage, Title: "Manage resources:"},
	)

	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}

	add(groupStart,
		newLoginCmd(), newLogoutCmd(), newWhoamiCmd(),
		newLinkCmd(), newUnlinkCmd(),
	)
	add(groupDeploy,
		newDeployCmd(), newStatusCmd(), newOpenCmd(), newLogCmd(),
	)
	add(groupManage,
		newProjectCmd(), newEnvCmd(), newDomainCmd(), newEnvironmentCmd(),
		newTeamCmd(), newMetaCmd(), newAuthCmd(),
	)
	root.SetHelpCommandGroupID(groupManage)
	root.SetCompletionCommandGroupID(groupManage)

	return root
}

// isLocalCommand reports commands that work without an API key.
func isLocalCommand(cmd *cobra.Command) bool {
	name := cmd.Name()
	parent := ""
	if cmd.Parent() != nil {
		parent = cmd.Parent().Name()
	}
	switch {
	case parent == "auth" && (name == "login" || name == "logout"):
		return true
	case parent == "brimble" && (name == "login" || name == "logout" || name == "unlink"):
		return true
	case name == "help" || name == "brimble" || name == "completion" || parent == "completion":
		return true
	}
	return false
}

func loadConfigOnly() error {
	if wd, err := os.Getwd(); err == nil {
		link, root, ok, err := config.FindLink(wd)
		if err != nil {
			return err
		}
		if ok {
			linked, linkRoot = &link, root
		}
	}

	linkTeam := ""
	if linked != nil {
		linkTeam = linked.TeamID
	}

	var err error
	cfg, err = config.Load(config.Overrides{
		APIKey:      flagAPIKey,
		TeamID:      flagTeam,
		LinkTeamID:  linkTeam,
		CoreBaseURL: flagBaseURL,
		AuthBaseURL: flagAuthURL,
		Output:      flagOutput,
		Timeout:     flagTimeout,
	})
	return err
}

func initClients(ctx context.Context) error {
	if err := loadConfigOnly(); err != nil {
		return err
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("not logged in — run `brimble login` (or set BRIMBLE_API_KEY)")
	}

	// --team accepts a name or slug; Core needs the ID.
	if cfg.TeamID != "" && !isObjectID(cfg.TeamID) {
		buildClients(auth.Credentials{APIKey: cfg.APIKey})
		teams, err := loadTeams(ctx)
		if err != nil {
			return friendlyError(err)
		}
		t, ok := findTeam(teams, cfg.TeamID)
		if !ok {
			return fmt.Errorf("team %q not found — see `brimble team list`", cfg.TeamID)
		}
		cfg.TeamID, cfg.TeamName = t.ID, t.Name
	}

	buildClients(auth.Credentials{APIKey: cfg.APIKey, TeamID: cfg.TeamID})
	return nil
}

func buildClients(creds auth.Credentials) {
	ua := "brimble-cli/" + version
	coreClient = client.NewCore(cfg.CoreBaseURL, cfg.Timeout, creds, ua)
	authClient = client.NewAuth(cfg.AuthBaseURL, cfg.Timeout, creds, ua)
}

func printResult(data any) error {
	return output.Print(os.Stdout, cfg.Output, data)
}

func jsonOutput() bool {
	return cfg.Output == "json"
}

func run(fn func(ctx context.Context) error) {
	if err := fn(context.Background()); err != nil {
		exitWithError(err)
	}
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	if err := NewRoot().Execute(); err != nil {
		exitWithError(err)
	}
	return 0
}

func exitWithError(err error) {
	switch {
	case errors.Is(err, errAborted):
		os.Exit(0)
	case errors.Is(err, huh.ErrUserAborted):
		fmt.Fprintln(os.Stderr)
		os.Exit(130)
	}
	ui.Fail(friendlyError(err))
}

// friendlyError adds a next step to errors users can act on.
func friendlyError(err error) error {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w\n  Your API key is invalid or expired — run `brimble login` to use a new one", err)
	case http.StatusForbidden:
		return fmt.Errorf("%w\n  Your API key or team role lacks permission for this action", err)
	}
	return err
}

func readJSONFile(path string) (json.RawMessage, error) {
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
	if !json.Valid(b) {
		return nil, fmt.Errorf("parse json: invalid JSON")
	}
	return json.RawMessage(b), nil
}

// confirmOrAbort asks before an action. It returns false when the user
// declines; without a TTY it requires --yes for destructive actions.
func confirmOrAbort(yes bool, prompt string, destructive bool) (bool, error) {
	if yes {
		return true, nil
	}
	if !ui.IsInteractive() {
		if destructive {
			return false, fmt.Errorf("refusing to continue without --yes in a non-interactive shell")
		}
		return true, nil
	}
	ok, err := ui.Confirm(prompt, !destructive)
	if err != nil {
		return false, err
	}
	if !ok {
		ui.Muted("Aborted")
	}
	return ok, nil
}
