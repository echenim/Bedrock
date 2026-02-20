#!/usr/bin/env bash
set -euo pipefail

# Build bedrockd binary with version info embedded via ldflags.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}"

echo "Building bedrockd ${VERSION} (${COMMIT})"
go build -trimpath -ldflags "${LDFLAGS}" -o bedrockd ./cmd/bedrockd
echo "Build complete: ./bedrockd"

# Show version info to confirm ldflags were applied.
./bedrockd version
