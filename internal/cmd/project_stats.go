package cmd

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/brimbleapp/brimble-cli/internal/ui"
)

func printProjectStats(project string, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return printResult(data)
	}

	if project == "" {
		project = "project"
	}
	ui.Title(project + " · stats")

	printKV("Type", firstString(m, "serviceType"))
	printKV("Live", formatBool(m["live"]))
	printKV("Supported", formatBool(m["supported"]))

	if rng, ok := m["range"].(map[string]any); ok && len(rng) > 0 {
		ui.Section("Range")
		printKV("From", formatTime(firstString(rng, "from")))
		printKV("To", formatTime(firstString(rng, "to")))
		if step := firstString(rng, "step"); step != "" {
			printKV("Step", step+"s")
		}
	}

	summary, _ := m["summary"].(map[string]any)
	if len(summary) > 0 {
		ui.Section("Resources")
		printMetricKV("CPU", summary["cpu"], formatPercent)
		printMetricKV("Memory", summary["memory"], formatPercent)
		printMetricKV("Network", summary["network"], formatBytesPerSec)
		printMetricKV("Disk", summary["disk"], formatPercent)
		printMetricKV("OOM kills", summary["oomKills"], formatPlain)

		ui.Section("Traffic")
		printMetricKV("Requests/sec", summary["requestsPerSecond"], formatRate)
		printMetricKV("Error rate", summary["errorRate"], formatPercent)
		printMetricKV("Avg response", summary["averageResponseTimeMs"], formatDurationMs)
		printMetricKV("Avg resp size", summary["averageResponseSizeBytes"], formatBytes)

		if rt, ok := summary["responseTime"].(map[string]any); ok && len(rt) > 0 {
			ui.Section("Latency")
			printMetricKV("Average", rt["avg"], formatDurationMs)
			printMetricKV("P50", rt["p50"], formatDurationMs)
			printMetricKV("P90", rt["p90"], formatDurationMs)
			printMetricKV("P95", rt["p95"], formatDurationMs)
			printMetricKV("P99", rt["p99"], formatDurationMs)
		}
	}

	if we, ok := m["workerExecution"].(map[string]any); ok && len(we) > 0 {
		ui.Section("Worker")
		printKV("Supported", formatBool(we["supported"]))
		printMetricKV("Executions", we["executions"], formatPlain)
		printMetricKV("Failures", we["failures"], formatPlain)
		printMetricKV("Duration", we["averageDurationMs"], formatDurationMs)
		for _, k := range sortedMapKeys(we) {
			switch k {
			case "supported", "executions", "failures", "averageDurationMs":
				continue
			}
			if s := formatScalar(we[k]); s != "" {
				printKV(k, s)
			}
		}
	}

	fmt.Fprintln(os.Stdout)
	return nil
}

func printMetricKV(key string, raw any, format func(float64) string) {
	m, ok := raw.(map[string]any)
	if !ok {
		return
	}
	val, ok := asFloat(m["value"])
	if !ok {
		return
	}
	out := format(val)
	if delta, ok := asFloat(m["delta"]); ok {
		out += "  ·  " + signedMetric(delta, format)
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

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	default:
		return 0, false
	}
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

func formatScalar(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return formatBool(t)
	case float64, float32, int, int64:
		if f, ok := asFloat(t); ok {
			return formatPlain(f)
		}
	case map[string]any, []any:
		return ""
	}
	return fmt.Sprint(v)
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
