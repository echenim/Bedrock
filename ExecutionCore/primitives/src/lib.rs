//! `bedrock-primitives` — foundational types for the BedRock execution layer.
//!
//! This crate provides the canonical types, error codes, gas accounting,
//! cryptographic operations, Merkle tree, state overlay, and serialization
//! shared by the execution engine, WASM guest, and sandbox host.
//!
//! Supports `#![no_std]` for WASM guest compatibility (use `default-features = false`).

#![cfg_attr(not(feature = "std"), no_std)]

extern crate alloc;

pub mod types;
pub mod error;
pub mod gas;
pub mod block;
pub mod execution;
pub mod crypto;
pub mod merkle;
pub mod state;
pub mod codec;

// Re-export commonly used types at the crate root for convenience.
pub use types::{Hash, Address, BlockHeight, Round, API_VERSION, MAX_KEY_LEN, MAX_VALUE_LEN};
pub use error::{ErrorCode, ExecError, ExecResult};
pub use gas::GasMeter;
pub use execution::{
    ExecutionRequest, ExecutionResponse, ExecutionStatus, ExecutionLimits,
    ExecutionContext, Receipt, Event, EventAttribute, LogLine,
};
pub use block::{Block, BlockHeader, Transaction};
pub use state::{StateOverlay, OverlayResult};
pub use merkle::{SparseMerkleTree, MerkleProof};
