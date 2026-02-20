#!/usr/bin/env bash
set -euo pipefail

# Spin up a 4-node local BedRock testnet.
# Each node gets its own home directory, key, and config.
# Nodes connect via localhost with different ports.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

NODES=${NODES:-4}
BASE_DIR="${BASE_DIR:-/tmp/bedrock-localnet}"
BASE_P2P_PORT=26656
BASE_GRPC_PORT=26657
BASE_HTTP_PORT=26700
BASE_ADMIN_PORT=26800
BINARY="${PROJECT_DIR}/bedrockd"
CHAIN_ID="bedrock-localnet"

echo "=== BedRock Local Testnet (${NODES} nodes) ==="
echo "  Chain ID:  ${CHAIN_ID}"
echo "  Base dir:  ${BASE_DIR}"
echo ""

# Build if binary doesn't exist.
if [ ! -f "${BINARY}" ]; then
    echo "Building bedrockd..."
    bash "${SCRIPT_DIR}/build.sh"
fi

# Clean previous run.
rm -rf "${BASE_DIR}"
mkdir -p "${BASE_DIR}"

# ─── Step 1: Initialize all nodes ───
echo "Step 1: Initializing ${NODES} nodes..."
declare -a PUB_KEYS
declare -a ADDRESSES

for i in $(seq 0 $((NODES - 1))); do
    HOME_DIR="${BASE_DIR}/node${i}"
    "${BINARY}" init "node${i}" --home "${HOME_DIR}" --chain-id "${CHAIN_ID}" >/dev/null

    # Extract public key and address from node_key.json.
    PUB_KEY=$(python3 -c "
import json, hashlib, binascii
with open('${HOME_DIR}/node_key.json') as f:
    kf = json.load(f)
pub = binascii.hexlify(bytes(kf['public_key'])).decode()
print(pub)
" 2>/dev/null || echo "")

    ADDRESS=$(python3 -c "
import json, hashlib, binascii
with open('${HOME_DIR}/node_key.json') as f:
    kf = json.load(f)
pub_bytes = bytes(kf['public_key'])
addr = hashlib.sha256(pub_bytes).hexdigest()
print(addr)
" 2>/dev/null || echo "")

    PUB_KEYS+=("${PUB_KEY}")
    ADDRESSES+=("${ADDRESS}")
    echo "  node${i}: addr=${ADDRESS:0:16}..."
done

# ─── Step 2: Create shared genesis with all validators ───
echo ""
echo "Step 2: Creating shared genesis..."
VALIDATORS_JSON=""
for i in $(seq 0 $((NODES - 1))); do
    if [ -n "${VALIDATORS_JSON}" ]; then
        VALIDATORS_JSON="${VALIDATORS_JSON},"
    fi
    VALIDATORS_JSON="${VALIDATORS_JSON}
    {
      \"address\": \"${ADDRESSES[$i]}\",
      \"public_key\": \"${PUB_KEYS[$i]}\",
      \"voting_power\": 100
    }"
done

GENESIS="{
  \"chain_id\": \"${CHAIN_ID}\",
  \"validators\": [${VALIDATORS_JSON}
  ]
}"

# ─── Step 3: Distribute genesis and configure peers ───
echo "Step 3: Configuring nodes..."

# Build seed peer list (all nodes know about all others).
SEEDS=""
for i in $(seq 0 $((NODES - 1))); do
    P2P_PORT=$((BASE_P2P_PORT + i))
    if [ -n "${SEEDS}" ]; then
        SEEDS="${SEEDS},"
    fi
    SEEDS="${SEEDS}/ip4/127.0.0.1/udp/${P2P_PORT}/quic-v1"
done

for i in $(seq 0 $((NODES - 1))); do
    HOME_DIR="${BASE_DIR}/node${i}"
    P2P_PORT=$((BASE_P2P_PORT + i))
    GRPC_PORT=$((BASE_GRPC_PORT + i * 10))
    HTTP_PORT=$((BASE_HTTP_PORT + i))

    # Write shared genesis.
    echo "${GENESIS}" > "${HOME_DIR}/genesis.json"

    # Update config with unique ports and seed peers.
    python3 -c "
import sys
try:
    import tomli_w as toml_write
    import tomllib as toml_read
except ImportError:
    try:
        import tomli as toml_read
        import tomli_w as toml_write
    except ImportError:
        # Fallback: just write a minimal TOML config.
        config = '''
moniker = \"node${i}\"
chain_id = \"${CHAIN_ID}\"

[consensus]
timeout_propose = \"3s\"
timeout_vote = \"1s\"
timeout_commit = \"1s\"
max_block_size = 2097152
max_block_gas = 100000000

[p2p]
listen_addr = \"/ip4/0.0.0.0/udp/${P2P_PORT}/quic-v1\"
max_peers = 50
peer_scoring = true

[mempool]
max_size = 10000
max_tx_bytes = 1048576
cache_size = 10000

[storage]
db_path = \"data/blockstore\"
backend = \"memory\"

[rpc]
grpc_addr = \"127.0.0.1:${GRPC_PORT}\"
http_addr = \"127.0.0.1:${HTTP_PORT}\"

[execution]
wasm_path = \"bedrock-execution.wasm\"
gas_limit = 100000000
fuel_limit = 100000000
max_memory_mb = 256

[telemetry]
enabled = false
addr = \"127.0.0.1:0\"
'''
        with open('${HOME_DIR}/config.toml', 'w') as f:
            f.write(config)
        sys.exit(0)

with open('${HOME_DIR}/config.toml', 'rb') as f:
    cfg = toml_read.load(f)

cfg['moniker'] = 'node${i}'
cfg['chain_id'] = '${CHAIN_ID}'
cfg['p2p']['listen_addr'] = '/ip4/0.0.0.0/udp/${P2P_PORT}/quic-v1'
cfg['rpc']['grpc_addr'] = '127.0.0.1:${GRPC_PORT}'
cfg['rpc']['http_addr'] = '127.0.0.1:${HTTP_PORT}'
cfg['storage']['backend'] = 'memory'
cfg['telemetry']['enabled'] = False

with open('${HOME_DIR}/config.toml', 'wb') as f:
    toml_write.dump(cfg, f)
" 2>/dev/null || true

    echo "  node${i}: p2p=${P2P_PORT} grpc=${GRPC_PORT} http=${HTTP_PORT}"
done

# ─── Step 4: Start all nodes ───
echo ""
echo "Step 4: Starting nodes..."
PIDS=()

cleanup() {
    echo ""
    echo "Shutting down localnet..."
    for pid in "${PIDS[@]}"; do
        kill "${pid}" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    echo "Localnet stopped."
}
trap cleanup EXIT INT TERM

for i in $(seq 0 $((NODES - 1))); do
    HOME_DIR="${BASE_DIR}/node${i}"
    LOG_FILE="${BASE_DIR}/node${i}.log"

    "${BINARY}" start --home "${HOME_DIR}" --log-level production \
        > "${LOG_FILE}" 2>&1 &
    PIDS+=($!)
    echo "  node${i} started (PID: ${PIDS[-1]}, log: ${LOG_FILE})"
done

# ─── Step 5: Wait for nodes to initialize ───
echo ""
echo "Step 5: Waiting for nodes to initialize..."
sleep 5

# Check that all nodes are running.
RUNNING=0
for i in $(seq 0 $((NODES - 1))); do
    if kill -0 "${PIDS[$i]}" 2>/dev/null; then
        RUNNING=$((RUNNING + 1))
    else
        echo "  WARNING: node${i} (PID ${PIDS[$i]}) is not running!"
        echo "  Log: $(tail -5 "${BASE_DIR}/node${i}.log" 2>/dev/null || echo "no log")"
    fi
done

echo "  ${RUNNING}/${NODES} nodes running."

# ─── Step 6: Verify gRPC endpoints ───
echo ""
echo "Step 6: Checking node status..."
for i in $(seq 0 $((NODES - 1))); do
    GRPC_PORT=$((BASE_GRPC_PORT + i * 10))
    HTTP_PORT=$((BASE_HTTP_PORT + i))

    # Try HTTP health endpoint.
    STATUS=$(curl -s "http://127.0.0.1:${HTTP_PORT}/health" 2>/dev/null || echo "unreachable")
    echo "  node${i}: http=${HTTP_PORT} status=${STATUS}"
done

echo ""
echo "=== Localnet Running ==="
echo "  Nodes: ${RUNNING}/${NODES}"
echo "  gRPC ports: $(seq -s ', ' $((BASE_GRPC_PORT)) 10 $((BASE_GRPC_PORT + (NODES - 1) * 10)))"
echo "  HTTP ports: $(seq -s ', ' $((BASE_HTTP_PORT)) 1 $((BASE_HTTP_PORT + NODES - 1)))"
echo ""
echo "Press Ctrl+C to stop."
echo ""

# Wait for all node processes.
wait
