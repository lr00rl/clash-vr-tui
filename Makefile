BINARY      := clash-vr-tui
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GOFLAGS     := -trimpath
PREFIX      ?= /usr/local
PLATFORMS   := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build run test race vet fmt lint install clean dist

all: build

build: ## Build the binary for the host platform
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) .

run: build ## Build and launch the TUI
	./$(BINARY)

test: ## Run unit tests
	go test ./...

race: ## Run unit tests with the race detector
	go test -race ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt the tree
	gofmt -w .

lint: vet ## Basic static checks
	@gofmt -l . | grep -q . && { echo "unformatted files:"; gofmt -l .; exit 1; } || echo "fmt clean"

install: build ## Install to $(PREFIX)/bin
	install -d $(PREFIX)/bin
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

clean: ## Remove build artifacts
	rm -rf $(BINARY) dist/

dist: ## Cross-compile release binaries into dist/
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="dist/$(BINARY)-$(VERSION)-$$os-$$arch$$ext"; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $$out . || exit 1; \
	done
	@echo "done -> dist/"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'
