GO ?= go

# packages with heavy concurrency — always exercised under -race at phase gates
RACE_PKGS = ./internal/graph/... ./internal/store/... ./internal/workspace/... ./internal/watch/... ./internal/admin/...

.PHONY: build vet test test-race fbs

build:
	$(GO) build -o bin/graphin ./cmd/graphin

vet:
	$(GO) vet ./...

test: vet
	$(GO) test ./...

test-race:
	$(GO) test -race $(RACE_PKGS)

fbs:
	flatc --go -o internal/graph/ schema/graph.fbs
