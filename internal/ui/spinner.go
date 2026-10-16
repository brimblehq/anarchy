package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

func WithSpinner(message string, fn func() error) error {
	var s *spinner.Spinner
	if term.IsTerminal(int(os.Stderr.Fd())) {
		s = spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(os.Stderr))
		s.Suffix = " " + message
		_ = s.Color("magenta", "bold")
		s.Start()
		defer s.Stop()
	}
	return fn()
}

func ClearScreen() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stdout)
		return
	}
	fmt.Fprint(os.Stdout, "\033[H\033[2J")
}

func Dim(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(s)
}
