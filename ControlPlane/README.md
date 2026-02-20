# BedRockControlPlane

Go-based Control Plane for the BedRock Protocol.

BedRockControlPlane is responsible for consensus, networking,
transaction admission, block orchestration, and lifecycle management of
the deterministic execution engine.

It does **not** implement state transition logic. It orchestrates it.

------------------------------------------------------------------------

## Philosophy

The Control Plane exists to guarantee:

-   Safety --- no two honest nodes finalize conflicting blocks.
-   Liveness --- the network continues making progress under partial
    failure.
-   Isolation --- execution logic remains a deterministic black box.
-   Operational clarity --- faults are observable, bounded, and
    recoverable.

Consensus logic, networking, and orchestration remain cleanly separated
from execution semantics to prevent subtle coupling that can halt
networks.

------------------------------------------------------------------------

## Responsibilities

### 1. Consensus Engine

HotStuff / Tendermint-style BFT consensus.

-   Validator rotation
-   Propose / vote / commit phases
-   Equivocation detection
-   ≥1/3 Byzantine fault tolerance
-   Deterministic finality

### 2. P2P Networking

libp2p + QUIC-based transport.

-   Peer discovery
-   Gossip propagation
-   Peer scoring
-   Rate limiting
-   Anti-spam controls

### 3. Mempool & Fee Market

-   Admission control
-   Replacement & eviction policy
-   Fee prioritization
-   Spam resistance
-   Deterministic ordering guarantees

### 4. Block Sync & Snapshots

-   Fast sync
-   Snapshot orchestration
-   State root verification
-   Replay validation

### 5. Execution Orchestration

Interface to BedRockExecutionCore (WASM).

-   Deterministic execution invocation
-   State root validation
-   Gas accounting enforcement
-   Execution failure handling
-   Version compatibility checks

### 6. Observability & Operations

-   Structured logging
-   Metrics (Prometheus)
-   Tracing hooks
-   Admin endpoints
-   Safe maintenance controls

------------------------------------------------------------------------

## Architecture Overview

                    +----------------------+
                    |   BedRockControlPlane|
                    |----------------------|
                    |  Consensus Engine    |
                    |  Mempool             |
                    |  P2P Networking      |
                    |  Sync & Snapshots    |
                    |  Execution Adapter   |
                    +----------+-----------+
                               |
                               v
                    +----------------------+
                    | BedRockExecutionCore |
                    |  (WASM deterministic)|
                    +----------------------+

Execution is treated as a deterministic state transition function:

    new_state_root = Execute(prev_state_root, block)

The Control Plane verifies the resulting root matches network consensus.

------------------------------------------------------------------------

## Project Structure

    cmd/bedrockd/        Main node binary
    internal/
      consensus/         BFT engine
      p2p/               Networking stack
      mempool/           Transaction logic
      sync/              Block sync
      storage/           Block store abstraction
      rpc/               gRPC / HTTP API
      node/              Lifecycle supervisor
      config/            Configuration loader
      telemetry/         Metrics & tracing
      admin/             Safe debug endpoints
    pkg/client/          Optional Go SDK
    proto/               gRPC definitions
    scripts/             Build/test/chaos tools
    deployments/         Docker / k8s configs

------------------------------------------------------------------------

## Determinism Boundary

The Control Plane must never:

-   Perform nondeterministic execution logic
-   Depend on local wall-clock for state transitions
-   Inject randomness into execution results
-   Modify execution outputs

All state transitions are produced by the WASM execution engine and
verified by hash.

------------------------------------------------------------------------

## Security Model

Assumes:

-   Up to 1/3 validators may be Byzantine
-   Peers may attempt spam, equivocation, or replay
-   Execution modules may fail deterministically
-   Network partitions will occur

Mitigations:

-   BFT safety proofs
-   Rate-limited gossip
-   Peer scoring & eviction
-   Snapshot validation
-   Strict execution root verification

------------------------------------------------------------------------

## Building

    make build

Or manually:

    go build -o bedrockd ./cmd/bedrockd

------------------------------------------------------------------------

## Running

    ./bedrockd --config config.yaml

Local testnet:

    ./scripts/localnet.sh

Chaos testing:

    ./scripts/chaos_partition.sh
    ./scripts/chaos_byzantine.sh

------------------------------------------------------------------------

## Versioning

Follows:

MAJOR.MINOR.PATCH

Compatibility defined against:

-   ExecutionCore version
-   WASM ABI version
-   Consensus protocol version

Breaking changes require explicit upgrade documentation.

------------------------------------------------------------------------

## License

See LICENSE file.
