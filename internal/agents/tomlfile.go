package agents

import (
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// Only [mcp_servers.<name>] (and its sub-tables) is rewritten; the rest of the
// Codex config.toml is kept byte for byte.
type codexServer struct {
	path string
	name string
}

type codexEntry struct {
	URL     string
	Headers map[string]string
}

func (s codexServer) isOwnTable(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "[") || strings.HasPrefix(t, "[[") {
		return false
	}
	t = strings.TrimSuffix(strings.TrimPrefix(t, "["), "]")
	t = strings.TrimSpace(t)
	for _, prefix := range []string{"mcp_servers." + s.name, `mcp_servers."` + s.name + `"`} {
		if t == prefix || strings.HasPrefix(t, prefix+".") {
			return true
		}
	}
	return false
}

func (s codexServer) split(content string) (rest []string, own []string) {
	inOwn := false
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			wasOwn := inOwn
			inOwn = s.isOwnTable(line)
			if wasOwn && !inOwn {
				i := len(own)
				for i > 0 {
					p := strings.TrimSpace(own[i-1])
					if p != "" && !strings.HasPrefix(p, "#") {
						break
					}
					i--
				}
				rest = append(rest, own[i:]...)
				own = own[:i]
			}
		}
		if inOwn {
			own = append(own, line)
		} else {
			rest = append(rest, line)
		}
	}
	return rest, own
}

func (s codexServer) read() (*codexEntry, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, own := s.split(string(b))
	if len(own) == 0 {
		return nil, nil
	}
	e := &codexEntry{Headers: map[string]string{}}
	for _, line := range own {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "url":
			e.URL = tomlUnquote(strings.TrimSpace(value))
		case "http_headers":
			e.Headers["(set)"] = ""
		}
	}
	return e, nil
}

func (s codexServer) write(entry *codexEntry) (bool, error) {
	b, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	content := string(b)
	rest, own := s.split(content)

	var block string
	if entry != nil {
		block = entry.toml(s.name)
		if strings.TrimSpace(strings.Join(own, "\n")) == strings.TrimSpace(block) {
			return false, nil
		}
	} else if len(own) == 0 {
		return false, nil
	}

	out := strings.TrimRight(strings.Join(rest, "\n"), "\n")
	if block != "" {
		if out != "" {
			out += "\n\n"
		}
		out += block
	}
	if out != "" {
		out += "\n"
	}
	return true, writeFileAtomic(s.path, []byte(out))
}

func (e codexEntry) toml(name string) string {
	var b strings.Builder
	b.WriteString("[mcp_servers." + name + "]\n")
	b.WriteString("url = " + tomlQuote(e.URL))
	if len(e.Headers) > 0 {
		keys := make([]string, 0, len(e.Headers))
		for k := range e.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, tomlQuote(k)+" = "+tomlQuote(e.Headers[k]))
		}
		b.WriteString("\nhttp_headers = { " + strings.Join(parts, ", ") + " }")
	}
	return b.String()
}

func tomlQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

func tomlUnquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s)
}
