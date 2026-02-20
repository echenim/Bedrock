//! Host interface trait — abstraction over host API calls.
//!
//! The `HostInterface` trait decouples the engine from the execution
//! environment (WASM sandbox vs. native tests). This maps directly to
//! the Host API defined in EXECUTION_SPEC.md §8.
//!
//! - In WASM: implemented by calling imported host functions
//! - In tests: implemented via `MockHost` (in-memory store)
//! - In sandbox: the host-side implementation that backs the imports

use bedrock_primitives::{
    ExecError, ExecResult, Event, ExecutionContext, Hash,
    GasMeter, StateOverlay, OverlayResult, LogLine,
    types::{MAX_KEY_LEN, MAX_VALUE_LEN},
    crypto,
};
use alloc::collections::BTreeMap;
use alloc::string::String;
use alloc::vec::Vec;

/// Abstraction over the host environment (EXECUTION_SPEC.md §8).
///
/// The engine calls these methods during block execution. Each method
/// corresponds to a Host API function. Implementations are responsible
/// for metering gas and enforcing limits.
pub trait HostInterface {
    /// Read a value from state (§8.1).
    ///
    /// Returns `Ok(Some(value))` if the key exists, `Ok(None)` if not.
    /// Reads must reflect committed state + any writes performed earlier
    /// in the same block execution (§6.2).
    fn state_get(&self, key: &[u8]) -> ExecResult<Option<Vec<u8>>>;

    /// Write a key-value pair to the state overlay (§8.1).
    ///
    /// The write is buffered until block execution completes.
    /// Enforces `MAX_KEY_LEN`, `MAX_VALUE_LEN`, and `max_write_bytes`.
    fn state_set(&mut self, key: &[u8], value: &[u8]) -> ExecResult<()>;

    /// Delete a key from state (§8.1).
    ///
    /// Records a tombstone in the overlay. Subsequent reads return `None`.
    fn state_delete(&mut self, key: &[u8]) -> ExecResult<()>;

    /// Emit an event (§8.2).
    ///
    /// Bounded by `max_events`. Events are collected and included in
    /// the `ExecutionResponse`.
    fn emit_event(&mut self, event: Event) -> ExecResult<()>;

    /// Write a debug log line (§8.2).
    ///
    /// Logs are NOT consensus-critical. The engine must never branch
    /// on log success/failure. The host may drop logs under pressure.
    fn log(&mut self, level: u32, message: &str) -> ExecResult<()>;

    /// Compute a BLAKE3 hash (§8.3).
    fn hash_blake3(&self, data: &[u8]) -> ExecResult<Hash>;

    /// Verify an Ed25519 signature (§8.3).
    ///
    /// Returns `Ok(true)` if valid, `Ok(false)` if invalid.
    /// Deterministic — no randomization.
    fn verify_ed25519(
        &self,
        message: &[u8],
        signature: &[u8; 64],
        public_key: &[u8; 32],
    ) -> ExecResult<bool>;

    /// Query remaining gas (§8.4).
    fn gas_remaining(&self) -> ExecResult<u64>;

    /// Get the execution context for this block (§8.6).
    ///
    /// Context is identical across all validators for the same block.
    fn get_context(&self) -> ExecResult<ExecutionContext>;

    /// Access the gas meter (for internal engine use).
    fn gas_meter(&self) -> &GasMeter;

    /// Access the gas meter mutably (for internal engine use).
    fn gas_meter_mut(&mut self) -> &mut GasMeter;

    /// Access the state overlay (for computing state root after execution).
    fn overlay(&self) -> &StateOverlay;

    /// Access collected events.
    fn events(&self) -> &[Event];

    /// Access collected logs.
    fn logs(&self) -> &[LogLine];
}

// ── MockHost: in-memory host for testing ──

/// In-memory host implementation for deterministic testing.
///
/// Uses a `BTreeMap` as the committed state store and a `StateOverlay`
/// for buffered writes. All gas metering is enforced.
pub struct MockHost {
    /// Committed state (simulates the state at `prev_state_root`).
    committed: BTreeMap<Vec<u8>, Vec<u8>>,
    /// Write buffer for current execution.
    overlay: StateOverlay,
    /// Gas meter tracking consumption.
    gas_meter: GasMeter,
    /// Execution context for this block.
    context: ExecutionContext,
    /// Collected events.
    events: Vec<Event>,
    /// Collected log lines.
    logs: Vec<LogLine>,
    /// Maximum events allowed.
    max_events: u32,
    /// Maximum write bytes allowed.
    max_write_bytes: u32,
}

impl MockHost {
    /// Create a new `MockHost` from committed state and an execution context.
    pub fn new(
        committed: BTreeMap<Vec<u8>, Vec<u8>>,
        context: ExecutionContext,
    ) -> Self {
        let gas_limit = context.gas_limit;
        let max_events = context.max_events;
        let max_write_bytes = context.max_write_bytes;
        Self {
            committed,
            overlay: StateOverlay::new(),
            gas_meter: GasMeter::new(gas_limit),
            context,
            events: Vec::new(),
            logs: Vec::new(),
            max_events,
            max_write_bytes,
        }
    }

    /// Create a minimal `MockHost` for simple tests with default limits.
    pub fn with_defaults() -> Self {
        let context = ExecutionContext {
            chain_id: b"test-chain".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: [0u8; 32],
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024,
            api_version: bedrock_primitives::types::API_VERSION,
            execution_seed: None,
        };
        Self::new(BTreeMap::new(), context)
    }

    /// Insert initial state for testing.
    pub fn set_committed(&mut self, key: Vec<u8>, value: Vec<u8>) {
        self.committed.insert(key, value);
    }

    /// Drain the overlay into committed state (simulating a commit).
    pub fn commit(&mut self) {
        let writes = core::mem::replace(&mut self.overlay, StateOverlay::new()).drain();
        for (key, value) in writes {
            match value {
                Some(v) => { self.committed.insert(key, v); }
                None => { self.committed.remove(&key); }
            }
        }
    }

    /// Access committed state for assertions.
    pub fn committed_state(&self) -> &BTreeMap<Vec<u8>, Vec<u8>> {
        &self.committed
    }
}

impl HostInterface for MockHost {
    fn state_get(&self, key: &[u8]) -> ExecResult<Option<Vec<u8>>> {
        // Validate key length
        if key.len() > MAX_KEY_LEN {
            return Err(ExecError::HostError(bedrock_primitives::ErrorCode::KeyTooLarge));
        }

        // Note: gas is charged by the caller (executor) before calling host methods
        // But for MockHost we charge here for standalone use
        // Check overlay first (§6.2: reads reflect committed + buffered writes)
        match self.overlay.get(key) {
            OverlayResult::Found(value) => Ok(Some(value)),
            OverlayResult::Deleted => Ok(None),
            OverlayResult::NotInOverlay => {
                Ok(self.committed.get(key).cloned())
            }
        }
    }

    fn state_set(&mut self, key: &[u8], value: &[u8]) -> ExecResult<()> {
        // Validate key length
        if key.len() > MAX_KEY_LEN {
            return Err(ExecError::HostError(bedrock_primitives::ErrorCode::KeyTooLarge));
        }
        // Validate value length
        if value.len() > MAX_VALUE_LEN {
            return Err(ExecError::HostError(bedrock_primitives::ErrorCode::ValueTooLarge));
        }

        // Check write limit
        let projected_bytes = self.overlay.total_write_bytes()
            + (key.len() + value.len()) as u64;
        if projected_bytes > self.max_write_bytes as u64 {
            return Err(ExecError::HostError(bedrock_primitives::ErrorCode::WriteLimit));
        }

        self.overlay.set(key.to_vec(), value.to_vec());
        Ok(())
    }

    fn state_delete(&mut self, key: &[u8]) -> ExecResult<()> {
        if key.len() > MAX_KEY_LEN {
            return Err(ExecError::HostError(bedrock_primitives::ErrorCode::KeyTooLarge));
        }

        self.overlay.delete(key.to_vec());
        Ok(())
    }

    fn emit_event(&mut self, event: Event) -> ExecResult<()> {
        if self.events.len() >= self.max_events as usize {
            return Err(ExecError::HostError(bedrock_primitives::ErrorCode::EventLimit));
        }
        self.events.push(event);
        Ok(())
    }

    fn log(&mut self, level: u32, message: &str) -> ExecResult<()> {
        self.logs.push(LogLine {
            level,
            message: String::from(message),
        });
        Ok(())
    }

    fn hash_blake3(&self, data: &[u8]) -> ExecResult<Hash> {
        Ok(crypto::hash_blake3(data))
    }

    fn verify_ed25519(
        &self,
        message: &[u8],
        signature: &[u8; 64],
        public_key: &[u8; 32],
    ) -> ExecResult<bool> {
        Ok(crypto::verify_ed25519(message, signature, public_key))
    }

    fn gas_remaining(&self) -> ExecResult<u64> {
        Ok(self.gas_meter.remaining())
    }

    fn get_context(&self) -> ExecResult<ExecutionContext> {
        Ok(self.context.clone())
    }

    fn gas_meter(&self) -> &GasMeter {
        &self.gas_meter
    }

    fn gas_meter_mut(&mut self) -> &mut GasMeter {
        &mut self.gas_meter
    }

    fn overlay(&self) -> &StateOverlay {
        &self.overlay
    }

    fn events(&self) -> &[Event] {
        &self.events
    }

    fn logs(&self) -> &[LogLine] {
        &self.logs
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_mock_host_state_roundtrip() {
        let mut host = MockHost::with_defaults();

        // Initially empty
        assert_eq!(host.state_get(b"key1").unwrap(), None);

        // Set and get
        host.state_set(b"key1", b"value1").unwrap();
        assert_eq!(
            host.state_get(b"key1").unwrap(),
            Some(b"value1".to_vec())
        );

        // Delete
        host.state_delete(b"key1").unwrap();
        assert_eq!(host.state_get(b"key1").unwrap(), None);
    }

    #[test]
    fn test_mock_host_committed_state() {
        let mut committed = BTreeMap::new();
        committed.insert(b"existing".to_vec(), b"old_val".to_vec());

        let mut host = MockHost::new(committed, ExecutionContext {
            chain_id: b"test".to_vec(),
            block_height: 1,
            block_time: 0,
            block_hash: [0u8; 32],
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024,
            api_version: 1,
            execution_seed: None,
        });

        // Can read committed state
        assert_eq!(
            host.state_get(b"existing").unwrap(),
            Some(b"old_val".to_vec())
        );

        // Overlay shadows committed state
        host.state_set(b"existing", b"new_val").unwrap();
        assert_eq!(
            host.state_get(b"existing").unwrap(),
            Some(b"new_val".to_vec())
        );
    }

    #[test]
    fn test_mock_host_key_too_large() {
        let mut host = MockHost::with_defaults();
        let big_key = vec![0u8; MAX_KEY_LEN + 1];

        assert!(host.state_get(&big_key).is_err());
        assert!(host.state_set(&big_key, b"val").is_err());
        assert!(host.state_delete(&big_key).is_err());
    }

    #[test]
    fn test_mock_host_value_too_large() {
        let mut host = MockHost::with_defaults();
        let big_value = vec![0u8; MAX_VALUE_LEN + 1];

        assert!(host.state_set(b"key", &big_value).is_err());
    }

    #[test]
    fn test_mock_host_event_limit() {
        let ctx = ExecutionContext {
            chain_id: b"test".to_vec(),
            block_height: 1,
            block_time: 0,
            block_hash: [0u8; 32],
            gas_limit: 10_000_000,
            max_events: 2,
            max_write_bytes: 4 * 1024 * 1024,
            api_version: 1,
            execution_seed: None,
        };
        let mut host = MockHost::new(BTreeMap::new(), ctx);

        let event = Event {
            tx_index: 0,
            event_type: String::from("test"),
            attributes: Vec::new(),
        };

        host.emit_event(event.clone()).unwrap();
        host.emit_event(event.clone()).unwrap();
        assert!(host.emit_event(event).is_err());
    }

    #[test]
    fn test_mock_host_gas_remaining() {
        let host = MockHost::with_defaults();
        assert_eq!(host.gas_remaining().unwrap(), 10_000_000);
    }

    #[test]
    fn test_mock_host_crypto() {
        let host = MockHost::with_defaults();

        // BLAKE3 should produce a non-zero hash
        let h = host.hash_blake3(b"hello").unwrap();
        assert_ne!(h, [0u8; 32]);

        // Invalid public key should return false
        // Use 0xFF bytes which are not a valid compressed Edwards point
        let result = host.verify_ed25519(
            b"msg",
            &[0u8; 64],
            &[0xFF; 32],
        ).unwrap();
        assert!(!result);
    }

    #[test]
    fn test_mock_host_log() {
        let mut host = MockHost::with_defaults();
        host.log(2, "info message").unwrap();
        assert_eq!(host.logs().len(), 1);
        assert_eq!(host.logs()[0].message, "info message");
    }
}
