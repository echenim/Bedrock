//! `bedrock-sandbox` — Wasmtime-based WASM sandbox for deterministic execution.
//!
//! This crate loads, validates, and runs the BedRock WASM execution module
//! inside a secure Wasmtime sandbox. It enforces:
//!
//! - **Determinism:** No SIMD, no threads, NaN canonicalization
//! - **Fuel metering:** Instruction-level metering to prevent infinite loops
//! - **Gas metering:** Host API call costs charged via `HostGasMeter`
//! - **Memory limits:** Bounded linear memory growth
//! - **Import whitelisting:** Only `bedrock_host` imports allowed, no WASI
//! - **ABI validation:** Required exports checked before execution
//!
//! The primary entry point is [`Sandbox::execute_block`].

pub mod error;
pub mod config;
pub mod memory;
pub mod host_impl;
pub mod validation;
pub mod linker;
pub mod runtime;

pub use error::SandboxError;
pub use config::SandboxConfig;
pub use runtime::Sandbox;
