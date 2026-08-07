package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brimbleapp/brimble-cli/internal/ui"
)

func printProjectList(data any) error {
	projects := projectOptions(data)
	if len(projects) == 0 {
		ui.Muted("(no projects)")
		return nil
	}
	nameW := 4
	for _, p := range projects {
		if len(p.Name) > nameW {
			nameW = len(p.Name)
		}
	}
	if nameW > 40 {
		nameW = 40
	}
	fmt.Fprintf(os.Stdout, "  %-*s  %-10s\n", nameW, ui.Dim("NAME"), ui.Dim("STATUS"))
	for _, p := range projects {
		name := p.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "…"
		}
		fmt.Fprintf(os.Stdout, "  %-*s  %s\n", nameW, name, p.Status)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}

func printProjectView(data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return printResult(data)
	}

	name := firstString(m, "name")
	status := firstString(m, "status")
	ui.Title(formatProjectHeading(name, status))

	printKV("Domain", firstString(m, "domain"))
	printKV("Type", joinNonEmpty(" · ", firstString(m, "serviceType"), firstString(m, "framework", "frameworkType")))
	printKV("Public", formatBool(m["public"]))
	printKV("Maintenance", formatBool(m["maintenance"]))
	printKV("Created", formatTime(firstString(m, "createdAt")))
	printKV("Updated", formatTime(firstString(m, "updatedAt")))

	if repo, ok := m["repo"].(map[string]any); ok && len(repo) > 0 {
		ui.Section("Repository")
		fullName := firstString(repo, "full_name", "fullName", "name")
		branch := firstString(repo, "branch")
		printKV("Repo", fullName)
		printKV("Branch", branch)
		if link := firstString(m, "gitLink"); link != "" {
			printKV("Link", link)
		}
	} else if link := firstString(m, "gitLink"); link != "" {
		ui.Section("Repository")
		printKV("Link", link)
	}

	if specs, ok := m["specs"].(map[string]any); ok && len(specs) > 0 {
		ui.Section("Resources")
		printKV("CPU", formatNum(specs["cpu"]))
		printKV("Memory", formatMemory(specs["memory"]))
		printKV("Region", formatRegion(specs["region"]))
		if disk := formatNum(m["diskSize"]); disk != "" && disk != "0" {
			printKV("Disk", disk)
		}
	}

	ui.Section("Deploy")
	printKV("Health", formatHealth(m["healthStatus"]))
	if log, ok := m["log"].(map[string]any); ok && len(log) > 0 {
		msg := firstString(log, "message", "status", "state")
		if msg != "" {
			printKV("Last log", truncate(msg, 80))
		}
		if at := firstString(log, "createdAt", "updatedAt", "finishedAt"); at != "" {
			printKV("Last log at", formatTime(at))
		}
	}
	printKV("In cluster", formatBool(m["deployedInCluster"]))
	printKV("Replica ready", formatBool(m["replica_ready"]))

	install := firstString(m, "installCommand")
	build := firstString(m, "buildCommand")
	start := firstString(m, "startCommand")
	root := firstString(m, "rootDirectory")
	output := firstString(m, "outputDirectory")
	if install != "" || build != "" || start != "" || root != "" || output != "" {
		ui.Section("Build")
		printKV("Install", install)
		printKV("Build", build)
		printKV("Start", start)
		printKV("Root dir", root)
		printKV("Output dir", output)
	}

	if domains := domainNames(m["domains"]); len(domains) > 0 {
		ui.Section("Domains")
		for _, d := range domains {
			ui.Bullet(d)
		}
	}

	if envNames := envVarNames(m["environments"]); len(envNames) > 0 {
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

func formatProjectHeading(name, status string) string {
	if name == "" {
		name = "project"
	}
	if status == "" {
		return name
	}
	return name + "  ·  " + status
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

func formatBool(v any) string {
	if v == nil {
		return ""
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Sprint(v)
	}
	if b {
		return "yes"
	}
	return "no"
}

func formatNum(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func formatMemory(v any) string {
	s := formatNum(v)
	if s == "" {
		return ""
	}
	return s + " GB"
}

func formatRegion(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprint(v)
	}
	return firstString(m, "name", "slug", "code", "region")
}

func formatHealth(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprint(v)
	}
	status := firstString(m, "status")
	code := formatNum(m["statusCode"])
	if status == "" {
		return ""
	}
	if code != "" {
		return status + " (" + code + ")"
	}
	return status
}

func formatTime(raw string) string {
	if raw == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format("2006-01-02 15:04 UTC")
		}
	}
	return raw
}

func domainNames(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var names []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if n := firstString(m, "name", "domain", "host"); n != "" {
			names = append(names, n)
		}
	}
	return names
}

func envVarNames(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var names []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if n := firstString(m, "name", "key"); n != "" {
			names = append(names, n)
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
