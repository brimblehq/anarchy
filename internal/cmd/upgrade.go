package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/brimblehq/anarchy/internal/selfupdate"
	"github.com/brimblehq/anarchy/internal/ui"
	"github.com/spf13/cobra"
)

func newUpgradeCmd() *cobra.Command {
	var check, yes bool
	var target string

	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update", "self-update"},
		Short:   "Upgrade the Brimble CLI to the latest version",
		Long: `Upgrade the Brimble CLI to the latest release.

Homebrew installs are upgraded with brew, and "go install" builds with go.
Installs from the install script or a downloaded archive are replaced in
place after the download is verified against the release checksums.`,
		Example: `  brimble upgrade
  brimble upgrade --check          # only report whether an update exists
  brimble upgrade --version v0.3.0`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}
			selfupdate.CleanupOld(exe)

			tag := target
			if tag == "" {
				err = ui.WithSpinner("Checking for updates…", func() error {
					var err error
					tag, err = selfupdate.Latest(ctx)
					return err
				})
				if errors.Is(err, selfupdate.ErrNoRelease) {
					ui.Muted("No Brimble CLI release has been published yet.")
					return nil
				}
				if err != nil {
					return err
				}
			} else if !strings.HasPrefix(tag, "v") {
				tag = "v" + tag
			}

			method := selfupdate.Method(exe, version, goBinDir())
			current := version
			if method != selfupdate.MethodSource && target == "" && !selfupdate.Newer(current, tag) {
				ui.Success("You're on the latest version (%s)", displayVersion(current))
				return nil
			}
			if check {
				if method == selfupdate.MethodSource {
					ui.Info("Latest release is %s; this is a source build (%s)", tag, displayVersion(current))
				} else {
					ui.Info("Update available: %s → %s", displayVersion(current), tag)
					ui.Hint("Run `brimble upgrade` to install it")
				}
				return nil
			}

			switch method {
			case selfupdate.MethodSource:
				ui.Info("This brimble was built from source (%s)", displayVersion(current))
				ui.Hint("Update the checkout and rebuild (git pull && make build), or install a release:")
				ui.Hint("  curl -fsSL https://github.com/brimblehq/anarchy/releases/latest/download/install.sh | sh")
				return nil
			case selfupdate.MethodHomebrew:
				return runUpgradeTool(yes, tag, "brew", "upgrade", "--cask", "brimble")
			case selfupdate.MethodGo:
				return runUpgradeTool(yes, tag, "go", "install", "github.com/brimblehq/anarchy/cmd/brimble@"+tag)
			}

			ok, err := confirmOrAbort(yes, fmt.Sprintf("Upgrade brimble %s → %s?", displayVersion(current), tag), false)
			if err != nil || !ok {
				return err
			}
			var bin []byte
			err = ui.WithSpinner("Downloading "+tag+"…", func() error {
				var err error
				bin, err = selfupdate.Download(ctx, tag, runtime.GOOS, runtime.GOARCH)
				return err
			})
			if err != nil {
				return err
			}
			if err := selfupdate.Replace(exe, bin, runtime.GOOS); err != nil {
				return err
			}
			out, err := exec.Command(exe, "--version").Output()
			if err != nil {
				return fmt.Errorf("installed %s but it didn't run: %w — reinstall with the install script", tag, err)
			}
			ui.Success("Upgraded to %s", strings.TrimSpace(strings.TrimPrefix(string(out), "brimble version ")))
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "Only check whether a newer version exists")
	cmd.Flags().StringVar(&target, "version", "", "Install this release instead of the latest, e.g. v0.3.0")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Don't ask for confirmation")
	return cmd
}

// runUpgradeTool upgrades through the package manager that owns the binary.
func runUpgradeTool(yes bool, tag string, name string, args ...string) error {
	command := name + " " + strings.Join(args, " ")
	if _, err := exec.LookPath(name); err != nil {
		ui.Info("Upgrade to %s with:", tag)
		ui.Hint("  %s", command)
		return nil
	}
	if !yes && !ui.IsInteractive() {
		ui.Info("Upgrade to %s with: %s", tag, command)
		return nil
	}
	ok, err := confirmOrAbort(yes, fmt.Sprintf("Run `%s` to upgrade to %s?", command, tag), false)
	if err != nil || !ok {
		return err
	}
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	ui.Success("Upgraded to %s", tag)
	return nil
}

func goBinDir() string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return gobin
	}
	out, err := exec.Command("go", "env", "GOPATH").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return filepath.Join(strings.TrimSpace(string(out)), "bin")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "go", "bin")
	}
	return ""
}

func displayVersion(v string) string {
	if v == "" || v == "dev" {
		return "dev"
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}
