package agents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testEnv(t *testing.T) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		Home:      home,
		ConfigDir: filepath.Join(home, "Library", "Application Support"),
		XDGConfig: filepath.Join(home, ".config"),
		CodexHome: filepath.Join(home, ".codex"),
		GOOS:      "linux",
		LookPath:  func(string) (string, error) { return "", errors.New("not found") },
		Run: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("unexpected command")
		},
	}
}

func agent(t *testing.T, id string) Agent {
	t.Helper()
	a, ok := Find(id)
	if !ok {
		t.Fatalf("agent %s not found", id)
	}
	return a
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCursorConnectKeepsOtherServersAndOrder(t *testing.T) {
	env := testEnv(t)
	cursor := agent(t, "cursor")
	path := cursor.ConfigPath(env)
	writeFile(t, path, `{"mcpServers":{"zeta":{"url":"https://z"},"alpha":{"command":"a","args":["-x"]}},"other":true}`)

	res, err := cursor.Connect(env, Options{APIKey: "br_test", TeamID: "team1"})
	if err != nil || !res.Changed {
		t.Fatalf("connect: %+v %v", res, err)
	}
	got := readFile(t, path)
	if strings.Index(got, `"zeta"`) > strings.Index(got, `"alpha"`) || !strings.Contains(got, `"other": true`) {
		t.Fatalf("existing config reordered or lost:\n%s", got)
	}
	for _, want := range []string{`"brimble"`, `"url": "https://mcp.brimble.io"`, `"x-brimble-key": "br_test"`, `"x-brimble-team": "team1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}

	st, err := cursor.Status(env)
	if err != nil || !st.Connected || !st.UsesKey {
		t.Fatalf("status = %+v, %v", st, err)
	}

	again, err := cursor.Connect(env, Options{APIKey: "br_test", TeamID: "team1"})
	if err != nil || again.Changed {
		t.Fatalf("reconnecting the same entry should be a no-op: %+v %v", again, err)
	}

	if res, err := cursor.Disconnect(env); err != nil || !res.Changed {
		t.Fatalf("disconnect: %+v %v", res, err)
	}
	got = readFile(t, path)
	if strings.Contains(got, "brimble") || !strings.Contains(got, `"zeta"`) || !strings.Contains(got, `"alpha"`) {
		t.Fatalf("disconnect should only remove brimble:\n%s", got)
	}
}

func TestOAuthEntryHasNoSecrets(t *testing.T) {
	env := testEnv(t)
	for _, id := range []string{"cursor", "vscode", "windsurf", "gemini", "opencode"} {
		a := agent(t, id)
		if _, err := a.Connect(env, Options{}); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		got := readFile(t, a.ConfigPath(env))
		if strings.Contains(got, "headers") {
			t.Errorf("%s: browser sign-in entry should have no headers:\n%s", id, got)
		}
		st, _ := a.Status(env)
		if !st.Connected || st.UsesKey {
			t.Errorf("%s: status = %+v", id, st)
		}
		info, err := os.Stat(a.ConfigPath(env))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s: new config mode = %v, want 0600", id, info.Mode().Perm())
		}
	}
}

func TestAgentSpecificShapes(t *testing.T) {
	env := testEnv(t)
	cases := map[string][]string{
		"vscode":   {`"servers"`, `"type": "http"`},
		"windsurf": {`"mcpServers"`, `"serverUrl"`},
		"gemini":   {`"mcpServers"`, `"httpUrl"`},
		"opencode": {`"mcp"`, `"type": "remote"`, `"enabled": true`},
		"zed":      {`"context_servers"`, `"url"`, `"x-brimble-key"`},
	}
	for id, wants := range cases {
		a := agent(t, id)
		if _, err := a.Connect(env, Options{APIKey: "br_k"}); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		got := readFile(t, a.ConfigPath(env))
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %s in:\n%s", id, w, got)
			}
		}
	}
}

func TestRefusesToRewriteCommentedJSON(t *testing.T) {
	env := testEnv(t)
	zed := agent(t, "zed")
	original := "// Zed settings\n{\n  \"theme\": \"One Dark\", // keep me\n}\n"
	writeFile(t, zed.ConfigPath(env), original)

	if _, err := zed.Connect(env, Options{APIKey: "br_k"}); err == nil || !strings.Contains(err.Error(), "comments") {
		t.Fatalf("expected a comments error, got %v", err)
	}
	if got := readFile(t, zed.ConfigPath(env)); got != original {
		t.Fatalf("file was modified:\n%s", got)
	}
}

func TestZedNeedsAPIKey(t *testing.T) {
	env := testEnv(t)
	if _, err := agent(t, "zed").Connect(env, Options{}); err == nil {
		t.Fatal("zed can't do browser sign-in; Connect without a key should fail")
	}
}

func TestCodexTOMLEdits(t *testing.T) {
	env := testEnv(t)
	codex := agent(t, "codex")
	path := codex.ConfigPath(env)
	original := `model = "o3"

[mcp_servers.linear]
url = "https://mcp.linear.app/mcp"

[mcp_servers.brimble]
url = "https://old.example"

[mcp_servers.brimble.env]
X = "1"

# Notion server
[mcp_servers.notion]
url = "https://mcp.notion.com/mcp"
`
	writeFile(t, path, original)

	res, err := codex.Connect(env, Options{APIKey: "br_k"})
	if err != nil || !res.Changed {
		t.Fatalf("connect: %+v %v", res, err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "old.example") || strings.Contains(got, "brimble.env") {
		t.Fatalf("old brimble tables should be replaced:\n%s", got)
	}
	for _, want := range []string{
		`model = "o3"`, "[mcp_servers.linear]", "# Notion server\n[mcp_servers.notion]",
		"[mcp_servers.brimble]\nurl = \"https://mcp.brimble.io\"\nhttp_headers = { \"x-brimble-key\" = \"br_k\" }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	st, _ := codex.Status(env)
	if !st.Connected || !st.UsesKey {
		t.Fatalf("status = %+v", st)
	}
	if again, _ := codex.Connect(env, Options{APIKey: "br_k"}); again.Changed {
		t.Fatal("reconnecting should be a no-op")
	}

	if _, err := codex.Disconnect(env); err != nil {
		t.Fatal(err)
	}
	got = readFile(t, path)
	if strings.Contains(got, "brimble") || !strings.Contains(got, "# Notion server") || !strings.Contains(got, "[mcp_servers.linear]") {
		t.Fatalf("disconnect should only remove brimble:\n%s", got)
	}
}

func TestClaudeCodeUsesItsCLI(t *testing.T) {
	env := testEnv(t)
	bin := filepath.Join(env.Home, ".local", "bin", "claude")
	writeFile(t, bin, "#!/bin/sh\n")
	var calls []string
	env.Run = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}

	claude := agent(t, "claude")
	if st, _ := claude.Status(env); !st.Installed || st.Connected {
		t.Fatalf("status before = %+v", st)
	}
	if _, err := claude.Connect(env, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "mcp add --transport http --scope user brimble https://mcp.brimble.io" {
		t.Fatalf("calls = %q", calls)
	}

	// Already registered with the same settings: nothing to do.
	writeFile(t, claude.ConfigPath(env), `{"mcpServers":{"brimble":{"type":"http","url":"https://mcp.brimble.io"}}}`)
	calls = nil
	if res, err := claude.Connect(env, Options{}); err != nil || res.Changed || len(calls) != 0 {
		t.Fatalf("expected no-op, got %+v %v %q", res, err, calls)
	}

	// Switching to an API key replaces the entry.
	if _, err := claude.Connect(env, Options{APIKey: "br_k"}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "mcp remove brimble") || !strings.Contains(calls[1], "--header x-brimble-key: br_k") {
		t.Fatalf("calls = %q", calls)
	}
}

func TestDetection(t *testing.T) {
	env := testEnv(t)
	for _, a := range All() {
		if st, _ := a.Status(env); st.Installed {
			t.Errorf("%s detected on an empty machine", a.ID)
		}
	}
	if err := os.MkdirAll(filepath.Join(env.Home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	env.LookPath = func(name string) (string, error) {
		if name == "gemini" {
			return "/usr/bin/gemini", nil
		}
		return "", errors.New("not found")
	}
	for _, id := range []string{"cursor", "gemini"} {
		if st, _ := agent(t, id).Status(env); !st.Installed {
			t.Errorf("%s should be detected", id)
		}
	}
}

func TestSnippetHidesKey(t *testing.T) {
	for _, id := range []string{"cursor", "codex", "claude-code"} {
		s := agent(t, id).Snippet(Options{APIKey: "br_secret"})
		if strings.Contains(s, "br_secret") || !strings.Contains(s, "<your Brimble API key>") {
			t.Errorf("%s snippet leaks or lacks placeholder:\n%s", id, s)
		}
	}
	if s := agent(t, "grok").Snippet(Options{}); strings.Contains(s, "%!") {
		t.Errorf("grok instructions are malformed: %s", s)
	}
}

func TestFindAliases(t *testing.T) {
	for alias, want := range map[string]string{"claude": "claude-code", "Cursor": "cursor", "code": "vscode", "VS Code (Copilot)": "vscode"} {
		if a, ok := Find(alias); !ok || a.ID != want {
			t.Errorf("Find(%q) = %v, %v", alias, a.ID, ok)
		}
	}
}
