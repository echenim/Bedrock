.PHONY: build test lint proto-gen clean fmt release localnet

BINARY := bedrockd
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)
GOFLAGS := -trimpath

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/bedrockd

test:
	go test -race -count=1 -timeout 5m ./...

lint:
	bash scripts/lint.sh

proto-gen:
	buf generate

clean:
	rm -f $(BINARY)
	rm -rf dist/
	go clean -cache

fmt:
	gofumpt -w .
	goimports -w .

release:
	bash scripts/release.sh

localnet:
	bash scripts/localnet.sh
