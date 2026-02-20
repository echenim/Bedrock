#!/usr/bin/env bash
set -euo pipefail

# Run all Go tests with the race detector enabled.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

TIMEOUT=${TIMEOUT:-5m}
COUNT=${COUNT:-1}

echo "=== BedRock ControlPlane Tests ==="
echo "  Timeout: ${TIMEOUT}"
echo "  Count:   ${COUNT}"
echo "  Race:    enabled"
echo ""

go test -race -count="${COUNT}" -timeout "${TIMEOUT}" ./...

echo ""
echo "All tests passed."
