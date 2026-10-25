package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestNilBodySendsNothing(t *testing.T) {
	var gotBody []byte
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotType = r.Header.Get("Content-Type")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "ok", "data": map[string]any{"logId": "L1"}})
	}))
	defer srv.Close()

	core := &Core{http: NewHTTP(HTTPOptions{BaseURL: srv.URL})}
	if _, err := core.RedeployProject(context.Background(), "p1", nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(gotBody) != 0 || gotType != "" {
		t.Fatalf("redeploy without settings sent body %q with Content-Type %q; Core rejects null", gotBody, gotType)
	}

	if _, err := core.RedeployProject(context.Background(), "p1", json.RawMessage(`{"startOnly":true}`), ""); err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != `{"startOnly":true}` || gotType != "application/json" {
		t.Fatalf("body = %q, type = %q", gotBody, gotType)
	}
}

func TestRollbackAndDuplicateRequests(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.Path+" "+r.Header.Get("x-brimble-team")+" "+string(body))
		data := map[string]any{"logId": "L2", "rollbackOf": "L1", "status": "queued", "projectId": "p2", "domainsCloned": 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "ok", "data": data})
	}))
	defer srv.Close()

	core := &Core{http: NewHTTP(HTTPOptions{BaseURL: srv.URL})}
	ctx := context.Background()
	rb, err := core.Rollback(ctx, "p1", "L1", "t1")
	if err != nil || rb.LogID != "L2" || rb.RollbackOf != "L1" {
		t.Fatalf("Rollback = %+v, %v", rb, err)
	}
	if err := core.ResumeAutoDeploy(ctx, "p1", ""); err != nil {
		t.Fatal(err)
	}
	dup, err := core.DuplicateProject(ctx, "p1", DuplicateProjectRequest{EnvironmentID: "e1"}, "")
	if err != nil || dup.ProjectID != "p2" || dup.DomainsCloned != 1 {
		t.Fatalf("DuplicateProject = %+v, %v", dup, err)
	}

	want := []string{
		"POST /v1/projects/p1/rollback/L1 t1 ",
		"POST /v1/projects/p1/auto-deploy/resume  ",
		`POST /v1/projects/p1/duplicate  {"environmentId":"e1"}`,
	}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("request %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestPITRRequests(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.Path+" "+string(body))
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "ok", "data": map[string]any{"pitr_enabled": true}})
	}))
	defer srv.Close()

	core := &Core{http: NewHTTP(HTTPOptions{BaseURL: srv.URL})}
	ctx := context.Background()
	on := true
	dest := &BackupDestination{Type: "brimble", BucketID: "b1"}
	if _, err := core.UpdateBackupSettings(ctx, "d1", UpdateBackupSettingsRequest{PITREnabled: &on, Destination: dest}, ""); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := core.UpdateBackupSettings(ctx, "d1", UpdateBackupSettingsRequest{PITREnabled: &off}, ""); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 25, 6, 30, 0, 0, time.FixedZone("WAT", 3600))
	if _, err := core.RestoreToPointInTime(ctx, "d1", at, "", ""); err != nil {
		t.Fatal(err)
	}

	want := []string{
		`PATCH /v1/projects/database/d1/backup-settings {"pitr_enabled":true,"destination":{"type":"brimble","bucket_id":"b1"}}`,
		`PATCH /v1/projects/database/d1/backup-settings {"pitr_enabled":false}`,
		`POST /v1/projects/database/d1/pitr/restore {"target_time":"2026-10-25T05:30:00Z"}`,
	}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("request %d = %q, want %q", i, got, want[i])
		}
	}
}
