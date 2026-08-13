GO ?= go

# packages with heavy concurrency — always exercised under -race at phase gates
RACE_PKGS = ./internal/graph/... ./internal/store/... ./internal/workspace/... ./internal/watch/...

# Build identity. These land in `graphin version` and the MCP handshake — the
# release workflow overrides VERSION on the command line.
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILDDATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILDDATE)

# NEVER add -extldflags -static: yalue/onnxruntime_go opens libonnxruntime
# with dlopen, which a static binary cannot do. The break would show up only
# at semantic warmup, long after the build looked fine.
.PHONY: build vet test test-race fbs

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/graphin ./cmd/graphin

vet:
	$(GO) vet ./...

test: vet
	$(GO) test ./...

test-race:
	$(GO) test -race $(RACE_PKGS)

fbs:
	flatc --go -o internal/graph/ schema/graph.fbs
