# VineReal build automation.
#
# The repo is a Go workspace of four modules (shared, server, client, e2e).
# Build each module from its own directory so the module's replace directives
# and dependency graph are respected. The client and server are pure Go, so
# Linux builds are static (CGO_ENABLED=0) and need no cgo toolchain.

GO      ?= go
BIN     ?= bin

# Version stamped into every binary's `-version` flag. On a tagged commit this
# is the tag (e.g. v1.0.0); elsewhere it falls back to a short SHA or "dev".
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Reproducible-ish, stripped builds: -trimpath removes local filesystem paths
# and -s -w drop symbol table/DWARF info.
TRIMFLAGS := -trimpath
LDFLAGS   := -s -w -X main.version=$(VERSION)

.PHONY: all build server keygen client-demo \
        client-linux server-linux linux \
        test vet clean \
        mobile-android mobile-ios

all: build

## ---- native builds (host OS/arch) ----

build: server keygen client-demo

server:
	cd server && $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/vinereal-server ./cmd/vinereal-server

keygen:
	cd server && $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/realitykeygen ./cmd/realitykeygen

client-demo:
	cd client && $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/vinereal-client-demo ./cmd/vinereal-client-demo

## ---- Linux cross builds (static, no cgo) ----

# The test client: a single self-contained binary you can copy to a Linux box
# and run directly.
client-linux:
	cd client && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/vinereal-client-demo-linux-amd64 ./cmd/vinereal-client-demo
	cd client && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/vinereal-client-demo-linux-arm64 ./cmd/vinereal-client-demo

# Same, for the server and keygen, in case you deploy the server on Linux too.
server-linux:
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/vinereal-server-linux-amd64 ./cmd/vinereal-server
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/vinereal-server-linux-arm64 ./cmd/vinereal-server
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/realitykeygen-linux-amd64 ./cmd/realitykeygen
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(TRIMFLAGS) -ldflags="$(LDFLAGS)" -o ../$(BIN)/realitykeygen-linux-arm64 ./cmd/realitykeygen

# Everything you'd ship onto Linux in one shot.
linux: client-linux server-linux

## ---- verification ----

test:
	cd shared && $(GO) test ./...
	cd server && $(GO) test ./...
	cd client && $(GO) test ./...
	cd test/e2e && $(GO) test ./...

vet:
	cd shared && $(GO) vet ./...
	cd server && $(GO) vet ./...
	cd client && $(GO) vet ./...
	cd test/e2e && $(GO) vet ./...

## ---- gomobile bindings (optional; requires `gomobile init`) ----

# One-time setup:
#   go install golang.org/x/mobile/cmd/gomobile@latest
#   gomobile init
mobile-android:
	cd client && gomobile bind -target=android -o ../$(BIN)/vinereal.aar ./mobile

mobile-ios:
	cd client && gomobile bind -target=ios -o ../$(BIN)/VineReal.xcframework ./mobile

clean:
	rm -rf $(BIN)
