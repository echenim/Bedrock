//! Sandbox runtime — Wasmtime engine, module loading, and block execution.
//!
//! The `Sandbox` struct is the main entry point. It loads a WASM module,
//! validates its ABI, and provides `execute_block` for running blocks.
//!
//! See EXECUTION_SPEC.md §2 for the execution lifecycle.

use std::path::Path;
use std::sync::Arc;

use wasmtime::{Config, Engine, Linker, Module, Store};

use bedrock_hostapi::{ExecutionConfig, StateStore};
use bedrock_primitives::{
    ExecutionContext, ExecutionRequest, ExecutionResponse,
    codec::{
        decode_execution_response, encode_execution_request,
    },
    types::API_VERSION,
};

use crate::config::SandboxConfig;
use crate::error::SandboxError;
use crate::host_impl::HostState;
use crate::linker::register_host_functions;
use crate::memory::{self, HOST_ALLOC_PAGES};
use crate::validation::validate_module;

/// The deterministic WASM execution sandbox.
///
/// Loads and validates a WASM module, then executes blocks by creating
/// a fresh Wasmtime instance for each execution (ensuring isolation).
pub struct Sandbox {
    engine: Engine,
    module: Module,
    config: SandboxConfig,
}

impl Sandbox {
    /// Create a new sandbox from WASM bytecode.
    ///
    /// Validates the module's exports and imports before accepting.
    pub fn new(wasm_bytes: &[u8], config: SandboxConfig) -> Result<Self, SandboxError> {
        let engine = create_engine(&config)?;
        let module = Module::new(&engine, wasm_bytes)?;
        validate_module(&module)?;
        Ok(Self {
            engine,
            module,
            config,
        })
    }

    /// Load from a `.wasm` file path.
    pub fn from_file(path: &Path, config: SandboxConfig) -> Result<Self, SandboxError> {
        let engine = create_engine(&config)?;
        let module = Module::from_file(&engine, path)?;
        validate_module(&module)?;
        Ok(Self {
            engine,
            module,
            config,
        })
    }

    /// Execute a block — the primary entry point.
    ///
    /// Creates a fresh WASM instance, runs `bedrock_init` and
    /// `bedrock_execute_block`, reads the response, and returns it.
    /// The WASM instance is discarded after this call.
    pub fn execute_block(
        &self,
        request: &ExecutionRequest,
        state_store: Arc<dyn StateStore>,
    ) -> Result<ExecutionResponse, SandboxError> {
        // 1. Serialize the request
        let req_bytes = encode_execution_request(request);

        // 2. Build execution context and config
        let context = ExecutionContext::from_request(request);
        let exec_config = ExecutionConfig::from_limits(&request.limits);

        // 3. Create host state
        let host_state = HostState::new(state_store, context, exec_config);

        // 4. Create store with fuel
        let mut store = Store::new(&self.engine, host_state);
        store.set_fuel(self.config.fuel_limit)?;

        // 5. Create linker and register host functions
        let mut linker = Linker::new(&self.engine);
        register_host_functions(&mut linker)?;

        // 6. Instantiate module
        let instance = linker.instantiate(&mut store, &self.module)?;

        // 7. Get memory and initialize host allocator
        let wasm_memory = instance
            .get_memory(&mut store, "memory")
            .ok_or_else(|| SandboxError::MemoryError("no memory export".into()))?;

        let current_pages = wasm_memory.size(&store);
        wasm_memory
            .grow(&mut store, HOST_ALLOC_PAGES)
            .map_err(|e| SandboxError::MemoryError(format!("initial grow: {}", e)))?;

        let alloc_base = (current_pages as usize) * 65536;
        let alloc_capacity = (HOST_ALLOC_PAGES as usize) * 65536;
        store.data_mut().host_alloc =
            memory::HostAllocator::new(alloc_base, alloc_capacity);

        // 8. Call bedrock_init with API version
        let version_bytes = API_VERSION.to_le_bytes();
        let version_ptr =
            alloc_and_write(&wasm_memory, &mut store, &version_bytes)?;

        let init_fn = instance
            .get_typed_func::<(i32, i32), i32>(&mut store, "bedrock_init")?;
        let init_result = handle_trap(init_fn.call(&mut store, (version_ptr, 4)))?;
        if init_result != 0 {
            return Err(SandboxError::InitFailed(init_result));
        }

        // 9. Write request to guest memory
        let req_ptr = alloc_and_write(&wasm_memory, &mut store, &req_bytes)?;

        // Allocate space for response pointer and length (two i32s)
        let resp_ptrs_ptr =
            alloc_and_write(&wasm_memory, &mut store, &[0u8; 8])?;
        let resp_ptr_ptr = resp_ptrs_ptr;
        let resp_len_ptr = resp_ptrs_ptr + 4;

        // 10. Call bedrock_execute_block
        let exec_fn = instance.get_typed_func::<(i32, i32, i32, i32), i32>(
            &mut store,
            "bedrock_execute_block",
        )?;

        let exec_result = handle_trap(exec_fn.call(
            &mut store,
            (req_ptr, req_bytes.len() as i32, resp_ptr_ptr, resp_len_ptr),
        ))?;

        if exec_result != 0 {
            return Err(SandboxError::ExecutionFailed(exec_result));
        }

        // 11. Read response pointer and length from guest memory
        let (resp_ptr, resp_len) = {
            let data = wasm_memory.data(&store);
            let rp = memory::read_i32(data, resp_ptr_ptr)
                .map_err(|_| SandboxError::MemoryError("read resp_ptr".into()))?;
            let rl = memory::read_i32(data, resp_len_ptr)
                .map_err(|_| SandboxError::MemoryError("read resp_len".into()))?;
            (rp, rl)
        };

        // 12. Read response bytes from guest memory
        let resp_bytes = {
            let data = wasm_memory.data(&store);
            memory::read_bytes(data, resp_ptr, resp_len)
                .map_err(|_| SandboxError::MemoryError("read response bytes".into()))?
        };

        // 13. Call bedrock_free to release guest buffer
        let free_fn = instance
            .get_typed_func::<(i32, i32), ()>(&mut store, "bedrock_free")?;
        let _ = handle_trap(free_fn.call(&mut store, (resp_ptr, resp_len)));

        // 14. Deserialize response
        let response = decode_execution_response(&resp_bytes)
            .map_err(|e| SandboxError::ResponseError(format!("{}", e)))?;

        Ok(response)
    }
}

/// Create a Wasmtime engine with deterministic configuration.
fn create_engine(config: &SandboxConfig) -> Result<Engine, SandboxError> {
    let mut wasm_config = Config::new();

    // Fuel metering — prevents infinite loops
    wasm_config.consume_fuel(true);

    // Determinism enforcement
    wasm_config.wasm_threads(false);
    wasm_config.wasm_simd(false);
    wasm_config.wasm_relaxed_simd(false);
    wasm_config.wasm_multi_memory(false);
    wasm_config.cranelift_nan_canonicalization(true);

    // Memory limits
    let max_bytes = (config.max_memory_pages as u64) * 65536;
    wasm_config.memory_guaranteed_dense_image_size(max_bytes.min(16 * 1024 * 1024));

    Ok(Engine::new(&wasm_config)?)
}

/// Allocate space in the host allocator region and write data there.
fn alloc_and_write(
    memory: &wasmtime::Memory,
    store: &mut Store<HostState>,
    data: &[u8],
) -> Result<i32, SandboxError> {
    if data.is_empty() {
        return Ok(0);
    }

    let (ptr, new_bump, new_cap, grow_pages) =
        store.data().host_alloc.compute_alloc(data.len());

    if grow_pages > 0 {
        memory
            .grow(&mut *store, grow_pages)
            .map_err(|e| SandboxError::MemoryError(format!("alloc grow: {}", e)))?;
    }

    memory.data_mut(&mut *store)[ptr..ptr + data.len()].copy_from_slice(data);
    store.data_mut().host_alloc.commit(new_bump, new_cap);

    Ok(ptr as i32)
}

/// Handle a guest function call result, converting traps to SandboxError.
///
/// Fuel exhaustion → `SandboxError::FuelExhausted`
/// Other traps → `SandboxError::GuestTrapped`
fn handle_trap<R>(result: Result<R, anyhow::Error>) -> Result<R, SandboxError> {
    match result {
        Ok(val) => Ok(val),
        Err(e) => {
            let msg = format!("{}", e);
            if msg.contains("fuel") {
                Err(SandboxError::FuelExhausted)
            } else {
                Err(SandboxError::GuestTrapped(msg))
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_engine() {
        let config = SandboxConfig::default();
        let engine = create_engine(&config);
        assert!(engine.is_ok());
    }

    #[test]
    fn test_sandbox_rejects_empty_wasm() {
        let config = SandboxConfig::default();
        let result = Sandbox::new(&[], config);
        assert!(result.is_err());
    }

    #[test]
    fn test_sandbox_accepts_minimal_valid_module() {
        let wat = r#"
            (module
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                    i32.const 0)
                (func (export "bedrock_free") (param i32 i32))
            )
        "#;
        let config = SandboxConfig::default();
        let sandbox = Sandbox::new(wat.as_bytes(), config);
        assert!(sandbox.is_ok());
    }

    #[test]
    fn test_sandbox_rejects_missing_export() {
        let wat = r#"
            (module
                (memory (export "memory") 1)
                (func (export "bedrock_init") (param i32 i32) (result i32)
                    i32.const 0)
            )
        "#;
        let config = SandboxConfig::default();
        let result = Sandbox::new(wat.as_bytes(), config);
        assert!(result.is_err());
    }

    // ── Integration tests using the real WASM artifact ──

    /// Path to the WASM artifact built by ExecutionCore.
    const WASM_ARTIFACT: &str = concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../ExecutionCore/wasm/artifacts/bedrock-execution-v0.1.0.wasm"
    );

    fn load_real_sandbox() -> Option<Sandbox> {
        let path = std::path::Path::new(WASM_ARTIFACT);
        if !path.exists() {
            eprintln!("WASM artifact not found at {:?}, skipping integration test", path);
            return None;
        }
        Some(Sandbox::from_file(path, SandboxConfig::default()).unwrap())
    }

    #[test]
    fn test_load_real_wasm_module() {
        let sandbox = load_real_sandbox();
        assert!(sandbox.is_some(), "WASM artifact must exist for integration tests");
    }

    #[test]
    fn test_execute_empty_block() {
        let sandbox = match load_real_sandbox() {
            Some(s) => s,
            None => return,
        };

        let store = Arc::new(bedrock_hostapi::MemStore::new());
        let request = bedrock_primitives::ExecutionRequest {
            api_version: API_VERSION,
            chain_id: b"test-chain".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: [0u8; 32],
            prev_state_root: [0u8; 32],
            transactions: vec![],
            limits: bedrock_primitives::ExecutionLimits::default(),
            execution_seed: None,
        };

        let response = sandbox.execute_block(&request, store).unwrap();
        assert!(
            response.status.is_ok(),
            "empty block should succeed, got: {:?}",
            response.status
        );
        assert!(response.receipts.is_empty());
        assert_eq!(response.api_version, API_VERSION);
    }

    #[test]
    fn test_execute_malformed_transaction() {
        let sandbox = match load_real_sandbox() {
            Some(s) => s,
            None => return,
        };

        let store = Arc::new(bedrock_hostapi::MemStore::new());
        let request = bedrock_primitives::ExecutionRequest {
            api_version: API_VERSION,
            chain_id: b"test-chain".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: [0u8; 32],
            prev_state_root: [0u8; 32],
            transactions: vec![b"not-a-valid-tx".to_vec()],
            limits: bedrock_primitives::ExecutionLimits::default(),
            execution_seed: None,
        };

        let response = sandbox.execute_block(&request, store).unwrap();
        // Malformed transaction should result in a failed receipt, not a block failure
        assert!(response.status.is_ok());
        assert_eq!(response.receipts.len(), 1);
        assert!(!response.receipts[0].success);
    }

    #[test]
    fn test_determinism_same_input_same_output() {
        let sandbox = match load_real_sandbox() {
            Some(s) => s,
            None => return,
        };

        let request = bedrock_primitives::ExecutionRequest {
            api_version: API_VERSION,
            chain_id: b"test-chain".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: [0u8; 32],
            prev_state_root: [0u8; 32],
            transactions: vec![],
            limits: bedrock_primitives::ExecutionLimits::default(),
            execution_seed: None,
        };

        let store1 = Arc::new(bedrock_hostapi::MemStore::new());
        let response1 = sandbox.execute_block(&request, store1).unwrap();

        let store2 = Arc::new(bedrock_hostapi::MemStore::new());
        let response2 = sandbox.execute_block(&request, store2).unwrap();

        // Determinism: same input must produce identical output
        assert_eq!(response1.status, response2.status);
        assert_eq!(response1.new_state_root, response2.new_state_root);
        assert_eq!(response1.gas_used, response2.gas_used);
        assert_eq!(response1.receipts.len(), response2.receipts.len());
    }
}
