package cmd

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

type streamFlags struct {
	since time.Duration
	query string
	limit int
}

func addStreamFlags(cmd *cobra.Command, f *streamFlags) {
	cmd.Flags().DurationVar(&f.since, "since", 15*time.Minute, "How far back to start (e.g. 15m, 1h)")
	cmd.Flags().StringVarP(&f.query, "query", "q", "", "Only show lines containing this text")
	cmd.Flags().IntVar(&f.limit, "limit", 150, "Max lines per batch")
}

func newLogCmd() *cobra.Command {
	var f streamFlags

	cmd := &cobra.Command{
		Use:     "logs [project]",
		Aliases: []string{"log"},
		Short:   "Stream live logs, or inspect deployments and requests",
		Long: `Stream live application logs for a project until you press Ctrl+C.

Subcommands show deployment history, build logs and HTTP request logs.`,
		Example: `  brimble logs
  brimble logs my-api -q error --since 1h
  brimble logs list
  brimble logs download`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				return streamLogs(ctx, argOr(args, 0), f)
			})
		},
	}
	addStreamFlags(cmd, &f)

	cmd.AddCommand(newLogBuildCmd())
	cmd.AddCommand(newLogStreamCmd())
	cmd.AddCommand(newLogListCmd())
	cmd.AddCommand(newLogSearchCmd())
	cmd.AddCommand(newLogDownloadCmd())
	cmd.AddCommand(newLogRequestsCmd())
	return cmd
}

func newLogBuildCmd() *cobra.Command {
	var noLogs bool
	cmd := &cobra.Command{
		Use:   "build [project] [deploymentId]",
		Short: "Stream a deployment's build logs until it finishes",
		Long: `Stream a deployment's build logs live, until the deployment finishes.

Without a deployment ID it follows the latest deployment when it's still
running (for example one started by a git push); otherwise you pick one.
Finished deployments print their full log and result.`,
		Example: `  brimble logs build
  brimble logs build my-api 6a81bed11470bfcd2a5dbcfa`,
		Args: cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				logID := argOr(args, 1)
				if logID != "" {
					d, err := findDeployment(ctx, ref, logID)
					if err != nil {
						return err
					}
					logID = d.ID
				} else {
					latest, err := coreClient.ListDeployLogs(ctx, ref.ID, url.Values{"limit": {"1"}}, cfg.TeamID)
					if err != nil {
						return err
					}
					if d := deployments(latest); len(d) > 0 && !deployFinished(d[0].Status) {
						logID = d[0].ID
					} else {
						d, err := pickDeployment(ctx, ref, "Show the build log of", nil)
						if err != nil {
							return err
						}
						logID = d.ID
					}
				}
				return followDeploy(ctx, ref, logID, !noLogs, false)
			})
		},
	}
	cmd.Flags().BoolVar(&noLogs, "no-logs", false, "Wait for the result without printing build output")
	return cmd
}

func newLogStreamCmd() *cobra.Command {
	var f streamFlags
	cmd := &cobra.Command{
		Use:   "stream [project]",
		Short: "Stream live application logs (same as `brimble logs`)",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				return streamLogs(ctx, argOr(args, 0), f)
			})
		},
	}
	addStreamFlags(cmd, &f)
	return cmd
}

func streamLogs(ctx context.Context, arg string, f streamFlags) error {
	ref, err := pickProject(ctx, arg)
	if err != nil {
		return err
	}

	since := f.since
	if since <= 0 {
		since = 15 * time.Minute
	}
	pipeline := ""
	if f.query != "" {
		escaped := strings.ReplaceAll(f.query, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		pipeline = `|= "` + escaped + `"`
	}

	ui.Info("Connecting to %s's logs…", ref.Display())

	streamCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = client.StreamLogs(streamCtx, client.StreamLogsOpts{
		ProjectID:   ref.ID,
		Query:       pipeline,
		StartNs:     time.Now().Add(-since).UnixNano(),
		Limit:       f.limit,
		APIKey:      cfg.APIKey,
		CoreBaseURL: cfg.CoreBaseURL,
		OnLine: func(line client.LogLine) {
			if jsonOutput() {
				_ = printResult(streamLine{
					Timestamp: time.UnixMilli(line.EpochMs).UTC().Format(time.RFC3339Nano),
					Message:   line.Message,
					Labels:    line.Labels,
				})
				return
			}
			if line.EpochMs > 0 {
				ts := time.UnixMilli(line.EpochMs).Local().Format("15:04:05")
				fmt.Fprintf(os.Stdout, "%s  %s\n", ui.Dim(ts), line.Message)
				return
			}
			fmt.Fprintln(os.Stdout, line.Message)
		},
		OnError: func(err error) {
			ui.Warn("%s", err.Error())
		},
		OnReady: func() {
			ui.Hint("Live — showing the last %s, then new lines as they arrive (Ctrl+C to stop)", formatSince(since))
		},
		Debug: debugf,
	})
	if err != nil && streamCtx.Err() == nil {
		return err
	}
	ui.Muted("Stream stopped")
	return nil
}

type deployListFlags struct {
	limit, page int
	status      string
	environment string
}

func (f deployListFlags) query() url.Values {
	q := url.Values{}
	if f.limit > 0 {
		q.Set("limit", strconv.Itoa(f.limit))
	}
	if f.page > 0 {
		q.Set("page", strconv.Itoa(f.page))
	}
	if f.status != "" {
		q.Set("statuses", strings.ToUpper(f.status))
	}
	if f.environment != "" {
		q.Set("environment", strings.ToUpper(f.environment))
	}
	return q
}

func addDeployListFlags(cmd *cobra.Command, f *deployListFlags) {
	cmd.Flags().IntVar(&f.limit, "limit", 15, "Deployments per page")
	cmd.Flags().IntVar(&f.page, "page", 0, "Page number")
	cmd.Flags().StringVar(&f.status, "status", "", "Filter by status, e.g. FAILED or ACTIVE")
	cmd.Flags().StringVarP(&f.environment, "environment", "e", "", "Filter by production or preview")
}

func newLogListCmd() *cobra.Command {
	var f deployListFlags
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:     "list [project]",
		Aliases: []string{"ls", "deployments"},
		Short:   "List deployments",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				fetch := func(ctx context.Context) error {
					var list client.DeployLogList
					var err error
					load := func() error {
						list, err = coreClient.ListDeployLogs(ctx, ref.ID, f.query(), cfg.TeamID)
						return err
					}
					if watch {
						err = load()
					} else {
						err = ui.WithSpinner("Loading deployments…", load)
					}
					if err != nil {
						return err
					}
					if jsonOutput() {
						return printResult(list.Logs)
					}
					return printDeployList(ref.Display(), list)
				}
				if watch {
					return runWatch(ctx, interval, fetch)
				}
				return fetch(ctx)
			})
		},
	}

	addDeployListFlags(cmd, &f)
	addWatchFlags(cmd, &watch, &interval)
	return cmd
}

func newLogSearchCmd() *cobra.Command {
	var query string
	var f deployListFlags

	cmd := &cobra.Command{
		Use:   "search [project]",
		Short: "Search deployments by commit message, branch or author",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				if query == "" && ui.IsInteractive() {
					query, err = ui.Text("Search for", "fix login", "")
					if err != nil {
						return err
					}
				}
				q := f.query()
				if query != "" {
					q.Set("search", query)
				}
				var list client.DeployLogList
				err = ui.WithSpinner("Searching…", func() error {
					var err error
					list, err = coreClient.SearchDeployLogs(ctx, ref.ID, q, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(list.Logs)
				}
				return printDeployList(ref.Display(), list)
			})
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Search text")
	addDeployListFlags(cmd, &f)
	return cmd
}

func newLogDownloadCmd() *cobra.Command {
	var outPath string

	cmd := &cobra.Command{
		Use:   "download [project] [deploymentId]",
		Short: "Print or save the build log of a deployment",
		Long: `Print or save the build log of a deployment.

Without a deployment ID you pick one from recent deployments (or, without a
terminal, the latest one is used).`,
		Args: cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				logID := argOr(args, 1)
				if logID != "" {
					d, err := findDeployment(ctx, ref, logID)
					if err != nil {
						return err
					}
					logID = d.ID
				} else {
					d, err := pickDeployment(ctx, ref, "Select a deployment", nil)
					if err != nil {
						return err
					}
					logID = d.ID
				}
				var body []byte
				err = ui.WithSpinner("Downloading build log…", func() error {
					var err error
					body, err = coreClient.DownloadLog(ctx, ref.ID, logID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if outPath == "" || outPath == "-" {
					_, err = os.Stdout.Write(body)
					return err
				}
				if err := os.WriteFile(outPath, body, 0o644); err != nil {
					return err
				}
				ui.Success("Wrote %s", outPath)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&outPath, "out", "O", "", "Write to a file instead of stdout")
	return cmd
}

func newLogRequestsCmd() *cobra.Command {
	var limit int
	var status, method, search string

	cmd := &cobra.Command{
		Use:   "requests [project]",
		Short: "Show recent HTTP requests",
		Example: `  brimble logs requests
  brimble logs requests --status 500 --method POST`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickProject(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				q := url.Values{}
				q.Set("limit", strconv.Itoa(limit))
				if status != "" {
					q.Set("statuses", status)
				}
				if method != "" {
					q.Set("methods", strings.ToUpper(method))
				}
				if search != "" {
					q.Set("search", search)
				}
				var list client.RequestLogList
				err = ui.WithSpinner("Loading requests…", func() error {
					var err error
					list, err = coreClient.RequestLogs(ctx, ref.ID, q, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(list.Logs)
				}
				printRequestLogs(ref.Display(), list.Logs)
				return nil
			})
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Number of requests")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status code, e.g. 500 or 404,500")
	cmd.Flags().StringVar(&method, "method", "", "Filter by HTTP method, e.g. POST")
	cmd.Flags().StringVarP(&search, "query", "q", "", "Filter by text in the URL or message")
	return cmd
}

type streamLine struct {
	Timestamp string            `json:"timestamp"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
}

func deployments(list client.DeployLogList) []client.DeployLog {
	out := make([]client.DeployLog, 0, len(list.Logs))
	for _, d := range list.Logs {
		d.Message = firstLine(d.Message)
		out = append(out, d)
	}
	return out
}

var fullObjectID = regexp.MustCompile(`^[0-9a-f]{24}$`)

// findDeployment resolves a deployment ID, accepting the short IDs that
// `brimble logs list` prints.
func findDeployment(ctx context.Context, ref ProjectRef, id string) (client.DeployLog, error) {
	if fullObjectID.MatchString(id) {
		return client.DeployLog{ID: id}, nil
	}
	raw, err := coreClient.ListDeployLogs(ctx, ref.ID, url.Values{"limit": {"50"}}, cfg.TeamID)
	if err != nil {
		return client.DeployLog{}, err
	}
	var matches []client.DeployLog
	for _, d := range deployments(raw) {
		if strings.HasSuffix(d.ID, id) || strings.HasPrefix(d.ID, id) {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return client.DeployLog{}, fmt.Errorf("no deployment %q among the last 50 of %s — see `brimble logs list %s`", id, ref.Display(), ref.Name)
	}
	return client.DeployLog{}, fmt.Errorf("%q matches more than one deployment of %s — use more of the ID", id, ref.Display())
}

func pickDeployment(ctx context.Context, ref ProjectRef, title string, keep func(client.DeployLog) bool) (client.DeployLog, error) {
	var raw client.DeployLogList
	err := ui.WithSpinner("Loading deployments…", func() error {
		var err error
		raw, err = coreClient.ListDeployLogs(ctx, ref.ID, url.Values{"limit": {"25"}}, cfg.TeamID)
		return err
	})
	if err != nil {
		return client.DeployLog{}, err
	}
	var list []client.DeployLog
	for _, d := range deployments(raw) {
		if keep == nil || keep(d) {
			list = append(list, d)
		}
	}
	if len(list) == 0 {
		return client.DeployLog{}, fmt.Errorf("no matching deployments for %s", ref.Display())
	}
	if !ui.IsInteractive() || len(list) == 1 {
		return list[0], nil
	}
	opts := make([]ui.Option, 0, len(list))
	byID := map[string]client.DeployLog{}
	for _, d := range list {
		label := fmt.Sprintf("%s  %s  %s  %s", shortID(d.ID), ui.Status(d.Status), truncate(cmp.Or(d.Message, d.Branch, "—"), 50), ui.Dim(formatAgo(d.CreatedAt)))
		opts = append(opts, ui.Option{Label: label, Value: d.ID})
		byID[d.ID] = d
	}
	id, err := ui.Select(title, opts)
	if err != nil {
		return client.DeployLog{}, err
	}
	return byID[id], nil
}

func printDeployList(project string, data client.DeployLogList) error {
	list := deployments(data)
	ui.Title(project + " · deployments")
	if len(list) == 0 {
		ui.Muted("  No deployments yet — run `brimble deploy`")
		return nil
	}
	rows := make([][]string, 0, len(list))
	for _, d := range list {
		msg := d.Message
		if d.Source == "rollback" {
			msg = "↩ rollback · " + msg
		}
		rows = append(rows, []string{
			shortID(d.ID), ui.Status(d.Status), truncate(msg, 48), d.Branch, strings.ToLower(d.Environment), formatAgo(d.CreatedAt),
		})
	}
	ui.Table([]string{"ID", "STATUS", "COMMIT", "BRANCH", "ENV", "CREATED"}, rows)

	if data.TotalPages > 1 {
		fmt.Fprintln(os.Stdout)
		ui.Muted("  Page %d of %d · --page to see more", max(1, data.CurrentPage), data.TotalPages)
	}
	fmt.Fprintln(os.Stdout)
	ui.Muted("  Commands that take a deployment ID accept these short IDs")
	return nil
}

func printRequestLogs(project string, items []client.RequestLog) {
	ui.Title(project + " · requests")
	if len(items) == 0 {
		ui.Muted("  No requests in this window")
		return
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		status := strconv.Itoa(item.Status)
		switch {
		case item.Status >= 500:
			status = ui.Red(status)
		case item.Status >= 400:
			status = ui.Yellow(status)
		}
		path := item.URL
		if u, err := url.Parse(path); err == nil && u.Path != "" {
			path = u.RequestURI()
		}
		rows = append(rows, []string{formatClock(item.Timestamp), item.Method, status, truncate(path, 70)})
	}
	ui.Table([]string{"TIME", "METHOD", "STATUS", "PATH"}, rows)
	fmt.Fprintln(os.Stdout)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// formatSince renders a --since window, e.g. 15m, 2h or 1h30m.
func formatSince(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	d = d.Round(time.Minute)
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
