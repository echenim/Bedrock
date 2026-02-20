#!/usr/bin/env bash
set -euo pipefail

# Cross-compile release binaries for supported platforms.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}"

OUTPUT_DIR="${PROJECT_DIR}/dist"
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# Target platforms.
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
)

echo "=== BedRock Release Build ==="
echo "  Version: ${VERSION}"
echo "  Commit:  ${COMMIT}"
echo "  Output:  ${OUTPUT_DIR}"
echo ""

for PLATFORM in "${PLATFORMS[@]}"; do
    OS="${PLATFORM%%/*}"
    ARCH="${PLATFORM##*/}"
    BINARY="bedrockd-${VERSION}-${OS}-${ARCH}"

    echo "Building ${BINARY}..."
    GOOS="${OS}" GOARCH="${ARCH}" CGO_ENABLED=0 \
        go build -trimpath -ldflags "${LDFLAGS}" \
        -o "${OUTPUT_DIR}/${BINARY}" ./cmd/bedrockd

    # Create checksum.
    (cd "${OUTPUT_DIR}" && shasum -a 256 "${BINARY}" >> checksums.txt)
done

echo ""
echo "Release binaries:"
ls -lh "${OUTPUT_DIR}/"
echo ""
echo "Checksums:"
cat "${OUTPUT_DIR}/checksums.txt"
