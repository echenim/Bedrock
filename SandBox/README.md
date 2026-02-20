# BedRockSandBox

Deterministic, capability-restricted WebAssembly execution environment
for BedRock.

BedRockSandBox embeds Wasmtime and enforces strict execution boundaries
around untrusted WASM modules. It assumes guest code is adversarial.

## Responsibilities

-   WASM runtime embedding (Wasmtime)
-   Deterministic execution constraints
-   Fuel-based metering (gas enforcement)
-   Memory limits and stack isolation
-   Host function capability gating
-   Import whitelisting
-   ABI compatibility validation
-   Sandbox-level threat modeling

This repository does NOT implement consensus or state transition logic.

------------------------------------------------------------------------

## Design Goals

### Determinism

Execution must produce identical results across: - Different machines -
Different operating systems - Different validator nodes

Forbidden sources of nondeterminism: - System time - Host randomness -
Filesystem access - Network access - Floating-point nondeterministic
behavior

------------------------------------------------------------------------

### Capability Security Model

Guest modules run with zero ambient authority.

Every host function must be: - Explicitly declared - Deterministically
implemented - Metered - Versioned

No implicit access to: - Disk - Network - Environment variables - OS
syscalls

------------------------------------------------------------------------

### Fuel Metering

-   Each instruction consumes fuel.
-   Fuel exhaustion results in deterministic trap.
-   Fuel limits are provided externally by the control plane.
-   Execution cannot exceed assigned limits.

------------------------------------------------------------------------

### Memory Isolation

Each WASM instance: - Has bounded linear memory - Enforces maximum
memory pages - Cannot access host memory directly - Cannot share memory
unless explicitly allowed

------------------------------------------------------------------------

### Strict ABI Requirements

Execution modules must export:

-   bedrock_init
-   bedrock_execute
-   bedrock_free

Modules are validated before instantiation.

------------------------------------------------------------------------

## Architecture

Control Plane → loads versioned WASM artifact\
→ initializes BedRockSandBox\
→ provides deterministic inputs\
→ receives deterministic outputs

All state persistence is external to the sandbox.

------------------------------------------------------------------------

## Host API

Example host functions:

-   state_get
-   state_set
-   crypto_verify
-   emit_event

Rules: - No hidden side effects - No nondeterministic behavior - All
calls are metered - Changes require version bump

------------------------------------------------------------------------

## Threat Model

Assume guest modules are malicious.

The sandbox defends against: - Resource exhaustion - Determinism
violations - Capability escalation - ABI abuse

Security boundary: WASM module ↔ host runtime.

------------------------------------------------------------------------

## Testing Strategy

-   Golden vector execution tests
-   ABI compatibility tests
-   Fuel exhaustion tests
-   Memory limit tests
-   Fuzz testing malformed modules
-   Host call sequence fuzzing

------------------------------------------------------------------------

## Build

``` bash
cargo build --release
```

## Test

``` bash
cargo test
```

------------------------------------------------------------------------

## Security Policy

Report vulnerabilities privately according to SECURITY.md.

------------------------------------------------------------------------

## Philosophy

Consensus keeps the network alive.\
Execution defines truth.\
The sandbox protects both.
