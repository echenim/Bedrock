//! Sandbox error types.

use bedrock_hostapi::HostError;

/// Top-level error type for the sandbox crate.
#[derive(Debug, thiserror::Error)]
pub enum SandboxError {
    /// Wasmtime engine, compilation, or instantiation error.
    #[error("wasmtime error: {0}")]
    Wasmtime(#[from] anyhow::Error),

    /// Module validation failed (missing exports, bad imports, etc.).
    #[error("validation error: {0}")]
    ValidationError(String),

    /// Host API error during execution.
    #[error("host error: {0}")]
    HostError(#[from] HostError),

    /// Guest returned non-zero from `bedrock_init`.
    #[error("bedrock_init failed with code {0}")]
    InitFailed(i32),

    /// Guest returned non-zero from `bedrock_execute_block`.
    #[error("bedrock_execute_block failed with code {0}")]
    ExecutionFailed(i32),

    /// Response deserialization failed.
    #[error("response error: {0}")]
    ResponseError(String),

    /// Memory operation failed (out-of-bounds, grow failure).
    #[error("memory error: {0}")]
    MemoryError(String),

    /// Fuel exhausted during execution.
    #[error("fuel exhausted (instruction limit)")]
    FuelExhausted,

    /// WASM guest trapped.
    #[error("guest trapped: {0}")]
    GuestTrapped(String),
}
