package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brimblehq/anarchy/internal/auth"
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

func TestValidationErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": []any{
			map[string]any{"type": "field", "msg": "Project name should be provided", "path": "name", "location": "body"},
			map[string]any{"type": "field", "msg": "Invalid value", "path": "repo.branch", "location": "body"},
		}})
	}))
	defer srv.Close()

	c := NewHTTP(HTTPOptions{BaseURL: srv.URL})
	_, err := c.DoJSON(context.Background(), Request{Method: http.MethodPost, Path: "/v1/projects"}, nil)
	want := "name: Project name should be provided; repo.branch: Invalid value (HTTP 400)"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestDropUploadSendsMultipartArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/drop" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("name"); got != "site" {
			t.Errorf("name = %q", got)
		}
		if _, ok := r.MultipartForm.Value["environmentId"]; ok {
			t.Error("empty fields should be omitted")
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		if hdr.Filename != "site.zip" || string(b) != "zipbytes" {
			t.Errorf("file %q = %q", hdr.Filename, b)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "ok", "data": map[string]any{"logId": "L1"}})
	}))
	defer srv.Close()

	core := &Core{http: NewHTTP(HTTPOptions{BaseURL: srv.URL})}
	data, err := core.DropUpload(context.Background(), DropOpts{Name: "site", Archive: strings.NewReader("zipbytes")})
	if err != nil {
		t.Fatal(err)
	}
	if data.LogID != "L1" {
		t.Fatalf("data = %+v", data)
	}
}

func TestCreateEnvVarsBodyShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		envs, ok := body["environments"].([]any)
		if r.URL.Path != "/v1/envs/p1/PREVIEW" || !ok || len(envs) != 1 {
			t.Errorf("%s body = %v", r.URL.Path, body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "ok", "data": envs})
	}))
	defer srv.Close()

	core := &Core{http: NewHTTP(HTTPOptions{BaseURL: srv.URL})}
	if err := core.CreateEnvVars(context.Background(), "p1", "PREVIEW", []EnvVar{{Name: "A", Value: "1"}}, ""); err != nil {
		t.Fatal(err)
	}
}
