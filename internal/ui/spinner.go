package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type Spinner struct {
	s *spinner.Spinner
}

func NewSpinner(message string) *Spinner {
	if !isTTY() {
		return &Spinner{}
	}

	s := spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " " + message
	_ = s.Color("magenta", "bold")
	return &Spinner{s: s}
}

func (sp *Spinner) Start() {
	if sp == nil || sp.s == nil {
		return
	}
	sp.s.Start()
}

func (sp *Spinner) Update(message string) {
	if sp == nil || sp.s == nil {
		return
	}
	sp.s.Suffix = " " + message
}

func (sp *Spinner) Stop() {
	if sp == nil || sp.s == nil {
		return
	}
	sp.s.Stop()
}

func (sp *Spinner) Success(message string) {
	sp.Stop()
	Success("%s", message)
}

func (sp *Spinner) Fail(message string) {
	sp.Stop()
	Error("%s", message)
}

func WithSpinner(message string, fn func() error) error {
	sp := NewSpinner(message)
	sp.Start()
	err := fn()
	sp.Stop()
	return err
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) || term.IsTerminal(int(os.Stderr.Fd()))
}

func IsTTY() bool {
	return isTTY()
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
