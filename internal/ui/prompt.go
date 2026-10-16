package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func compactTheme() *huh.Theme {
	t := huh.ThemeCharm()
	flat := lipgloss.NewStyle()
	t.Focused.Base = flat
	t.Focused.Card = flat
	t.Blurred.Base = flat
	t.Blurred.Card = flat
	return t
}

func Confirm(title string, defaultYes bool) (bool, error) {
	if !IsInteractive() {
		return defaultYes, nil
	}
	value := defaultYes
	err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&value).
		WithButtonAlignment(lipgloss.Left).
		WithTheme(compactTheme()).
		Run()
	return value, err
}

func Text(title, placeholder, defaultValue string) (string, error) {
	if !IsInteractive() {
		if defaultValue != "" {
			return defaultValue, nil
		}
		return "", fmt.Errorf("not a terminal; cannot prompt for %q", title)
	}
	value := defaultValue
	input := huh.NewInput().
		Title(title).
		Value(&value)
	if placeholder != "" {
		input = input.Placeholder(placeholder)
	}
	if err := input.WithTheme(compactTheme()).Run(); err != nil {
		return "", err
	}
	return value, nil
}

func Password(title, placeholder string) (string, error) {
	if !IsInteractive() {
		return "", fmt.Errorf("not a terminal; pass flags or env vars instead")
	}
	var value string
	input := huh.NewInput().
		Title(title).
		EchoMode(huh.EchoModePassword).
		Value(&value)
	if placeholder != "" {
		input = input.Placeholder(placeholder)
	}
	if err := input.WithTheme(compactTheme()).Run(); err != nil {
		return "", err
	}
	return value, nil
}

type Option struct {
	Label string
	Value string
}

func Select(title string, options []Option) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("nothing to select")
	}
	if len(options) == 1 {
		if IsInteractive() {
			fmt.Fprintf(os.Stderr, "%s %s\n", styleMuted.Render(strings.TrimSuffix(title, "?")+":"), options[0].Label)
		}
		return options[0].Value, nil
	}
	if !IsInteractive() {
		return "", fmt.Errorf("not a terminal; pass the argument explicitly")
	}

	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o.Label, o.Value))
	}

	var value string
	err := huh.NewSelect[string]().
		Title(title).
		Options(opts...).
		Value(&value).
		Height(min(12, len(options)+2)).
		WithTheme(compactTheme()).
		Run()
	return value, err
}

func LoginForm(apiKey *string) error {
	if !IsInteractive() {
		return fmt.Errorf("not a terminal; pass --api-key or set BRIMBLE_API_KEY")
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Log in to Brimble").
				Description("Create a key at https://app.brimble.io/settings/api-keys\nKeys look like br_…"),
			huh.NewInput().
				Title("API key").
				Placeholder("br_…").
				EchoMode(huh.EchoModePassword).
				Value(apiKey).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}),
		),
	).WithTheme(compactTheme()).Run()
}

func MultiSelect(title, description string, options []Option, selected []string) ([]string, error) {
	if len(options) == 0 {
		return nil, fmt.Errorf("nothing to select")
	}
	if !IsInteractive() {
		return nil, fmt.Errorf("not a terminal; pass the choices explicitly")
	}
	pre := map[string]bool{}
	for _, v := range selected {
		pre[v] = true
	}
	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o.Label, o.Value).Selected(pre[o.Value]))
	}
	var values []string
	field := huh.NewMultiSelect[string]().
		Title(title).
		Options(opts...).
		Value(&values).
		Height(min(14, len(options)+3))
	if description != "" {
		field = field.Description(description)
	}
	err := field.WithTheme(compactTheme()).Run()
	return values, err
}
