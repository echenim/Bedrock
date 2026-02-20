//! Resource limit tests — fuel exhaustion, memory limits, ABI validation.
//!
//! These tests verify that the sandbox correctly enforces resource limits
//! and rejects invalid WASM modules.

mod common;

use std::sync::Arc;

use bedrock_hostapi::MemStore;
use bedrock_sandbox::{Sandbox, SandboxConfig, SandboxError};

use common::*;

// ── Test: fuel exhaustion ──

#[test]
fn test_fuel_exhaustion() {
    let config = SandboxConfig {
        fuel_limit: 1_000, // extremely low — should exhaust during init or execute
        ..SandboxConfig::default()
    };
    let sandbox = load_sandbox_with_config(config);
    let store = Arc::new(MemStore::new());
    let request = make_request(vec![]);

    let result = sandbox.execute_block(&request, store);

    // Fuel exhaustion may manifest as FuelExhausted or GuestTrapped depending
    // on where in the WASM execution the fuel runs out.
    assert!(
        result.is_err(),
        "execution with 1000 fuel should fail"
    );
    match result {
        Err(SandboxError::FuelExhausted) | Err(SandboxError::GuestTrapped(_)) => {}
        Err(e) => panic!("expected FuelExhausted or GuestTrapped, got: {:?}", e),
        Ok(_) => unreachable!(),
    }
}

// ── Test: sufficient fuel for empty block ──

#[test]
fn test_sufficient_fuel_empty_block() {
    let config = SandboxConfig {
        fuel_limit: 10_000_000, // moderate fuel — enough for empty block
        ..SandboxConfig::default()
    };
    let sandbox = load_sandbox_with_config(config);
    let store = Arc::new(MemStore::new());
    let request = make_request(vec![]);

    let response = sandbox.execute_block(&request, store).unwrap();
    assert!(response.status.is_ok());
}

// ── Test: memory limit enforced via WAT module with max 1 page ──

#[test]
fn test_memory_limit_too_low() {
    // Use a WAT module where memory is capped at 1 page (64 KiB).
    // The sandbox runtime grows memory by HOST_ALLOC_PAGES (4 pages),
    // which exceeds the 1-page max and should fail.
    let wat = r#"
        (module
            (import "bedrock_host" "state_get"
                (func (param i32 i32 i32 i32) (result i32)))
            (import "bedrock_host" "state_set"
                (func (param i32 i32 i32 i32) (result i32)))
            (import "bedrock_host" "state_delete"
                (func (param i32 i32) (result i32)))
            (import "bedrock_host" "emit_event"
                (func (param i32 i32) (result i32)))
            (import "bedrock_host" "log"
                (func (param i32 i32 i32) (result i32)))
            (import "bedrock_host" "hash_blake3"
                (func (param i32 i32 i32 i32) (result i32)))
            (import "bedrock_host" "verify_ed25519"
                (func (param i32 i32 i32 i32 i32 i32) (result i32)))
            (import "bedrock_host" "gas_remaining"
                (func (param i32) (result i32)))
            (import "bedrock_host" "host_free"
                (func (param i32 i32) (result i32)))
            (import "bedrock_host" "get_context"
                (func (param i32 i32) (result i32)))
            (memory (export "memory") 1 1)
            (func (export "bedrock_init") (param i32 i32) (result i32)
                i32.const 0)
            (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                i32.const 0)
            (func (export "bedrock_free") (param i32 i32))
        )
    "#;
    let config = SandboxConfig::default();
    let sandbox = Sandbox::new(wat.as_bytes(), config).expect("module should validate");
    let store = Arc::new(MemStore::new());
    let request = make_request(vec![]);

    let result = sandbox.execute_block(&request, store);

    assert!(
        result.is_err(),
        "execution with 1-page max memory should fail on grow"
    );
    match result {
        Err(SandboxError::MemoryError(_)) => {}
        Err(e) => panic!("expected MemoryError, got: {:?}", e),
        Ok(_) => unreachable!(),
    }
}

// ── Test: ABI missing export ──

#[test]
fn test_abi_missing_export() {
    let wat = r#"
        (module
            (memory (export "memory") 1)
            (func (export "bedrock_init") (param i32 i32) (result i32)
                i32.const 0)
            (func (export "bedrock_free") (param i32 i32))
        )
    "#;
    let config = SandboxConfig::default();
    let result = Sandbox::new(wat.as_bytes(), config);

    match result {
        Err(SandboxError::ValidationError(msg)) => {
            assert!(
                msg.contains("bedrock_execute_block"),
                "error should mention missing export: {}",
                msg
            );
        }
        Err(e) => panic!("expected ValidationError, got: {:?}", e),
        Ok(_) => panic!("should reject module missing bedrock_execute_block"),
    }
}

// ── Test: WASI import rejected ──

#[test]
fn test_abi_wasi_rejected() {
    let wat = r#"
        (module
            (import "wasi_snapshot_preview1" "fd_write"
                (func (param i32 i32 i32 i32) (result i32)))
            (memory (export "memory") 1)
            (func (export "bedrock_init") (param i32 i32) (result i32)
                i32.const 0)
            (func (export "bedrock_execute_block") (param i32 i32 i32 i32) (result i32)
                i32.const 0)
            (func (export "bedrock_free") (param i32 i32))
        )
    "#;
    let config = SandboxConfig::default();
    let result = Sandbox::new(wat.as_bytes(), config);

    match result {
        Err(SandboxError::ValidationError(msg)) => {
            assert!(
                msg.contains("WASI") || msg.contains("wasi"),
                "error should mention WASI rejection: {}",
                msg
            );
        }
        Err(e) => panic!("expected ValidationError about WASI, got: {:?}", e),
        Ok(_) => panic!("should reject module with WASI imports"),
    }
}
