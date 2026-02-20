//! Host API trait — host-side function signatures for the BedRock sandbox.
//!
//! The `HostApi` trait is the mirror of the guest's `HostInterface`.
//! Each method corresponds to a function in EXECUTION_SPEC.md §8.
//! The sandbox creates an instance for each block execution.
//!
//! Pointer validation happens in the sandbox (task 006), not here.
//! This trait works with Rust slices and types, not raw WASM pointers.

use crate::error::HostError;

/// Host-side implementation of the execution host API.
///
/// The sandbox creates an instance of this for each block execution.
/// Each method corresponds to a function imported by the WASM guest
/// under the `bedrock_host` module (EXECUTION_SPEC.md §8).
///
/// Gas is charged by the implementation before performing the operation.
/// If gas runs out, methods return `Err(HostError::out_of_gas())`.
pub trait HostApi {
    // ── State Access (§8.1) ──

    /// Read a value from state.
    ///
    /// Reads check the write buffer (overlay) first, then fall through
    /// to committed state. Returns `Ok(None)` if the key doesn't exist
    /// in either layer.
    ///
    /// Gas: `G_STATE_GET + (key_len * G_PER_BYTE)`
    fn state_get(&self, key: &[u8]) -> Result<Option<Vec<u8>>, HostError>;

    /// Write a key-value pair to the execution write buffer.
    ///
    /// Enforces `max_key_len`, `max_value_len`, and `max_write_bytes` limits.
    ///
    /// Gas: `G_STATE_SET + ((key_len + val_len) * G_PER_BYTE)`
    fn state_set(&mut self, key: &[u8], value: &[u8]) -> Result<(), HostError>;

    /// Delete a key from state (tombstone in write buffer).
    ///
    /// Subsequent reads for this key will return `None` even if the key
    /// exists in committed state.
    ///
    /// Gas: `G_STATE_DEL + (key_len * G_PER_BYTE)`
    fn state_delete(&mut self, key: &[u8]) -> Result<(), HostError>;

    // ── Events & Logs (§8.2) ──

    /// Emit a serialized event.
    ///
    /// Bounded by `max_events`. The `event_data` is the serialized Event
    /// bytes as produced by the guest.
    ///
    /// Gas: proportional to `event_data.len()`
    fn emit_event(&mut self, event_data: &[u8]) -> Result<(), HostError>;

    /// Debug log. Not consensus-critical.
    ///
    /// Guest must not branch on log success/failure (EXECUTION_SPEC.md §8.2).
    /// The host may drop logs under pressure.
    fn log(&mut self, level: u32, message: &str) -> Result<(), HostError>;

    // ── Crypto (§8.3) ──

    /// Compute BLAKE3 hash.
    ///
    /// `output` must be exactly 32 bytes.
    fn hash_blake3(&self, input: &[u8], output: &mut [u8]) -> Result<(), HostError>;

    /// Verify an Ed25519 signature.
    ///
    /// Returns `Ok(true)` if the signature is valid, `Ok(false)` if invalid.
    /// Returns `Err` only for malformed inputs or internal failures.
    fn verify_ed25519(
        &self,
        msg: &[u8],
        sig: &[u8],
        pk: &[u8],
    ) -> Result<bool, HostError>;

    /// Verify an aggregated BLS signature.
    ///
    /// BLS is listed as optional in SPEC.md §3. Initial implementation
    /// returns `Err(HostError::crypto_failed())` as a stub.
    fn verify_bls_agg(
        &self,
        msg: &[u8],
        sig: &[u8],
        pks: &[u8],
    ) -> Result<bool, HostError>;

    // ── Gas Introspection (§8.4) ──

    /// Returns remaining gas.
    ///
    /// Guest must not use this to introduce nondeterministic behavior.
    fn gas_remaining(&self) -> Result<u64, HostError>;

    // ── Context (§8.6) ──

    /// Returns the serialized `ExecutionContext`.
    ///
    /// Context must be identical across validators for the same block.
    fn get_context(&self) -> Result<Vec<u8>, HostError>;
}
