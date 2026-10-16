// Package selfupdate finds the latest Brimble CLI release and replaces the
// running binary with it.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const repo = "brimblehq/anarchy"

// Base URLs; tests point them at a local server.
var (
	apiBase      = "https://api.github.com"
	downloadBase = "https://github.com"
	httpClient   = &http.Client{Timeout: 2 * time.Minute}
)

func init() {
	// BRIMBLE_UPGRADE_MIRROR serves the same paths as GitHub (the releases API
	// and release downloads), for mirrors and testing.
	if m := strings.TrimRight(os.Getenv("BRIMBLE_UPGRADE_MIRROR"), "/"); m != "" {
		apiBase, downloadBase = m, m
	}
}

// ErrNoRelease means the repository has no published release (yet).
var ErrNoRelease = errors.New("no release has been published yet")

// Latest returns the newest release tag, e.g. "v0.3.0".
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", ErrNoRelease
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("checking for updates: GitHub returned %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("checking for updates: %w", err)
	}
	if rel.TagName == "" {
		return "", ErrNoRelease
	}
	return rel.TagName, nil
}

// ArchiveName is the release asset for an OS/arch, matching .goreleaser.yaml.
func ArchiveName(goos, goarch string) (string, error) {
	osName := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[goos]
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	if osName == "" || arch == "" {
		return "", fmt.Errorf("no release build for %s/%s", goos, goarch)
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "brimble_" + osName + "_" + arch + ext, nil
}

// Download fetches the release archive for goos/goarch, checks it against
// checksums.txt and returns the brimble binary inside it.
func Download(ctx context.Context, tag, goos, goarch string) ([]byte, error) {
	archive, err := ArchiveName(goos, goarch)
	if err != nil {
		return nil, err
	}
	base := downloadBase + "/" + repo + "/releases/download/" + tag + "/"
	sums, err := fetch(ctx, base+"checksums.txt")
	if err != nil {
		return nil, err
	}
	want, err := checksumFor(sums, archive)
	if err != nil {
		return nil, err
	}
	data, err := fetch(ctx, base+archive)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s — the download may be corrupted; try again", archive)
	}
	name := "brimble"
	if goos == "windows" {
		name = "brimble.exe"
	}
	if strings.HasSuffix(archive, ".zip") {
		return fromZip(data, name)
	}
	return fromTarGz(data, name)
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", filepath.Base(url), resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func checksumFor(sums []byte, archive string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[1] == archive {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in checksums.txt", archive)
}

func fromTarGz(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == name && h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in the release archive", name)
}

func fromZip(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in the release archive", name)
}

// Replace swaps the binary at path for data. The new file is written next to
// it and renamed over it, so an interrupted upgrade never leaves a broken
// binary. Windows can't overwrite a running .exe, so the old one is moved
// aside first (and cleaned up on the next upgrade).
func Replace(path string, data []byte, goos string) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".brimble-upgrade-*")
	if err != nil {
		return permissionHint(err, dir)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()|0o111); err != nil {
		return err
	}

	if goos == "windows" {
		old := path + ".old"
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			return permissionHint(err, dir)
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			_ = os.Rename(old, path)
			return err
		}
		return nil
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return permissionHint(err, dir)
	}
	return nil
}

// CleanupOld removes a binary left aside by a previous Windows upgrade.
func CleanupOld(path string) {
	_ = os.Remove(path + ".old")
}

func permissionHint(err error, dir string) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("no permission to write to %s — re-run with sudo, or reinstall with the install script", dir)
	}
	return err
}

// Install methods that own the binary and should do the upgrade themselves.
const (
	MethodBinary   = "binary"   // install script or manual download: self-replace
	MethodHomebrew = "homebrew" // brew upgrade --cask brimble
	MethodGo       = "go"       // go install …@latest
	MethodSource   = "source"   // built from a checkout
)

var gitDescribe = regexp.MustCompile(`-\d+-g[0-9a-f]+(-dirty)?$|-dirty$`)

// Method works out how the binary at path (symlinks resolved) was installed.
func Method(path, version, gopathBin string) string {
	p := filepath.ToSlash(path)
	switch {
	case IsSourceVersion(version):
		return MethodSource
	case strings.Contains(p, "/Caskroom/") || strings.Contains(p, "/Cellar/"):
		return MethodHomebrew
	case gopathBin != "" && filepath.Dir(path) == filepath.Clean(gopathBin):
		return MethodGo
	}
	return MethodBinary
}

// Newer reports whether release tag b is newer than version a. Both may have
// a leading "v"; pre-release suffixes sort before the release.
func Newer(a, b string) bool {
	return compare(parse(b), parse(a)) > 0
}

type semver struct {
	nums [3]int
	pre  string
}

func parse(v string) semver {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var s semver
	core, pre, _ := strings.Cut(v, "-")
	s.pre = pre
	for i, part := range strings.SplitN(core, ".", 3) {
		n, _ := strconv.Atoi(part)
		s.nums[i] = n
	}
	return s
}

func compare(a, b semver) int {
	for i := range a.nums {
		if a.nums[i] != b.nums[i] {
			if a.nums[i] > b.nums[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	case a.pre > b.pre:
		return 1
	}
	return -1
}
