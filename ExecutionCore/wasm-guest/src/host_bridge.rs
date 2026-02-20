//! WASM host bridge — implements `HostInterface` by calling imported functions.
//!
//! This module bridges the engine's `HostInterface` trait to the actual WASM
//! host imports defined in `imports.rs`. Each method marshals pointers,
//! calls the import, checks error codes, and returns Rust types.
//!
//! The bridge maintains local state for:
//! - `GasMeter` — tracks gas consumed by host API calls
//! - `StateOverlay` — mirrors writes for state root computation
//! - `Vec<Event>` — collected events
//! - `Vec<LogLine>` — collected log lines
//! - `ExecutionContext` — cached from host

use alloc::string::String;
use alloc::vec::Vec;
use bedrock_engine::HostInterface;
use bedrock_primitives::{
    ErrorCode, ExecError, ExecResult, Event, ExecutionContext, Hash,
    GasMeter, StateOverlay, LogLine,
    codec::{encode_single_event, decode_execution_context},
};
use crate::imports;

/// Bridges `HostInterface` to WASM imported host functions.
///
/// All state-modifying calls are forwarded to the host AND recorded
/// locally so the executor can compute the state root from the overlay.
pub struct WasmHostBridge {
    gas_meter: GasMeter,
    overlay: StateOverlay,
    events: Vec<Event>,
    logs: Vec<LogLine>,
    context: ExecutionContext,
}

impl WasmHostBridge {
    /// Create a new bridge. Fetches the execution context from the host.
    ///
    /// # Safety
    /// Must only be called from within a WASM execution context where
    /// the host has set up the imported functions.
    #[allow(dead_code)]
    pub fn new(gas_limit: u64) -> ExecResult<Self> {
        let context = fetch_context()?;
        Ok(Self {
            gas_meter: GasMeter::new(gas_limit),
            overlay: StateOverlay::new(),
            events: Vec::new(),
            logs: Vec::new(),
            context,
        })
    }

    /// Create with a pre-fetched context (avoids double host call).
    pub fn with_context(context: ExecutionContext) -> Self {
        let gas_limit = context.gas_limit;
        Self {
            gas_meter: GasMeter::new(gas_limit),
            overlay: StateOverlay::new(),
            events: Vec::new(),
            logs: Vec::new(),
            context,
        }
    }
}

/// Convert an i32 host return code to a Result.
/// 0 = Ok, non-zero = error mapped via ErrorCode.
fn check_host_result(code: i32) -> ExecResult<()> {
    if code == 0 {
        return Ok(());
    }
    let error_code = ErrorCode::from_i32(code)
        .unwrap_or(ErrorCode::Internal);
    Err(ExecError::HostError(error_code))
}

/// Fetch the execution context from the host.
#[allow(dead_code)]
fn fetch_context() -> ExecResult<ExecutionContext> {
    let mut out_ptr: i32 = 0;
    let mut out_len: i32 = 0;

    let code = unsafe {
        imports::get_context(
            &mut out_ptr as *mut i32 as i32,
            &mut out_len as *mut i32 as i32,
        )
    };
    check_host_result(code)?;

    if out_len <= 0 || out_ptr == 0 {
        return Err(ExecError::SerializationError(
            "get_context returned empty data".into(),
        ));
    }

    let data = unsafe {
        core::slice::from_raw_parts(out_ptr as *const u8, out_len as usize)
    };
    let context = decode_execution_context(data)?;

    // Free host-allocated buffer
    unsafe {
        imports::host_free(out_ptr, out_len);
    }

    Ok(context)
}

impl HostInterface for WasmHostBridge {
    fn state_get(&self, key: &[u8]) -> ExecResult<Option<Vec<u8>>> {
        let mut out_ptr: i32 = 0;
        let mut out_len: i32 = 0;

        let code = unsafe {
            imports::state_get(
                key.as_ptr() as i32,
                key.len() as i32,
                &mut out_ptr as *mut i32 as i32,
                &mut out_len as *mut i32 as i32,
            )
        };
        check_host_result(code)?;

        // Key not found: host returns out_len=0, out_ptr=0
        if out_len == 0 {
            return Ok(None);
        }

        // Copy data from host-allocated buffer
        let data = unsafe {
            core::slice::from_raw_parts(out_ptr as *const u8, out_len as usize)
        };
        let result = data.to_vec();

        // Free host buffer
        unsafe {
            imports::host_free(out_ptr, out_len);
        }

        Ok(Some(result))
    }

    fn state_set(&mut self, key: &[u8], value: &[u8]) -> ExecResult<()> {
        let code = unsafe {
            imports::state_set(
                key.as_ptr() as i32,
                key.len() as i32,
                value.as_ptr() as i32,
                value.len() as i32,
            )
        };
        check_host_result(code)?;

        // Mirror in local overlay for state root computation
        self.overlay.set(key.to_vec(), value.to_vec());
        Ok(())
    }

    fn state_delete(&mut self, key: &[u8]) -> ExecResult<()> {
        let code = unsafe {
            imports::state_delete(
                key.as_ptr() as i32,
                key.len() as i32,
            )
        };
        check_host_result(code)?;

        // Mirror in local overlay
        self.overlay.delete(key.to_vec());
        Ok(())
    }

    fn emit_event(&mut self, event: Event) -> ExecResult<()> {
        // Serialize the event for the host call
        let encoded = encode_single_event(&event);
        let code = unsafe {
            imports::emit_event(
                encoded.as_ptr() as i32,
                encoded.len() as i32,
            )
        };
        check_host_result(code)?;

        // Track locally
        self.events.push(event);
        Ok(())
    }

    fn log(&mut self, level: u32, message: &str) -> ExecResult<()> {
        let code = unsafe {
            imports::log(
                level as i32,
                message.as_ptr() as i32,
                message.len() as i32,
            )
        };
        // Log failures are not consensus-critical — ignore the result
        // per EXECUTION_SPEC §8.2: "guest must never branch on log success/failure"
        let _ = check_host_result(code);

        self.logs.push(LogLine {
            level,
            message: String::from(message),
        });
        Ok(())
    }

    fn hash_blake3(&self, data: &[u8]) -> ExecResult<Hash> {
        let mut out = [0u8; 32];
        let code = unsafe {
            imports::hash_blake3(
                data.as_ptr() as i32,
                data.len() as i32,
                out.as_mut_ptr() as i32,
                32,
            )
        };
        check_host_result(code)?;
        Ok(out)
    }

    fn verify_ed25519(
        &self,
        message: &[u8],
        signature: &[u8; 64],
        public_key: &[u8; 32],
    ) -> ExecResult<bool> {
        let code = unsafe {
            imports::verify_ed25519(
                message.as_ptr() as i32,
                message.len() as i32,
                signature.as_ptr() as i32,
                64,
                public_key.as_ptr() as i32,
                32,
            )
        };

        match code {
            0 => Ok(true),                        // OK = valid signature
            8 => Ok(false),                       // ERR_SIG_INVALID = invalid
            _ => Err(check_host_result(code).unwrap_err()), // other errors propagate
        }
    }

    fn gas_remaining(&self) -> ExecResult<u64> {
        let mut out = [0u8; 8];
        let code = unsafe {
            imports::gas_remaining(out.as_mut_ptr() as i32)
        };
        check_host_result(code)?;
        Ok(u64::from_le_bytes(out))
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
