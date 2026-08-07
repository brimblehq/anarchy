package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "log",
		Aliases: []string{"logs"},
		Short:   "Deployment and request logs",
	}

	cmd.AddCommand(newLogListCmd())
	cmd.AddCommand(newLogSearchCmd())
	cmd.AddCommand(newLogDownloadCmd())
	cmd.AddCommand(newLogRequestsCmd())
	cmd.AddCommand(newLogStreamCmd())
	return cmd
}

func newLogStreamCmd() *cobra.Command {
	var since time.Duration
	var query string
	var limit int

	cmd := &cobra.Command{
		Use:   "stream [project]",
		Short: "Stream live application logs",
		Long: `Tail live application logs (same path as the dashboard).

Connects to Core's /loki Socket.IO namespace, emits "tail", and prints
lines from "logs" events until you hit Ctrl+C.

Requires log.read on the API key. Auth uses your API key as the socket token.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				ref, err := pickProject(ctx, arg)
				if err != nil {
					return err
				}

				startNs := time.Now().Add(-since).UnixNano()
				if since <= 0 {
					startNs = time.Now().Add(-1 * time.Hour).UnixNano()
				}

				pipeline := ""
				if query != "" {
					escaped := strings.ReplaceAll(query, `\`, `\\`)
					escaped = strings.ReplaceAll(escaped, `"`, `\"`)
					pipeline = `|= "` + escaped + `"`
				}

				ui.Info("Streaming logs for %s  (Ctrl+C to stop)", ref.Display())

				streamCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
				defer stop()

				err = client.StreamLogs(streamCtx, client.StreamLogsOpts{
					ProjectID:   ref.APIKey(),
					Query:       pipeline,
					StartNs:     startNs,
					Limit:       limit,
					APIKey:      cfg.APIKey,
					CoreBaseURL: cfg.CoreBaseURL,
					OnLine: func(line client.LogLine) {
						ts := ""
						if line.EpochMs > 0 {
							ts = time.UnixMilli(line.EpochMs).UTC().Format("15:04:05")
							fmt.Fprintf(os.Stdout, "%s  %s\n", ui.Dim(ts), line.Message)
							return
						}
						fmt.Fprintln(os.Stdout, line.Message)
					},
					OnError: func(err error) {
						ui.Warn("%s", err.Error())
					},
				})
				if err != nil && streamCtx.Err() == nil {
					return err
				}
				ui.Muted("Stream stopped")
				return nil
			})
		},
	}

	cmd.Flags().DurationVar(&since, "since", time.Hour, "How far back to start (e.g. 15m, 1h)")
	cmd.Flags().StringVarP(&query, "query", "q", "", "Filter text (LogQL line contains)")
	cmd.Flags().IntVar(&limit, "limit", 150, "Max lines per Loki batch")
	return cmd
}

func newLogListCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "list [project]",
		Short: "List deploy logs",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				ref, err := pickProject(ctx, arg)
				if err != nil {
					return err
				}
				fetch := func(ctx context.Context) error {
					var data any
					var err error
					load := func() error {
						data, err = coreClient.ListDeployLogs(ctx, ref.APIKey(), cfg.TeamID)
						return err
					}
					if watch {
						err = load()
					} else {
						err = ui.WithSpinner("Loading logs…", load)
					}
					if err != nil {
						return err
					}
					if cfg.Output == "json" {
						return printResult(unwrapList(data))
					}
					return printLogList(ref.Display(), unwrapList(data))
				}
				if watch {
					return runWatch(ctx, interval, fetch)
				}
				return fetch(ctx)
			})
		},
	}

	addWatchFlags(cmd, &watch, &interval)
	return cmd
}

func newLogSearchCmd() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "search [project]",
		Short: "Search deploy history",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				ref, err := pickProject(ctx, arg)
				if err != nil {
					return err
				}
				if query == "" && ui.IsInteractive() {
					query, err = ui.Text("Search query", "", "")
					if err != nil {
						return err
					}
				}
				q := url.Values{}
				if query != "" {
					q.Set("q", query)
				}
				var data any
				err = ui.WithSpinner("Searching…", func() error {
					var err error
					data, err = coreClient.SearchDeployLogs(ctx, ref.APIKey(), q, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if cfg.Output == "json" {
					return printResult(unwrapList(data))
				}
				return printLogList(ref.Display(), unwrapList(data))
			})
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Search query")
	return cmd
}

func newLogDownloadCmd() *cobra.Command {
	var outPath string

	cmd := &cobra.Command{
		Use:   "download [project] [logId]",
		Short: "Download a deploy log",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				projectArg := ""
				logID := ""
				if len(args) > 0 {
					projectArg = args[0]
				}
				if len(args) > 1 {
					logID = args[1]
				}
				ref, err := pickProject(ctx, projectArg)
				if err != nil {
					return err
				}
				if logID == "" {
					if !ui.IsInteractive() {
						return fmt.Errorf("deploy log id is required")
					}
					logID, err = ui.Text("Deploy log id", "", "")
					if err != nil {
						return err
					}
					if logID == "" {
						return fmt.Errorf("deploy log id is required")
					}
				}
				var body []byte
				err = ui.WithSpinner("Downloading…", func() error {
					var err error
					body, err = coreClient.DownloadLog(ctx, ref.APIKey(), logID, cfg.TeamID)
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

	cmd.Flags().StringVarP(&outPath, "out", "O", "", "Output file (default stdout)")
	return cmd
}

func newLogRequestsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "requests [project]",
		Short: "Fetch request logs",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				arg := ""
				if len(args) > 0 {
					arg = args[0]
				}
				ref, err := pickProject(ctx, arg)
				if err != nil {
					return err
				}
				var data any
				err = ui.WithSpinner("Loading request logs…", func() error {
					var err error
					data, err = coreClient.RequestLogs(ctx, ref.APIKey(), cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				return printResult(unwrapList(data))
			})
		},
	}
}

func printLogList(project string, data any) error {
	items := asSlice(data)
	ui.Title(project + " · deploys")
	if len(items) == 0 {
		ui.Muted("  (no deploy logs)")
		return nil
	}

	for i, item := range items {
		if i >= 20 {
			ui.Muted("  … and %d more", len(items)-20)
			break
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		status := firstString(m, "status", "state")
		msg := firstString(m, "message", "commitMessage", "commit_message")
		at := firstString(m, "createdAt", "updatedAt", "finishedAt")
		line := status
		if msg != "" {
			if line != "" {
				line += "  ·  "
			}
			line += truncate(msg, 60)
		}
		if at != "" {
			if line != "" {
				line += "  ·  "
			}
			line += formatTime(at)
		}
		if line == "" {
			continue
		}
		ui.Bullet(line)
	}
	return nil
}
