#!/usr/bin/env bash
set -euo pipefail

# Run linters: golangci-lint for Go code, buf lint for protobuf, gofmt check.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

ERRORS=0

# Go linting.
echo "=== golangci-lint ==="
if command -v golangci-lint &>/dev/null; then
    golangci-lint run ./... || ERRORS=$((ERRORS + 1))
else
    echo "SKIP: golangci-lint not installed"
    echo "  Install: https://golangci-lint.run/welcome/install/"
fi

echo ""

# Go vet (always available).
echo "=== go vet ==="
go vet ./... || ERRORS=$((ERRORS + 1))

echo ""

# Protobuf linting.
echo "=== buf lint ==="
if command -v buf &>/dev/null; then
    if [ -f "buf.yaml" ] || [ -f "buf.gen.yaml" ]; then
        buf lint || ERRORS=$((ERRORS + 1))
    else
        echo "SKIP: no buf.yaml found"
    fi
else
    echo "SKIP: buf not installed"
    echo "  Install: https://buf.build/docs/installation"
fi

echo ""

# Go formatting check.
echo "=== gofmt check ==="
UNFORMATTED=$(gofmt -l . 2>/dev/null | grep -v "gen/" || true)
if [ -n "${UNFORMATTED}" ]; then
    echo "ERROR: The following files are not formatted:"
    echo "${UNFORMATTED}"
    ERRORS=$((ERRORS + 1))
else
    echo "All files formatted correctly."
fi

echo ""

if [ "${ERRORS}" -gt 0 ]; then
    echo "FAILED: ${ERRORS} linter(s) reported errors."
    exit 1
fi

echo "All linters passed."
