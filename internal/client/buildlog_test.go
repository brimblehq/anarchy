package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brimblehq/anarchy/internal/auth"
)

func sseServer(t *testing.T, handler func(w http.ResponseWriter, attempt int)) *Core {
	t.Helper()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/logs/stream/p1/l1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("x-brimble-key") != "br_k" {
			t.Errorf("headers = %v", r.Header)
		}
		handler(w, int(atomic.AddInt32(&attempts, 1)))
	}))
	t.Cleanup(srv.Close)
	return NewCore(srv.URL, 5*time.Second, testCreds(), "test")
}

func send(w http.ResponseWriter, s string) {
	fmt.Fprint(w, s)
	w.(http.Flusher).Flush()
}

func lineFrame(id, content string) string {
	return fmt.Sprintf("data: {\"type\":\"line\",\"id\":%q,\"content\":%q,\"timestamp\":\"t\"}\n\n", id, content)
}

func collect(t *testing.T, c *Core) ([]string, string, error) {
	t.Helper()
	var lines []string
	status, err := c.StreamBuildLogs(context.Background(), "p1", "l1", "", func(l BuildLogLine) {
		lines = append(lines, l.Content)
	})
	return lines, status, err
}

func TestStreamBuildLogsLinesThenEnd(t *testing.T) {
	c := sseServer(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Content-Type", "text/event-stream")
		send(w, ": open\n\n")
		send(w, lineFrame("1", "Cloning"))
		send(w, ": ping\n\n")
		send(w, "data: {\"type\":\"line\",\"id\":\"2\",\r\ndata: \"content\":\"Building\"}\r\n\r\n")
		send(w, `data: {"type":"end","status":"ACTIVE"}`+"\n\n")
		send(w, lineFrame("3", "after end is ignored"))
	})
	lines, status, err := collect(t, c)
	if err != nil || status != "ACTIVE" || strings.Join(lines, "|") != "Cloning|Building" {
		t.Fatalf("lines=%q status=%q err=%v", lines, status, err)
	}
}

func TestStreamBuildLogsReconnectsWithoutDuplicates(t *testing.T) {
	buildLogRetryDelay = time.Millisecond
	t.Cleanup(func() { buildLogRetryDelay = time.Second })

	c := sseServer(t, func(w http.ResponseWriter, attempt int) {
		send(w, lineFrame("1", "one"))
		send(w, lineFrame("2", "two"))
		if attempt == 1 {
			return // connection drops before "end"
		}
		send(w, lineFrame("3", "three"))
		send(w, `data: {"type":"end","status":"FAILED"}`+"\n\n")
	})
	lines, status, err := collect(t, c)
	if err != nil || status != "FAILED" || strings.Join(lines, "|") != "one|two|three" {
		t.Fatalf("lines=%q status=%q err=%v", lines, status, err)
	}
}

func TestStreamBuildLogsUnavailable(t *testing.T) {
	c := sseServer(t, func(w http.ResponseWriter, _ int) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"message":"Log streaming is unavailable"}`)
	})
	_, _, err := collect(t, c)
	if !IsStreamUnavailable(err) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestStreamBuildLogsDetectsStall(t *testing.T) {
	buildLogIdleTimeout, buildLogRetryDelay = 50*time.Millisecond, time.Millisecond
	t.Cleanup(func() { buildLogIdleTimeout, buildLogRetryDelay = 45*time.Second, time.Second })

	c := sseServer(t, func(w http.ResponseWriter, _ int) {
		send(w, ": open\n\n")
		time.Sleep(300 * time.Millisecond) // silent: no pings
	})
	_, _, err := collect(t, c)
	if err == nil || !strings.Contains(err.Error(), "no data for") {
		t.Fatalf("expected a stall error, got %v", err)
	}
}

func testCreds() auth.Credentials { return auth.Credentials{APIKey: "br_k"} }
