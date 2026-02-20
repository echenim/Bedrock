#!/usr/bin/env bash
set -euo pipefail

# Network partition simulation for BedRock testnet.
# Tests SPEC.md §16 — partition recovery.
#
# This script simulates a network partition by stopping and restarting
# a node. In a production chaos test, you'd use iptables/pf rules to
# drop packets, but for local testing we use process signals.
#
# Prerequisites: localnet must be running (scripts/localnet.sh).

BASE_DIR="${BASE_DIR:-/tmp/bedrock-localnet}"
BASE_HTTP_PORT=26700
NODES=4
PARTITION_NODE=0
PARTITION_DURATION=${PARTITION_DURATION:-15}

echo "=== Chaos Test: Network Partition ==="
echo "  Target:   node${PARTITION_NODE}"
echo "  Duration: ${PARTITION_DURATION}s"
echo ""

# ─── Phase 1: Record baseline ───
echo "Phase 1: Recording baseline heights..."
for i in $(seq 0 $((NODES - 1))); do
    HTTP_PORT=$((BASE_HTTP_PORT + i))
    STATUS=$(curl -s "http://127.0.0.1:${HTTP_PORT}/status" 2>/dev/null || echo '{}')
    HEIGHT=$(echo "${STATUS}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latestBlockHeight',0))" 2>/dev/null || echo "0")
    echo "  node${i}: height=${HEIGHT}"
done

sleep 3

# ─── Phase 2: Partition node0 ───
echo ""
echo "Phase 2: Partitioning node${PARTITION_NODE}..."

# Find node0's PID from the localnet run.
NODE_PID=$(pgrep -f "bedrockd start --home ${BASE_DIR}/node${PARTITION_NODE}" 2>/dev/null || echo "")

if [ -z "${NODE_PID}" ]; then
    echo "ERROR: Could not find node${PARTITION_NODE} process."
    echo "  Make sure localnet is running: bash scripts/localnet.sh"
    exit 1
fi

# Pause the process (simulates network partition — node can't send/receive).
echo "  Sending SIGSTOP to node${PARTITION_NODE} (PID: ${NODE_PID})..."
kill -STOP "${NODE_PID}"
echo "  node${PARTITION_NODE} is now partitioned."

# ─── Phase 3: Verify remaining nodes make progress ───
echo ""
echo "Phase 3: Verifying progress without node${PARTITION_NODE} (waiting ${PARTITION_DURATION}s)..."
sleep "${PARTITION_DURATION}"

PROGRESS_OK=true
for i in $(seq 0 $((NODES - 1))); do
    if [ "${i}" -eq "${PARTITION_NODE}" ]; then
        echo "  node${i}: [PARTITIONED]"
        continue
    fi

    HTTP_PORT=$((BASE_HTTP_PORT + i))
    STATUS=$(curl -s "http://127.0.0.1:${HTTP_PORT}/status" 2>/dev/null || echo '{}')
    HEIGHT=$(echo "${STATUS}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latestBlockHeight',0))" 2>/dev/null || echo "0")
    SYNCING=$(echo "${STATUS}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('syncing',False))" 2>/dev/null || echo "unknown")
    echo "  node${i}: height=${HEIGHT} syncing=${SYNCING}"
done

# ─── Phase 4: Heal partition ───
echo ""
echo "Phase 4: Healing partition..."
kill -CONT "${NODE_PID}"
echo "  node${PARTITION_NODE} resumed (PID: ${NODE_PID})."

# ─── Phase 5: Verify node catches up ───
echo ""
echo "Phase 5: Waiting for node${PARTITION_NODE} to catch up (15s)..."
sleep 15

echo ""
echo "Phase 6: Verifying consistency..."
for i in $(seq 0 $((NODES - 1))); do
    HTTP_PORT=$((BASE_HTTP_PORT + i))
    STATUS=$(curl -s "http://127.0.0.1:${HTTP_PORT}/status" 2>/dev/null || echo '{}')
    HEIGHT=$(echo "${STATUS}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('latestBlockHeight',0))" 2>/dev/null || echo "0")
    SYNCING=$(echo "${STATUS}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('syncing',False))" 2>/dev/null || echo "unknown")
    echo "  node${i}: height=${HEIGHT} syncing=${SYNCING}"
done

echo ""
echo "=== Chaos Test: Network Partition Complete ==="
echo ""
echo "Manual verification:"
echo "  - All nodes should be at approximately the same height."
echo "  - node${PARTITION_NODE} should have caught up after healing."
echo "  - No conflicting state roots across nodes."
