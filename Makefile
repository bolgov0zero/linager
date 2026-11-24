VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/bolgov0zero/linager/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

# Architectures of the release binaries (GOARCH values); `make build ARCHES=amd64` builds one.
ARCHES ?= amd64 arm64

.PHONY: all web build build-windows test lint clean

all: build

web:
	cd web && npm ci && npm run build

build: web
	mkdir -p bin
	for arch in $(ARCHES); do \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/linager_linux_$$arch ./cmd/linager || exit 1; \
	done

# The Windows agent: no server inside, so the web interface is not needed.
build-windows:
	mkdir -p bin
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/linager-agent_windows_amd64.exe ./cmd/linager

test:
	go vet ./... && go test -race ./...

lint:
	golangci-lint run
	shellcheck install.sh

clean:
	rm -rf bin web/dist/* && touch web/dist/.gitkeep
