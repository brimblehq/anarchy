// Package agents detects coding agents on this machine and adds the Brimble
// MCP server to their configuration.
package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	ServerName = "brimble"
	DefaultURL = "https://mcp.brimble.io"
)

type Env struct {
	Home      string
	ConfigDir string
	XDGConfig string
	CodexHome string
	GOOS      string
	LookPath  func(string) (string, error)
	Run       func(name string, args ...string) ([]byte, error)
}

func LocalEnv(goos string) (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Env{}, err
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	return Env{
		Home: home, ConfigDir: configDir, XDGConfig: xdg, CodexHome: codexHome, GOOS: goos,
		LookPath: exec.LookPath,
		Run: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
	}, nil
}

type Options struct {
	URL    string
	APIKey string // empty → browser OAuth
	TeamID string
}

func (o Options) headers() map[string]string {
	h := map[string]string{}
	if o.APIKey != "" {
		h["x-brimble-key"] = o.APIKey
	}
	if o.TeamID != "" {
		h["x-brimble-team"] = o.TeamID
	}
	return h
}

type Status struct {
	Installed bool
	Connected bool
	UsesKey   bool
	Path      string
}

type Result struct {
	Changed bool
	Path    string
}

type kind int

const (
	kindJSON kind = iota
	kindCodex
	kindClaude
	kindManual
)

type Agent struct {
	ID      string
	Name    string
	Aliases []string
	OAuth   bool

	kind       kind
	serversKey string
	entry      func(Options) serverEntry
	path       func(Env) string
	detect     func(Env) bool

	NextOAuth string
	NextKey   string
	Manual    string
}

func (a Agent) IsManual() bool { return a.kind == kindManual }

func (a Agent) Next(opts Options) string {
	if opts.APIKey != "" || !a.OAuth {
		return a.NextKey
	}
	return a.NextOAuth
}

func (a Agent) ConfigPath(env Env) string {
	if a.path == nil {
		return ""
	}
	return a.path(env)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func onPath(env Env, names ...string) bool {
	for _, n := range names {
		if _, err := env.LookPath(n); err == nil {
			return true
		}
	}
	return false
}

func macApp(env Env, name string) bool {
	return env.GOOS == "darwin" &&
		(exists("/Applications/"+name+".app") || exists(filepath.Join(env.Home, "Applications", name+".app")))
}

func claudeBinary(env Env) string {
	if p, err := env.LookPath("claude"); err == nil {
		return p
	}
	for _, p := range []string{
		filepath.Join(env.Home, ".local", "bin", "claude"),
		filepath.Join(env.Home, ".claude", "local", "claude"),
	} {
		if exists(p) {
			return p
		}
	}
	return ""
}

func codeUserDir(env Env) string {
	return filepath.Join(env.ConfigDir, "Code", "User")
}

func zedSettings(env Env) string {
	if env.GOOS == "windows" {
		return filepath.Join(env.ConfigDir, "Zed", "settings.json")
	}
	return filepath.Join(env.XDGConfig, "zed", "settings.json")
}

func All() []Agent {
	return []Agent{
		{
			ID: "claude-code", Name: "Claude Code", Aliases: []string{"claude"}, OAuth: true,
			kind:      kindClaude,
			path:      func(e Env) string { return filepath.Join(e.Home, ".claude.json") },
			detect:    func(e Env) bool { return claudeBinary(e) != "" },
			NextOAuth: "In Claude Code, run /mcp, choose brimble and sign in",
			NextKey:   "Start a new Claude Code session to use it",
		},
		{
			ID: "cursor", Name: "Cursor", OAuth: true,
			kind: kindJSON, serversKey: "mcpServers",
			entry: func(o Options) serverEntry { return withHeaders(serverEntry{URL: o.URL}, o) },
			path:  func(e Env) string { return filepath.Join(e.Home, ".cursor", "mcp.json") },
			detect: func(e Env) bool {
				return exists(filepath.Join(e.Home, ".cursor")) || onPath(e, "cursor") || macApp(e, "Cursor")
			},
			NextOAuth: "In Cursor, open Settings → MCP and click Connect next to brimble",
			NextKey:   "Cursor picks it up from Settings → MCP (restart Cursor if it doesn't appear)",
		},
		{
			ID: "vscode", Name: "VS Code (Copilot)", Aliases: []string{"code", "vs-code", "copilot"}, OAuth: true,
			kind: kindJSON, serversKey: "servers",
			entry: func(o Options) serverEntry {
				return withHeaders(serverEntry{Type: "http", URL: o.URL}, o)
			},
			path: func(e Env) string { return filepath.Join(codeUserDir(e), "mcp.json") },
			detect: func(e Env) bool {
				return exists(codeUserDir(e)) || onPath(e, "code") || macApp(e, "Visual Studio Code")
			},
			NextOAuth: "In VS Code, run “MCP: List Servers”, start brimble and sign in",
			NextKey:   "In VS Code, run “MCP: List Servers” and start brimble",
		},
		{
			ID: "windsurf", Name: "Windsurf", OAuth: true,
			kind: kindJSON, serversKey: "mcpServers",
			entry: func(o Options) serverEntry { return withHeaders(serverEntry{ServerURL: o.URL}, o) },
			path:  func(e Env) string { return filepath.Join(e.Home, ".codeium", "windsurf", "mcp_config.json") },
			detect: func(e Env) bool {
				return exists(filepath.Join(e.Home, ".codeium", "windsurf")) || onPath(e, "windsurf") || macApp(e, "Windsurf")
			},
			NextOAuth: "In Windsurf, refresh MCP servers in Cascade and sign in to brimble",
			NextKey:   "In Windsurf, refresh MCP servers in Cascade",
		},
		{
			ID: "codex", Name: "Codex", Aliases: []string{"codex-cli"}, OAuth: true,
			kind:      kindCodex,
			path:      func(e Env) string { return filepath.Join(e.CodexHome, "config.toml") },
			detect:    func(e Env) bool { return onPath(e, "codex") || exists(e.CodexHome) },
			NextOAuth: "Run `codex mcp login brimble` to sign in",
			NextKey:   "Start a new Codex session to use it",
		},
		{
			ID: "gemini", Name: "Gemini CLI", Aliases: []string{"gemini-cli"}, OAuth: true,
			kind: kindJSON, serversKey: "mcpServers",
			entry: func(o Options) serverEntry { return withHeaders(serverEntry{HTTPURL: o.URL}, o) },
			path:  func(e Env) string { return filepath.Join(e.Home, ".gemini", "settings.json") },
			detect: func(e Env) bool {
				return onPath(e, "gemini") || exists(filepath.Join(e.Home, ".gemini"))
			},
			NextOAuth: "In Gemini CLI, run /mcp auth brimble (Gemini only loads MCP servers in trusted folders)",
			NextKey:   "Restart Gemini CLI to use it (Gemini only loads MCP servers in trusted folders)",
		},
		{
			ID: "opencode", Name: "opencode", OAuth: true,
			kind: kindJSON, serversKey: "mcp",
			entry: func(o Options) serverEntry {
				return withHeaders(serverEntry{Type: "remote", URL: o.URL, Enabled: true}, o)
			},
			path: func(e Env) string { return filepath.Join(e.XDGConfig, "opencode", "opencode.json") },
			detect: func(e Env) bool {
				return onPath(e, "opencode") || exists(filepath.Join(e.Home, ".opencode", "bin", "opencode")) ||
					exists(filepath.Join(e.XDGConfig, "opencode"))
			},
			NextOAuth: "Run `opencode mcp auth brimble` to sign in",
			NextKey:   "Restart opencode to use it",
		},
		{
			ID: "zed", Name: "Zed",
			kind: kindJSON, serversKey: "context_servers",
			entry: func(o Options) serverEntry { return withHeaders(serverEntry{URL: o.URL}, o) },
			path:  zedSettings,
			detect: func(e Env) bool {
				return onPath(e, "zed") || macApp(e, "Zed") || exists(filepath.Dir(zedSettings(e)))
			},
			NextKey: "Zed loads it automatically; check the Agent panel's settings",
		},
		{
			ID: "claude-desktop", Name: "Claude Desktop", Aliases: []string{"claude-app"}, OAuth: true,
			kind: kindManual,
			detect: func(e Env) bool {
				return macApp(e, "Claude") || exists(filepath.Join(e.ConfigDir, "Claude"))
			},
			Manual: "Open Claude Desktop → Settings → Connectors → Add custom connector.\n" +
				"Name it Brimble, set the URL to %s, then click Connect and sign in.",
		},
		{
			ID: "grok", Name: "Grok Build", OAuth: true,
			kind: kindManual,
			detect: func(e Env) bool {
				return onPath(e, "grok") || exists(filepath.Join(e.Home, ".grok"))
			},
			Manual: "In Grok Build, open /plugins, search for Brimble and install it.\n" +
				"Grok opens Brimble sign-in in your browser on first use.",
		},
	}
}

// Agents use different JSON keys for the same URL (url / serverUrl / httpUrl).
type serverEntry struct {
	Type      string            `json:"type,omitempty"`
	URL       string            `json:"url,omitempty"`
	ServerURL string            `json:"serverUrl,omitempty"`
	HTTPURL   string            `json:"httpUrl,omitempty"`
	Enabled   bool              `json:"enabled,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

func withHeaders(entry serverEntry, o Options) serverEntry {
	if h := o.headers(); len(h) > 0 {
		entry.Headers = h
	}
	return entry
}

func (e *serverEntry) usesKey() bool {
	if e == nil {
		return false
	}
	_, ok := e.Headers["x-brimble-key"]
	if !ok {
		_, ok = e.Headers["Authorization"]
	}
	return ok
}

func Find(key string) (Agent, bool) {
	k := strings.ToLower(strings.TrimSpace(key))
	for _, a := range All() {
		if k == a.ID || strings.ToLower(a.Name) == k {
			return a, true
		}
		for _, alias := range a.Aliases {
			if k == alias {
				return a, true
			}
		}
	}
	return Agent{}, false
}

func IDs() []string {
	var out []string
	for _, a := range All() {
		out = append(out, a.ID)
	}
	return out
}

func (a Agent) Status(env Env) (Status, error) {
	st := Status{Installed: a.detect(env), Path: a.ConfigPath(env)}
	switch a.kind {
	case kindJSON:
		entry, err := a.jsonServer(env).read()
		if err != nil {
			return st, err
		}
		st.Connected = entry != nil
		st.UsesKey = entry.usesKey()
	case kindClaude:
		entry, err := jsonServer{path: st.Path, serversKey: "mcpServers", name: ServerName}.read()
		if err != nil {
			return st, err
		}
		st.Connected = entry != nil
		st.UsesKey = entry.usesKey()
	case kindCodex:
		entry, err := a.codexServer(env).read()
		if err != nil {
			return st, err
		}
		st.Connected = entry != nil
		st.UsesKey = entry != nil && len(entry.Headers) > 0
	}
	return st, nil
}

func (a Agent) jsonServer(env Env) jsonServer {
	return jsonServer{path: a.ConfigPath(env), serversKey: a.serversKey, name: ServerName}
}

func (a Agent) codexServer(env Env) codexServer {
	return codexServer{path: a.ConfigPath(env), name: ServerName}
}

var ErrManual = errors.New("finish setup in the app")

func (a Agent) Connect(env Env, opts Options) (Result, error) {
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	if !a.OAuth && opts.APIKey == "" {
		return Result{}, fmt.Errorf("%s can't sign in through the browser; connect it with an API key", a.Name)
	}
	path := a.ConfigPath(env)
	switch a.kind {
	case kindJSON:
		entry := a.entry(opts)
		changed, err := a.jsonServer(env).write(&entry)
		return Result{Changed: changed, Path: path}, err
	case kindCodex:
		changed, err := a.codexServer(env).write(&codexEntry{URL: opts.URL, Headers: opts.headers()})
		return Result{Changed: changed, Path: path}, err
	case kindClaude:
		return a.connectClaude(env, opts)
	}
	return Result{}, ErrManual
}

func (a Agent) connectClaude(env Env, opts Options) (Result, error) {
	bin := claudeBinary(env)
	if bin == "" {
		return Result{}, fmt.Errorf("the claude command was not found")
	}
	path := a.ConfigPath(env)
	if entry, err := (jsonServer{path: path, serversKey: "mcpServers", name: ServerName}).read(); err == nil && entry != nil {
		if sameClaudeEntry(entry, opts) {
			return Result{Path: path}, nil
		}
		if out, err := env.Run(bin, "mcp", "remove", ServerName, "--scope", "user"); err != nil {
			return Result{}, fmt.Errorf("claude mcp remove: %s", strings.TrimSpace(string(out)))
		}
	}
	args := []string{"mcp", "add", "--transport", "http", "--scope", "user", ServerName, opts.URL}
	for k, v := range opts.headers() {
		args = append(args, "--header", k+": "+v)
	}
	if out, err := env.Run(bin, args...); err != nil {
		return Result{}, fmt.Errorf("claude mcp add: %s", strings.TrimSpace(string(out)))
	}
	return Result{Changed: true, Path: path}, nil
}

func sameClaudeEntry(entry *serverEntry, opts Options) bool {
	if entry == nil || entry.URL != opts.URL {
		return false
	}
	want := opts.headers()
	if len(entry.Headers) != len(want) {
		return false
	}
	for k, v := range want {
		if entry.Headers[k] != v {
			return false
		}
	}
	return true
}

func (a Agent) Disconnect(env Env) (Result, error) {
	path := a.ConfigPath(env)
	switch a.kind {
	case kindJSON:
		changed, err := a.jsonServer(env).write(nil)
		return Result{Changed: changed, Path: path}, err
	case kindCodex:
		changed, err := a.codexServer(env).write(nil)
		return Result{Changed: changed, Path: path}, err
	case kindClaude:
		st, err := a.Status(env)
		if err != nil || !st.Connected {
			return Result{Path: path}, err
		}
		bin := claudeBinary(env)
		if bin == "" {
			return Result{}, fmt.Errorf("the claude command was not found")
		}
		if out, err := env.Run(bin, "mcp", "remove", ServerName, "--scope", "user"); err != nil {
			return Result{}, fmt.Errorf("claude mcp remove: %s", strings.TrimSpace(string(out)))
		}
		return Result{Changed: true, Path: path}, nil
	}
	return Result{}, ErrManual
}

// Snippet redacts the API key so it never lands in scrollback.
func (a Agent) Snippet(opts Options) string {
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	if opts.APIKey != "" {
		opts.APIKey = "<your Brimble API key>"
	}
	switch a.kind {
	case kindJSON:
		entry, err := marshal(a.entry(opts))
		if err != nil {
			return ""
		}
		servers, err := object{{Key: ServerName, Value: entry}}.MarshalJSON()
		if err != nil {
			return ""
		}
		root, err := object{{Key: a.serversKey, Value: servers}}.MarshalJSON()
		if err != nil {
			return ""
		}
		return indentJSON(root)
	case kindCodex:
		return codexEntry{URL: opts.URL, Headers: opts.headers()}.toml(ServerName)
	case kindClaude:
		cmd := "claude mcp add --transport http --scope user " + ServerName + " " + opts.URL
		for k, v := range opts.headers() {
			cmd += fmt.Sprintf(" --header %q", k+": "+v)
		}
		return cmd
	}
	return a.Instructions(opts.URL)
}

func (a Agent) Instructions(url string) string {
	if strings.Contains(a.Manual, "%s") {
		return fmt.Sprintf(a.Manual, url)
	}
	return a.Manual
}

func indentJSON(b []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return string(b)
	}
	return out.String()
}
