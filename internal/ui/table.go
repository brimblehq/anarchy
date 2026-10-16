package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table prints rows under dimmed headers, padding by visible width so styled
// cells line up. Empty cells print as "—".
func Table(headers []string, rows [][]string) {
	TableTo(os.Stdout, headers, rows)
}

func TableTo(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i := range headers {
			if i < len(row) {
				widths[i] = max(widths[i], lipgloss.Width(cell(row[i])))
			}
		}
	}

	line := func(cells []string, style func(string) string) {
		var b strings.Builder
		b.WriteString("  ")
		for i := range headers {
			v := ""
			if i < len(cells) {
				v = cells[i]
			}
			if style == nil {
				v = cell(v)
			}
			if style != nil {
				v = style(v)
			}
			b.WriteString(v)
			if i < len(headers)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-lipgloss.Width(v)+3))
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}

	line(headers, Dim)
	for _, row := range rows {
		line(row, nil)
	}
}

func cell(s string) string {
	if s == "" {
		return Dim("—")
	}
	return s
}

// Status colors a deployment/project status for display.
func Status(status string) string {
	s := strings.ToUpper(strings.TrimSpace(status))
	switch s {
	case "":
		return ""
	case "ACTIVE", "NOMAD_READY":
		return styleOK.Render("● " + friendlyStatus(s))
	case "FAILED", "NOMAD_FAILED", "PAYMENT DUE":
		return styleErr.Render("● " + friendlyStatus(s))
	case "INPROGRESS", "PENDING", "AWAITING_CONFIG_APPROVAL", "DEGRADED":
		return styleWarn.Render("● " + friendlyStatus(s))
	default:
		return styleMuted.Render("● " + friendlyStatus(s))
	}
}

// StatusText is the plain label for a status, e.g. "Building".
func StatusText(status string) string {
	return friendlyStatus(strings.ToUpper(strings.TrimSpace(status)))
}

func friendlyStatus(s string) string {
	switch s {
	case "ACTIVE", "NOMAD_READY":
		return "Ready"
	case "INPROGRESS":
		return "Building"
	case "PENDING":
		return "Queued"
	case "FAILED", "NOMAD_FAILED":
		return "Failed"
	case "CANCELLED":
		return "Cancelled"
	case "SUPERSEDED":
		return "Superseded"
	case "INACTIVE":
		return "Stopped"
	case "DEGRADED":
		return "Degraded"
	case "AWAITING_CONFIG_APPROVAL":
		return "Awaiting approval"
	case "PAYMENT DUE":
		return "Payment due"
	}
	return strings.ToLower(s)
}

// Hint prints a dimmed "next step" suggestion to stderr.
func Hint(format string, args ...any) {
	fmt.Fprintln(os.Stderr, styleMuted.Render("  "+fmt.Sprintf(format, args...)))
}

// Code renders a command or identifier for inline display.
func Code(s string) string {
	return styleBrand.Render(s)
}

func Red(s string) string    { return styleErr.Render(s) }
func Yellow(s string) string { return styleWarn.Render(s) }
func Green(s string) string  { return styleOK.Render(s) }
