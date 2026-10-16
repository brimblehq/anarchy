package cmd

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/ui"
)

func printProjectStats(project string, stats client.ProjectStats) error {
	if project == "" {
		project = "project"
	}
	ui.Title(project + " · stats")

	if !stats.Supported {
		ui.Muted("  Metrics aren't available for this %s project.", cmp.Or(stats.ServiceType, "type of"))
		fmt.Fprintln(os.Stdout)
		return nil
	}
	if stats.Range.From != "" && stats.Range.To != "" {
		printKV("Window", formatClock(stats.Range.From)+" → "+formatClock(stats.Range.To))
		ui.Muted("  ↑/↓ compare with the previous window of the same length.")
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
	if *m.Value == 0 {
		out = "0"
	}
	// Delta is a relative change in percent, whatever the metric's unit.
	if m.Delta != nil && math.Abs(*m.Delta) >= 0.5 {
		arrow := "↑ "
		if *m.Delta < 0 {
			arrow = "↓ "
		}
		out += "  " + ui.Dim(arrow+strconv.FormatFloat(math.Round(math.Abs(*m.Delta)), 'f', -1, 64)+"%")
	}
	printKV(key, out)
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
