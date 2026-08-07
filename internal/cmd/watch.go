package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/ui"
	"github.com/spf13/cobra"
)

func addWatchFlags(cmd *cobra.Command, watch *bool, interval *time.Duration) {
	cmd.Flags().BoolVarP(watch, "watch", "w", false, "Watch for changes and refresh the view")
	cmd.Flags().DurationVar(interval, "interval", 5*time.Second, "Refresh interval when using --watch")
}

func runWatch(ctx context.Context, interval time.Duration, tick func(ctx context.Context) error) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	first := true
	for {
		if err := watchCtx.Err(); err != nil {
			ui.Muted("Watch stopped")
			return nil
		}

		if !first {
			ui.ClearScreen()
		}
		first = false

		updated := time.Now().UTC()
		if err := tick(watchCtx); err != nil {
			if watchCtx.Err() != nil {
				ui.Muted("Watch stopped")
				return nil
			}
			ui.Warn("%s", err.Error())
		}

		fmt.Fprintln(os.Stdout)
		ui.Muted("  Updated %s  ·  every %s  ·  Ctrl+C to stop",
			updated.Format("15:04:05 UTC"), interval.Round(time.Millisecond))

		timer := time.NewTimer(interval)
		select {
		case <-watchCtx.Done():
			timer.Stop()
			ui.Muted("Watch stopped")
			return nil
		case <-timer.C:
		}
	}
}
