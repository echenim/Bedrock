//! `bedrock-engine` — deterministic block execution engine.
//!
//! This crate implements the core state transition function:
//! `f(prev_state_root, block) → (new_state_root, receipts, events, gas_used, status)`
//!
//! It processes blocks transaction-by-transaction, validates inputs, manages
//! gas metering through the `HostInterface`, and produces an `ExecutionResponse`.
//!
//! ## Architecture
//!
//! - [`host::HostInterface`] — trait abstracting host API calls (state, crypto, gas)
//! - [`host::MockHost`] — in-memory implementation for testing
//! - [`validation`] — request/block validation before execution
//! - [`transaction`] — transaction decoding, signature verification, processing
//! - [`executor::BlockExecutor`] — top-level block execution entry point
//!
//! See EXECUTION_SPEC.md §2 for the full execution lifecycle.

#![cfg_attr(not(feature = "std"), no_std)]

extern crate alloc;

pub mod host;
pub mod validation;
pub mod transaction;
pub mod executor;

// Re-export key types for convenience
pub use executor::BlockExecutor;
pub use host::{HostInterface, MockHost};
pub use transaction::{DecodedTransaction, TransactionPayload};
