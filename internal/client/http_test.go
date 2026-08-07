package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brimbleapp/brimble-cli/internal/auth"
)

func TestDoJSONSuccessEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-brimble-key") != "br_test" {
			t.Errorf("missing api key header")
		}
		if r.Header.Get("x-brimble-team") != "team1" {
			t.Errorf("missing team header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "ok",
			"data":    map[string]any{"name": "app"},
		})
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{
		BaseURL:     srv.URL,
		Credentials: auth.Credentials{APIKey: "br_test", TeamID: "team1"},
	})

	var data map[string]any
	msg, err := c.DoJSON(context.Background(), Request{Method: http.MethodGet, Path: "/v1/projects"}, &data)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "ok" {
		t.Fatalf("message = %q", msg)
	}
	if data["name"] != "app" {
		t.Fatalf("data = %#v", data)
	}
}

func TestDoJSONAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "Invalid API key"})
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{BaseURL: srv.URL})
	_, err := c.DoJSON(context.Background(), Request{Method: http.MethodGet, Path: "/x"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("got %T", err)
	}
	if apiErr.Status != 401 || apiErr.Message != "Invalid API key" {
		t.Fatalf("got %+v", apiErr)
	}
}
