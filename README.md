 Bedrock  
**A Production-Grade BFT Protocol Node**

A Byzantine Fault Tolerant (BFT) full node engineered for adversarial environments.  
Designed from first principles to assume the network is hostile, partitions are inevitable, and validators may equivocate.

Bedrock is not a toy blockchain. It is a deterministic state machine wrapped in a consensus engine built to survive ≥1/3 Byzantine validators while preserving safety and finality.

---

## Overview

Bedrock is a full protocol node integrating:

- BFT consensus (HotStuff / Tendermint-style)
- Validator rotation & equivocation detection
- Deterministic state execution (WASM-based)
- Adversarial P2P networking
- Mempool & transaction propagation
- Snapshot synchronization & fast bootstrap
- Multi-region testnet deployment & observability

The design assumption is simple:  
The network lies. Peers misbehave. Time drifts. Messages arrive out of order.  
The protocol must still converge.

---

## Architecture

Client RPC → Mempool → Consensus → Deterministic State Machine → Storage

Networking Layer:
- libp2p over QUIC
- Kademlia-style peer discovery
- Gossip propagation
- Peer scoring & rate limiting

---

## Consensus Model

HotStuff / Tendermint-inspired BFT protocol:

- Rotating proposer model
- Quorum certificates
- Locking rules for safety
- Deterministic finality
- Explicit equivocation detection
- BLS / Ed25519 signature verification
- Liveness recovery after partitions

### Safety
No two honest validators commit conflicting blocks even with ≤1/3 Byzantine validators.

### Liveness
The network resumes finalization automatically after partitions heal.

---

## Deterministic Execution

- WASM-based execution (Wasmtime)
- Merkleized state tree
- Identical state root reproduction across independent nodes
- Snapshot synchronization for fast bootstrap
- Deterministic replay harness

If two honest nodes disagree, it is treated as a critical protocol failure.

---

## Byzantine Testing

Fault-injection harness simulates:

- ≥1/3 Byzantine validators
- Equivocation attempts
- Network partitions
- Adversarial block proposals
- Delayed vote propagation

Measured metrics:

- Finality time
- Fork rate
- Bandwidth per node
- Recovery time after partition

---

## Observability

- Prometheus metrics
- Grafana dashboards
- OpenTelemetry tracing
- Finality latency tracking
- Peer churn & bandwidth monitoring

---

## Multi-Region Testnet

Deployed across AWS and GCP using:

- Docker
- Kubernetes
- Terraform
- GitHub Actions

Validates real-world latency, partitions, and validator churn.

---

## Tech Stack

Core:
- Go
- Rust
- Protobuf
- libp2p
- QUIC
- BLS / Ed25519

State & Storage:
- RocksDB
- Merkle Trees
- Wasmtime (WASM)

Infra & Ops:
- Docker
- Kubernetes
- Terraform
- AWS
- GCP
- GitHub Actions
- Prometheus
- Grafana
- OpenTelemetry

---

## Design Principles

Determinism over convenience.  
Safety before liveness.  
Explicit trust assumptions.  
Measure everything.

---

## Timeline

January 2023 – November 2023

- Protocol specification authored
- Consensus engine implemented
- Deterministic state machine completed
- Fault-injection harness built
- Multi-region testnet deployed
- Byzantine safety validated

---

Bedrock represents foundational infrastructure engineering:  
consensus as a correctness guarantee under adversarial conditions.
