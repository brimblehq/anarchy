package client

import (
	"strings"
	"testing"
)

func TestTerminalPTYURLGateway(t *testing.T) {
	u, err := terminalPTYURL("https://api.brimble.io/core", ResourceSandbox, "sbx123", "sess456", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	want := "wss://api.brimble.io/core/ws/sandboxes/sbx123/terminal/sessions/sess456/pty?cols=120&rows=40"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

func TestTerminalPTYURLProject(t *testing.T) {
	u, err := terminalPTYURL("https://api.brimble.io/core", ResourceProject, "prj1", "sess2", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/core/ws/projects/prj1/terminal/sessions/sess2/pty") {
		t.Fatalf("url = %q", u)
	}
	if !strings.HasPrefix(u, "wss://") {
		t.Fatalf("expected wss scheme, got %q", u)
	}
}

func TestTerminalPTYURLLocal(t *testing.T) {
	u, err := terminalPTYURL("http://localhost:3000", ResourceSandbox, "sbx1", "s1", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	want := "ws://localhost:3000/ws/sandboxes/sbx1/terminal/sessions/s1/pty?cols=80&rows=24"
	if u != want {
		t.Fatalf("url = %q, want %q", u, want)
	}
}

func TestTerminalPTYURLEscapesIDs(t *testing.T) {
	u, err := terminalPTYURL("https://api.brimble.io/core", ResourceSandbox, "sbx/1", "s ess", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u, "sbx/1") || strings.Contains(u, "s ess") {
		t.Fatalf("ids not escaped: %q", u)
	}
}
