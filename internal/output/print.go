package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
	keyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	emptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Italic(true)
)

func Print(w io.Writer, format string, data any) error {
	if w == nil {
		w = os.Stdout
	}
	if format == "json" {
		return printJSON(w, data)
	}
	return printTable(w, data)
}

func PrintJSON(w io.Writer, data any) error {
	if w == nil {
		w = os.Stdout
	}
	return printJSON(w, data)
}

func printJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func printTable(w io.Writer, data any) error {
	if data == nil {
		fmt.Fprintln(w, emptyStyle.Render("null"))
		return nil
	}

	switch v := data.(type) {
	case map[string]any:
		return printMap(w, v)
	case []any:
		return printSlice(w, v)
	default:
		return printJSON(w, data)
	}
}

func printMap(w io.Writer, m map[string]any) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	keys := sortedKeys(m)
	for _, k := range keys {
		fmt.Fprintf(tw, "%s\t%s\n", keyStyle.Render(k), formatCell(m[k]))
	}
	return tw.Flush()
}

func printSlice(w io.Writer, items []any) error {
	if len(items) == 0 {
		fmt.Fprintln(w, emptyStyle.Render("(empty)"))
		return nil
	}

	maps := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return printJSON(w, items)
		}
		maps = append(maps, m)
	}

	cols := preferredColumns(maps)
	if len(cols) == 0 {
		return printJSON(w, items)
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = headerStyle.Render(c)
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, m := range maps {
		cells := make([]string, len(cols))
		for i, col := range cols {
			v, _ := lookup(m, col)
			cells[i] = formatCell(v)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func preferredColumns(maps []map[string]any) []string {
	preferred := []string{"name", "slug", "status", "email", "region", "createdAt", "updatedAt"}
	skip := map[string]bool{
		"_id": true, "id": true, "__v": true,
	}
	seen := map[string]bool{}
	var cols []string

	for _, p := range preferred {
		for _, m := range maps {
			if _, ok := lookup(m, p); ok {
				if !seen[p] {
					seen[p] = true
					cols = append(cols, p)
				}
				break
			}
		}
	}

	if len(cols) > 0 {
		return cols
	}

	for k := range maps[0] {
		if skip[k] {
			continue
		}
		cols = append(cols, k)
		if len(cols) >= 6 {
			break
		}
	}
	return cols
}

func lookup(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	return nil, false
}

func formatCell(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		return fmt.Sprintf("%t", t)
	case map[string]any, []any:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		s := string(b)
		if len(s) > 48 {
			return s[:45] + "..."
		}
		return s
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr && rv.IsNil() {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func sortedKeys(m map[string]any) []string {
	preferred := []string{"name", "slug", "email", "status", "region", "createdAt", "updatedAt"}
	skip := map[string]bool{"_id": true, "id": true, "__v": true}
	var keys []string
	seen := map[string]bool{}
	for _, p := range preferred {
		if _, ok := m[p]; ok {
			keys = append(keys, p)
			seen[p] = true
		}
	}
	for k := range m {
		if seen[k] || skip[k] {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}
