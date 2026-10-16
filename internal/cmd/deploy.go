package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/config"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	var dir string
	var restart, detach, noLogs bool

	cmd := &cobra.Command{
		Use:   "deploy [project]",
		Short: "Deploy a project and follow the build",
		Long: `Deploy a project and follow the build until it is live.

Git and Docker projects rebuild the latest commit of their production branch
(or image tag). Static upload projects deploy the files in the current
directory — or the folder given with --dir.

Run it in a directory without a linked project to link an existing project
or create a new one. Exits non-zero when the deployment fails, so it is safe
to use in CI. Ctrl+C stops following; the deployment keeps running.`,
		Example: `  brimble deploy
  brimble deploy my-api --detach
  brimble deploy my-site --dir ./dist
  brimble deploy --restart`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, created, err := resolveDeployTarget(ctx, argOr(args, 0))
				if err != nil || created {
					return err
				}
				project, err := projectData(ctx, &ref)
				if err != nil {
					return err
				}

				var logID string
				if dir != "" || project.Repo == nil {
					if restart {
						return fmt.Errorf("--restart is not supported for static upload projects")
					}
					if dir == "" {
						dir = "."
					}
					if project.Repo != nil {
						return fmt.Errorf("%s deploys from %s — --dir is only for static upload projects", ref.Display(), project.Repo.Label())
					}
					logID, err = uploadDrop(ctx, client.DropOpts{ProjectID: ref.ID, TeamID: cfg.TeamID}, dir, ref.Display())
				} else {
					var body json.RawMessage
					if restart {
						body, err = json.Marshal(client.RedeployRequest{StartOnly: true})
						if err != nil {
							return err
						}
					}
					logID, err = redeploy(ctx, ref, body, restart)
				}
				if err != nil {
					return err
				}
				return afterDeployStarted(ctx, ref, logID, detach, !noLogs)
			})
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Folder to upload (static upload projects; default: current directory)")
	cmd.Flags().BoolVar(&restart, "restart", false, "Restart the running deployment without rebuilding")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Start the deployment and exit without following it")
	cmd.Flags().BoolVar(&noLogs, "no-logs", false, "Follow the deployment without printing build output")
	return cmd
}

// resolveDeployTarget finds the project to deploy. In an unlinked directory it
// offers to link an existing project or create one (created=true means the
// create flow already deployed it).
func resolveDeployTarget(ctx context.Context, arg string) (ref ProjectRef, created bool, err error) {
	if arg != "" || linked != nil || !ui.IsInteractive() {
		ref, err = pickProject(ctx, arg)
		return ref, false, err
	}

	ui.Info("This directory is not linked to a Brimble project")
	choice, err := ui.Select("What do you want to deploy?", []ui.Option{
		{Label: "An existing project", Value: "existing"},
		{Label: "A new project", Value: "new"},
	})
	if err != nil {
		return ProjectRef{}, false, err
	}
	if choice == "new" {
		return ProjectRef{}, true, runCreate(ctx, nil, createOptions{offerLink: true})
	}

	projects, err := loadProjects(ctx)
	if err != nil {
		return ProjectRef{}, false, err
	}
	if len(projects) == 0 {
		return ProjectRef{}, false, fmt.Errorf("no projects yet — choose \"A new project\" instead")
	}
	ref, err = selectProject("Select a project", projects)
	if err != nil {
		return ProjectRef{}, false, err
	}
	offerLink(ref)
	return ref, false, nil
}

// offerLink asks to link the working directory to ref (interactive only).
func offerLink(ref ProjectRef) {
	if linked != nil || !ui.IsInteractive() {
		return
	}
	ok, err := ui.Confirm(fmt.Sprintf("Link this directory to %s?", ref.Display()), true)
	if err != nil || !ok {
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		ui.Warn("Could not link: %s", err)
		return
	}
	if err := writeLink(wd, ref); err != nil {
		ui.Warn("Could not link: %s", err)
	}
}

func writeLink(dir string, ref ProjectRef) error {
	link := config.Link{ProjectID: ref.ID, ProjectName: ref.Name, TeamID: cfg.TeamID}
	path, err := config.SaveLink(dir, link)
	if err != nil {
		return err
	}
	linked, linkRoot = &link, dir
	ui.Success("Linked %s to %s", ui.Dim(dir), ref.Display())
	ui.Hint("Saved %s", path)
	return nil
}

func redeploy(ctx context.Context, ref ProjectRef, body json.RawMessage, restart bool) (string, error) {
	msg := "Starting deployment of " + ref.Display() + "…"
	if restart {
		msg = "Restarting " + ref.Display() + "…"
	}
	var result client.DeployResult
	err := ui.WithSpinner(msg, func() error {
		var err error
		result, err = coreClient.RedeployProject(ctx, ref.ID, body, cfg.TeamID)
		return err
	})
	if err != nil {
		return "", err
	}
	return result.LogID, nil
}

type deployStarted struct {
	ProjectID string `json:"projectId"`
	Project   string `json:"project"`
	LogID     string `json:"logId"`
}

func afterDeployStarted(ctx context.Context, ref ProjectRef, logID string, detach, showLogs bool) error {
	if jsonOutput() && detach {
		return printResult(deployStarted{ProjectID: ref.ID, Project: ref.Name, LogID: logID})
	}
	if logID == "" {
		ui.Success("Deployment requested for %s", ref.Display())
		ui.Hint("Track it with `brimble logs list %s`", ref.Name)
		return nil
	}
	if detach {
		ui.Success("Deployment started for %s", ref.Display())
		ui.Hint("Follow it with `brimble logs list %s --watch`", ref.Name)
		return nil
	}
	return followDeploy(ctx, ref, logID, showLogs, true)
}

// Deployment status semantics (PROJECT_STATUS in Core).
func deploySucceeded(status string) bool { return status == "ACTIVE" || status == "NOMAD_READY" }

func deployFailed(status string) bool {
	switch status {
	case "FAILED", "NOMAD_FAILED", "CANCELLED", "DEGRADED", "PAYMENT DUE":
		return true
	}
	return false
}

func deployFinished(status string) bool {
	return deploySucceeded(status) || deployFailed(status) ||
		status == "SUPERSEDED" || status == "INACTIVE" || status == "AWAITING_CONFIG_APPROVAL"
}

const followInterval = 2 * time.Second

// startedHere is false when attaching to a deploy started elsewhere (no elapsed time).
func followDeploy(ctx context.Context, ref ProjectRef, logID string, showLogs, startedHere bool) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	started := time.Now()
	ui.Info("Following deployment %s of %s  %s", shortID(logID), ref.Display(), ui.Dim("(Ctrl+C to stop following)"))

	tail := &lineTail{n: 20}
	seen := map[string]bool{} // lines already printed, across reconnects
	onLine := func(l client.BuildLogLine) {
		if l.ID != "" {
			if seen[l.ID] {
				return
			}
			seen[l.ID] = true
		}
		tail.add(l.Content)
		if showLogs {
			fmt.Fprintf(os.Stdout, "%s %s\n", ui.Dim("│"), l.Content)
		}
	}

	for {
		status, awaiting, err := streamUntilApproval(ctx, ref, logID, showLogs, onLine)
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr)
			ui.Info("Stopped following — the deployment continues on Brimble")
			ui.Hint("Follow it again with `brimble logs build %s %s`", ref.Name, logID)
			return nil
		}
		if awaiting {
			resume, err := handleConfigApproval(ctx, ref)
			if err != nil || !resume {
				return err
			}
			continue
		}
		if client.IsStreamUnavailable(err) {
			ui.Warn("Live build logs are unavailable right now — checking the deployment status instead")
			status, err = pollDeploy(ctx, ref, logID, showLogs)
			tail = nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if status == "AWAITING_CONFIG_APPROVAL" {
			resume, err := handleConfigApproval(ctx, ref)
			if err != nil || !resume {
				return err
			}
			continue
		}
		var elapsed time.Duration
		if startedHere {
			elapsed = time.Since(started)
		}
		return reportDeploy(ctx, ref, logID, status, elapsed, showLogs, tail)
	}
}

const approvalCheckInterval = 5 * time.Second

// streamUntilApproval streams build logs until the deploy finishes, or stops
// early (awaiting=true) when it pauses for brimble.toml approval — the server
// keeps the stream open while it waits, so we watch the status alongside.
func streamUntilApproval(ctx context.Context, ref ProjectRef, logID string, showLogs bool, onLine func(client.BuildLogLine)) (status string, awaiting bool, err error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var paused atomic.Bool
	go func() {
		t := time.NewTicker(approvalCheckInterval)
		defer t.Stop()
		for {
			select {
			case <-streamCtx.Done():
				return
			case <-t.C:
				if st, err := deploymentStatus(streamCtx, ref.ID, logID); err == nil && st == "AWAITING_CONFIG_APPROVAL" {
					paused.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	run := func() error {
		var serr error
		status, serr = coreClient.StreamBuildLogs(streamCtx, ref.ID, logID, cfg.TeamID, onLine)
		return serr
	}
	if showLogs {
		err = run()
	} else {
		err = ui.WithSpinner("Building "+ref.Display()+"…", run)
	}
	if paused.Load() && ctx.Err() == nil {
		return "", true, nil
	}
	return status, false, err
}

// handleConfigApproval shows the brimble.toml plan a deploy is waiting on and
// lets the user decide. resume reports whether to keep following the deploy.
func handleConfigApproval(ctx context.Context, ref ProjectRef) (resume bool, err error) {
	fmt.Fprintln(os.Stderr)
	ui.Warn("This deploy is waiting for approval of brimble.toml changes")
	plan, err := latestConfigPlan(ctx, ref)
	if err != nil {
		return false, err
	}
	if plan == nil || plan.Status != "awaiting_approval" {
		ui.Hint("Review it in the dashboard under Configuration → Config as code")
		return false, nil
	}
	printConfigPlan(ref, *plan)
	if !ui.IsInteractive() {
		ui.Hint("Approve with `brimble config approve %s` (the deploy then resumes), or reject with `brimble config reject %s`", ref.Name, ref.Name)
		return false, nil
	}
	choice, err := ui.Select("What do you want to do?", []ui.Option{
		{Label: "Approve and apply — the deploy continues", Value: "approve"},
		{Label: "Reject — the deploy is cancelled", Value: "reject"},
		{Label: "Decide later", Value: "later"},
	})
	if err != nil {
		return false, err
	}
	switch choice {
	case "approve":
		if err := decideConfigPlan(ctx, ref, *plan, "approve"); err != nil {
			return false, err
		}
		return true, nil
	case "reject":
		return false, decideConfigPlan(ctx, ref, *plan, "reject")
	}
	ui.Hint("The deploy keeps waiting. Approve later with `brimble config approve %s`.", ref.Name)
	return false, nil
}

func reportDeploy(ctx context.Context, ref ProjectRef, logID, status string, elapsed time.Duration, showLogs bool, tail *lineTail) error {
	switch {
	case deploySucceeded(status):
		if elapsed > 0 {
			ui.Success("Deployed %s in %s", ref.Display(), elapsed.Round(time.Second))
		} else {
			ui.Success("%s is deployed", ref.Display())
		}
		if domain := projectDomain(ctx, ref.ID); domain != "" {
			fmt.Fprintf(os.Stdout, "  %s\n", ui.Code(domain))
		}
		return nil
	case status == "SUPERSEDED":
		ui.Warn("A newer deployment replaced this one")
		return nil
	case status == "AWAITING_CONFIG_APPROVAL":
		ui.Warn("The deployment is waiting for configuration approval in the dashboard")
		return nil
	case status == "INACTIVE":
		ui.Warn("The project is stopped — start it with `brimble project start %s`", ref.Name)
		return nil
	case status == "DEGRADED":
		return fmt.Errorf("%s deployed but is failing health checks — see `brimble logs %s`", ref.Display(), ref.Name)
	default:
		if !showLogs {
			if tail != nil {
				tail.print()
			} else {
				printLogTail(ctx, ref.ID, logID, 20)
			}
		}
		return fmt.Errorf("deployment %s — full log: `brimble logs download %s %s`",
			strings.ToLower(ui.StatusText(status)), ref.Name, logID)
	}
}

type lineTail struct {
	n     int
	lines []string
}

func (t *lineTail) add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > t.n {
		t.lines = t.lines[len(t.lines)-t.n:]
	}
}

func (t *lineTail) print() {
	if len(t.lines) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, ui.Dim("  Last lines of the build log:"))
	for _, l := range t.lines {
		fmt.Fprintf(os.Stderr, "%s %s\n", ui.Dim("│"), l)
	}
}

func pollDeploy(ctx context.Context, ref ProjectRef, logID string, showLogs bool) (string, error) {
	var printed int
	var status string
	var failures int
	for {
		current, err := deploymentStatus(ctx, ref.ID, logID)
		switch {
		case ctx.Err() != nil:
			return "", ctx.Err()
		case err != nil:
			failures++
			if failures == 5 {
				ui.Warn("Still unable to read deployment status: %s", err)
			}
		default:
			failures = 0
			if current != "" && current != status {
				status = current
				if !deployFinished(status) {
					ui.Info("%s", ui.Status(status))
				}
			}
		}
		if showLogs {
			printed = printNewLogLines(ctx, ref.ID, logID, printed, deployFinished(status))
		}
		if deployFinished(status) {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(followInterval):
		}
	}
}

func printLogTail(ctx context.Context, projectID, logID string, n int) {
	body, err := coreClient.DownloadLog(ctx, projectID, logID, cfg.TeamID)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	fmt.Fprintln(os.Stderr, ui.Dim("  Last lines of the build log:"))
	for _, l := range lines {
		fmt.Fprintf(os.Stderr, "%s %s\n", ui.Dim("│"), l)
	}
}

func deploymentStatus(ctx context.Context, projectID, logID string) (string, error) {
	q := url.Values{"limit": {"20"}}
	list, err := coreClient.ListDeployLogs(ctx, projectID, q, cfg.TeamID)
	if err != nil {
		return "", err
	}
	for _, d := range list.Logs {
		if d.ID == logID {
			return d.Status, nil
		}
	}
	return "", nil
}

// printNewLogLines prints build log lines after the first `printed` ones and
// returns the new count. Until final, a trailing partial line is held back.
func printNewLogLines(ctx context.Context, projectID, logID string, printed int, final bool) int {
	body, err := coreClient.DownloadLog(ctx, projectID, logID, cfg.TeamID)
	if err != nil {
		// No entries yet early in a build: 404 / empty body.
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return printed
		}
		return printed
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if !final && !strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := printed; i < len(lines); i++ {
		fmt.Fprintf(os.Stdout, "%s %s\n", ui.Dim("│"), lines[i])
	}
	return max(printed, len(lines))
}

func projectDomain(ctx context.Context, projectID string) string {
	project, err := coreClient.GetProject(ctx, projectID, cfg.TeamID)
	if err != nil {
		return ""
	}
	return project.Domain
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}

// uploadDrop zips dir and uploads it, returning the new deployment's log ID.
func uploadDrop(ctx context.Context, opts client.DropOpts, dir, display string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	warnIfSourceFolder(abs)

	archive, files, size, err := zipDir(abs)
	if err != nil {
		return "", err
	}
	if files == 0 {
		return "", fmt.Errorf("%s has no files to upload", abs)
	}

	summary := fmt.Sprintf("%d files (%s) from %s", files, formatBytes(float64(size)), abs)
	if !ui.IsInteractive() {
		ui.Info("Uploading %s to %s", summary, display)
	}
	ok, err := confirmOrAbort(false, fmt.Sprintf("Upload %s to %s?", summary, display), false)
	if err != nil || !ok {
		if err == nil {
			err = errAborted
		}
		return "", err
	}

	opts.Archive = bytes.NewReader(archive)
	var result client.DeployResult
	err = ui.WithSpinner(fmt.Sprintf("Uploading %s…", formatBytes(float64(len(archive)))), func() error {
		var err error
		result, err = coreClient.DropUpload(ctx, opts)
		return err
	})
	if err != nil {
		return "", err
	}
	return result.LogID, nil
}

var errAborted = errors.New("aborted")

// Never uploaded: VCS metadata, dependencies, local link files and secrets.
var skipDirs = map[string]bool{".git": true, "node_modules": true, ".brimble": true, ".hg": true, ".svn": true}

func skipFile(name string) bool {
	return name == ".DS_Store" || name == ".env" || strings.HasPrefix(name, ".env.")
}

func zipDir(root string) (archive []byte, files int, size int64, err error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || skipFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > 100<<20 {
			return fmt.Errorf("%s is larger than the 100 MB upload limit per file", rel)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(w, f); err != nil {
			return err
		}
		files++
		size += info.Size()
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}
	if err := zw.Close(); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), files, size, nil
}

// warnIfSourceFolder nudges toward uploading build output instead of sources.
func warnIfSourceFolder(dir string) {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		ui.Warn("%s has a package.json but no index.html — upload your build output instead (e.g. --dir dist)", dir)
	}
}
