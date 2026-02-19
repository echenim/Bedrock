
# EXECUTION_SPEC.md
**Bedrock Deterministic Execution Contract (WASM Sandbox + Strict Host API)**

This document defines the execution boundary between Bedrock’s **Go control plane** (consensus / networking / orchestration) and its **Rust execution core** compiled to **WASM** and run inside **Wasmtime**.

The execution engine is treated as a deterministic function:

```
f(previous_state_root, block) -> (new_state_root, receipts, events, gas_used, status)
```

The WASM module **must not** access the OS, filesystem, network, wall-clock time, or nondeterministic entropy.  
All interaction occurs through the **Host API** described here.

---

## 0. Terms

- **Host**: Go control plane embedding Wasmtime and implementing host functions.
- **Guest**: Rust-compiled WASM module that executes transactions and emits state updates.
- **State**: Key-value store representing application state, committed via a Merkle root.
- **Context**: Immutable per-block execution metadata and limits.
- **Determinism**: Given the same input `(prev_root, block, context)` the guest must produce identical outputs.

---

## 1. Goals and Non‑Goals

### Goals
- Deterministic execution across independent nodes
- Strict sandbox isolation (guest cannot escape)
- Minimal, explicit host surface area
- Replayability: all execution can be reproduced from recorded inputs
- Metered computation (gas) with deterministic accounting

### Non‑Goals
- Supporting nondeterministic syscalls (time, randomness, network, filesystem)
- Allowing the guest to manage consensus, networking, validator sets, or peer state
- Allowing host-defined “magic behavior” not captured in the context

---

## 2. Execution Lifecycle

### 2.1 Module Interface (Guest → Host)
The guest WASM must export the following functions:

- `bedrock_init(version_ptr, version_len) -> i32`
- `bedrock_execute_block(req_ptr, req_len, resp_ptr_ptr, resp_len_ptr) -> i32`
- `bedrock_free(ptr, len) -> void`

**Notes**
- The guest controls allocation for response buffers and must provide `bedrock_free`.
- All guest memory pointers are **linear memory offsets** (32-bit).
- The host validates that pointers and lengths are within the guest memory bounds.

### 2.2 Block Execution Phases
1. Host constructs an `ExecutionRequest` (Section 3)
2. Host calls `bedrock_execute_block`
3. Guest deserializes request, executes deterministically
4. Guest uses Host API for state access, events, gas, crypto verification
5. Guest returns an `ExecutionResponse` (Section 3)
6. Host verifies:
   - response schema/version
   - gas accounting validity
   - state root length/format
   - execution status
7. Consensus commits block **only after** the response is accepted

### 2.3 Atomicity
Execution is **atomic at the block level**:

- Either the response is accepted and the state root advances,
- Or execution fails and **no state changes are committed**.

The host must provide transactional semantics for state writes (Section 6.3).

---

## 3. Wire Format

### 3.1 Encoding
All request/response payloads are **Protocol Buffers** (recommended) or **canonical CBOR**.

**Requirement:** Encoding must be deterministic:
- Protobuf: use deterministic serialization in both host and guest
- CBOR: canonical encoding only

### 3.2 ExecutionRequest (logical schema)
Minimum required fields:

- `api_version: u32`
- `chain_id: bytes`
- `block_height: u64`
- `block_time: u64` *(logical time from consensus header; guest must not read OS time)*
- `block_hash: bytes32`
- `prev_state_root: bytes32`
- `txs: repeated bytes` *(opaque transaction payloads)*
- `limits: { gas_limit: u64, max_events: u32, max_write_bytes: u32, ... }`
- `execution_seed: bytes32` *(optional; must be derived deterministically from block header if used)*

### 3.3 ExecutionResponse (logical schema)
Minimum required fields:

- `api_version: u32`
- `status: enum { OK, INVALID_BLOCK, EXECUTION_ERROR, OUT_OF_GAS }`
- `new_state_root: bytes32`
- `gas_used: u64`
- `receipts: repeated Receipt`
- `events: repeated Event`
- `logs: repeated LogLine` *(optional; bounded)*

### 3.4 Receipt (logical schema)
- `tx_index: u32`
- `success: bool`
- `gas_used: u64`
- `result_code: u32`
- `return_data: bytes` *(bounded)*

### 3.5 Event (logical schema)
- `tx_index: u32`
- `type: string`
- `attributes: repeated { key: string, value: bytes }` *(bounded)*

---

## 4. Determinism Rules (Hard Requirements)

The guest MUST:
- Use **no wall-clock** or OS time
- Use **no OS randomness**
- Use **no threading** with nondeterministic scheduling
- Use deterministic iteration order (no hash-map iteration without stable ordering)
- Avoid floating point nondeterminism (prefer integer arithmetic)
- Use deterministic serialization (Section 3)
- Consume gas deterministically (Section 7)

The host MUST:
- Expose only the Host API listed in this spec
- Provide deterministic responses to host calls for a given state/version
- Ensure any execution seed is derived deterministically from block header (if used)
- Provide transactional state semantics

---

## 5. Memory & ABI

### 5.1 Pointer Safety
All host functions receiving pointers:
- Validate pointer range
- Validate length bounds
- Fail with `ERR_BAD_POINTER` on invalid access

### 5.2 Endianness
- All numeric values passed as raw bytes must be **little-endian**, unless specified otherwise.

### 5.3 Strings
- UTF-8 only
- Length-bounded
- Invalid UTF-8 → `ERR_INVALID_ENCODING`

---

## 6. State Model

### 6.1 Keyspace
State is a byte-addressed key-value store:
- Keys: `bytes` (1..MAX_KEY_LEN)
- Values: `bytes` (0..MAX_VALUE_LEN)

Keys are recommended to be namespaced:
- `acct/<addr>/balance`
- `acct/<addr>/nonce`
- `module/<name>/...`

### 6.2 Read Semantics
Reads must reflect:
- The committed state at `prev_state_root`
- Plus any writes performed earlier in the same block execution

### 6.3 Write Semantics (Transactional)
Writes are buffered during execution and become visible to subsequent reads in the same execution.
On success, the buffered writes are committed and contribute to `new_state_root`.

On failure:
- All buffered writes are discarded

### 6.4 Merkle Commitment
The host and/or guest may implement the Merkle structure, but **the resulting `new_state_root` must be computed deterministically** from the buffered writes.

---

## 7. Gas & Metering

### 7.1 Gas Accounting
- Gas is consumed for:
  - Host calls (state reads/writes, crypto verification)
  - Guest compute (instruction metering via Wasmtime fuel or equivalent)
  - Event emission and receipt/log sizes

### 7.2 Enforcement
If gas exceeds the block gas limit:
- Guest must terminate with `OUT_OF_GAS`
- Host must treat this as an invalid execution and discard writes

### 7.3 Deterministic Metering
The same block under the same protocol version must consume identical gas across nodes.

Implementation recommendation:
- Use Wasmtime fuel for instruction metering
- Charge fixed gas for each Host API call + proportional charges for bytes processed

---

## 8. Strict Host API (Normative)

All host functions are imported under the module name:

- `bedrock_host`

All functions return `i32` error codes:
- `0` = OK
- non-zero = error (Section 9)

### 8.1 State Access

#### `state_get(key_ptr: i32, key_len: i32, out_ptr_ptr: i32, out_len_ptr: i32) -> i32`
Reads a value for `key`.  
If key does not exist: returns OK with `out_len = 0` and `out_ptr = 0`.

- Charges gas: `G_STATE_GET + (key_len * G_PER_BYTE)`
- Output buffer is allocated by the host; guest must call `host_free` (below) when done.

#### `state_set(key_ptr: i32, key_len: i32, val_ptr: i32, val_len: i32) -> i32`
Writes value for `key` into the execution write buffer.

- Charges gas: `G_STATE_SET + ((key_len + val_len) * G_PER_BYTE)`
- Enforces max write bytes and key/value limits

#### `state_delete(key_ptr: i32, key_len: i32) -> i32`
Deletes key (tombstone) in the execution write buffer.

- Charges gas: `G_STATE_DEL + (key_len * G_PER_BYTE)`

### 8.2 Events & Logs

#### `emit_event(evt_ptr: i32, evt_len: i32) -> i32`
Appends an event (serialized Event message). Must be bounded by `max_events` and size limits.

- Charges gas proportional to `evt_len`

#### `log(level: i32, msg_ptr: i32, msg_len: i32) -> i32`
Bounded debug logging. Logs are **not** consensus-critical outputs and may be dropped by host under pressure.

- Logging must not affect determinism. Guest must never branch on log success/failure.

### 8.3 Cryptography

#### `hash_blake3(in_ptr: i32, in_len: i32, out_ptr: i32, out_len: i32) -> i32`
Computes BLAKE3 hash (or other protocol-selected hash). `out_len` must match required digest length.

#### `verify_ed25519(msg_ptr: i32, msg_len: i32, sig_ptr: i32, sig_len: i32, pk_ptr: i32, pk_len: i32) -> i32`
Returns OK if signature is valid, else `ERR_SIG_INVALID`.

#### `verify_bls_agg(msg_ptr: i32, msg_len: i32, sig_ptr: i32, sig_len: i32, pks_ptr: i32, pks_len: i32) -> i32`
Verifies aggregated BLS signature (format defined by protocol).

**Note:** Crypto verification must be deterministic. No randomization.

### 8.4 Gas Introspection (Optional)

#### `gas_remaining(out_ptr: i32) -> i32`
Writes remaining gas (u64 LE) to `out_ptr`.

Guest must not use gas remaining to introduce nondeterministic behavior; it may only enforce its own internal limits.

### 8.5 Host Memory Management

#### `host_free(ptr: i32, len: i32) -> i32`
Frees buffers allocated by host (e.g., `state_get` output).

**Rule:** Guest must not call `host_free` on non-host allocated memory.

### 8.6 Context Access (Read-only)

#### `get_context(out_ptr_ptr: i32, out_len_ptr: i32) -> i32`
Returns serialized `ExecutionContext` (chain_id, height, hash, limits, protocol params).

Context must be identical across validators for the same block.

---

## 9. Error Codes (Normative)

Error codes are `i32`:

- `0` OK
- `1` ERR_BAD_POINTER
- `2` ERR_INVALID_ENCODING
- `3` ERR_KEY_TOO_LARGE
- `4` ERR_VALUE_TOO_LARGE
- `5` ERR_WRITE_LIMIT
- `6` ERR_EVENT_LIMIT
- `7` ERR_OUT_OF_GAS
- `8` ERR_SIG_INVALID
- `9` ERR_CRYPTO_FAILED
- `10` ERR_INTERNAL

**Host behavior:**
- Any non-OK error returned during execution must fail the block execution deterministically.
- The host must discard buffered writes on failure.

---

## 10. Versioning & Compatibility

### 10.1 API Version
`api_version` is a monotonic u32.

Rules:
- Host rejects requests/responses with unsupported `api_version`
- Guest must return `INVALID_BLOCK` if request version is unsupported

### 10.2 Host API Evolution
New host functions may be added only via:
- new `api_version`
- feature flags in `ExecutionContext` that are consensus-defined

**Breaking changes** require a protocol upgrade and new version.

---

## 11. Canonical Test Vectors

To prove determinism, maintain golden vectors:
- `(prev_root, block, context) -> (new_root, receipts_hash, events_hash, gas_used)`

### Required CI checks
1. Run `bedrock_execute_block` against test vectors
2. Verify exact output match
3. Verify state root match
4. Verify gas match

Any mismatch is a consensus-critical failure.

---

## 12. Security Notes

- Guest must be sandboxed with no WASI filesystem/network modules enabled.
- Memory limits must be enforced (linear memory max pages).
- Fuel (instruction metering) must be enabled to prevent infinite loops.
- Host must validate all outputs for size limits to prevent memory exhaustion attacks.

---

## 13. Reference Call Sequence (Illustrative)

1. Host sets fuel/gas limits for the module instance
2. Host calls `bedrock_init(...)`
3. Host calls `bedrock_execute_block(req)`
4. Guest:
   - reads context via `get_context`
   - validates block header fields
   - for each tx:
     - `state_get` (read account/nonce)
     - verify signatures via `verify_ed25519`
     - apply deterministic state transition via `state_set`
     - emit event via `emit_event`
5. Guest returns response
6. Host verifies, commits writes, persists block

---

## 14. Minimal Host API Summary

| Category | Function | Deterministic | Metered |
|---|---|---:|---:|
| State | `state_get` | ✅ | ✅ |
| State | `state_set` | ✅ | ✅ |
| State | `state_delete` | ✅ | ✅ |
| Events | `emit_event` | ✅ | ✅ |
| Logs | `log` | ✅* | ✅ |
| Crypto | `verify_ed25519` | ✅ | ✅ |
| Crypto | `verify_bls_agg` | ✅ | ✅ |
| Hash | `hash_blake3` | ✅ | ✅ |
| Memory | `host_free` | ✅ | ✅ |
| Context | `get_context` | ✅ | ✅ |
| Gas | `gas_remaining` | ✅ | ✅ |

\* Logging must not affect control flow or outputs.

---

## 15. Implementation Notes (Practical)

### Recommended directory split
- `go/` — control plane, consensus, networking, runtime embedding
- `rust/` — execution engine, deterministic logic
- `rust/wasm/` — wasm build target + guest exports
- `specs/` — this document + protocol specs
- `test-vectors/` — golden inputs/outputs

### Build target
- Rust guest compiled to `wasm32-unknown-unknown` (or `wasm32-wasi` **with WASI disabled**)
- Wasmtime instantiated with:
  - fuel enabled
  - memory limits
  - no filesystem/network

---

## Appendix A: Determinism Checklist

- [ ] No wall-clock calls
- [ ] No OS randomness
- [ ] No filesystem/network imports
- [ ] Stable iteration ordering
- [ ] Deterministic serialization
- [ ] Deterministic gas accounting
- [ ] Golden vectors passing in CI
- [ ] Fuel limits enforced
- [ ] Output size bounds enforced

---

**This spec is normative.** Implementations that deviate risk consensus failure.
