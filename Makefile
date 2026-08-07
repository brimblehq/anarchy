.PHONY: build install test tidy snapshot release-dry-run

BIN := bin/brimble
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/brimbleapp/brimble-cli/internal/cmd.version=$(VERSION) \
	-X github.com/brimbleapp/brimble-cli/internal/cmd.commit=$(COMMIT) \
	-X github.com/brimbleapp/brimble-cli/internal/cmd.date=$(DATE)

# Prefer a local goreleaser binary; otherwise run via Go modules (no install needed).
GORELEASER ?= $(shell command -v goreleaser 2>/dev/null)
ifeq ($(GORELEASER),)
GORELEASER := go run github.com/goreleaser/goreleaser/v2@v2.9.0
endif

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/brimble

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/brimble

test:
	go test ./...

tidy:
	go mod tidy

snapshot:
	$(GORELEASER) release --snapshot --clean --skip=publish

release-dry-run:
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		$(GORELEASER) release --clean --skip=publish; \
	else \
		echo "No git repository detected — using snapshot mode (same as make snapshot)."; \
		echo "Init git + tag (e.g. git tag v0.1.0) for a full release dry-run."; \
		$(GORELEASER) release --snapshot --clean --skip=publish; \
	fi
