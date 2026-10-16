package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/brimblehq/anarchy/internal/selfupdate"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Wait at most this long, after the command, for a check still in flight.
const noticeWait = 500 * time.Millisecond

// startUpdateNotice checks for a newer release alongside the command (at most
// once per CheckInterval, cached) and returns a func that prints the notice
// afterwards. It does nothing in scripts, CI, JSON output and source builds.
func startUpdateNotice(root *cobra.Command, args []string) func() {
	if !updateNoticeEnabled(root, args) {
		return func() {}
	}
	path, err := selfupdate.StatePath()
	if err != nil {
		return func() {}
	}
	state := selfupdate.LoadState(path)

	var done chan selfupdate.State
	if time.Since(state.CheckedAt) > selfupdate.CheckInterval {
		done = make(chan selfupdate.State, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if s, err := selfupdate.Refresh(ctx, path); err == nil {
				done <- s
			}
			close(done)
		}()
	}

	return func() {
		if done != nil {
			select {
			case s, ok := <-done:
				if ok {
					state = s
				}
			case <-time.After(noticeWait):
			}
		}
		if state.Latest != "" && selfupdate.Newer(version, state.Latest) {
			printUpdateNotice(state.Latest)
		}
	}
}

func printUpdateNotice(latest string) {
	fmt.Fprintln(os.Stderr)
	ui.Info("A new version of brimble is available: %s → %s", displayVersion(version), latest)
	ui.Hint("Run `brimble upgrade` to install it")
}

func updateNoticeEnabled(root *cobra.Command, args []string) bool {
	if selfupdate.IsSourceVersion(version) || os.Getenv("BRIMBLE_NO_UPDATE_NOTIFIER") != "" || os.Getenv("CI") != "" {
		return false
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return false
	}
	if slices.Contains(args, "-o=json") || slices.Contains(args, "--output=json") {
		return false
	}
	for i, a := range args {
		if (a == "-o" || a == "--output") && i+1 < len(args) && args[i+1] == "json" {
			return false
		}
	}
	cmd, _, err := root.Find(args)
	if err != nil {
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "upgrade", "completion", "__complete", "__completeNoDesc":
			return false
		}
	}
	return true
}
