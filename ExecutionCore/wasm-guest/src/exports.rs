//! Guest exported functions (EXECUTION_SPEC.md §2.1).
//!
//! These are the three functions the host calls to drive execution:
//! - `bedrock_init` — initialize the engine (called once after instantiation)
//! - `bedrock_execute_block` — execute a block (the main entry point)
//! - `bedrock_free` — free guest-allocated memory (called by host after reading response)
//!
//! All exported functions return `i32` error codes (0 = OK).
//! They must never panic — panics in WASM cause traps.

use alloc::vec::Vec;
use bedrock_engine::BlockExecutor;
use bedrock_primitives::{
    ErrorCode,
    codec::{
        decode_execution_request, encode_execution_response,
        decode_execution_context,
    },
    types::API_VERSION,
};
use crate::host_bridge::WasmHostBridge;
use crate::imports;

/// Initialize the execution engine.
///
/// Called once after WASM module instantiation. Validates that the host
/// and guest agree on the API version.
///
/// # Arguments
/// - `version_ptr`: pointer to a u32 LE-encoded API version
/// - `version_len`: length (must be 4)
///
/// # Returns
/// 0 on success, non-zero error code on failure.
#[no_mangle]
pub extern "C" fn bedrock_init(version_ptr: i32, version_len: i32) -> i32 {
    // Validate pointer and length
    if version_len != 4 || version_ptr == 0 {
        return ErrorCode::BadPointer as i32;
    }

    let version_bytes = unsafe {
        core::slice::from_raw_parts(version_ptr as *const u8, 4)
    };
    let host_version = u32::from_le_bytes([
        version_bytes[0],
        version_bytes[1],
        version_bytes[2],
        version_bytes[3],
    ]);

    if host_version != API_VERSION {
        return ErrorCode::InvalidEncoding as i32;
    }

    0 // OK
}

/// Execute a block.
///
/// Deserializes the `ExecutionRequest` from the host-provided buffer,
/// runs `BlockExecutor::execute_block`, serializes the `ExecutionResponse`,
/// and writes the response pointer/length to the output locations.
///
/// # Arguments
/// - `req_ptr`: pointer to serialized ExecutionRequest bytes
/// - `req_len`: length of the request
/// - `resp_ptr_ptr`: pointer to an i32 where the response pointer will be written
/// - `resp_len_ptr`: pointer to an i32 where the response length will be written
///
/// # Returns
/// 0 on success, non-zero error code on failure.
/// On success, the host reads the response from `*resp_ptr_ptr` / `*resp_len_ptr`
/// and must call `bedrock_free` to release the buffer.
#[no_mangle]
pub extern "C" fn bedrock_execute_block(
    req_ptr: i32,
    req_len: i32,
    resp_ptr_ptr: i32,
    resp_len_ptr: i32,
) -> i32 {
    // Validate input pointers
    if req_ptr == 0 || req_len <= 0 || resp_ptr_ptr == 0 || resp_len_ptr == 0 {
        return ErrorCode::BadPointer as i32;
    }

    // Read request bytes from linear memory
    let req_bytes = unsafe {
        core::slice::from_raw_parts(req_ptr as *const u8, req_len as usize)
    };

    // Deserialize the execution request
    let request = match decode_execution_request(req_bytes) {
        Ok(req) => req,
        Err(_) => return ErrorCode::InvalidEncoding as i32,
    };

    // Create the host bridge
    // Fetch context from host to initialize the bridge
    let context = match fetch_context_for_bridge() {
        Ok(ctx) => ctx,
        Err(code) => return code,
    };
    let mut host = WasmHostBridge::with_context(context);

    // Execute the block
    let response = BlockExecutor::execute_block(&request, &mut host);

    // Serialize the response
    let resp_bytes = encode_execution_response(&response);

    // Leak the response Vec into raw parts for the host to read.
    // The host will call bedrock_free to reclaim this memory.
    let len = resp_bytes.len();
    let ptr = resp_bytes.as_ptr();
    core::mem::forget(resp_bytes);

    // Write response pointer and length to output locations
    unsafe {
        core::ptr::write(resp_ptr_ptr as *mut i32, ptr as i32);
        core::ptr::write(resp_len_ptr as *mut i32, len as i32);
    }

    0 // OK
}

/// Free guest-allocated memory.
///
/// Called by the host after reading a response buffer from `bedrock_execute_block`.
/// Reconstructs the `Vec<u8>` and drops it.
///
/// # Safety
/// - `ptr` must point to memory previously allocated by the guest
/// - `len` must be the exact length of the allocation
/// - Must not be called twice on the same pointer
#[no_mangle]
pub extern "C" fn bedrock_free(ptr: i32, len: i32) {
    if ptr == 0 || len <= 0 {
        return;
    }

    // Reconstruct the Vec and drop it to free the memory
    unsafe {
        let _ = Vec::from_raw_parts(ptr as *mut u8, len as usize, len as usize);
    }
    // Vec is dropped here, memory is freed
}

/// Helper: fetch execution context from host for bridge initialization.
fn fetch_context_for_bridge() -> Result<bedrock_primitives::ExecutionContext, i32> {
    let mut out_ptr: i32 = 0;
    let mut out_len: i32 = 0;

    let code = unsafe {
        imports::get_context(
            &mut out_ptr as *mut i32 as i32,
            &mut out_len as *mut i32 as i32,
        )
    };

    if code != 0 {
        return Err(code);
    }

    if out_len <= 0 || out_ptr == 0 {
        return Err(ErrorCode::Internal as i32);
    }

    let data = unsafe {
        core::slice::from_raw_parts(out_ptr as *const u8, out_len as usize)
    };
    let context = decode_execution_context(data)
        .map_err(|_| ErrorCode::InvalidEncoding as i32)?;

    // Free host-allocated buffer
    unsafe {
        imports::host_free(out_ptr, out_len);
    }

    Ok(context)
}
