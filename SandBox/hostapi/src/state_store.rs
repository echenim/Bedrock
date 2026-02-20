//! Backend state storage abstraction for the BedRock sandbox.
//!
//! `StateStore` defines the interface for reading committed state
//! (the state at `prev_state_root`). The sandbox combines a `StateStore`
//! (committed state) with a `StateOverlay` (write buffer) to implement
//! the layered read semantics from EXECUTION_SPEC.md §6.2:
//! reads check the overlay first, then fall through to the store.
//!
//! Implementations:
//! - `MemStore` (this crate) — in-memory BTreeMap for testing
//! - RocksDB (production, provided by Go side via FFI in task 006+)

use crate::error::HostError;

/// Abstraction over committed state storage.
///
/// Implementations must be deterministic: the same key always returns
/// the same value for a given state root.
pub trait StateStore: Send + Sync {
    /// Get the value for a key from committed state.
    ///
    /// Returns `Ok(None)` if the key does not exist.
    fn get(&self, key: &[u8]) -> Result<Option<Vec<u8>>, HostError>;

    /// Check if a key exists in committed state.
    ///
    /// Default implementation uses `get()`, but backends may optimize this.
    fn contains(&self, key: &[u8]) -> Result<bool, HostError> {
        Ok(self.get(key)?.is_some())
    }
}
