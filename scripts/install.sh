#!/bin/sh
# Install the Brimble CLI.
#
#   curl -fsSL <install-url> | sh
#
# Environment:
#   BRIMBLE_VERSION       Release tag to install, e.g. v0.2.0 (default: latest)
#   BRIMBLE_INSTALL_DIR   Where to put the binary (default: /usr/local/bin when
#                         writable, otherwise ~/.local/bin)
#   BRIMBLE_DOWNLOAD_BASE Download archives from this URL instead of GitHub
#                         releases (for mirrors); must hold the same files
set -eu

REPO="brimbleapp/brimble-cli"
BINARY="brimble"

say() { printf '%s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required but not installed"
}

detect_os() {
	case "$(uname -s)" in
		Darwin) echo Darwin ;;
		Linux) echo Linux ;;
		MINGW* | MSYS* | CYGWIN*) fail "on Windows, download the zip from https://github.com/$REPO/releases" ;;
		*) fail "unsupported operating system: $(uname -s)" ;;
	esac
}

# Matches the archive names in .goreleaser.yaml.
detect_arch() {
	case "$(uname -m)" in
		x86_64 | amd64) echo x86_64 ;;
		arm64 | aarch64) echo arm64 ;;
		*) fail "unsupported architecture: $(uname -m)" ;;
	esac
}

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		fail "curl or wget is required"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		fail "sha256sum or shasum is required to verify the download"
	fi
}

pick_install_dir() {
	if [ -n "${BRIMBLE_INSTALL_DIR:-}" ]; then
		echo "$BRIMBLE_INSTALL_DIR"
	elif [ -w /usr/local/bin ]; then
		echo /usr/local/bin
	else
		echo "$HOME/.local/bin"
	fi
}

main() {
	need tar
	need uname

	os=$(detect_os)
	arch=$(detect_arch)
	version=${BRIMBLE_VERSION:-latest}
	archive="${BINARY}_${os}_${arch}.tar.gz"

	if [ -n "${BRIMBLE_DOWNLOAD_BASE:-}" ]; then
		base="${BRIMBLE_DOWNLOAD_BASE%/}"
	elif [ "$version" = latest ]; then
		base="https://github.com/$REPO/releases/latest/download"
	else
		case "$version" in v*) ;; *) version="v$version" ;; esac
		base="https://github.com/$REPO/releases/download/$version"
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	say "Downloading $archive ($version)…"
	download "$base/$archive" "$tmp/$archive" || fail "could not download $base/$archive"
	download "$base/checksums.txt" "$tmp/checksums.txt" || fail "could not download checksums.txt"

	expected=$(awk -v f="$archive" '$2 == f {print $1}' "$tmp/checksums.txt")
	[ -n "$expected" ] || fail "no checksum for $archive in checksums.txt"
	actual=$(sha256 "$tmp/$archive")
	[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive (expected $expected, got $actual)"

	tar -xzf "$tmp/$archive" -C "$tmp"
	[ -f "$tmp/$BINARY" ] || fail "archive did not contain $BINARY"

	dir=$(pick_install_dir)
	mkdir -p "$dir" || fail "could not create $dir"
	if [ -w "$dir" ]; then
		install -m 0755 "$tmp/$BINARY" "$dir/$BINARY"
	else
		say "Installing to $dir requires sudo"
		sudo install -m 0755 "$tmp/$BINARY" "$dir/$BINARY"
	fi

	say "Installed $("$dir/$BINARY" --version) to $dir/$BINARY"

	case ":$PATH:" in
		*":$dir:"*) ;;
		*)
			say ""
			say "$dir is not on your PATH. Add it with:"
			say "  echo 'export PATH=\"$dir:\$PATH\"' >> ~/.profile"
			;;
	esac

	say ""
	say "Get started:  $BINARY login"
}

main "$@"
