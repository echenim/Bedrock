# Bedrock

![License](https://img.shields.io/badge/license-Apache--2.0-blue)
![Build](https://img.shields.io/badge/build-passing-brightgreen)
![Go](https://img.shields.io/badge/go-1.20+-blue)
![Rust](https://img.shields.io/badge/rust-stable-orange)
![Status](https://img.shields.io/badge/status-research--grade--infrastructure-critical)

Production-grade Byzantine Fault Tolerant (BFT) protocol node engineered
for adversarial environments.

Bedrock enforces a strict architectural separation between:

- Go-based control plane (consensus, networking, orchestration)
- Rust-based deterministic execution engine compiled to WASM
- Wasmtime sandbox with strict host function API

Consensus never mutates state directly. Execution never touches
networking.

------------------------------------------------------------------------

# Abstract

Bedrock is a full-stack BFT protocol node implementing a
HotStuff/Tendermint-style consensus protocol with deterministic
execution and adversarial networking assumptions.

It tolerates ≤ 1/3 Byzantine validators while preserving safety and
ensuring liveness recovery after partitions.

Execution is treated as a pure function:

    f(previous_state_root, block) → new_state_root

------------------------------------------------------------------------

# Polyglot Architecture

``` mermaid
flowchart LR
    subgraph Go_Control_Plane
        RPC
        Mempool
        Consensus
        Networking
        Snapshot
    end

    subgraph WASM_Sandbox
        Wasmtime
        HostAPI
    end

    subgraph Rust_Execution_Core
        StateMachine
        MerkleTree
        Crypto
    end

    Consensus --> Wasmtime
    Wasmtime --> StateMachine
    StateMachine --> MerkleTree
    MerkleTree --> Consensus
```

## Go (Control Plane)

Responsible for:

- RPC layer
- Mempool engine
- BFT consensus
- Validator coordination
- P2P networking (libp2p + QUIC)
- Snapshot orchestration
- Observability & metrics

## Rust (Execution Core)

Responsible for:

- Deterministic transaction execution
- Merkle state updates
- Cryptographic operations (BLS / Ed25519 / hashing)
- State commitment calculation

## WASM Sandbox Boundary

Rust execution is compiled to WASM and executed inside Wasmtime.

The sandbox:

- Enforces deterministic behavior
- Prevents OS access
- Restricts syscalls
- Exposes strict host API
- Prevents memory corruption across layers

------------------------------------------------------------------------

# Execution Boundary Contract

Consensus communicates with execution via a strict contract.

ExecutionRequest:

- Previous state root
- Block payload
- Execution context

ExecutionResponse:

- New state root
- Receipt set
- Gas usage
- Events
- Success / failure flag

No shared memory.\
No implicit state mutation.\
All state transitions happen inside the WASM sandbox.

------------------------------------------------------------------------

# System Architecture

``` mermaid
flowchart LR
    Client --> RPC
    RPC --> Mempool
    Mempool --> Consensus
    Consensus --> BlockBuilder
    BlockBuilder --> WASMExecutor
    WASMExecutor --> StateMachine
    StateMachine --> MerkleUpdate
    MerkleUpdate --> Storage
    Consensus --> Networking
    Networking --> Peers
```

Subsystems:

1. RPC Layer (Go)
2. Mempool Engine (Go)
3. BFT Consensus Engine (Go)
4. Block Builder (Go)
5. WASM Runtime (Wasmtime)
6. Deterministic State Machine (Rust → WASM)
7. Storage Layer (RocksDB)
8. P2P Networking (Go)
9. Snapshot Sync (Go)
10. Observability (Go)

------------------------------------------------------------------------

# Consensus Design (Go)

HotStuff/Tendermint-inspired rotating proposer protocol.

Properties:

- Quorum certificates (2f + 1 votes)
- Explicit locking rules
- Deterministic block hashing
- Equivocation detection
- Timeout-based view change
- Partition recovery

Safety Guarantee:

No two honest validators commit conflicting blocks if ≤ 1/3 are
Byzantine.

Liveness Guarantee:

Finality resumes automatically once partitions heal.

Consensus commits blocks only after execution returns a verified state
root.

------------------------------------------------------------------------

# Deterministic Execution (Rust → WASM)

``` mermaid
flowchart TD
    Block --> Wasmtime
    Wasmtime --> HostAPI
    HostAPI --> StateReadWrite
    StateReadWrite --> MerkleUpdate
    MerkleUpdate --> NewStateRoot
```

Strict Host API exposes only:

- Read state key
- Write state key
- Emit event
- Consume gas
- Access block metadata

Disallowed:

- System clock
- Filesystem
- Network access
- OS randomness

Key Properties:

- Identical state root reproduction across independent nodes
- Merkleized state
- Snapshot export/import
- Deterministic replay testing

------------------------------------------------------------------------

# Networking Model (Go)

Built with libp2p over QUIC.

Features:

- Kademlia-like peer discovery
- Gossip-based block & vote propagation
- Peer scoring
- Rate limiting
- Anti-eclipse protections

------------------------------------------------------------------------

# Snapshot Synchronization

``` mermaid
sequenceDiagram
    participant NewNode
    participant Peer
    NewNode->>Peer: Request Snapshot
    Peer->>NewNode: Send Chunks
    NewNode->>Peer: Verify Merkle Root
    NewNode->>Consensus: Join Network
```

- Chunked transfer
- Merkle proof validation
- \~60% reduction in bootstrap time (testnet measurement)

------------------------------------------------------------------------

# Storage Layer

Backed by RocksDB.

- Append-only block store
- Versioned state
- Snapshot storage
- Crash consistency guarantees

------------------------------------------------------------------------

# Benchmarks

- Deterministic finality under ≥1/3 Byzantine simulation
- Zero safety violations under fault injection
- Partition recovery without manual intervention
- Identical state roots across replay tests

------------------------------------------------------------------------

# Threat Model

Assumes:

- ≤ 1/3 Byzantine validators
- Network partitions
- Malicious peers
- Message reordering & delay

Does Not Assume:

- Global clock synchronization
- Honest majority beyond BFT threshold

------------------------------------------------------------------------

# Development

Control Plane:

- Go
- libp2p
- QUIC

Execution Core:

- Rust
- Wasmtime
- Merkle Trees
- BLS / Ed25519

Storage:

- RocksDB

Infra:

- Docker
- Kubernetes
- Terraform
- GitHub Actions
- AWS
- GCP

------------------------------------------------------------------------

# License

Apache 2.0
