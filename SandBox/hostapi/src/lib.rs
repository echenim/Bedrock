//! `bedrock-hostapi` — host API trait definitions and types for the BedRock WASM sandbox.
//!
//! This crate defines the host-side interface that the sandbox implements
//! when running the WASM guest. It provides:
//!
//! - `HostApi` trait — the mirror of the guest's `HostInterface`
//! - `HostGasMeter` — authoritative host-side gas enforcement
//! - `StateStore` trait — backend state storage abstraction
//! - `MemStore` — in-memory `StateStore` for testing
//! - `ExecutionConfig` — resource limits for block execution
//! - `HostError` — host-side error type with `ErrorCode` conversion
//!
//! This crate depends on `bedrock-primitives` from ExecutionCore for
//! shared types (ErrorCode, gas constants, ExecutionContext, etc.).

pub mod error;
pub mod types;
pub mod gas_meter;
pub mod state_store;
pub mod mem_store;
pub mod traits;

// Re-export commonly used types at the crate root.
pub use error::HostError;
pub use types::ExecutionConfig;
pub use gas_meter::HostGasMeter;
pub use state_store::StateStore;
pub use mem_store::MemStore;
pub use traits::HostApi;
