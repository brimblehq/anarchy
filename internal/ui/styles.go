package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorPrimary = lipgloss.Color("#7C3AED")
	colorSuccess = lipgloss.Color("#22C55E")
	colorError   = lipgloss.Color("#EF4444")
	colorWarn    = lipgloss.Color("#F59E0B")
	colorMuted   = lipgloss.Color("#9CA3AF")
	colorInfo    = lipgloss.Color("#38BDF8")

	styleBrand = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	styleOK    = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	styleErr   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	styleWarn  = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	styleInfo  = lipgloss.NewStyle().Foreground(colorInfo)
	styleMuted = lipgloss.NewStyle().Foreground(colorMuted)
	styleKey   = lipgloss.NewStyle().Foreground(colorMuted)
	styleVal   = lipgloss.NewStyle().Bold(true)
)

func Brand(msg string) string {
	return styleBrand.Render(msg)
}

func Success(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stdout, styleOK.Render("✓")+" "+msg)
}

func Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, styleErr.Render("✗")+" "+msg)
}

func Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stdout, styleWarn.Render("!")+" "+msg)
}

func Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stdout, styleInfo.Render("→")+" "+msg)
}

func Muted(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stdout, styleMuted.Render(msg))
}

func Title(text string) {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, styleBrand.Render(text))
	fmt.Fprintln(os.Stdout)
}

func Section(text string) {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, styleInfo.Render(text))
}

func KeyValue(key, value string) {
	padded := fmt.Sprintf("%-14s", key)
	fmt.Fprintf(os.Stdout, "  %s  %s\n", styleKey.Render(padded), styleVal.Render(value))
}

func Bullet(text string) {
	fmt.Fprintf(os.Stdout, "  %s %s\n", styleMuted.Render("•"), text)
}

func Fail(err error) {
	if err == nil {
		return
	}
	Error("%s", err.Error())
	os.Exit(1)
}
