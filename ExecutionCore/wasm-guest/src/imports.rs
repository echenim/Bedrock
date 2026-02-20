//! Host function imports (EXECUTION_SPEC.md §8).
//!
//! All host functions are imported under the `bedrock_host` WASM module.
//! Each function returns an `i32` error code (0 = OK, non-zero = error).
//! See EXECUTION_SPEC.md §9 for the error code table.

#[link(wasm_import_module = "bedrock_host")]
extern "C" {
    // ── State access (§8.1) ──

    /// Read a value from state.
    /// Host allocates the output buffer; guest reads from `*out_ptr_ptr` with
    /// length `*out_len_ptr`. Guest must call `host_free` when done.
    /// If key not found: `*out_len_ptr = 0`, `*out_ptr_ptr = 0`.
    pub fn state_get(
        key_ptr: i32,
        key_len: i32,
        out_ptr_ptr: i32,
        out_len_ptr: i32,
    ) -> i32;

    /// Write a key-value pair to the state write buffer.
    pub fn state_set(
        key_ptr: i32,
        key_len: i32,
        val_ptr: i32,
        val_len: i32,
    ) -> i32;

    /// Delete a key from state (tombstone in write buffer).
    pub fn state_delete(key_ptr: i32, key_len: i32) -> i32;

    // ── Events & Logs (§8.2) ──

    /// Emit a serialized event.
    pub fn emit_event(evt_ptr: i32, evt_len: i32) -> i32;

    /// Write a debug log line. Not consensus-critical.
    pub fn log(level: i32, msg_ptr: i32, msg_len: i32) -> i32;

    // ── Cryptography (§8.3) ──

    /// Compute BLAKE3 hash. `out_ptr` must point to a 32-byte buffer.
    pub fn hash_blake3(
        in_ptr: i32,
        in_len: i32,
        out_ptr: i32,
        out_len: i32,
    ) -> i32;

    /// Verify an Ed25519 signature.
    /// Returns OK (0) if valid, ERR_SIG_INVALID (8) if invalid.
    pub fn verify_ed25519(
        msg_ptr: i32,
        msg_len: i32,
        sig_ptr: i32,
        sig_len: i32,
        pk_ptr: i32,
        pk_len: i32,
    ) -> i32;

    // ── Gas introspection (§8.4) ──

    /// Write remaining gas (u64 LE) to `out_ptr`.
    pub fn gas_remaining(out_ptr: i32) -> i32;

    // ── Host memory management (§8.5) ──

    /// Free a buffer allocated by the host (e.g., from `state_get`).
    pub fn host_free(ptr: i32, len: i32) -> i32;

    // ── Context (§8.6) ──

    /// Get serialized execution context.
    /// Host allocates the buffer; guest reads from `*out_ptr_ptr` / `*out_len_ptr`.
    pub fn get_context(out_ptr_ptr: i32, out_len_ptr: i32) -> i32;
}
