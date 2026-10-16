package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	eio "github.com/zishang520/engine.io-client-go/transports"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-client-go/socket"
)

type LogLine struct {
	TimestampNs string
	EpochMs     int64
	Message     string
	Labels      map[string]string
}

type StreamLogsOpts struct {
	ProjectID   string
	Query       string
	StartNs     int64
	Limit       int
	APIKey      string
	CoreBaseURL string
	OnLine      func(LogLine)
	OnError     func(error)
	// OnReady runs once the server's tail is open and lines can arrive.
	OnReady func()
	// Debug, when set, receives a trace of socket events (BRIMBLE_DEBUG=1).
	Debug func(format string, args ...any)
}

func StreamLogs(ctx context.Context, opts StreamLogsOpts) error {
	if opts.ProjectID == "" {
		return fmt.Errorf("project id is required")
	}
	if opts.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	if opts.OnLine == nil {
		return fmt.Errorf("OnLine handler is required")
	}
	if opts.Limit <= 0 {
		opts.Limit = 150
	}
	if opts.StartNs <= 0 {
		opts.StartNs = time.Now().Add(-1 * time.Hour).UnixNano()
	}
	debug := opts.Debug
	if debug == nil {
		debug = func(string, ...any) {}
	}

	uri, path, err := logsSocketEndpoint(opts.CoreBaseURL)
	if err != nil {
		return err
	}

	sockOpts := socket.DefaultOptions()
	sockOpts.SetPath(path)
	sockOpts.SetAuth(map[string]any{"token": opts.APIKey})
	// WebSocket only: the HTTP long-polling transport of engine.io-client-go
	// v1.1.0 panics on zstd-compressed responses (it passes a nil option to
	// zstd.NewReader), which Brimble's edge sends.
	sockOpts.SetTransports(types.NewSet(eio.WebSocket))
	sockOpts.SetReconnection(false)
	sockOpts.SetTimeout(10 * time.Second)
	sockOpts.SetAutoConnect(true)

	s, err := socket.Connect(uri+"/loki", sockOpts)
	if err != nil {
		return fmt.Errorf("socket connect setup: %w", err)
	}

	var (
		once     sync.Once
		fatalErr error
		done     = make(chan struct{})
		fail     = func(err error) {
			once.Do(func() {
				fatalErr = err
				close(done)
			})
		}
	)

	emitTail := func(...any) {
		params := map[string]any{
			"projectId": opts.ProjectID,
			"start":     strconv.FormatInt(opts.StartNs, 10),
			"limit":     strconv.Itoa(opts.Limit),
			"delay_for": "0",
		}
		if opts.Query != "" {
			params["query"] = opts.Query
		}
		debug("connected; emitting tail %v", params)
		s.Emit("tail", params)
	}
	// Core emits "connected" once its upstream Loki tail is open; the tail
	// request itself goes out on the socket "connect" event.
	s.On("connect", emitTail)
	var ready sync.Once
	s.On("connected", func(...any) {
		debug("server opened the Loki tail")
		if opts.OnReady != nil {
			ready.Do(opts.OnReady)
		}
	})
	s.On("disconnected", func(args ...any) {
		debug("server closed the Loki tail: %v", args)
		fail(errors.New("log stream closed by the server"))
	})
	s.On("logs", func(args ...any) {
		if len(args) == 0 {
			return
		}
		lines := parseLokiPayload(args[0])
		debug("logs event: %d lines (payload %T)", len(lines), args[0])
		for _, line := range lines {
			opts.OnLine(line)
		}
	})

	s.On("error", func(args ...any) {
		debug("error event: %v", args)
		msg := extractSocketMessage(args)
		if msg == "" {
			msg = "log stream error"
		}
		if opts.OnError != nil {
			opts.OnError(errors.New(msg))
		}
	})

	s.On("connect_error", func(args ...any) {
		debug("connect_error: %v", args)
		msg := extractSocketMessage(args)
		if msg == "" {
			msg = "connection refused"
		}
		fail(fmt.Errorf("could not connect to the log stream: %s", msg))
	})

	s.On("disconnect", func(args ...any) {
		debug("disconnect: %v", args)
		msg := extractSocketMessage(args)
		// Only our own Close() is a clean end; anything else lost the stream.
		if msg == "io client disconnect" {
			fail(nil)
			return
		}
		if msg == "" {
			msg = "connection lost"
		}
		fail(fmt.Errorf("log stream disconnected: %s", msg))
	})

	select {
	case <-ctx.Done():
		s.Close()
		return nil
	case <-done:
		s.Close()
		return fatalErr
	}
}

func logsSocketEndpoint(coreBaseURL string) (origin string, socketIOPath string, err error) {
	if coreBaseURL == "" {
		return "", "", fmt.Errorf("core base URL is empty")
	}
	u, err := url.Parse(coreBaseURL)
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid core base URL %q", coreBaseURL)
	}
	origin = u.Scheme + "://" + u.Host
	prefix := strings.TrimSuffix(u.Path, "/")
	if prefix == "" || prefix == "/" {
		socketIOPath = "/socket.io/"
	} else {
		socketIOPath = prefix + "/socket.io/"
	}
	return origin, socketIOPath, nil
}

func parseLokiPayload(payload any) []LogLine {
	var raw []byte
	switch t := payload.(type) {
	case string:
		raw = []byte(t)
	case []byte:
		raw = t
	default:
		var err error
		raw, err = json.Marshal(t)
		if err != nil {
			return nil
		}
	}
	var envelope struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}

	var lines []LogLine
	for _, stream := range envelope.Streams {
		for _, tuple := range stream.Values {
			if len(tuple) < 2 {
				continue
			}
			msg := strings.TrimSpace(tuple[1])
			if msg == "" {
				continue
			}
			ns := tuple[0]
			lines = append(lines, LogLine{
				TimestampNs: ns,
				EpochMs:     nsToEpochMs(ns),
				Message:     msg,
				Labels:      stream.Stream,
			})
		}
	}
	return lines
}

func nsToEpochMs(raw string) int64 {
	if raw == "" {
		return 0
	}
	var ns int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0
		}
		ns = ns*10 + int64(c-'0')
	}
	return ns / 1_000_000
}

func extractSocketMessage(args []any) string {
	for _, a := range args {
		switch t := a.(type) {
		case string:
			if t != "" {
				return t
			}
		case error:
			return t.Error()
		case map[string]any:
			if m, ok := t["message"].(string); ok && m != "" {
				return m
			}
		}
	}
	return ""
}
