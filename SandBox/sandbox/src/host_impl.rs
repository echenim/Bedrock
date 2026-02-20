//! Per-execution mutable state held in the Wasmtime Store.
//!
//! `HostState` combines the gas meter, state overlay, committed state store,
//! event/log tracking, and host allocator into a single struct that lives
//! inside `Store<HostState>` for the duration of one block execution.

use std::sync::Arc;

use bedrock_hostapi::{ExecutionConfig, HostError, HostGasMeter, StateStore};
use bedrock_primitives::{
    Event, ExecutionContext, LogLine, StateOverlay, OverlayResult,
    codec::encode_execution_context,
};

use crate::memory::HostAllocator;

/// Per-execution mutable state held in the Wasmtime `Store`.
///
/// Created fresh for each `execute_block` call. Dropped when the
/// WASM instance is discarded at the end of execution.
pub struct HostState {
    /// Authoritative host-side gas meter.
    pub gas_meter: HostGasMeter,
    /// Write buffer overlaying committed state.
    pub overlay: StateOverlay,
    /// Committed state backend (read-only during execution).
    pub state_store: Arc<dyn StateStore>,
    /// Execution context (chain_id, block height, etc.).
    pub context: ExecutionContext,
    /// Serialized execution context (cached for get_context calls).
    pub encoded_context: Vec<u8>,
    /// Execution resource limits.
    pub config: ExecutionConfig,
    /// Events emitted during execution.
    pub events: Vec<Event>,
    /// Log lines emitted during execution.
    pub logs: Vec<LogLine>,
    /// Cumulative event count for limit enforcement.
    pub event_count: u32,
    /// Host-side bump allocator for guest memory.
    pub host_alloc: HostAllocator,
}

impl HostState {
    /// Create a new host state for a block execution.
    pub fn new(
        state_store: Arc<dyn StateStore>,
        context: ExecutionContext,
        config: ExecutionConfig,
    ) -> Self {
        let gas_meter = HostGasMeter::new(config.gas_limit);
        let encoded_context = encode_execution_context(&context);
        Self {
            gas_meter,
            overlay: StateOverlay::new(),
            state_store,
            context,
            encoded_context,
            config,
            events: Vec::new(),
            logs: Vec::new(),
            event_count: 0,
            // Initialized with dummy values; runtime sets real values after instantiation
            host_alloc: HostAllocator::new(0, 0),
        }
    }

    /// Read a value: check overlay first, then committed state.
    ///
    /// Implements EXECUTION_SPEC.md §6.2: reads reflect committed state
    /// plus any writes performed earlier in the same execution.
    pub fn state_get(&self, key: &[u8]) -> Result<Option<Vec<u8>>, HostError> {
        match self.overlay.get(key) {
            OverlayResult::Found(v) => Ok(Some(v)),
            OverlayResult::Deleted => Ok(None),
            OverlayResult::NotInOverlay => self.state_store.get(key),
        }
    }

    /// Write to the overlay. Enforces key/value size and total write byte limits.
    pub fn state_set(&mut self, key: &[u8], value: &[u8]) -> Result<(), HostError> {
        if key.len() > self.config.max_key_len {
            return Err(HostError::key_too_large());
        }
        if value.len() > self.config.max_value_len {
            return Err(HostError::value_too_large());
        }
        if key.is_empty() {
            return Err(HostError::key_too_large());
        }
        self.overlay.set(key.to_vec(), value.to_vec());
        if self.overlay.total_write_bytes() > self.config.max_write_bytes as u64 {
            return Err(HostError::write_limit());
        }
        Ok(())
    }

    /// Delete a key from the overlay.
    pub fn state_delete(&mut self, key: &[u8]) -> Result<(), HostError> {
        if key.len() > self.config.max_key_len {
            return Err(HostError::key_too_large());
        }
        if key.is_empty() {
            return Err(HostError::key_too_large());
        }
        self.overlay.delete(key.to_vec());
        Ok(())
    }

    /// Record an emitted event. Enforces event count limit.
    pub fn add_event(&mut self, event: Event) -> Result<(), HostError> {
        self.event_count += 1;
        if self.event_count > self.config.max_events {
            return Err(HostError::event_limit());
        }
        self.events.push(event);
        Ok(())
    }

    /// Record a log line. Enforces log count and line length limits.
    pub fn add_log(&mut self, level: u32, message: String) -> Result<(), HostError> {
        if message.len() > self.config.max_log_line_len {
            return Ok(()); // Silently truncate/drop oversized logs
        }
        if self.logs.len() >= self.config.max_log_lines as usize {
            return Ok(()); // Silently drop if at limit
        }
        self.logs.push(LogLine { level, message });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bedrock_hostapi::MemStore;
    use bedrock_primitives::types::{ZERO_HASH, API_VERSION};

    fn test_context() -> ExecutionContext {
        ExecutionContext {
            chain_id: b"test".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: ZERO_HASH,
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024,
            api_version: API_VERSION,
            execution_seed: None,
        }
    }

    fn test_host_state() -> HostState {
        let store = Arc::new(MemStore::new());
        HostState::new(store, test_context(), ExecutionConfig::default())
    }

    #[test]
    fn test_state_get_from_overlay() {
        let mut state = test_host_state();
        state.state_set(b"key1", b"value1").unwrap();
        assert_eq!(
            state.state_get(b"key1").unwrap(),
            Some(b"value1".to_vec())
        );
    }

    #[test]
    fn test_state_get_from_committed() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"committed".to_vec());
        let state = HostState::new(
            Arc::new(store),
            test_context(),
            ExecutionConfig::default(),
        );
        assert_eq!(
            state.state_get(b"key1").unwrap(),
            Some(b"committed".to_vec())
        );
    }

    #[test]
    fn test_state_overlay_shadows_committed() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"old".to_vec());
        let mut state = HostState::new(
            Arc::new(store),
            test_context(),
            ExecutionConfig::default(),
        );
        state.state_set(b"key1", b"new").unwrap();
        assert_eq!(state.state_get(b"key1").unwrap(), Some(b"new".to_vec()));
    }

    #[test]
    fn test_state_delete_masks_committed() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"value".to_vec());
        let mut state = HostState::new(
            Arc::new(store),
            test_context(),
            ExecutionConfig::default(),
        );
        state.state_delete(b"key1").unwrap();
        assert_eq!(state.state_get(b"key1").unwrap(), None);
    }

    #[test]
    fn test_state_set_key_too_large() {
        let mut state = test_host_state();
        let big_key = vec![0u8; 257];
        let err = state.state_set(&big_key, b"value").unwrap_err();
        assert_eq!(err.to_error_code(), 3); // ERR_KEY_TOO_LARGE
    }

    #[test]
    fn test_state_set_value_too_large() {
        let mut state = test_host_state();
        let big_val = vec![0u8; 65537];
        let err = state.state_set(b"key", &big_val).unwrap_err();
        assert_eq!(err.to_error_code(), 4); // ERR_VALUE_TOO_LARGE
    }

    #[test]
    fn test_event_limit() {
        let ctx = test_context();
        let config = ExecutionConfig {
            max_events: 2,
            ..ExecutionConfig::default()
        };
        let mut state = HostState::new(Arc::new(MemStore::new()), ctx, config);
        let event = Event {
            tx_index: 0,
            event_type: "test".into(),
            attributes: vec![],
        };
        state.add_event(event.clone()).unwrap();
        state.add_event(event.clone()).unwrap();
        let err = state.add_event(event).unwrap_err();
        assert_eq!(err.to_error_code(), 6); // ERR_EVENT_LIMIT
    }

    #[test]
    fn test_log_limit_silently_drops() {
        let ctx = test_context();
        let config = ExecutionConfig {
            max_log_lines: 2,
            ..ExecutionConfig::default()
        };
        let mut state = HostState::new(Arc::new(MemStore::new()), ctx, config);
        state.add_log(2, "msg1".into()).unwrap();
        state.add_log(2, "msg2".into()).unwrap();
        // Third log should be silently dropped
        state.add_log(2, "msg3".into()).unwrap();
        assert_eq!(state.logs.len(), 2);
    }
}
