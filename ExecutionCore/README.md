# BedRockExecutionCore

BedRockExecutionCore is the deterministic state transition engine for
the BedRock protocol.

It defines how blocks are executed, how state changes are applied, and
how canonical state commitments are produced. The engine is written in
Rust and compiled to WebAssembly (WASM) to guarantee portability,
determinism, and isolation from the control plane.

This repository contains no networking, consensus, or orchestration
logic. It is a pure execution environment.

------------------------------------------------------------------------

## Design Goals

Determinism is non-negotiable. Given identical inputs, the engine must
produce identical outputs across machines, operating systems, and time.

Security through minimalism. The execution core only defines state
transitions and commitment rules. It does not manage peers, gossip,
leader election, or block propagation.

WASM as a portability layer. The engine compiles to a versioned `.wasm`
artifact consumed by the control plane and executed inside the BedRock
sandbox.

Reproducibility. Execution results are validated via golden test vectors
and fuzz testing.

------------------------------------------------------------------------

## Responsibilities

-   Transaction execution
-   State transition logic
-   Canonical serialization
-   Merkle/state root computation
-   Gas accounting model
-   Deterministic collections and data structures
-   Versioned execution artifact generation

------------------------------------------------------------------------

## Non-Goals

-   Consensus logic
-   P2P networking
-   Node orchestration
-   Runtime sandboxing
-   Host capability enforcement

------------------------------------------------------------------------

## Architecture Overview

Input: - Previous state root - Block metadata - Ordered list of
transactions

Output: - New state root - Execution receipts - Events - Deterministic
logs

State transitions must be: - Pure (no hidden global state) -
Order-dependent but deterministic - Independent of wall clock time -
Independent of host randomness - Independent of machine architecture

All randomness must be derived from deterministic inputs.

------------------------------------------------------------------------

## Repository Structure

engine/ Core execution logic\
primitives/ Merkle, codecs, crypto wrappers\
tests/\
vectors/ Golden test vectors\
fuzz/ Property-based and adversarial testing\
wasm/artifacts/ Versioned compiled WASM output\
docs/ Execution model and determinism documentation

------------------------------------------------------------------------

## Determinism Rules

To preserve consensus safety:

-   No use of system time
-   No use of OS entropy
-   No floating point arithmetic
-   No non-deterministic iteration over hash maps
-   No architecture-dependent behavior
-   Stable serialization for all state objects

Deterministic collections must be used in place of standard HashMap or
HashSet where ordering matters.

------------------------------------------------------------------------

## Build

cargo build --release

To build the WASM artifact:

cargo build --release --target wasm32-unknown-unknown

The resulting artifact should be placed under:

wasm/artifacts/bedrock-execution-vX.Y.Z.wasm

Versioning must follow semantic versioning and reflect execution logic
changes.

------------------------------------------------------------------------

## Testing

Run unit tests:

cargo test

Run fuzz tests (if configured):

cargo test --features fuzz

Golden test vectors validate cross-platform determinism and must pass
before release.

------------------------------------------------------------------------

## Versioning & Compatibility

The execution engine is versioned independently from the control plane.

Any change that affects: - State layout - Serialization format - Gas
accounting - Cryptographic hashing - Execution ordering

Requires a version bump and corresponding test vector updates.

Backward compatibility must be explicitly documented in docs/.

------------------------------------------------------------------------

## Security Model

The execution engine assumes: - It may be executed inside an untrusted
host - Input blocks may be adversarial - Transactions may attempt
resource exhaustion

Resource limits (fuel, memory caps, import restrictions) are enforced by
BedRockSandBox, not this repository.

Security vulnerabilities should be reported according to SECURITY.md.

------------------------------------------------------------------------

## Development Principles

-   Favor explicit state transitions over implicit mutation
-   Prefer immutable data structures where possible
-   Keep execution logic isolated from environment assumptions
-   Treat every change as a potential consensus fork

This repository defines protocol truth. Changes here must be reviewed
with extreme care.
