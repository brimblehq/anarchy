#!/bin/sh
# Generate shell completions for packaging (run by GoReleaser before builds).
set -eu
rm -rf completions
mkdir completions
for shell in bash zsh fish; do
	go run ./cmd/brimble completion "$shell" > "completions/brimble.$shell"
done
