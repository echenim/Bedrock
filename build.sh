#!/usr/bin/env bash
set -euo pipefail

# BedRock unified build script.
# Builds all components: ExecutionCore (Rust), SandBox (Rust), WASM artifact, ControlPlane (Go).
#
# Usage:
#   ./build.sh              Build everything
#   ./build.sh rust         Build Rust crates only
#   ./build.sh wasm         Build WASM artifact only
#   ./build.sh go           Build Go binary only
#   ./build.sh test         Run all tests
#   ./build.sh clean        Clean all build artifacts

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# ── Version info ──
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

WASM_TARGET="wasm32-unknown-unknown"
WASM_ARTIFACT="ExecutionCore/target/${WASM_TARGET}/release/bedrock_wasm_guest.wasm"

# ── Cargo: prefer rustup's cargo to ensure WASM target is available ──
if command -v rustup &>/dev/null; then
    CARGO="rustup run stable cargo"
else
    CARGO="cargo"
fi

# ── Helpers ──

build_execution_core() {
    echo "=== Building ExecutionCore (Rust — native) ==="
    cd "${SCRIPT_DIR}/ExecutionCore"
    ${CARGO} build --release --workspace --exclude bedrock-wasm-guest
    echo "  Done."
    cd "${SCRIPT_DIR}"
}

build_sandbox() {
    echo "=== Building SandBox (Rust) ==="
    cd "${SCRIPT_DIR}/SandBox"
    ${CARGO} build --release
    echo "  Done."
    cd "${SCRIPT_DIR}"
}

build_wasm() {
    echo "=== Building WASM artifact ==="
    cd "${SCRIPT_DIR}/ExecutionCore"
    ${CARGO} build --release --target "${WASM_TARGET}" -p bedrock-wasm-guest
    echo "  → ${WASM_ARTIFACT}"
    ls -lh "${SCRIPT_DIR}/${WASM_ARTIFACT}"
    cd "${SCRIPT_DIR}"
}

build_go() {
    echo "=== Building ControlPlane (Go) ==="
    cd "${SCRIPT_DIR}/ControlPlane"
    LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}"
    go build -trimpath -ldflags "${LDFLAGS}" -o bedrockd ./cmd/bedrockd
    echo "  → ControlPlane/bedrockd"
    ./bedrockd version
    cd "${SCRIPT_DIR}"
}

test_all() {
    echo "=== Testing ExecutionCore ==="
    cd "${SCRIPT_DIR}/ExecutionCore"
    ${CARGO} test --all
    echo ""

    echo "=== Testing SandBox ==="
    cd "${SCRIPT_DIR}/SandBox"
    ${CARGO} test --all
    echo ""

    echo "=== Testing ControlPlane ==="
    cd "${SCRIPT_DIR}/ControlPlane"
    go test -race -count=1 -timeout 5m ./...
    echo ""

    echo "=== All tests passed ==="
    cd "${SCRIPT_DIR}"
}

clean_all() {
    echo "=== Cleaning all build artifacts ==="
    cd "${SCRIPT_DIR}/ExecutionCore" && ${CARGO} clean
    cd "${SCRIPT_DIR}/SandBox" && ${CARGO} clean
    cd "${SCRIPT_DIR}/ControlPlane" && rm -f bedrockd && rm -rf dist/ && go clean -cache
    echo "  Done."
    cd "${SCRIPT_DIR}"
}

build_all() {
    echo "BedRock Build — v${VERSION} (${COMMIT})"
    echo ""
    build_execution_core
    echo ""
    build_sandbox
    echo ""
    build_wasm
    echo ""
    build_go
    echo ""
    echo "=== Build complete ==="
    echo "  Binary: ControlPlane/bedrockd"
    echo "  WASM:   ${WASM_ARTIFACT}"
}

# ── Main ──

COMMAND="${1:-all}"

case "${COMMAND}" in
    all|build)
        build_all
        ;;
    rust)
        build_execution_core
        echo ""
        build_sandbox
        ;;
    wasm)
        build_wasm
        ;;
    go)
        build_go
        ;;
    test)
        test_all
        ;;
    clean)
        clean_all
        ;;
    *)
        echo "Usage: $0 {all|build|rust|wasm|go|test|clean}"
        echo ""
        echo "  all, build    Build everything (Rust + WASM + Go)"
        echo "  rust          Build Rust crates (ExecutionCore + SandBox)"
        echo "  wasm          Build WASM execution artifact"
        echo "  go            Build Go control plane binary"
        echo "  test          Run all tests"
        echo "  clean         Clean all build artifacts"
        exit 1
        ;;
esac
