package cmd

import (
	"errors"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

func TestSanitizeNetworkError(t *testing.T) {
	opErr := &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET},
	}
	uerr := &url.Error{Op: "Get", URL: "https://api.brimble.io/core/v1/projects", Err: opErr}

	got := sanitizeNetworkError(uerr)
	if got == nil {
		t.Fatal("expected a sanitized error")
	}
	want := "could not reach api.brimble.io: connection reset by peer"
	if got.Error() != want {
		t.Fatalf("got %q, want %q", got.Error(), want)
	}
}

func TestSanitizeNetworkErrorBare(t *testing.T) {
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	got := sanitizeNetworkError(opErr)
	if got == nil || got.Error() != "connection failed: connection refused" {
		t.Fatalf("got %v", got)
	}
}

func TestSanitizeNetworkErrorIgnoresOther(t *testing.T) {
	if got := sanitizeNetworkError(errors.New("some other error")); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
