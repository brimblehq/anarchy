package cmd

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/brimblehq/anarchy/internal/client"
	"github.com/brimblehq/anarchy/internal/config"
)

func TestDotenvRoundTrip(t *testing.T) {
	in := `# comment
export A=1
B="multi\nline \"quoted\""
C='literal $HOME'
D=plain # trailing comment
E=
`
	vars, err := parseDotenv(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []client.EnvVar{
		{Name: "A", Value: "1"},
		{Name: "B", Value: "multi\nline \"quoted\""},
		{Name: "C", Value: "literal $HOME"},
		{Name: "D", Value: "plain"},
		{Name: "E", Value: ""},
	}
	if !reflect.DeepEqual(vars, want) {
		t.Fatalf("parse:\n got %#v\nwant %#v", vars, want)
	}

	again, err := parseDotenv(formatDotenv(vars))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("round trip:\n got %#v\nwant %#v", again, want)
	}
}

func TestParseDotenvErrors(t *testing.T) {
	for _, in := range []string{"NOEQUALS", `A="unterminated`} {
		if _, err := parseDotenv(in); err == nil {
			t.Errorf("parseDotenv(%q): expected error", in)
		}
	}
}

func TestZipDirSkipsSecretsAndDependencies(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"index.html":            "<h1>hi</h1>",
		"assets/app.js":         "x",
		".env":                  "SECRET=1",
		".env.production":       "SECRET=2",
		"node_modules/x/i.js":   "m",
		".git/config":           "g",
		".brimble/project.json": "{}",
		"sub/.DS_Store":         "",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	archive, count, _, err := zipDir(root)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{"assets/app.js", "index.html"}
	if !reflect.DeepEqual(names, want) || count != 2 {
		t.Fatalf("zipped %v (count %d), want %v", names, count, want)
	}
}

func TestRegionCatalogFlattensProviders(t *testing.T) {
	catalog := client.RegionCatalog{Providers: []client.RegionProvider{
		{Name: "a", Regions: []client.Region{{ID: "r1"}, {ID: "r2"}}},
		{Name: "b", Regions: []client.Region{{ID: "r3"}}},
	}}
	items := catalog.Regions()
	if len(items) != 3 || items[2].ID != "r3" {
		t.Fatalf("got %v", items)
	}
	if (client.RegionCatalog{}).Regions() != nil {
		t.Fatal("empty catalog should have no regions")
	}
}

func TestDomainRefsReadMappedShape(t *testing.T) {
	got := domainRefs(client.DomainList{Domains: []client.Domain{
		{ID: "d1", Name: "app.example.com", Project: "p1", Active: true, Primary: true},
	}})
	if len(got) != 1 || got[0].ID != "d1" || got[0].ProjectID != "p1" || !got[0].Active {
		t.Fatalf("got %+v", got)
	}
}

func TestSplitProjectAndPairs(t *testing.T) {
	defer func() { linked = nil }()

	linked = nil
	project, pairs := splitProjectAndPairs([]string{"my-api", "A=1", "TOKEN"})
	if project != "my-api" || !reflect.DeepEqual(pairs, []string{"A=1", "TOKEN"}) {
		t.Fatalf("unlinked: got %q %v", project, pairs)
	}

	linked = &config.Link{ProjectID: "p", ProjectName: "web"}
	project, pairs = splitProjectAndPairs([]string{"TOKEN", "A=1"})
	if project != "" || !reflect.DeepEqual(pairs, []string{"TOKEN", "A=1"}) {
		t.Fatalf("linked: got %q %v", project, pairs)
	}
}

func TestEnvScope(t *testing.T) {
	cases := map[string]string{"": "", "production": "PRODUCTION", "Preview": "PREVIEW"}
	for in, want := range cases {
		got, err := envScope(in)
		if err != nil || got != want {
			t.Errorf("envScope(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := envScope("staging"); err == nil {
		t.Error("expected error for staging")
	}
}

func TestDeployStatusClassification(t *testing.T) {
	for _, s := range []string{"ACTIVE", "NOMAD_READY"} {
		if !deploySucceeded(s) || !deployFinished(s) {
			t.Errorf("%s should be a finished success", s)
		}
	}
	for _, s := range []string{"FAILED", "NOMAD_FAILED", "CANCELLED", "DEGRADED"} {
		if !deployFailed(s) || !deployFinished(s) {
			t.Errorf("%s should be a finished failure", s)
		}
	}
	for _, s := range []string{"PENDING", "INPROGRESS", ""} {
		if deployFinished(s) {
			t.Errorf("%s should still be running", s)
		}
	}
}

func TestDockerPayloadMatchesCoreSchema(t *testing.T) {
	d := createDraft{
		Name: "web", Source: "docker", ServiceType: "web-service", Framework: "docker",
		RepoName: "nginx:1.27", RepoBranch: "1.27", RegionID: "r1", CPU: 0.5, Memory: 1, Deploy: true,
		RegistryUser: "u", RegistryToken: "t",
	}
	body := d.toPayload()
	if body.Git != "DOCKER" || body.ServiceType != "web-service" {
		t.Fatalf("got %+v", body)
	}
	if body.Repo.Name != "nginx:1.27" || body.Repo.Git != "DOCKER" {
		t.Fatalf("repo = %+v", body.Repo)
	}
	if body.Configurations.Region != "r1" || body.Configurations.Storage != 3 {
		t.Fatalf("configurations = %+v", body.Configurations)
	}
	if body.RegistryCredentials == nil {
		t.Fatal("expected registry_credentials")
	}
	if body.AuthEnabled != nil {
		t.Fatal("authEnabled is for MCP servers only")
	}
}

func TestSlugifyName(t *testing.T) {
	cases := map[string]string{"My App": "my-app", "  web__site!! ": "web-site", "a--b": "a-b"}
	for in, want := range cases {
		if got := slugifyName(in); got != want {
			t.Errorf("slugifyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatSince(t *testing.T) {
	cases := map[time.Duration]string{
		15 * time.Minute: "15m", 2 * time.Hour: "2h", 90 * time.Minute: "1h30m", 30 * time.Second: "30s",
	}
	for in, want := range cases {
		if got := formatSince(in); got != want {
			t.Errorf("formatSince(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseRestoreTime(t *testing.T) {
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"15m":                       now.Add(-15 * time.Minute),
		"2h ago":                    now.Add(-2 * time.Hour),
		"1d":                        now.Add(-24 * time.Hour),
		"90s":                       now.Add(-90 * time.Second),
		"2026-10-25 06:30":          time.Date(2026, 10, 25, 6, 30, 0, 0, time.UTC),
		"2026-10-25T06:30:15":       time.Date(2026, 10, 25, 6, 30, 15, 0, time.UTC),
		"2026-10-25T07:30:00+01:00": time.Date(2026, 10, 25, 6, 30, 0, 0, time.UTC),
	}
	for in, want := range cases {
		got, err := parseRestoreTime(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("parseRestoreTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "yesterday", "15", "2026-10-25"} {
		if _, err := parseRestoreTime(bad, now); err == nil {
			t.Errorf("parseRestoreTime(%q) should fail", bad)
		}
	}
}
