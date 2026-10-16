package cmd

import (
	"fmt"
	"math"
	"os"

	"github.com/brimbleapp/brimble-cli/internal/client"
	"github.com/brimbleapp/brimble-cli/internal/ui"
)

func printProjectStats(project string, stats client.ProjectStats) error {
	if project == "" {
		project = "project"
	}
	ui.Title(project + " · stats")

	printKV("Type", stats.ServiceType)
	printKV("Live", yesNo(stats.Live))
	printKV("Supported", yesNo(stats.Supported))

	if stats.Range.From != "" || stats.Range.To != "" || stats.Range.Step != "" {
		ui.Section("Range")
		printKV("From", formatTime(stats.Range.From))
		printKV("To", formatTime(stats.Range.To))
		if step := stats.Range.Step.String(); step != "" {
			printKV("Step", step+"s")
		}
	}

	s := stats.Summary
	if s != (client.StatsSummary{}) {
		ui.Section("Resources")
		printMetricKV("CPU", s.CPU, formatPercent)
		printMetricKV("Memory", s.Memory, formatPercent)
		printMetricKV("Network", s.Network, formatBytesPerSec)
		printMetricKV("Disk", s.Disk, formatPercent)
		printMetricKV("OOM kills", s.OOMKills, formatPlain)

		ui.Section("Traffic")
		printMetricKV("Requests/sec", s.RequestsPerSecond, formatRate)
		printMetricKV("Error rate", s.ErrorRate, formatPercent)
		printMetricKV("Avg response", s.AverageResponseTimeMs, formatDurationMs)
		printMetricKV("Avg resp size", s.AverageResponseSizeBytes, formatBytes)

		if s.ResponseTime != (client.Latency{}) {
			ui.Section("Latency")
			printMetricKV("Average", s.ResponseTime.Avg, formatDurationMs)
			printMetricKV("P50", s.ResponseTime.P50, formatDurationMs)
			printMetricKV("P90", s.ResponseTime.P90, formatDurationMs)
			printMetricKV("P95", s.ResponseTime.P95, formatDurationMs)
			printMetricKV("P99", s.ResponseTime.P99, formatDurationMs)
		}
	}

	we := stats.WorkerExecution
	if we != (client.WorkerExecution{}) {
		ui.Section("Worker")
		printKV("Supported", yesNo(we.Supported))
		printMetricKV("Executions", we.Executions, formatPlain)
		printMetricKV("Failures", we.Failures, formatPlain)
		printMetricKV("Duration", we.AverageDurationMs, formatDurationMs)
	}

	fmt.Fprintln(os.Stdout)
	return nil
}

func printMetricKV(key string, m client.Metric, format func(float64) string) {
	if m.Value == nil {
		return
	}
	out := format(*m.Value)
	if m.Delta != nil {
		out += "  ·  " + signedMetric(*m.Delta, format)
	}
	printKV(key, out)
}

func signedMetric(v float64, format func(float64) string) string {
	s := format(math.Abs(v))
	if v > 0 {
		return "+" + s
	}
	if v < 0 {
		return "-" + s
	}
	return s
}

func formatPercent(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

func formatDurationMs(v float64) string {
	if math.Abs(v) >= 1000 {
		return fmt.Sprintf("%.2f s", v/1000)
	}
	if math.Abs(v) >= 100 {
		return fmt.Sprintf("%.0f ms", v)
	}
	return fmt.Sprintf("%.1f ms", v)
}

func formatBytes(v float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	n := v
	i := 0
	for math.Abs(n) >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", n, units[i])
}

func formatBytesPerSec(v float64) string {
	return formatBytes(v) + "/s"
}

func formatRate(v float64) string {
	if math.Abs(v) >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	if math.Abs(v) >= 1 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.3f", v)
}

func formatPlain(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}
