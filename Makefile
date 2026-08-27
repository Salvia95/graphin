GO ?= go

# packages with heavy concurrency — always exercised under -race at phase gates
RACE_PKGS = ./internal/graph/... ./internal/store/... ./internal/workspace/... ./internal/watch/... ./internal/mcp/...

# Build identity. These land in `graphin version` and the MCP handshake — the
# release workflow overrides VERSION on the command line.
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILDDATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILDDATE)

# NEVER add -extldflags -static: yalue/onnxruntime_go opens libonnxruntime
# with dlopen, which a static binary cannot do. The break would show up only
# at semantic warmup, long after the build looked fine.
.PHONY: build ui vet test test-race fbs

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/graphin ./cmd/graphin

# The console interface, compiled into the binary by //go:embed.
#
# Deliberately NOT a prerequisite of `build`. A Go toolchain is the only thing
# this repo asks of a contributor, and making every build need node would trade
# that away for one subcommand most people never open. A binary built without
# this carries no interface and says so at runtime; CI and the release run it
# and hand the result to the Go build.
ui:
	cd internal/console/ui && npm ci && npm run build

vet:
	$(GO) vet ./...

test: vet
	$(GO) test ./...

test-race:
	$(GO) test -race $(RACE_PKGS)

fbs:
	flatc --go -o internal/graph/ schema/graph.fbs
