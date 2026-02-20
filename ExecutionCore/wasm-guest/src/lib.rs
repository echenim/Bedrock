//! `bedrock-wasm-guest` — WASM cdylib for the BedRock execution engine.
//!
//! This crate compiles to a `.wasm` artifact that exports the three
//! functions required by EXECUTION_SPEC.md §2.1:
//!
//! - `bedrock_init` — version handshake
//! - `bedrock_execute_block` — execute a block
//! - `bedrock_free` — release guest-allocated buffers
//!
//! Host functions are imported under the `bedrock_host` WASM module.
//!
//! **Determinism:** The guest does not use OS randomness, filesystem,
//! networking, or system time. All non-deterministic features are excluded
//! by depending on `bedrock-primitives` and `bedrock-engine` without
//! their `std` features (which gate rand/getrandom).
//!
//! The `wasm32-unknown-unknown` standard library provides the allocator
//! and panic handler. It does not expose OS-level functionality.

#![no_std]

extern crate alloc;

// ── Modules ──

mod imports;
mod host_bridge;
mod exports;

// Re-export the exported functions so the linker sees them.
// They are already #[no_mangle] pub extern "C" in exports.rs.
pub use exports::{bedrock_init, bedrock_execute_block, bedrock_free};
