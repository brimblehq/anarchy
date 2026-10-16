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

func printDatabaseInsights(project string, in client.DatabaseInsights) error {
	if project == "" {
		project = "database"
	}
	ui.Title(project + " · stats")

	if !in.Supported {
		ui.Muted("  Metrics aren't available for this %s database.", cmp.Or(in.EngineName, in.Engine, "type of"))
		fmt.Fprintln(os.Stdout)
		return nil
	}
	if in.Range.From != "" && in.Range.To != "" {
		printKV("Window", formatClock(in.Range.From)+" → "+formatClock(in.Range.To))
		ui.Muted("  ↑/↓ compare with the previous window of the same length.")
	}

	s := in.Summary
	ui.Section("Resources")
	printMetricKV("CPU", s.CPU, formatPercent)
	printMetricKV("Memory", s.Memory, formatBytes)
	printMetricKV("Disk", s.Disk, formatPercent)
	printMetricKV("IOPS", s.IOPS, formatRate)

	ui.Section("Queries")
	printMetricKV("Ops/sec", s.Throughput, formatRate)
	printMetricKV("Error rate", scaled(s.ErrorRate, 100), formatPercent)
	printMetricKV("Connections", s.Connections, formatPlain)
	if in.HA {
		printMetricKV("Replica lag", scaled(s.ReplicationLag, 1000), formatDurationMs)
	}

	if len(in.Members) > 0 {
		ui.Section("Members")
		rows := make([][]string, 0, len(in.Members))
		for _, m := range in.Members {
			health, lag := "—", "—"
			if m.Healthy != nil {
				health = map[bool]string{true: "healthy", false: "unhealthy"}[*m.Healthy]
			}
			if m.LagSeconds != nil {
				lag = formatDurationMs(*m.LagSeconds * 1000)
			}
			rows = append(rows, []string{strconv.Itoa(m.Index), m.Role, health, lag})
		}
		ui.Table([]string{"#", "ROLE", "HEALTH", "LAG"}, rows)
	}

	if ops := in.TopOperations; len(ops) > 0 {
		ui.Section("Top operations")
		if len(ops) > 5 {
			ops = ops[:5]
		}
		opt := func(v *float64, f func(float64) string) string {
			if v == nil {
				return "—"
			}
			return f(*v)
		}
		rows := make([][]string, 0, len(ops))
		for _, op := range ops {
			rows = append(rows, []string{
				truncate(op.Operation, 60),
				opt(op.Calls, formatPlain),
				opt(op.MeanMs, formatDurationMs),
				opt(op.PercentOfDBTime, formatPercent),
			})
		}
		ui.Table([]string{"OPERATION", "CALLS", "MEAN", "DB TIME"}, rows)
	}

	fmt.Fprintln(os.Stdout)
	return nil
}

// scaled converts a metric's unit; the delta is relative, so it stays.
func scaled(m client.Metric, by float64) client.Metric {
	if m.Value != nil {
		v := *m.Value * by
		m.Value = &v
	}
	return m
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
