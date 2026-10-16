package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body []byte
	}{{"README.md", []byte("readme")}, {name, body}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(f.body)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipped(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	w.Write(body)
	zw.Close()
	return buf.Bytes()
}

// releaseServer serves a GitHub-like release with the given assets.
func releaseServer(t *testing.T, tag string, assets map[string][]byte, corruptChecksums bool) {
	t.Helper()
	sums := ""
	for name, data := range assets {
		h := sha256.Sum256(data)
		sum := hex.EncodeToString(h[:])
		if corruptChecksums {
			sum = "00" + sum[2:]
		}
		sums += fmt.Sprintf("%s  %s\n", sum, name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/brimblehq/anarchy/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if tag == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	})
	mux.HandleFunc("/brimblehq/anarchy/releases/download/"+tag+"/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if name == "checksums.txt" {
			fmt.Fprint(w, sums)
			return
		}
		data, ok := assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	apiBase, downloadBase = srv.URL, srv.URL
	t.Cleanup(func() { apiBase, downloadBase = "https://api.github.com", "https://github.com" })
}

func TestLatestAndDownloadVerifiesChecksum(t *testing.T) {
	bin := []byte("#!/bin/sh\necho new\n")
	releaseServer(t, "v0.3.0", map[string][]byte{
		"brimble_Linux_arm64.tar.gz": tarGz(t, "brimble", bin),
		"brimble_Windows_x86_64.zip": zipped(t, "brimble.exe", []byte("exe")),
	}, false)

	tag, err := Latest(context.Background())
	if err != nil || tag != "v0.3.0" {
		t.Fatalf("Latest = %q, %v", tag, err)
	}
	got, err := Download(context.Background(), tag, "linux", "arm64")
	if err != nil || !bytes.Equal(got, bin) {
		t.Fatalf("Download = %q, %v", got, err)
	}
	exe, err := Download(context.Background(), tag, "windows", "amd64")
	if err != nil || string(exe) != "exe" {
		t.Fatalf("windows Download = %q, %v", exe, err)
	}
	if _, err := Download(context.Background(), tag, "freebsd", "amd64"); err == nil {
		t.Fatal("expected an error for an unsupported platform")
	}
}

func TestDownloadRejectsBadChecksum(t *testing.T) {
	releaseServer(t, "v0.3.0", map[string][]byte{"brimble_Darwin_arm64.tar.gz": tarGz(t, "brimble", []byte("x"))}, true)
	_, err := Download(context.Background(), "v0.3.0", "darwin", "arm64")
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("checksum mismatch")) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestLatestWithoutReleases(t *testing.T) {
	releaseServer(t, "", nil, false)
	if _, err := Latest(context.Background()); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("expected ErrNoRelease, got %v", err)
	}
}

func TestReplace(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "brimble")
		if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := Replace(path, []byte("new"), goos); err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		got, _ := os.ReadFile(path)
		info, _ := os.Stat(path)
		if string(got) != "new" || info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s: content %q mode %v", goos, got, info.Mode())
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.Name() != "brimble" && e.Name() != "brimble.old" {
				t.Errorf("%s: leftover file %s", goos, e.Name())
			}
		}
		CleanupOld(path)
		if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
			t.Errorf("%s: .old not cleaned up", goos)
		}
	}
}

func TestMethod(t *testing.T) {
	cases := []struct{ path, version, gobin, want string }{
		{"/opt/homebrew/Caskroom/brimble/0.3.0/brimble", "0.3.0", "", MethodHomebrew},
		{"/home/ada/go/bin/brimble", "0.3.0", "/home/ada/go/bin", MethodGo},
		{"/usr/local/bin/brimble", "0.3.0", "/home/ada/go/bin", MethodBinary},
		{"/src/brimble-cli/bin/brimble", "v0.1.0-1-g43d04a5-dirty", "", MethodSource},
		{"/usr/local/bin/brimble", "dev", "", MethodSource},
	}
	for _, c := range cases {
		if got := Method(c.path, c.version, c.gobin); got != c.want {
			t.Errorf("Method(%s, %s) = %s, want %s", c.path, c.version, got, c.want)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"0.10.0", "v0.9.0", false},
		{"1.0.0-rc.1", "v1.0.0", true},
		{"1.2.3", "v1.2.4", true},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.tag); got != c.want {
			t.Errorf("Newer(%s, %s) = %v, want %v", c.current, c.tag, got, c.want)
		}
	}
}
