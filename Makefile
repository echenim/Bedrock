.PHONY: all build build-go build-rust build-wasm \
       test test-go test-rust \
       lint lint-go lint-rust \
       fmt fmt-go fmt-rust \
       clean clean-go clean-rust \
       run localnet release

# ── Version info (Go) ──
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)

# ── Cargo (use rustup's cargo to ensure correct toolchain + WASM target) ──
CARGO := $(shell command -v rustup >/dev/null 2>&1 && echo "rustup run stable cargo" || echo "cargo")

# ── Directories ──
CONTROL_PLANE := ControlPlane
EXECUTION_CORE := ExecutionCore
SANDBOX := SandBox

# ── WASM artifact ──
WASM_TARGET := wasm32-unknown-unknown
WASM_ARTIFACT := $(EXECUTION_CORE)/target/$(WASM_TARGET)/release/bedrock_wasm_guest.wasm

# ── Default: build everything ──
all: build

build: build-rust build-wasm build-go
	@echo ""
	@echo "=== Build complete ==="
	@echo "  Binary: $(CONTROL_PLANE)/bedrockd"
	@echo "  WASM:   $(WASM_ARTIFACT)"

# ── Go ──────────────────────────────────────────────────────────────

build-go:
	@echo "=== Building ControlPlane (Go) ==="
	cd $(CONTROL_PLANE) && go build -trimpath -ldflags "$(GO_LDFLAGS)" -o bedrockd ./cmd/bedrockd
	@echo "  → $(CONTROL_PLANE)/bedrockd"

test-go:
	@echo "=== Testing ControlPlane (Go) ==="
	cd $(CONTROL_PLANE) && go test -race -count=1 -timeout 5m ./...

lint-go:
	@echo "=== Linting ControlPlane (Go) ==="
	cd $(CONTROL_PLANE) && bash scripts/lint.sh

fmt-go:
	@echo "=== Formatting ControlPlane (Go) ==="
	cd $(CONTROL_PLANE) && gofumpt -w . && goimports -w .

clean-go:
	cd $(CONTROL_PLANE) && rm -f bedrockd && rm -rf dist/ && go clean -cache

# ── Rust ────────────────────────────────────────────────────────────

build-rust: build-execution build-sandbox

build-execution:
	@echo "=== Building ExecutionCore (Rust — native) ==="
	cd $(EXECUTION_CORE) && $(CARGO) build --release --workspace --exclude bedrock-wasm-guest

build-sandbox:
	@echo "=== Building SandBox (Rust) ==="
	cd $(SANDBOX) && $(CARGO) build --release

build-wasm:
	@echo "=== Building WASM artifact ==="
	cd $(EXECUTION_CORE) && $(CARGO) build --release --target $(WASM_TARGET) -p bedrock-wasm-guest
	@echo "  → $(WASM_ARTIFACT)"
	@ls -lh $(WASM_ARTIFACT)

test-rust: test-execution test-sandbox

test-execution:
	@echo "=== Testing ExecutionCore ==="
	cd $(EXECUTION_CORE) && $(CARGO) test --all

test-sandbox:
	@echo "=== Testing SandBox ==="
	cd $(SANDBOX) && $(CARGO) test --all

lint-rust: lint-execution lint-sandbox

lint-execution:
	@echo "=== Linting ExecutionCore ==="
	cd $(EXECUTION_CORE) && $(CARGO) fmt --all --check
	cd $(EXECUTION_CORE) && $(CARGO) clippy --all -- -D warnings

lint-sandbox:
	@echo "=== Linting SandBox ==="
	cd $(SANDBOX) && $(CARGO) fmt --all --check
	cd $(SANDBOX) && $(CARGO) clippy --all -- -D warnings

fmt-rust:
	@echo "=== Formatting Rust ==="
	cd $(EXECUTION_CORE) && $(CARGO) fmt --all
	cd $(SANDBOX) && $(CARGO) fmt --all

clean-rust:
	cd $(EXECUTION_CORE) && $(CARGO) clean
	cd $(SANDBOX) && $(CARGO) clean

# ── Combined targets ────────────────────────────────────────────────

test: test-rust test-go
	@echo ""
	@echo "=== All tests passed ==="

lint: lint-rust lint-go

fmt: fmt-rust fmt-go

clean: clean-rust clean-go
	@echo "=== Clean complete ==="

# ── Run ─────────────────────────────────────────────────────────────

run: build-go
	cd $(CONTROL_PLANE) && ./bedrockd start

localnet: build-go
	cd $(CONTROL_PLANE) && bash scripts/localnet.sh

release: build-wasm
	cd $(CONTROL_PLANE) && bash scripts/release.sh

# ── Help ────────────────────────────────────────────────────────────

help:
	@echo "BedRock Build System"
	@echo ""
	@echo "  make              Build everything (Rust + WASM + Go)"
	@echo "  make build        Same as above"
	@echo ""
	@echo "  make build-go     Build Go control plane (bedrockd)"
	@echo "  make build-rust   Build Rust crates (native)"
	@echo "  make build-wasm   Build WASM execution artifact"
	@echo ""
	@echo "  make test         Run all tests (Rust + Go)"
	@echo "  make test-go      Run Go tests with race detector"
	@echo "  make test-rust    Run Rust tests"
	@echo ""
	@echo "  make lint         Lint everything"
	@echo "  make fmt          Format everything"
	@echo "  make clean        Clean all build artifacts"
	@echo ""
	@echo "  make run          Build and start a node"
	@echo "  make localnet     Build and start 4-node local testnet"
	@echo "  make release      Cross-compile release binaries"
	@echo ""
	@echo "  make help         Show this help"
