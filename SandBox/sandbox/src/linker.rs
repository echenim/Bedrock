//! Host function registration via Wasmtime linker.
//!
//! Registers all 10 `bedrock_host` functions with the Wasmtime `Linker`.
//! Each function:
//! 1. Extracts memory and HostState from the Caller
//! 2. Validates pointer/length arguments against linear memory
//! 3. Charges gas via HostGasMeter
//! 4. Performs the operation
//! 5. Returns i32 error code (0 = OK)
//!
//! See EXECUTION_SPEC.md §8 for function specifications.

use wasmtime::{Caller, Linker, Memory};

use bedrock_primitives::{
    ErrorCode,
    codec::decode_single_event,
    gas::*,
};

use crate::error::SandboxError;
use crate::host_impl::HostState;
use crate::memory;

/// Get the guest's exported memory from a Caller.
fn get_memory(caller: &mut Caller<'_, HostState>) -> Option<Memory> {
    caller.get_export("memory").and_then(|e| e.into_memory())
}

/// Register all `bedrock_host` functions with the linker.
pub fn register_host_functions(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    register_state_get(linker)?;
    register_state_set(linker)?;
    register_state_delete(linker)?;
    register_emit_event(linker)?;
    register_log(linker)?;
    register_hash_blake3(linker)?;
    register_verify_ed25519(linker)?;
    register_gas_remaining(linker)?;
    register_host_free(linker)?;
    register_get_context(linker)?;
    Ok(())
}

// ── State Access (§8.1) ──

fn register_state_get(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "state_get",
        |mut caller: Caller<'_, HostState>,
         key_ptr: i32,
         key_len: i32,
         out_ptr_ptr: i32,
         out_len_ptr: i32|
         -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            // Read key from guest memory
            let key = {
                let data = mem.data(&caller);
                match memory::read_bytes(data, key_ptr, key_len) {
                    Ok(k) => k,
                    Err(_) => return ErrorCode::BadPointer as i32,
                }
            };

            // Validate output pointer locations
            {
                let size = mem.data(&caller).len();
                if memory::validate_range(size, out_ptr_ptr, 4).is_err()
                    || memory::validate_range(size, out_len_ptr, 4).is_err()
                {
                    return ErrorCode::BadPointer as i32;
                }
            }

            // Charge gas
            let cost = gas_cost_state_get(key.len());
            if let Err(e) = caller.data_mut().gas_meter.charge(cost) {
                return e.to_error_code();
            }

            // Look up value: overlay first, then committed state
            let value = match caller.data().state_get(&key) {
                Ok(v) => v,
                Err(e) => return e.to_error_code(),
            };

            match value {
                Some(val) => {
                    // Allocate in guest memory and write value
                    let (ptr, new_bump, new_cap, grow_pages) =
                        caller.data().host_alloc.compute_alloc(val.len());

                    if grow_pages > 0
                        && mem.grow(&mut caller, grow_pages).is_err()
                    {
                        return ErrorCode::Internal as i32;
                    }

                    {
                        let data = mem.data_mut(&mut caller);
                        data[ptr..ptr + val.len()].copy_from_slice(&val);
                        // Write pointer and length to output locations
                        if memory::write_i32(data, out_ptr_ptr, ptr as i32).is_err()
                            || memory::write_i32(data, out_len_ptr, val.len() as i32).is_err()
                        {
                            return ErrorCode::BadPointer as i32;
                        }
                    }

                    caller.data_mut().host_alloc.commit(new_bump, new_cap);
                }
                None => {
                    // Key not found: write 0, 0
                    let data = mem.data_mut(&mut caller);
                    if memory::write_i32(data, out_ptr_ptr, 0).is_err()
                        || memory::write_i32(data, out_len_ptr, 0).is_err()
                    {
                        return ErrorCode::BadPointer as i32;
                    }
                }
            }

            0 // OK
        },
    )?;
    Ok(())
}

fn register_state_set(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "state_set",
        |mut caller: Caller<'_, HostState>,
         key_ptr: i32,
         key_len: i32,
         val_ptr: i32,
         val_len: i32|
         -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            // Read key and value from guest memory
            let (key, value) = {
                let data = mem.data(&caller);
                let key = match memory::read_bytes(data, key_ptr, key_len) {
                    Ok(k) => k,
                    Err(_) => return ErrorCode::BadPointer as i32,
                };
                let value = match memory::read_bytes(data, val_ptr, val_len) {
                    Ok(v) => v,
                    Err(_) => return ErrorCode::BadPointer as i32,
                };
                (key, value)
            };

            // Charge gas
            let cost = gas_cost_state_set(key.len(), value.len());
            if let Err(e) = caller.data_mut().gas_meter.charge(cost) {
                return e.to_error_code();
            }

            // Write to overlay
            if let Err(e) = caller.data_mut().state_set(&key, &value) {
                return e.to_error_code();
            }

            0
        },
    )?;
    Ok(())
}

fn register_state_delete(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "state_delete",
        |mut caller: Caller<'_, HostState>, key_ptr: i32, key_len: i32| -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            let key = {
                let data = mem.data(&caller);
                match memory::read_bytes(data, key_ptr, key_len) {
                    Ok(k) => k,
                    Err(_) => return ErrorCode::BadPointer as i32,
                }
            };

            let cost = gas_cost_state_delete(key.len());
            if let Err(e) = caller.data_mut().gas_meter.charge(cost) {
                return e.to_error_code();
            }

            if let Err(e) = caller.data_mut().state_delete(&key) {
                return e.to_error_code();
            }

            0
        },
    )?;
    Ok(())
}

// ── Events & Logs (§8.2) ──

fn register_emit_event(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "emit_event",
        |mut caller: Caller<'_, HostState>, evt_ptr: i32, evt_len: i32| -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            let evt_bytes = {
                let data = mem.data(&caller);
                match memory::read_bytes(data, evt_ptr, evt_len) {
                    Ok(b) => b,
                    Err(_) => return ErrorCode::BadPointer as i32,
                }
            };

            // Charge gas proportional to event size
            let cost = gas_cost_emit_event(evt_bytes.len());
            if let Err(e) = caller.data_mut().gas_meter.charge(cost) {
                return e.to_error_code();
            }

            // Deserialize the event
            let event = match decode_single_event(&evt_bytes) {
                Ok(e) => e,
                Err(_) => return ErrorCode::InvalidEncoding as i32,
            };

            // Add event (enforces count limit)
            if let Err(e) = caller.data_mut().add_event(event) {
                return e.to_error_code();
            }

            0
        },
    )?;
    Ok(())
}

fn register_log(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "log",
        |mut caller: Caller<'_, HostState>,
         level: i32,
         msg_ptr: i32,
         msg_len: i32|
         -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            let msg_bytes = {
                let data = mem.data(&caller);
                match memory::read_bytes(data, msg_ptr, msg_len) {
                    Ok(b) => b,
                    Err(_) => return ErrorCode::BadPointer as i32,
                }
            };

            // Charge gas
            let cost = gas_cost_log(msg_bytes.len());
            if let Err(e) = caller.data_mut().gas_meter.charge(cost) {
                return e.to_error_code();
            }

            // Validate UTF-8
            let message = match std::str::from_utf8(&msg_bytes) {
                Ok(s) => s.to_string(),
                Err(_) => return ErrorCode::InvalidEncoding as i32,
            };

            // Log (best-effort, not consensus-critical)
            let _ = caller.data_mut().add_log(level as u32, message);

            0
        },
    )?;
    Ok(())
}

// ── Crypto (§8.3) ──

fn register_hash_blake3(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "hash_blake3",
        |mut caller: Caller<'_, HostState>,
         in_ptr: i32,
         in_len: i32,
         out_ptr: i32,
         out_len: i32|
         -> i32 {
            if out_len != 32 {
                return ErrorCode::BadPointer as i32;
            }

            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            let input = {
                let data = mem.data(&caller);
                if memory::validate_range(data.len(), out_ptr, 32).is_err() {
                    return ErrorCode::BadPointer as i32;
                }
                match memory::read_bytes(data, in_ptr, in_len) {
                    Ok(b) => b,
                    Err(_) => return ErrorCode::BadPointer as i32,
                }
            };

            // Charge gas
            let cost = gas_cost_hash_blake3(input.len());
            if let Err(e) = caller.data_mut().gas_meter.charge(cost) {
                return e.to_error_code();
            }

            // Compute hash
            let hash = blake3::hash(&input);

            // Write result to guest memory
            {
                let data = mem.data_mut(&mut caller);
                if memory::write_bytes(data, out_ptr, hash.as_bytes()).is_err() {
                    return ErrorCode::BadPointer as i32;
                }
            }

            0
        },
    )?;
    Ok(())
}

fn register_verify_ed25519(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "verify_ed25519",
        |mut caller: Caller<'_, HostState>,
         msg_ptr: i32,
         msg_len: i32,
         sig_ptr: i32,
         sig_len: i32,
         pk_ptr: i32,
         pk_len: i32|
         -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            // Read all inputs
            let (msg, sig_bytes, pk_bytes) = {
                let data = mem.data(&caller);
                let msg = match memory::read_bytes(data, msg_ptr, msg_len) {
                    Ok(b) => b,
                    Err(_) => return ErrorCode::BadPointer as i32,
                };
                let sig = match memory::read_bytes(data, sig_ptr, sig_len) {
                    Ok(b) => b,
                    Err(_) => return ErrorCode::BadPointer as i32,
                };
                let pk = match memory::read_bytes(data, pk_ptr, pk_len) {
                    Ok(b) => b,
                    Err(_) => return ErrorCode::BadPointer as i32,
                };
                (msg, sig, pk)
            };

            if sig_bytes.len() != 64 || pk_bytes.len() != 32 {
                return ErrorCode::BadPointer as i32;
            }

            // Charge gas
            if let Err(e) = caller.data_mut().gas_meter.charge(G_VERIFY_ED25519) {
                return e.to_error_code();
            }

            // Verify signature
            use ed25519_dalek::{Signature, Verifier, VerifyingKey};

            let pk_arr: [u8; 32] = pk_bytes[..32].try_into().unwrap();
            let verifying_key = match VerifyingKey::from_bytes(&pk_arr) {
                Ok(k) => k,
                Err(_) => return ErrorCode::CryptoFailed as i32,
            };

            let sig_arr: [u8; 64] = sig_bytes[..64].try_into().unwrap();
            let signature = Signature::from_bytes(&sig_arr);

            match verifying_key.verify(&msg, &signature) {
                Ok(()) => 0,                           // Valid signature
                Err(_) => ErrorCode::SigInvalid as i32, // Invalid signature
            }
        },
    )?;
    Ok(())
}

// ── Gas Introspection (§8.4) ──

fn register_gas_remaining(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "gas_remaining",
        |mut caller: Caller<'_, HostState>, out_ptr: i32| -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            // Validate output pointer
            {
                let size = mem.data(&caller).len();
                if memory::validate_range(size, out_ptr, 8).is_err() {
                    return ErrorCode::BadPointer as i32;
                }
            }

            // Charge gas for this call
            if let Err(e) = caller.data_mut().gas_meter.charge(G_GAS_REMAINING) {
                return e.to_error_code();
            }

            let remaining = caller.data().gas_meter.remaining();

            // Write u64 LE to guest memory
            {
                let data = mem.data_mut(&mut caller);
                if memory::write_bytes(data, out_ptr, &remaining.to_le_bytes()).is_err() {
                    return ErrorCode::BadPointer as i32;
                }
            }

            0
        },
    )?;
    Ok(())
}

// ── Host Memory Management (§8.5) ──

fn register_host_free(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "host_free",
        |mut caller: Caller<'_, HostState>, _ptr: i32, _len: i32| -> i32 {
            // Charge minimal gas
            if let Err(e) = caller.data_mut().gas_meter.charge(G_HOST_FREE) {
                return e.to_error_code();
            }

            // No-op: WASM memory can only grow, not shrink.
            // Host-allocated buffers live in the bump allocator region.
            // The entire WASM instance is discarded after execution, so
            // all memory is freed implicitly.
            0
        },
    )?;
    Ok(())
}

// ── Context (§8.6) ──

fn register_get_context(linker: &mut Linker<HostState>) -> Result<(), SandboxError> {
    linker.func_wrap(
        "bedrock_host",
        "get_context",
        |mut caller: Caller<'_, HostState>, out_ptr_ptr: i32, out_len_ptr: i32| -> i32 {
            let mem = match get_memory(&mut caller) {
                Some(m) => m,
                None => return ErrorCode::Internal as i32,
            };

            // Validate output pointer locations
            {
                let size = mem.data(&caller).len();
                if memory::validate_range(size, out_ptr_ptr, 4).is_err()
                    || memory::validate_range(size, out_len_ptr, 4).is_err()
                {
                    return ErrorCode::BadPointer as i32;
                }
            }

            // Charge gas
            if let Err(e) = caller.data_mut().gas_meter.charge(G_GET_CONTEXT) {
                return e.to_error_code();
            }

            // Get pre-encoded context
            let ctx_bytes = caller.data().encoded_context.clone();

            // Allocate in guest memory and write context
            let (ptr, new_bump, new_cap, grow_pages) =
                caller.data().host_alloc.compute_alloc(ctx_bytes.len());

            if grow_pages > 0
                && mem.grow(&mut caller, grow_pages).is_err()
            {
                return ErrorCode::Internal as i32;
            }

            {
                let data = mem.data_mut(&mut caller);
                data[ptr..ptr + ctx_bytes.len()].copy_from_slice(&ctx_bytes);
                if memory::write_i32(data, out_ptr_ptr, ptr as i32).is_err()
                    || memory::write_i32(data, out_len_ptr, ctx_bytes.len() as i32).is_err()
                {
                    return ErrorCode::BadPointer as i32;
                }
            }

            caller.data_mut().host_alloc.commit(new_bump, new_cap);

            0
        },
    )?;
    Ok(())
}
