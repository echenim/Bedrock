.PHONY: build test lint clean fmt

BINARY := bedrockd
GOFLAGS := -trimpath

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/bedrockd

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)
	go clean -cache

fmt:
	gofumpt -w .
	goimports -w .
