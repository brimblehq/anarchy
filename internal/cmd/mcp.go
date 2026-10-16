package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/agents"
	"github.com/brimblehq/anarchy/internal/auth"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

type mcpFlags struct {
	all  bool
	auth string // "", browser or api-key
	yes  bool
	url  string
}

func newMCPCmd() *cobra.Command {
	var f mcpFlags

	cmd := &cobra.Command{
		Use:   "mcp [agent...]",
		Short: "Connect Brimble to your coding agents (Claude Code, Cursor, Codex, …)",
		Long: `Connect Brimble's MCP server to the coding agents on this machine, so you
can ask them to deploy, read logs or manage projects on Brimble.

Detected agents are offered first; you can pick any supported agent.
You choose how agents sign in: through the browser (nothing secret is saved)
or with this CLI's API key (works right away). Pass --auth to skip the question.

Supported: ` + strings.Join(agents.IDs(), ", "),
		Example: `  brimble mcp                     # detect agents and choose
  brimble mcp cursor claude-code   # connect specific agents
  brimble mcp --all --auth api-key # every detected agent, no prompts
  brimble mcp status
  brimble mcp disconnect cursor`,
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error { return runMCPConnect(ctx, args, f) })
		},
	}
	addMCPConnectFlags(cmd, &f)

	connect := &cobra.Command{
		Use:   "connect [agent...]",
		Short: "Connect Brimble to coding agents (same as `brimble mcp`)",
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error { return runMCPConnect(ctx, args, f) })
		},
	}
	addMCPConnectFlags(connect, &f)

	cmd.AddCommand(connect, newMCPStatusCmd(), newMCPDisconnectCmd())
	return cmd
}

func addMCPConnectFlags(cmd *cobra.Command, f *mcpFlags) {
	cmd.Flags().BoolVar(&f.all, "all", false, "Connect every detected agent")
	cmd.Flags().StringVar(&f.auth, "auth", "", "How agents sign in: browser or api-key (asked when omitted)")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Don't ask; use browser sign-in unless --auth api-key")
	cmd.Flags().StringVar(&f.url, "url", agents.DefaultURL, "Brimble MCP server URL")
	_ = cmd.Flags().MarkHidden("url")
}

type agentRow struct {
	agents.Agent
	status agents.Status
	err    error
}

func loadAgentRows(env agents.Env) []agentRow {
	var rows []agentRow
	for _, a := range agents.All() {
		st, err := a.Status(env)
		rows = append(rows, agentRow{Agent: a, status: st, err: err})
	}
	return rows
}

func runMCPConnect(ctx context.Context, args []string, f mcpFlags) error {
	env, err := agents.LocalEnv(runtime.GOOS)
	if err != nil {
		return err
	}
	rows := loadAgentRows(env)

	targets, err := chooseAgents(rows, args, f)
	if err != nil || len(targets) == 0 {
		return err
	}

	useKey, err := chooseSignIn(targets, f)
	if err != nil {
		return err
	}

	opts := agents.Options{URL: f.url}
	if useKey {
		if opts.APIKey, opts.TeamID, err = verifiedAgentKey(ctx); err != nil {
			return err
		}
	} else if cfg.TeamID != "" && isObjectID(cfg.TeamID) {
		opts.TeamID = cfg.TeamID
	}

	checkMCPHealth(ctx, f.url)

	type outcome struct {
		Agent   string `json:"agent"`
		Status  string `json:"status"`
		Path    string `json:"config,omitempty"`
		Next    string `json:"next,omitempty"`
		Message string `json:"message,omitempty"`
	}
	var results []outcome
	var nexts []string
	var codexToLogin bool

	if !jsonOutput() {
		fmt.Fprintln(os.Stderr)
	}
	for _, t := range targets {
		o := outcome{Agent: t.ID}
		if t.IsManual() {
			o.Status, o.Message = "manual", t.Instructions(opts.URL)
			if !jsonOutput() {
				ui.Info("%s — finish in the app:", t.Name)
				for _, line := range strings.Split(o.Message, "\n") {
					ui.Hint("%s", line)
				}
			}
			results = append(results, o)
			continue
		}

		agentOpts := opts
		if !useKey && !t.OAuth {
			key, team, kerr := verifiedAgentKey(ctx)
			if kerr != nil {
				o.Status, o.Message = "skipped", t.Name+" needs an API key — run `brimble login`, then `brimble mcp "+t.ID+" --auth api-key`"
				if !jsonOutput() {
					ui.Warn("%s", o.Message)
				}
				results = append(results, o)
				continue
			}
			agentOpts.APIKey, agentOpts.TeamID = key, team
			if !jsonOutput() {
				ui.Hint("%s doesn't support browser sign-in; using your API key", t.Name)
			}
		}

		res, err := t.Connect(env, agentOpts)
		o.Path = res.Path
		switch {
		case err != nil:
			o.Status, o.Message = "failed", err.Error()
			if !jsonOutput() {
				ui.Error("%s: %s", t.Name, err)
				where := t.ConfigPath(env)
				if where == "" {
					where = "its MCP config"
				}
				ui.Hint("Add this to %s yourself:", tildePath(env, where))
				for _, line := range strings.Split(t.Snippet(agentOpts), "\n") {
					ui.Hint("  %s", line)
				}
			}
		case !res.Changed:
			o.Status = "unchanged"
			if !jsonOutput() {
				ui.Success("%s already connected %s", t.Name, ui.Dim(tildePath(env, res.Path)))
			}
		default:
			o.Status, o.Next = "connected", t.Next(agentOpts)
			if !jsonOutput() {
				ui.Success("%s connected %s", t.Name, ui.Dim(tildePath(env, res.Path)))
			}
			if o.Next != "" {
				nexts = append(nexts, t.Name+": "+o.Next)
			}
			if t.ID == "codex" && agentOpts.APIKey == "" {
				codexToLogin = true
			}
		}
		results = append(results, o)
	}

	if jsonOutput() {
		return printResult(results)
	}

	if codexToLogin {
		if bin, err := exec.LookPath("codex"); err == nil && ui.IsInteractive() {
			ok, err := ui.Confirm("Sign in to Brimble in Codex now? (opens your browser)", true)
			if err != nil {
				return err
			}
			if ok {
				c := exec.Command(bin, "mcp", "login", agents.ServerName)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					ui.Warn("Codex sign-in didn't finish — run `codex mcp login brimble` later")
				} else {
					nexts = removeNext(nexts, "Codex: ")
				}
			}
		}
	}

	if len(nexts) > 0 {
		fmt.Fprintln(os.Stderr)
		ui.Info("Next steps")
		for _, n := range nexts {
			ui.Hint("• %s", n)
		}
	}
	if opts.TeamID != "" && cfg.TeamName != "" {
		ui.Hint("Agents act in the %s workspace. After `brimble team switch`, run `brimble mcp` again.", cfg.TeamName)
	}
	fmt.Fprintln(os.Stderr)
	ui.Hint("Try asking your agent: “Deploy this project to Brimble” or “Why did my last deploy fail?”")
	return nil
}

func removeNext(nexts []string, prefix string) []string {
	var out []string
	for _, n := range nexts {
		if !strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

func chooseAgents(rows []agentRow, args []string, f mcpFlags) ([]agentRow, error) {
	byID := map[string]agentRow{}
	var detected []agentRow
	for _, r := range rows {
		byID[r.ID] = r
		if r.status.Installed {
			detected = append(detected, r)
		}
	}

	if len(args) > 0 {
		var out []agentRow
		for _, arg := range args {
			a, ok := agents.Find(arg)
			if !ok {
				return nil, fmt.Errorf("unknown agent %q — supported: %s", arg, strings.Join(agents.IDs(), ", "))
			}
			out = append(out, byID[a.ID])
		}
		return out, nil
	}
	if f.all {
		if len(detected) == 0 {
			return nil, fmt.Errorf("no coding agents detected — name one, e.g. `brimble mcp cursor`")
		}
		return detected, nil
	}
	if !ui.IsInteractive() {
		names := make([]string, 0, len(detected))
		for _, d := range detected {
			names = append(names, d.ID)
		}
		hint := "none detected"
		if len(names) > 0 {
			hint = "detected: " + strings.Join(names, ", ")
		}
		return nil, fmt.Errorf("pass agent names or --all (%s)", hint)
	}

	ui.Title("Connect Brimble to your coding agents")

	if len(detected) == 1 {
		d := detected[0]
		label := fmt.Sprintf("Found %s. Connect Brimble to it?", d.Name)
		if d.status.Connected {
			label = fmt.Sprintf("Found %s (already connected). Reconnect it?", d.Name)
		}
		ok, err := ui.Confirm(label, true)
		if err != nil {
			return nil, err
		}
		if ok {
			return detected, nil
		}
	} else if len(detected) == 0 {
		ui.Muted("  No coding agents detected — pick the ones you use.")
	}

	var opts []ui.Option
	var preselect []string
	add := func(r agentRow) {
		label := r.Name
		switch {
		case r.status.Connected:
			label += ui.Dim("  connected")
		case r.status.Installed:
			label += ui.Dim("  detected")
		}
		if r.IsManual() {
			label += ui.Dim("  · set up in the app")
		}
		opts = append(opts, ui.Option{Label: label, Value: r.ID})
	}
	for _, r := range detected {
		add(r)
		if !r.status.Connected {
			preselect = append(preselect, r.ID)
		}
	}
	for _, r := range rows {
		if !r.status.Installed {
			add(r)
		}
	}

	ids, err := ui.MultiSelect("Which agents should Brimble connect to?", "space to toggle · enter to confirm", opts, preselect)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		ui.Muted("Nothing selected")
		return nil, nil
	}
	out := make([]agentRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out, nil
}

func chooseSignIn(targets []agentRow, f mcpFlags) (bool, error) {
	switch strings.ToLower(f.auth) {
	case "api-key", "apikey", "key":
		return true, nil
	case "browser", "oauth":
		return false, nil
	case "":
		if f.yes {
			return false, nil
		}
	default:
		return false, fmt.Errorf("--auth must be browser or api-key, got %q", f.auth)
	}
	automatic := false
	for _, t := range targets {
		automatic = automatic || !t.IsManual()
	}
	if !automatic || !ui.IsInteractive() {
		return false, nil
	}
	opts := []ui.Option{
		{Label: "Browser sign-in (recommended) — nothing secret is saved; approve once per agent", Value: "oauth"},
		{Label: "This CLI's API key — works right away; the key is saved in each agent's config", Value: "key"},
	}
	choice, err := ui.Select("How should your agents sign in to Brimble?", opts)
	if err != nil {
		return false, err
	}
	return choice == "key", nil
}

func verifiedAgentKey(ctx context.Context) (string, string, error) {
	if cfg.APIKey == "" {
		return "", "", fmt.Errorf("not logged in — run `brimble login` first, or use browser sign-in (--auth browser)")
	}
	err := ui.WithSpinner("Checking your API key…", func() error {
		_, err := newAuthClient(cfg.APIKey).Me(ctx)
		return err
	})
	if err != nil {
		return "", "", friendlyError(err)
	}
	teamID := cfg.TeamID
	if teamID != "" && !isObjectID(teamID) {
		buildClients(auth.Credentials{APIKey: cfg.APIKey})
		t, err := pickTeam(ctx, teamID)
		if err != nil {
			return "", "", err
		}
		teamID, cfg.TeamName = t.ID, t.Name
	}
	return cfg.APIKey, teamID, nil
}

func checkMCPHealth(ctx context.Context, base string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/health", nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ui.Warn("Couldn't reach %s right now — agents will connect once it's back", base)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		ui.Warn("Couldn't reach %s right now — agents will connect once it's back", base)
	}
}

func newMCPStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"list", "ls"},
		Short:   "Show which coding agents have Brimble connected",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := agents.LocalEnv(runtime.GOOS)
			if err != nil {
				return err
			}
			rows := loadAgentRows(env)
			if jsonOutput() {
				type row struct {
					Agent     string `json:"agent"`
					Name      string `json:"name"`
					Installed bool   `json:"installed"`
					Connected bool   `json:"connected"`
					APIKey    bool   `json:"apiKey"`
					Config    string `json:"config,omitempty"`
				}
				out := make([]row, 0, len(rows))
				for _, r := range rows {
					out = append(out, row{
						Agent: r.ID, Name: r.Name, Installed: r.status.Installed,
						Connected: r.status.Connected, APIKey: r.status.UsesKey, Config: r.status.Path,
					})
				}
				return printResult(out)
			}
			var table [][]string
			for _, r := range rows {
				installed := ui.Dim("not found")
				if r.status.Installed {
					installed = "detected"
				}
				state := ui.Dim("—")
				switch {
				case r.err != nil:
					state = ui.Yellow("unreadable config")
				case r.IsManual():
					state = ui.Dim("set up in the app")
				case r.status.Connected && r.status.UsesKey:
					state = ui.Green("● connected") + ui.Dim(" (API key)")
				case r.status.Connected:
					state = ui.Green("● connected") + ui.Dim(" (browser sign-in)")
				}
				table = append(table, []string{r.Name, installed, state, tildePath(env, r.status.Path)})
			}
			fmt.Fprintln(os.Stdout)
			ui.Table([]string{"AGENT", "ON THIS MACHINE", "BRIMBLE", "CONFIG"}, table)
			fmt.Fprintln(os.Stdout)
			ui.Muted("  Connect with `brimble mcp`, or `brimble mcp <agent>`")
			return nil
		},
	}
}

func newMCPDisconnectCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "disconnect [agent...]",
		Aliases: []string{"remove", "rm"},
		Short:   "Remove Brimble from coding agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := agents.LocalEnv(runtime.GOOS)
			if err != nil {
				return err
			}
			rows := loadAgentRows(env)
			var targets []agentRow
			if len(args) > 0 {
				for _, arg := range args {
					a, ok := agents.Find(arg)
					if !ok {
						return fmt.Errorf("unknown agent %q — supported: %s", arg, strings.Join(agents.IDs(), ", "))
					}
					for _, r := range rows {
						if r.ID == a.ID {
							targets = append(targets, r)
						}
					}
				}
			} else {
				var connected []agentRow
				for _, r := range rows {
					if r.status.Connected {
						connected = append(connected, r)
					}
				}
				if len(connected) == 0 {
					ui.Muted("No agents have Brimble connected")
					return nil
				}
				if !ui.IsInteractive() {
					return fmt.Errorf("pass the agents to disconnect")
				}
				var opts []ui.Option
				for _, r := range connected {
					opts = append(opts, ui.Option{Label: r.Name, Value: r.ID})
				}
				ids, err := ui.MultiSelect("Disconnect Brimble from", "space to toggle · enter to confirm", opts, nil)
				if err != nil {
					return err
				}
				for _, id := range ids {
					for _, r := range connected {
						if r.ID == id {
							targets = append(targets, r)
						}
					}
				}
				if len(targets) == 0 {
					return nil
				}
			}

			names := make([]string, len(targets))
			for i, t := range targets {
				names[i] = t.Name
			}
			ok, err := confirmOrAbort(yes, "Remove Brimble from "+strings.Join(names, ", ")+"?", false)
			if err != nil || !ok {
				return err
			}
			for _, t := range targets {
				res, err := t.Disconnect(env)
				switch {
				case errors.Is(err, agents.ErrManual):
					ui.Info("%s: remove the Brimble connector in the app's settings", t.Name)
				case err != nil:
					ui.Error("%s: %s", t.Name, err)
				case res.Changed:
					ui.Success("Removed Brimble from %s", t.Name)
				default:
					ui.Muted("%s didn't have Brimble connected", t.Name)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func tildePath(env agents.Env, p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, env.Home+string(os.PathSeparator)) {
		return "~" + p[len(env.Home):]
	}
	return p
}
