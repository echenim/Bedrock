# Bedrock Protocol Specification (SPEC.md)

Version: 0.1\
Status: Draft -- Research Grade Infrastructure

------------------------------------------------------------------------

# 1. Introduction

This document specifies the Bedrock Byzantine Fault Tolerant (BFT)
protocol.

Bedrock is a partially synchronous, leader-rotating BFT consensus
protocol with deterministic execution and Merkleized state commitments.
The protocol tolerates up to f Byzantine validators in a validator set
of size n, where:

n ≥ 3f + 1

The protocol guarantees:

-   Safety: No two honest validators commit conflicting blocks.
-   Liveness: The system makes progress once network synchrony is
    restored.

------------------------------------------------------------------------

# 2. System Model

## 2.1 Network Model

-   Partially synchronous network.
-   Messages may be delayed, duplicated, or reordered.
-   Eventually, after GST (Global Stabilization Time), messages are
    delivered within bounded delay.

## 2.2 Fault Model

-   Up to f Byzantine validators.
-   Byzantine behavior includes:
    -   Equivocation
    -   Arbitrary message fabrication
    -   Message withholding
    -   Vote duplication

Honest validators follow protocol strictly.

------------------------------------------------------------------------

# 3. Cryptographic Primitives

-   Ed25519 (validator identity signatures)
-   BLS (optional aggregated vote signatures)
-   SHA-256 (block hashing)
-   Merkle Trees (state commitment)
-   Protobuf (canonical encoding)

All hashes are computed over canonical serialized structures.

------------------------------------------------------------------------

# 4. Data Structures

## 4.1 Block

Block { height: uint64 round: uint64 parent_hash: bytes32 state_root:
bytes32 tx_root: bytes32 proposer_id: ValidatorID quorum_certificate: QC
transactions: \[\]Transaction }

## 4.2 Quorum Certificate (QC)

QC { block_hash: bytes32 round: uint64 signatures: \[\]Signature (≥
2f + 1) }

## 4.3 Vote

Vote { block_hash: bytes32 round: uint64 voter_id: ValidatorID
signature: Signature }

------------------------------------------------------------------------

# 5. Protocol Overview

The protocol proceeds in rounds.

Each round has:

1.  Proposal Phase
2.  Vote Phase
3.  Commit Phase

Leader (proposer) rotates deterministically per round.

------------------------------------------------------------------------

# 6. Proposal Phase

The designated proposer:

1.  Selects transactions from mempool.
2.  Builds block referencing highest QC.
3.  Signs proposal.
4.  Broadcasts to validators.

Block validity rules:

-   Parent exists.
-   State transition deterministic.
-   Transactions valid.
-   QC valid (≥ 2f + 1 signatures).

Invalid blocks are rejected.

------------------------------------------------------------------------

# 7. Vote Phase

Upon receiving valid proposal:

Validator:

1.  Verifies block validity.
2.  Ensures no lock violation.
3.  Signs vote.
4.  Broadcasts vote.

Votes must be unique per (height, round).

Equivocation detection:

If a validator signs two different blocks at same height/round, proof of
equivocation is recorded.

------------------------------------------------------------------------

# 8. Commit Rule

A block B is committed if:

-   B has QC
-   Its parent also has QC (two-chain rule)
-   Lock conditions satisfied

Commit is deterministic and identical across honest nodes.

------------------------------------------------------------------------

# 9. Locking Rules

Each validator maintains:

-   highestQC
-   lockedBlock

Validator only votes for blocks that extend lockedBlock or justify
unlocking via higher QC.

This prevents conflicting commits.

------------------------------------------------------------------------

# 10. View Change (Round Change)

If timeout expires without QC formation:

1.  Validator increments round.
2.  Broadcasts timeout message.
3.  Next proposer selected deterministically.

Timeout increases exponentially under repeated failures.

------------------------------------------------------------------------

# 11. Deterministic Execution

Transactions execute via WASM engine.

State transition function:

State\_{h+1} = Apply(Block_h, State_h)

Properties:

-   No nondeterminism.
-   No wall-clock access.
-   No random syscalls.
-   Identical state_root across honest nodes.

State commitment uses Merkle tree root.

------------------------------------------------------------------------

# 12. Snapshot Synchronization

New node:

1.  Requests snapshot at height h.
2.  Verifies state_root via Merkle proofs.
3.  Downloads block headers.
4.  Joins consensus.

Snapshot integrity verified against committed QC.

------------------------------------------------------------------------

# 13. Safety Proof Sketch

Given n ≥ 3f + 1:

Any two QCs require ≥ 2f + 1 votes.

Two conflicting QCs would require at least f + 1 honest validators to
double-vote.

Honest validators never double-vote due to locking rule.

Therefore, conflicting commits cannot occur.

------------------------------------------------------------------------

# 14. Liveness Conditions

After GST:

-   Network delay bounded.
-   Honest proposer eventually selected.
-   Honest validators respond in time.

Thus QC forms and protocol progresses.

------------------------------------------------------------------------

# 15. Mempool Rules

-   Transactions validated statelessly then statefully.
-   Deterministic ordering.
-   Size-bounded.
-   Replay protection via nonce.
-   Signature verification required.

------------------------------------------------------------------------

# 16. Threat Model Summary

Protected Against:

-   ≤ 1/3 Byzantine validators
-   Network partitions
-   Message reordering
-   Equivocation

Not Protected Against:

-   1/3 coordinated Byzantine validators

-   Cryptographic primitive failure

------------------------------------------------------------------------

# 17. Parameters

Validator Set Size: n ≥ 4 (minimum)

Byzantine Threshold: f = floor((n - 1) / 3)

QC Threshold: 2f + 1

Block Time: configurable (default 2--4s in testnet)

------------------------------------------------------------------------

# 18. Metrics

Measured in testnet:

-   Finality latency
-   Fork rate
-   Vote propagation delay
-   Partition recovery time
-   Bootstrap time (snapshot sync)

------------------------------------------------------------------------

# 19. Future Extensions

-   Dynamic validator set updates
-   Slashing conditions
-   Aggregated BLS signatures
-   Formal verification of locking rules
-   zk-proof-compatible state commitments

------------------------------------------------------------------------

End of Specification.
