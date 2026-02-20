//! Execution boundary types — request, response, receipts, and events.
//!
//! These types define the wire format for the execution boundary between
//! the Go control plane and the Rust execution engine.
//! See EXECUTION_SPEC.md §3 (Wire Format).

use alloc::string::String;
use alloc::vec::Vec;
use crate::types::{BlockHeight, Hash};

/// Execution limits per block (EXECUTION_SPEC.md §3.2).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExecutionLimits {
    /// Maximum gas that can be consumed in this block.
    pub gas_limit: u64,
    /// Maximum number of events that can be emitted.
    pub max_events: u32,
    /// Maximum total bytes that can be written to state.
    pub max_write_bytes: u32,
}

impl Default for ExecutionLimits {
    fn default() -> Self {
        Self {
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024, // 4 MiB
        }
    }
}

/// Input to the execution engine (EXECUTION_SPEC.md §3.2).
///
/// The host constructs this from a proposed block and passes it to the
/// guest via `bedrock_execute_block`. All fields must be identical across
/// validators for the same block.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExecutionRequest {
    /// API version for compatibility checking (§10.1).
    pub api_version: u32,
    /// Chain identifier.
    pub chain_id: Vec<u8>,
    /// Height of the block being executed.
    pub block_height: BlockHeight,
    /// Logical block time from consensus header.
    pub block_time: u64,
    /// Hash of the block being executed.
    pub block_hash: Hash,
    /// State root from the previous committed block.
    pub prev_state_root: Hash,
    /// Ordered transaction payloads (opaque bytes).
    pub transactions: Vec<Vec<u8>>,
    /// Resource limits for this execution.
    pub limits: ExecutionLimits,
    /// Optional deterministic seed derived from block header.
    pub execution_seed: Option<Hash>,
}

/// Execution status returned by the engine (EXECUTION_SPEC.md §3.3).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
#[repr(u8)]
pub enum ExecutionStatus {
    /// Execution completed successfully.
    Ok = 0,
    /// Block was structurally invalid (bad fields, unsupported version).
    InvalidBlock = 1,
    /// Execution encountered a runtime error.
    ExecutionError = 2,
    /// Gas limit was exceeded during execution.
    OutOfGas = 3,
}

impl ExecutionStatus {
    /// Returns true if execution was successful.
    pub fn is_ok(self) -> bool {
        matches!(self, Self::Ok)
    }

    /// Convert from a u8 value.
    pub fn from_u8(v: u8) -> Option<Self> {
        match v {
            0 => Some(Self::Ok),
            1 => Some(Self::InvalidBlock),
            2 => Some(Self::ExecutionError),
            3 => Some(Self::OutOfGas),
            _ => None,
        }
    }
}

impl core::fmt::Display for ExecutionStatus {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            Self::Ok => write!(f, "OK"),
            Self::InvalidBlock => write!(f, "INVALID_BLOCK"),
            Self::ExecutionError => write!(f, "EXECUTION_ERROR"),
            Self::OutOfGas => write!(f, "OUT_OF_GAS"),
        }
    }
}

/// Output from the execution engine (EXECUTION_SPEC.md §3.3).
///
/// Contains the new state root, gas usage, per-transaction receipts,
/// emitted events, and optional debug logs.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExecutionResponse {
    /// API version echoed back for verification.
    pub api_version: u32,
    /// Execution status.
    pub status: ExecutionStatus,
    /// New state root after applying all state transitions.
    pub new_state_root: Hash,
    /// Total gas consumed across all transactions.
    pub gas_used: u64,
    /// Per-transaction receipts.
    pub receipts: Vec<Receipt>,
    /// Events emitted during execution.
    pub events: Vec<Event>,
    /// Optional debug logs (not consensus-critical, may be dropped).
    pub logs: Vec<LogLine>,
}

impl ExecutionResponse {
    /// Create a failure response with no state changes.
    pub fn failure(api_version: u32, status: ExecutionStatus, prev_state_root: Hash) -> Self {
        Self {
            api_version,
            status,
            new_state_root: prev_state_root,
            gas_used: 0,
            receipts: Vec::new(),
            events: Vec::new(),
            logs: Vec::new(),
        }
    }
}

/// Per-transaction receipt (EXECUTION_SPEC.md §3.4).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Receipt {
    /// Index of this transaction in the block.
    pub tx_index: u32,
    /// Whether the transaction executed successfully.
    pub success: bool,
    /// Gas consumed by this transaction.
    pub gas_used: u64,
    /// Application-defined result code.
    pub result_code: u32,
    /// Optional return data (bounded).
    pub return_data: Vec<u8>,
}

/// Emitted event (EXECUTION_SPEC.md §3.5).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Event {
    /// Index of the transaction that emitted this event.
    pub tx_index: u32,
    /// Event type identifier.
    pub event_type: String,
    /// Key-value attributes.
    pub attributes: Vec<EventAttribute>,
}

/// A single key-value attribute within an event.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EventAttribute {
    /// Attribute key (UTF-8).
    pub key: String,
    /// Attribute value (arbitrary bytes).
    pub value: Vec<u8>,
}

/// Debug log line emitted by the guest (EXECUTION_SPEC.md §8.2).
///
/// Logs are NOT consensus-critical. Guest must never branch on log
/// success/failure. The host may drop logs under pressure.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LogLine {
    /// Log level (0=trace, 1=debug, 2=info, 3=warn, 4=error).
    pub level: u32,
    /// Log message.
    pub message: String,
}

/// Context passed to the guest via `get_context` (EXECUTION_SPEC.md §8.6).
///
/// Must be identical across validators for the same block.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExecutionContext {
    /// Chain identifier.
    pub chain_id: Vec<u8>,
    /// Block height being executed.
    pub block_height: BlockHeight,
    /// Logical block time.
    pub block_time: u64,
    /// Block hash.
    pub block_hash: Hash,
    /// Gas limit for this block.
    pub gas_limit: u64,
    /// Maximum events allowed.
    pub max_events: u32,
    /// Maximum state write bytes allowed.
    pub max_write_bytes: u32,
    /// API version.
    pub api_version: u32,
    /// Optional deterministic execution seed.
    pub execution_seed: Option<Hash>,
}

impl ExecutionContext {
    /// Build an `ExecutionContext` from an `ExecutionRequest`.
    pub fn from_request(req: &ExecutionRequest) -> Self {
        Self {
            chain_id: req.chain_id.clone(),
            block_height: req.block_height,
            block_time: req.block_time,
            block_hash: req.block_hash,
            gas_limit: req.limits.gas_limit,
            max_events: req.limits.max_events,
            max_write_bytes: req.limits.max_write_bytes,
            api_version: req.api_version,
            execution_seed: req.execution_seed,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{ZERO_HASH, API_VERSION};

    fn sample_request() -> ExecutionRequest {
        ExecutionRequest {
            api_version: API_VERSION,
            chain_id: b"bedrock-test".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: ZERO_HASH,
            prev_state_root: ZERO_HASH,
            transactions: vec![b"tx1".to_vec(), b"tx2".to_vec()],
            limits: ExecutionLimits::default(),
            execution_seed: None,
        }
    }

    #[test]
    fn test_execution_status_is_ok() {
        assert!(ExecutionStatus::Ok.is_ok());
        assert!(!ExecutionStatus::OutOfGas.is_ok());
        assert!(!ExecutionStatus::InvalidBlock.is_ok());
        assert!(!ExecutionStatus::ExecutionError.is_ok());
    }

    #[test]
    fn test_execution_status_from_u8() {
        assert_eq!(ExecutionStatus::from_u8(0), Some(ExecutionStatus::Ok));
        assert_eq!(ExecutionStatus::from_u8(3), Some(ExecutionStatus::OutOfGas));
        assert_eq!(ExecutionStatus::from_u8(255), None);
    }

    #[test]
    fn test_execution_response_failure() {
        let resp = ExecutionResponse::failure(API_VERSION, ExecutionStatus::OutOfGas, ZERO_HASH);
        assert_eq!(resp.status, ExecutionStatus::OutOfGas);
        assert_eq!(resp.new_state_root, ZERO_HASH);
        assert_eq!(resp.gas_used, 0);
        assert!(resp.receipts.is_empty());
        assert!(resp.events.is_empty());
    }

    #[test]
    fn test_execution_context_from_request() {
        let req = sample_request();
        let ctx = ExecutionContext::from_request(&req);
        assert_eq!(ctx.chain_id, req.chain_id);
        assert_eq!(ctx.block_height, req.block_height);
        assert_eq!(ctx.block_time, req.block_time);
        assert_eq!(ctx.block_hash, req.block_hash);
        assert_eq!(ctx.gas_limit, req.limits.gas_limit);
        assert_eq!(ctx.max_events, req.limits.max_events);
        assert_eq!(ctx.api_version, req.api_version);
        assert_eq!(ctx.execution_seed, None);
    }

    #[test]
    fn test_default_limits() {
        let limits = ExecutionLimits::default();
        assert_eq!(limits.gas_limit, 10_000_000);
        assert_eq!(limits.max_events, 1024);
        assert_eq!(limits.max_write_bytes, 4 * 1024 * 1024);
    }

    #[test]
    fn test_receipt_fields() {
        let receipt = Receipt {
            tx_index: 0,
            success: true,
            gas_used: 500,
            result_code: 0,
            return_data: vec![1, 2, 3],
        };
        assert!(receipt.success);
        assert_eq!(receipt.gas_used, 500);
    }

    #[test]
    fn test_event_with_attributes() {
        let event = Event {
            tx_index: 0,
            event_type: "transfer".into(),
            attributes: vec![
                EventAttribute {
                    key: "from".into(),
                    value: b"alice".to_vec(),
                },
                EventAttribute {
                    key: "to".into(),
                    value: b"bob".to_vec(),
                },
            ],
        };
        assert_eq!(event.attributes.len(), 2);
        assert_eq!(event.event_type, "transfer");
    }
}
