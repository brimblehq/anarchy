package cmd

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newDatabaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "database",
		Aliases: []string{"databases", "db"},
		Short:   "Manage database backups and point-in-time recovery",
		Long: `Manage a database project's backups and point-in-time recovery.

Database metrics are under "brimble project stats"; everything else about a
database is in the dashboard.`,
	}
	cmd.AddCommand(newDatabaseBackupsCmd())
	cmd.AddCommand(newPITRCmd())
	return cmd
}

func pickDatabase(ctx context.Context, arg string) (ProjectRef, error) {
	if arg == "" && linked == nil && ui.IsInteractive() {
		var list client.ProjectList
		err := ui.WithSpinner("Loading databases…", func() error {
			var err error
			list, err = coreClient.ListProjects(ctx, client.ListProjectsOpts{TeamID: cfg.TeamID, Limit: 100, ServiceType: "database"})
			return err
		})
		if err != nil {
			return ProjectRef{}, err
		}
		dbs := projectRefs(list)
		if len(dbs) == 0 {
			return ProjectRef{}, fmt.Errorf("no databases%s", teamSuffix())
		}
		return selectProject("Select a database", dbs)
	}
	ref, err := pickProject(ctx, arg)
	if err != nil {
		return ref, err
	}
	p, err := projectData(ctx, &ref)
	if err != nil {
		return ref, err
	}
	if p.ServiceType != "database" {
		return ref, fmt.Errorf("%s isn't a database", ref.Display())
	}
	return ref, nil
}

func newDatabaseBackupsCmd() *cobra.Command {
	var page, limit int
	cmd := &cobra.Command{
		Use:   "backups [database]",
		Short: "List a database's backups and restores",
		Long: `List a database's backups and restores, newest first. Point-in-time
restores show the time they restored to and the database they created.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickDatabase(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var h client.BackupHistory
				err = ui.WithSpinner("Loading backups…", func() error {
					var err error
					h, err = coreClient.BackupHistory(ctx, ref.ID, page, limit, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(h)
				}
				return printBackupHistory(ref, h, restoredDatabaseNames(ctx, h))
			})
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "Page number")
	cmd.Flags().IntVar(&limit, "limit", 20, "Entries per page (max 100)")
	return cmd
}

func backupStatus(status string) string {
	label := "● " + strings.ToUpper(status[:min(1, len(status))]) + status[min(1, len(status)):]
	switch status {
	case "completed":
		return ui.Green(label)
	case "failed":
		return ui.Red(label)
	case "pending", "running":
		return ui.Yellow(label)
	}
	return ui.Dim(label)
}

func printBackupHistory(ref ProjectRef, h client.BackupHistory, names map[string]string) error {
	ui.Title(ref.Display() + " · backups")
	if len(h.Data) == 0 {
		ui.Muted("  No backups or restores yet")
		fmt.Fprintln(os.Stdout)
		return nil
	}
	rows := make([][]string, 0, len(h.Data))
	var failures []client.BackupHistoryEntry
	for _, e := range h.Data {
		detail := ""
		switch e.Type {
		case "restore":
			if e.TargetTime != "" {
				detail = "to " + formatInstant(e.TargetTime)
			}
			if e.TargetProjectID != "" {
				into := cmp.Or(names[e.TargetProjectID], e.TargetProjectID)
				if e.TargetProjectDeleted {
					into = "a deleted database"
				}
				detail = strings.TrimSpace(detail + " → " + into)
			}
		default:
			detail = joinNonEmpty(" · ", e.Trigger, formatSize(e.SizeBytes))
		}
		if e.FailureReason != "" {
			failures = append(failures, e)
		}
		rows = append(rows, []string{shortID(e.ID), e.Type, backupStatus(e.Status), detail, formatAgo(e.CreatedAt)})
	}
	ui.Table([]string{"ID", "TYPE", "STATUS", "DETAIL", "STARTED"}, rows)
	for _, e := range failures {
		ui.Muted("  %s failed: %s", shortID(e.ID), truncate(e.FailureReason, 120))
	}
	if h.TotalPages > 1 {
		fmt.Fprintln(os.Stdout)
		ui.Muted("  Page %d of %d · --page to see more", max(1, h.CurrentPage), h.TotalPages)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

func restoredDatabaseNames(ctx context.Context, h client.BackupHistory) map[string]string {
	names := map[string]string{}
	for _, e := range h.Data {
		if e.Type == "restore" && e.TargetProjectID != "" && !e.TargetProjectDeleted {
			list, err := coreClient.ListProjects(ctx, client.ListProjectsOpts{TeamID: cfg.TeamID, Limit: 100, ServiceType: "database"})
			if err != nil {
				return names
			}
			for _, p := range list.Projects {
				names[p.ProjectID()] = p.Name
			}
			return names
		}
	}
	return names
}

func formatSize(n int64) string {
	if n <= 0 {
		return ""
	}
	return formatBytes(float64(n))
}

func newPITRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pitr",
		Aliases: []string{"point-in-time"},
		Short:   "Turn on point-in-time recovery and restore to a moment in time",
		Long: `Point-in-time recovery continuously archives a database's changes to your
own storage bucket, so you can restore it to any second within the
recovery window. A restore creates a new database; the original is left
as it is.

Point-in-time recovery needs a paid plan and a database engine that
supports it. While it's on, scheduled backups are replaced by its own daily
base backup.`,
	}
	cmd.AddCommand(newPITRStatusCmd())
	cmd.AddCommand(newPITREnableCmd())
	cmd.AddCommand(newPITRDisableCmd())
	cmd.AddCommand(newPITRRestoreCmd())
	return cmd
}

func loadPITRStatus(ctx context.Context, ref ProjectRef) (client.PITRStatus, error) {
	var s client.PITRStatus
	err := ui.WithSpinner("Loading point-in-time recovery…", func() error {
		var err error
		s, err = coreClient.PITRStatus(ctx, ref.ID, cfg.TeamID)
		return err
	})
	return s, err
}

func newPITRStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [database]",
		Short: "Show whether point-in-time recovery is on, and the restore window",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickDatabase(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				s, err := loadPITRStatus(ctx, ref)
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(s)
				}
				var settings client.BackupSettings
				if s.Enabled {
					settings, _ = coreClient.BackupSettings(ctx, ref.ID, cfg.TeamID)
				}
				printPITRStatus(ref, s, settings.Destination)
				return nil
			})
		},
	}
}

func printPITRStatus(ref ProjectRef, s client.PITRStatus, dest client.BackupDestination) {
	ui.Title(ref.Display() + " · point-in-time recovery")
	switch {
	case !s.Supported:
		ui.Muted("  This database engine doesn't support point-in-time recovery.")
		fmt.Fprintln(os.Stdout)
		return
	case !s.Enabled:
		status := "off"
		if s.Applying {
			status += ui.Yellow("  the database is applying a change")
		}
		printKV("Status", status)
		ui.Hint("Turn it on with `brimble db pitr enable %s`", ref.Name)
		fmt.Fprintln(os.Stdout)
		return
	}

	status := ui.Green("on")
	if s.Applying {
		status += ui.Yellow("  applying — restores are available once it finishes")
	}
	printKV("Status", status)
	printKV("Bucket", describeDestination(cmp.Or(dest.Type, s.DestinationType), dest))
	printKV("Window", fmt.Sprintf("last %d days", s.WindowDays))
	if s.EarliestRestorableAt == "" {
		printKV("Restorable", ui.Yellow("not yet — waiting for the first base backup"))
	} else {
		printKV("Restorable", formatInstant(s.EarliestRestorableAt)+" → "+formatInstant(s.LatestRestorableAt))
	}
	if n := len(s.BaseBackups); n > 0 {
		printKV("Base backups", fmt.Sprintf("%d, latest %s", n, formatAgo(s.BaseBackups[n-1])))
	}
	if s.EarliestRestorableAt != "" {
		ui.Hint("Restore with `brimble db pitr restore %s --to <time>`", ref.Name)
	}
	fmt.Fprintln(os.Stdout)
}

func describeDestination(kind string, d client.BackupDestination) string {
	switch kind {
	case "brimble":
		return joinNonEmpty(" ", cmp.Or(d.Name, d.BucketName, d.BucketID), "(Brimble storage)")
	case "external":
		return joinNonEmpty(" ", d.BucketName, "at "+d.Endpoint)
	}
	return "Brimble's backup storage"
}

func newPITREnableCmd() *cobra.Command {
	var bucket, endpoint, bucketName, region, accessKeyID string
	var yes bool

	cmd := &cobra.Command{
		Use:   "enable [database]",
		Short: "Turn on point-in-time recovery",
		Long: `Turn on point-in-time recovery. Changes are archived to your own bucket:
a Brimble storage bucket (--bucket) or any S3-compatible bucket
(--endpoint and --bucket-name, with access keys).

If the database already backs up to your own bucket, that bucket is used.
In a terminal you can pick a Brimble bucket from a list.

Scheduled backups stop while point-in-time recovery is on; it takes its own
daily base backup instead. Some databases restart to apply the change.

For an external bucket, the secret key is read from
BRIMBLE_BUCKET_SECRET_ACCESS_KEY, or asked for in a terminal.`,
		Example: `  brimble db pitr enable my-db --bucket db-archive
  brimble db pitr enable my-db --endpoint https://s3.us-east-1.amazonaws.com \
    --bucket-name my-db-pitr --region us-east-1 --access-key-id AKIA…`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickDatabase(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				var settings client.BackupSettings
				err = ui.WithSpinner("Loading backup settings…", func() error {
					var err error
					settings, err = coreClient.BackupSettings(ctx, ref.ID, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if !settings.PITRSupported {
					return fmt.Errorf("%s's database engine doesn't support point-in-time recovery", ref.Display())
				}

				dest, err := pitrDestination(ctx, settings, bucket, endpoint, bucketName, region, accessKeyID)
				if err != nil {
					return err
				}
				if settings.PITREnabled && dest == nil {
					ui.Success("Point-in-time recovery is already on for %s", ref.Display())
					return nil
				}

				where := describeDestination(settings.Destination.Type, settings.Destination)
				if dest != nil {
					where = describeDestination(dest.Type, *dest)
				}
				prompt := fmt.Sprintf("Turn on point-in-time recovery for %s, archiving to %s? Scheduled backups stop while it's on.", ref.Display(), where)
				ok, err := confirmOrAbort(yes, prompt, false)
				if err != nil || !ok {
					return err
				}

				on := true
				if dest != nil {
					dest.Name = "" // display only; Core takes the bucket ID
				}
				err = ui.WithSpinner("Turning on point-in-time recovery…", func() error {
					var err error
					settings, err = coreClient.UpdateBackupSettings(ctx, ref.ID, client.UpdateBackupSettingsRequest{PITREnabled: &on, Destination: dest}, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(settings)
				}
				ui.Success("Point-in-time recovery is on for %s", ref.Display())
				ui.Hint("Restores are available once the first base backup finishes — check with `brimble db pitr status %s`", ref.Name)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "Brimble storage bucket to archive to (name or ID)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "S3 endpoint URL of an external bucket")
	cmd.Flags().StringVar(&bucketName, "bucket-name", "", "External bucket name")
	cmd.Flags().StringVar(&region, "region", "", "External bucket region")
	cmd.Flags().StringVar(&accessKeyID, "access-key-id", "", "External bucket access key ID")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	cmd.MarkFlagsMutuallyExclusive("bucket", "endpoint")
	cmd.MarkFlagsRequiredTogether("endpoint", "bucket-name", "access-key-id")
	return cmd
}

// pitrDestination returns nil to keep the current bucket.
func pitrDestination(ctx context.Context, settings client.BackupSettings, bucket, endpoint, bucketName, region, accessKeyID string) (*client.BackupDestination, error) {
	if endpoint != "" {
		secret := os.Getenv("BRIMBLE_BUCKET_SECRET_ACCESS_KEY")
		if secret == "" {
			var err error
			if secret, err = ui.Password("Secret access key for "+bucketName, ""); err != nil {
				return nil, fmt.Errorf("set BRIMBLE_BUCKET_SECRET_ACCESS_KEY to the bucket's secret key")
			}
		}
		return &client.BackupDestination{
			Type: "external", Endpoint: endpoint, BucketName: bucketName, Region: region,
			AccessKeyID: accessKeyID, SecretAccessKey: secret,
		}, nil
	}
	if bucket == "" && settings.Destination.Type != "platform" && settings.Destination.Type != "" {
		return nil, nil
	}
	if !settings.CustomDestination {
		return nil, fmt.Errorf("your plan doesn't include backing up to your own bucket, which point-in-time recovery needs")
	}

	var buckets []client.StorageBucket
	err := ui.WithSpinner("Loading buckets…", func() error {
		var err error
		buckets, err = coreClient.ListStorageBuckets(ctx, cfg.TeamID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if bucket != "" {
		for _, b := range buckets {
			if b.ID == bucket || b.Name == bucket || b.BucketName == bucket {
				return &client.BackupDestination{Type: "brimble", BucketID: b.ID, Name: b.Name}, nil
			}
		}
		return nil, fmt.Errorf("bucket %q not found%s", bucket, teamSuffix())
	}
	if !ui.IsInteractive() {
		return nil, fmt.Errorf("point-in-time recovery needs your own bucket — pass --bucket, or --endpoint for an external one")
	}
	if len(buckets) == 0 {
		return nil, fmt.Errorf("point-in-time recovery needs your own bucket — create one in the dashboard, or pass --endpoint for an external one")
	}
	opts := make([]ui.Option, 0, len(buckets))
	for _, b := range buckets {
		opts = append(opts, ui.Option{Label: b.Name, Value: b.ID})
	}
	id, err := ui.Select("Archive to which bucket?", opts)
	if err != nil {
		return nil, err
	}
	for _, b := range buckets {
		if b.ID == id {
			return &client.BackupDestination{Type: "brimble", BucketID: b.ID, Name: b.Name}, nil
		}
	}
	return nil, fmt.Errorf("bucket not found")
}

func newPITRDisableCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "disable [database]",
		Short: "Turn off point-in-time recovery",
		Long: `Turn off point-in-time recovery. Scheduled backups resume, and you can no
longer restore to a point in time. Some databases restart to apply it.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickDatabase(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				prompt := fmt.Sprintf("Turn off point-in-time recovery for %s? You won't be able to restore it to a point in time.", ref.Display())
				ok, err := confirmOrAbort(yes, prompt, true)
				if err != nil || !ok {
					return err
				}
				off := false
				var settings client.BackupSettings
				err = ui.WithSpinner("Turning off point-in-time recovery…", func() error {
					var err error
					settings, err = coreClient.UpdateBackupSettings(ctx, ref.ID, client.UpdateBackupSettingsRequest{PITREnabled: &off}, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(settings)
				}
				ui.Success("Point-in-time recovery is off for %s; scheduled backups resume", ref.Display())
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newPITRRestoreCmd() *cobra.Command {
	var to, name string
	var yes bool

	cmd := &cobra.Command{
		Use:   "restore [database] --to <time>",
		Short: "Restore a database to a point in time, as a new database",
		Long: `Restore a database as it was at a point in time. The restore creates a new
database (named "<database>-restored" unless you pass --name) with the same
plan, region and settings; the original keeps running unchanged.

--to takes a UTC time (2026-10-25T06:30:00Z, "2026-10-25 06:30") or how long
ago ("15m", "2h", "1d"). See the restorable range with
"brimble db pitr status". Follow the restore with "brimble db backups".`,
		Example: `  brimble db pitr restore my-db --to 15m
  brimble db pitr restore my-db --to "2026-10-25 06:30" --name my-db-before-migration`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			run(func(ctx context.Context) error {
				ref, err := pickDatabase(ctx, argOr(args, 0))
				if err != nil {
					return err
				}
				s, err := loadPITRStatus(ctx, ref)
				if err != nil {
					return err
				}
				switch {
				case !s.Supported:
					return fmt.Errorf("%s's database engine doesn't support point-in-time recovery", ref.Display())
				case !s.Enabled:
					return fmt.Errorf("point-in-time recovery is off for %s — turn it on with `brimble db pitr enable %s`", ref.Display(), ref.Name)
				case s.EarliestRestorableAt == "":
					return fmt.Errorf("%s has nothing to restore yet — the first base backup hasn't finished", ref.Display())
				}
				earliest, _ := parseTime(s.EarliestRestorableAt)
				latest, _ := parseTime(s.LatestRestorableAt)
				window := formatInstant(s.EarliestRestorableAt) + " → " + formatInstant(s.LatestRestorableAt)

				if to == "" {
					if !ui.IsInteractive() {
						return fmt.Errorf("--to is required; you can restore to %s", window)
					}
					ui.Info("You can restore to %s", window)
					if to, err = ui.Text("Restore to (UTC time, or how long ago like 15m)", "2026-10-25 06:30 or 15m", ""); err != nil {
						return err
					}
				}
				target, err := parseRestoreTime(to, time.Now())
				if err != nil {
					return err
				}
				if target.Before(earliest) || target.After(latest) {
					return fmt.Errorf("%s is outside the restorable range %s", formatInstant(target.Format(time.RFC3339)), window)
				}

				into := cmp.Or(name, ref.Name+"-restored")
				prompt := fmt.Sprintf("Restore %s as it was at %s into a new database %s?", ref.Display(), formatInstant(target.Format(time.RFC3339)), into)
				ok, err := confirmOrAbort(yes, prompt, false)
				if err != nil || !ok {
					return err
				}

				var result client.PITRRestoreResult
				err = ui.WithSpinner("Starting the restore…", func() error {
					var err error
					result, err = coreClient.RestoreToPointInTime(ctx, ref.ID, target, name, cfg.TeamID)
					return err
				})
				if err != nil {
					return err
				}
				if jsonOutput() {
					return printResult(result)
				}
				ui.Success("Restoring %s to %s into %s", ref.Display(), formatInstant(target.Format(time.RFC3339)), cmp.Or(result.Project.Name, into))
				printKV("Database ID", result.Project.ID)
				ui.Hint("Follow it with `brimble db backups %s`; the new database is ready when the restore shows completed", ref.Name)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&to, "to", "", `Time to restore to: UTC ("2026-10-25 06:30") or how long ago ("15m", "2h", "1d")`)
	cmd.Flags().StringVar(&name, "name", "", "Name of the new database (default: <database>-restored)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

var agoPattern = regexp.MustCompile(`^(\d+)\s*(s|m|h|d)(?:\s+ago)?$`)

func parseRestoreTime(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if m := agoPattern.FindStringSubmatch(strings.ToLower(raw)); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		return now.Add(-time.Duration(n) * unit).Truncate(time.Second), nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("can't read %q as a time — use UTC like \"2026-10-25 06:30\" or how long ago like \"15m\"", raw)
}

func formatInstant(raw string) string {
	t, ok := parseTime(raw)
	if !ok {
		return raw
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}
