package client

import (
	"testing"
)

func TestLogsSocketEndpointGateway(t *testing.T) {
	origin, path, err := logsSocketEndpoint("https://api.brimble.io/core")
	if err != nil {
		t.Fatal(err)
	}
	if origin != "https://api.brimble.io" {
		t.Fatalf("origin = %q", origin)
	}
	if path != "/core/socket.io/" {
		t.Fatalf("path = %q", path)
	}
}

func TestLogsSocketEndpointLocal(t *testing.T) {
	origin, path, err := logsSocketEndpoint("http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if origin != "http://localhost:3000" {
		t.Fatalf("origin = %q", origin)
	}
	if path != "/socket.io/" {
		t.Fatalf("path = %q", path)
	}
}

func TestParseLokiPayload(t *testing.T) {
	payload := map[string]any{
		"streams": []any{
			map[string]any{
				"stream": map[string]any{"project": "abc"},
				"values": []any{
					[]any{"1700000000000000000", "hello world"},
					[]any{"1700000001000000000", "  "},
				},
			},
		},
	}
	lines := parseLokiPayload(payload)
	if len(lines) != 1 {
		t.Fatalf("got %d lines", len(lines))
	}
	if lines[0].Message != "hello world" {
		t.Fatalf("message = %q", lines[0].Message)
	}
	if lines[0].EpochMs == 0 {
		t.Fatal("expected epoch ms")
	}
}
