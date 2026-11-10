package cmd

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newSSHCmd() *cobra.Command {
	var sandbox bool
	var cwd string

	cmd := &cobra.Command{
		Use:   "ssh [project|sandbox]",
		Short: "Open an interactive terminal on a project or sandbox",
		Long: `Open an interactive terminal session on a project or sandbox.

By default the target is a project; pass --sandbox to target a sandbox.
Without an argument you can pick either from a list. Only web-service and
worker projects support a terminal.

To disconnect, press Ctrl+D, type "exit", or press ~. at the start of a line.`,
		Example: `  brimble ssh my-api
  brimble ssh --sandbox my-sandbox
  brimble ssh --sandbox my-sandbox --cwd /app`,
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if cmd.Flags().Changed("sandbox") {
				return completeSandboxNames(cmd, args, toComplete)
			}
			return completeProjectNames(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				if !ui.IsInteractive() {
					return fmt.Errorf("an interactive terminal is required — run `brimble ssh` from a real terminal")
				}
				target, err := resolveTerminalTarget(ctx, argOr(args, 0), sandbox)
				if err != nil {
					return err
				}
				return startTerminal(ctx, target, cwd)
			})
		},
	}

	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "Target a sandbox instead of a project")
	cmd.Flags().StringVar(&cwd, "cwd", "", "Working directory to start the shell in")
	return cmd
}

type terminalTarget struct {
	resource string // client.ResourceSandbox or client.ResourceProject
	id       string
	label    string
}

func resolveTerminalTarget(ctx context.Context, arg string, sandbox bool) (terminalTarget, error) {
	if sandbox {
		sb, err := pickSandbox(ctx, arg)
		if err != nil {
			return terminalTarget{}, err
		}
		return terminalTarget{resource: client.ResourceSandbox, id: sb.ID, label: sb.Name}, nil
	}
	if arg != "" {
		ref, err := pickProject(ctx, arg)
		if err != nil {
			return terminalTarget{}, err
		}
		return projectTerminalTarget(ctx, ref)
	}
	return pickTerminalTarget(ctx)
}

func projectTerminalTarget(ctx context.Context, ref ProjectRef) (terminalTarget, error) {
	p, err := projectData(ctx, &ref)
	if err != nil {
		return terminalTarget{}, err
	}
	if p.ServiceType != "web-service" && p.ServiceType != "worker" {
		return terminalTarget{}, fmt.Errorf("%s is a %s project — a terminal is only available for web-service and worker projects", ref.Display(), cmp.Or(p.ServiceType, "static"))
	}
	return terminalTarget{resource: client.ResourceProject, id: ref.ID, label: ref.Display()}, nil
}

// pickTerminalTarget offers projects and sandboxes in one list when no
// argument was given.
func pickTerminalTarget(ctx context.Context) (terminalTarget, error) {
	projects, err := loadProjects(ctx)
	if err != nil {
		return terminalTarget{}, err
	}
	sandboxes, err := loadSandboxes(ctx, "")
	if err != nil {
		return terminalTarget{}, err
	}
	if len(projects) == 0 && len(sandboxes) == 0 {
		return terminalTarget{}, fmt.Errorf("nothing to connect to — create a project with `brimble deploy` or a sandbox in the dashboard")
	}

	byKey := map[string]terminalTarget{}
	opts := make([]ui.Option, 0, len(projects)+len(sandboxes))
	for _, p := range projects {
		key := "p:" + p.ID
		label := "project · " + p.Display()
		opts = append(opts, ui.Option{Label: label, Value: key})
		byKey[key] = terminalTarget{resource: client.ResourceProject, id: p.ID, label: p.Display()}
	}
	for _, s := range sandboxes {
		key := "s:" + s.ID
		label := "sandbox · " + s.Name
		if s.Status != "" {
			label += "  " + ui.Status(s.Status)
		}
		opts = append(opts, ui.Option{Label: label, Value: key})
		byKey[key] = terminalTarget{resource: client.ResourceSandbox, id: s.ID, label: s.Name}
	}

	key, err := ui.Select("Open a terminal on", opts)
	if err != nil {
		return terminalTarget{}, err
	}
	chosen := byKey[key]
	if chosen.resource == client.ResourceProject {
		return projectTerminalTarget(ctx, ProjectRef{ID: chosen.id, Name: chosen.label})
	}
	return chosen, nil
}

// loadSandboxes lists sandboxes, optionally narrowed by a search term.
func loadSandboxes(ctx context.Context, search string) ([]client.Sandbox, error) {
	var sandboxes []client.Sandbox
	err := ui.WithSpinner("Loading sandboxes…", func() error {
		var err error
		sandboxes, err = coreClient.ListSandboxes(ctx, search, cfg.TeamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}

// pickSandbox resolves a sandbox by name or ID, else shows a picker.
func pickSandbox(ctx context.Context, arg string) (client.Sandbox, error) {
	if arg != "" {
		sandboxes, err := loadSandboxes(ctx, arg)
		if err != nil {
			return client.Sandbox{}, err
		}
		for _, s := range sandboxes {
			if s.ID == arg || strings.EqualFold(s.Name, arg) {
				return s, nil
			}
		}
		return client.Sandbox{}, fmt.Errorf("sandbox %q not found%s", arg, teamSuffix())
	}

	sandboxes, err := loadSandboxes(ctx, "")
	if err != nil {
		return client.Sandbox{}, err
	}
	if len(sandboxes) == 0 {
		return client.Sandbox{}, fmt.Errorf("no sandboxes%s — create one in the dashboard", teamSuffix())
	}
	opts := make([]ui.Option, 0, len(sandboxes))
	byID := map[string]client.Sandbox{}
	for _, s := range sandboxes {
		label := s.Name
		if s.Status != "" {
			label += "  " + ui.Status(s.Status)
		}
		opts = append(opts, ui.Option{Label: label, Value: s.ID})
		byID[s.ID] = s
	}
	id, err := ui.Select("Select a sandbox", opts)
	if err != nil {
		return client.Sandbox{}, err
	}
	return byID[id], nil
}

func startTerminal(ctx context.Context, target terminalTarget, cwd string) error {
	var sess client.TerminalSession
	err := ui.WithSpinner("Starting a terminal on "+target.label+"…", func() error {
		var err error
		sess, err = coreClient.CreateTerminalSession(ctx, target.resource, target.id, cwd, cfg.TeamID)
		return err
	})
	if err != nil {
		return err
	}

	// Dial the PTY before entering raw mode so connection errors render
	// against a normal terminal.
	var conn *client.TerminalConn
	err = ui.WithSpinner("Connecting…", func() error {
		var err error
		conn, err = coreClient.OpenTerminal(ctx, target.resource, target.id, sess.SessionID, 80, 24)
		return err
	})
	if err != nil {
		_ = coreClient.CloseTerminalSession(context.Background(), target.resource, target.id, sess.SessionID, cfg.TeamID)
		return err
	}
	defer conn.Close()
	defer func() {
		_ = coreClient.CloseTerminalSession(context.Background(), target.resource, target.id, sess.SessionID, cfg.TeamID)
	}()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("enable raw terminal mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	cols, rows, _ := term.GetSize(fd)
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	_ = conn.Resize(cols, rows)

	fmt.Fprintf(os.Stderr, "\r\n%s %s — %s to disconnect\r\n\r\n",
		ui.Green("●"), ui.Code(target.label), ui.Dim("Ctrl+D, `exit`, or ~."))

	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)

	// stdout: PTY -> local terminal
	go func() {
		for {
			msg, rerr := conn.ReadMessage()
			if rerr != nil {
				errCh <- rerr
				return
			}
			if _, werr := os.Stdout.Write(msg); werr != nil {
				errCh <- werr
				return
			}
		}
	}()

	// stdin: local terminal -> PTY, with the ~. escape sequence handled
	go func() {
		st := stdinEscapeState{atLineStart: true}
		buf := make([]byte, 32*1024)
		for {
			n, rerr := os.Stdin.Read(buf)
			out, detach := processStdin(buf[:n], &st)
			if len(out) > 0 {
				if werr := conn.WriteStdin(out); werr != nil {
					errCh <- werr
					return
				}
			}
			if detach {
				errCh <- errDetached
				return
			}
			if rerr != nil {
				errCh <- rerr
				return
			}
		}
	}()

	// resize: poll the terminal size and forward changes
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastCols, lastRows := cols, rows
		for {
			select {
			case <-pctx.Done():
				return
			case <-ticker.C:
				c, r, err := term.GetSize(fd)
				if err == nil && c > 0 && r > 0 && (c != lastCols || r != lastRows) {
					lastCols, lastRows = c, r
					_ = conn.Resize(c, r)
				}
			}
		}
	}()

	err = <-errCh
	fmt.Fprint(os.Stdout, "\r\n")

	if errors.Is(err, errDetached) {
		fmt.Fprintf(os.Stderr, "disconnected from %s\r\n", target.label)
		return nil
	}
	if isCleanExit(err) {
		return nil
	}
	return err
}

// errDetached signals that the user ended the session with the ~. escape.
var errDetached = errors.New("detached")

type stdinEscapeState struct {
	atLineStart bool
	escape      bool
}

// processStdin scans raw terminal input for the ssh-style escape sequence
// (~. to detach, ~~ for a literal tilde) and returns the bytes to forward to
// the shell, plus whether the user requested a detach. A ~ is only treated as
// an escape at the start of a line; otherwise it passes through.
func processStdin(b []byte, st *stdinEscapeState) (out []byte, detach bool) {
	for _, c := range b {
		if st.escape {
			switch c {
			case '.':
				return out, true
			case '~':
				out = append(out, '~')
			default:
				out = append(out, '~', c)
			}
			st.escape = false
			st.atLineStart = c == '\r' || c == '\n'
			continue
		}
		if st.atLineStart && c == '~' {
			st.escape = true
			continue
		}
		out = append(out, c)
		st.atLineStart = c == '\r' || c == '\n'
	}
	return out, false
}

// isCleanExit reports whether a terminal ended because the shell exited
// (server close 1000/1001) or the local terminal went away, rather than an error.
func isCleanExit(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return ce.Code == websocket.CloseNormalClosure || ce.Code == websocket.CloseGoingAway
	}
	return false
}
