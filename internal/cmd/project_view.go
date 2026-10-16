package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
)

func printProjectList(list client.ProjectList) error {
	if len(list.Projects) == 0 {
		ui.Muted("No projects%s yet — create one with `brimble deploy` or `brimble project create`", teamSuffix())
		return nil
	}
	rows := make([][]string, 0, len(list.Projects))
	for _, p := range list.Projects {
		if p.Name == "" {
			continue
		}
		name := p.Name
		if linked != nil && linked.ProjectID == p.ProjectID() {
			name += ui.Dim(" (linked)")
		}
		rows = append(rows, []string{
			name, p.ServiceType, ui.Status(p.Status), strings.TrimPrefix(p.Domain, "https://"), formatAgo(p.UpdatedAt),
		})
	}
	fmt.Fprintln(os.Stdout)
	ui.Table([]string{"NAME", "TYPE", "STATUS", "URL", "UPDATED"}, rows)

	if list.TotalPages > 1 {
		fmt.Fprintln(os.Stdout)
		ui.Muted("  %d projects · page %d of %d · --page to see more", list.TotalProjects, max(1, list.CurrentPage), list.TotalPages)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

func printProjectView(p client.Project) error {
	ui.Title(p.Name)

	printKV("Status", ui.Status(p.Status))
	printKV("URL", p.Domain)
	if p.Team != nil {
		printKV("Team", p.Team.Name)
	}
	if p.HasUpdates {
		printKV("Pending", ui.Yellow("settings changed — run `brimble deploy` to apply"))
	}
	if p.Disabled {
		printKV("Disabled", ui.Red("yes — the project is not serving traffic"))
	}
	if p.PasswordEnabled {
		printKV("Password", "on")
	}
	if p.PreviewDeploymentsEnabled {
		printKV("Previews", "on")
	}
	printKV("Type", joinNonEmpty(" · ", p.ServiceType, p.Framework))
	printKV("Public", yesNo(p.Public))
	printKV("Maintenance", yesNo(p.Maintenance))
	printKV("Created", formatTime(p.CreatedAt))
	printKV("Updated", formatTime(p.UpdatedAt))

	if p.Repo != nil {
		ui.Section("Repository")
		printKV("Repo", p.Repo.Label())
		printKV("Branch", p.Repo.Branch)
		printKV("Link", p.GitLink)
	} else if p.GitLink != "" {
		ui.Section("Repository")
		printKV("Link", p.GitLink)
	}

	if p.Specs != nil {
		ui.Section("Resources")
		printKV("CPU", formatFloat(p.Specs.CPU))
		printKV("Memory", formatMemory(p.Specs.Memory))
		printKV("Region", p.Specs.Region.Name)
		if disk := formatFloat(p.DiskSize); disk != "" && disk != "0" {
			printKV("Disk", disk)
		}
	}

	ui.Section("Deploy")
	printKV("Health", formatHealth(p.Health))
	if p.Log != nil {
		printKV("Last commit", truncate(firstLine(p.Log.Message), 80))
	}
	printKV("In cluster", yesNo(p.DeployedInCluster))
	printKV("Replica ready", yesNo(p.ReplicaReady))

	if p.InstallCommand != "" || p.BuildCommand != "" || p.StartCommand != "" || p.RootDirectory != "" || p.OutputDirectory != "" {
		ui.Section("Build")
		printKV("Install", p.InstallCommand)
		printKV("Build", p.BuildCommand)
		printKV("Start", p.StartCommand)
		printKV("Root dir", p.RootDirectory)
		printKV("Output dir", p.OutputDirectory)
	}

	if names := namedNames(p.Domains); len(names) > 0 {
		ui.Section("Domains")
		for _, d := range names {
			ui.Bullet(d)
		}
	}

	if envNames := namedNames(p.Environments); len(envNames) > 0 {
		ui.Section(fmt.Sprintf("Environment variables (%d)", len(envNames)))
		shown := envNames
		if len(shown) > 12 {
			shown = shown[:12]
		}
		for _, n := range shown {
			ui.Bullet(n)
		}
		if len(envNames) > 12 {
			ui.Muted("  … and %d more", len(envNames)-12)
		}
	}

	fmt.Fprintln(os.Stdout)
	return nil
}

func printKV(key, value string) {
	if value == "" {
		return
	}
	ui.KeyValue(key, value)
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func formatMemory(v float64) string {
	s := formatFloat(v)
	if s == "" {
		return ""
	}
	return s + " GB"
}

func formatHealth(h client.HealthStatus) string {
	if h.Status == "" {
		return ""
	}
	if h.StatusCode != 0 {
		return h.Status + " (" + strconv.Itoa(h.StatusCode) + ")"
	}
	return h.Status
}

func formatTime(raw string) string {
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC().Format("2006-01-02 15:04 UTC")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format("2006-01-02 15:04 UTC")
	}
	return raw
}

func namedNames(items []client.Named) []string {
	var names []string
	for _, item := range items {
		if item.Name != "" {
			names = append(names, item.Name)
		}
	}
	return names
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func parseTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func formatAgo(raw string) string {
	t, ok := parseTime(raw)
	if !ok {
		return raw
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return t.Local().Format("Jan 2 15:04")
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("Jan 2, 2006")
}

func formatClock(raw string) string {
	t, ok := parseTime(raw)
	if !ok {
		return raw
	}
	t = t.Local()
	if y, m, d := t.Date(); y == time.Now().Year() && m == time.Now().Month() && d == time.Now().Day() {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 2 15:04:05")
}
