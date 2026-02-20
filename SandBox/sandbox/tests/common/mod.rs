//! Shared test helpers for integration tests.
//!
//! Provides deterministic keypairs, transaction encoding, state helpers,
//! and sandbox factory functions used across all integration test files.

#![allow(dead_code)]

use std::collections::BTreeMap;
use std::sync::Arc;

use bedrock_primitives::{
    Address, ExecutionLimits, ExecutionRequest,
    crypto::sign_ed25519,
    types::{u64_to_le_bytes, API_VERSION, ZERO_HASH},
};
use bedrock_hostapi::MemStore;
use bedrock_sandbox::{Sandbox, SandboxConfig};

/// Transfer payload type tag (must match engine/transaction.rs).
const PAYLOAD_TRANSFER: u8 = 0x01;

/// WASM artifact path (relative to sandbox crate manifest dir).
const WASM_ARTIFACT: &str = concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../ExecutionCore/wasm/artifacts/bedrock-execution-v0.1.0.wasm"
);

// ── Deterministic Keypairs ──

/// Create a deterministic Ed25519 signing key from a single seed byte.
///
/// The secret key is `[seed; 32]`, giving reproducible keys across machines.
pub fn deterministic_keypair(seed: u8) -> (ed25519_dalek::VerifyingKey, ed25519_dalek::SigningKey) {
    let secret_bytes = [seed; 32];
    let signing_key = ed25519_dalek::SigningKey::from_bytes(&secret_bytes);
    let verifying_key = signing_key.verifying_key();
    (verifying_key, signing_key)
}

/// Alice: seed=1, stable address across all tests.
pub fn alice() -> (Address, ed25519_dalek::SigningKey) {
    let (vk, sk) = deterministic_keypair(1);
    (*vk.as_bytes(), sk)
}

/// Bob: seed=2, stable address across all tests.
pub fn bob() -> (Address, ed25519_dalek::SigningKey) {
    let (vk, sk) = deterministic_keypair(2);
    (*vk.as_bytes(), sk)
}

/// Charlie: seed=3, stable address across all tests.
pub fn charlie() -> (Address, ed25519_dalek::SigningKey) {
    let (vk, sk) = deterministic_keypair(3);
    (*vk.as_bytes(), sk)
}

// ── Transaction Encoding ──

/// Encode a transfer transaction into raw bytes.
///
/// Reimplements the wire format from engine/transaction.rs to avoid
/// cross-workspace dependencies. Wire format:
/// ```text
/// [sender: 32] [nonce: 8 LE] [0x01] [to: 32] [amount: 8 LE] [pubkey: 32] [sig: 64]
/// ```
pub fn encode_transfer_tx(
    sender: &Address,
    nonce: u64,
    to: &Address,
    amount: u64,
    signing_key: &ed25519_dalek::SigningKey,
) -> Vec<u8> {
    let mut signed_data = Vec::with_capacity(81);
    signed_data.extend_from_slice(sender);
    signed_data.extend_from_slice(&u64_to_le_bytes(nonce));
    signed_data.push(PAYLOAD_TRANSFER);
    signed_data.extend_from_slice(to);
    signed_data.extend_from_slice(&u64_to_le_bytes(amount));

    let signature = sign_ed25519(&signed_data, signing_key);
    let public_key = signing_key.verifying_key();

    let mut raw = signed_data;
    raw.extend_from_slice(public_key.as_bytes());
    raw.extend_from_slice(&signature);
    raw
}

// ── State Key Builders ──

/// Build the state key for an account's balance.
/// Format: `acct/<address_32_bytes>/balance`
pub fn balance_key(addr: &Address) -> Vec<u8> {
    let mut key = Vec::with_capacity(5 + 32 + 8);
    key.extend_from_slice(b"acct/");
    key.extend_from_slice(addr);
    key.extend_from_slice(b"/balance");
    key
}

/// Build the state key for an account's nonce.
/// Format: `acct/<address_32_bytes>/nonce`
pub fn nonce_key(addr: &Address) -> Vec<u8> {
    let mut key = Vec::with_capacity(5 + 32 + 6);
    key.extend_from_slice(b"acct/");
    key.extend_from_slice(addr);
    key.extend_from_slice(b"/nonce");
    key
}

// ── State Store Builders ──

/// Create a MemStore with one account funded.
pub fn funded_store(addr: &Address, balance: u64) -> Arc<MemStore> {
    let mut data = BTreeMap::new();
    data.insert(balance_key(addr), u64_to_le_bytes(balance).to_vec());
    Arc::new(MemStore::with_data(data))
}

/// Create a MemStore with multiple accounts funded.
pub fn funded_store_multi(accounts: &[(&Address, u64)]) -> Arc<MemStore> {
    let mut data = BTreeMap::new();
    for (addr, balance) in accounts {
        data.insert(balance_key(addr), u64_to_le_bytes(*balance).to_vec());
    }
    Arc::new(MemStore::with_data(data))
}

// ── Sandbox Loaders ──

/// Load the real WASM artifact into a Sandbox with default config.
pub fn load_sandbox() -> Sandbox {
    let path = std::path::Path::new(WASM_ARTIFACT);
    assert!(
        path.exists(),
        "WASM artifact not found at {:?}. Build with: cd ExecutionCore && cargo build --release --target wasm32-unknown-unknown",
        path
    );
    Sandbox::from_file(path, SandboxConfig::default()).expect("failed to load sandbox")
}

/// Load the real WASM artifact with a custom SandboxConfig.
pub fn load_sandbox_with_config(config: SandboxConfig) -> Sandbox {
    let path = std::path::Path::new(WASM_ARTIFACT);
    assert!(path.exists(), "WASM artifact not found");
    Sandbox::from_file(path, config).expect("failed to load sandbox with config")
}

// ── Request Builders ──

/// Build a standard ExecutionRequest with the given transactions.
pub fn make_request(transactions: Vec<Vec<u8>>) -> ExecutionRequest {
    ExecutionRequest {
        api_version: API_VERSION,
        chain_id: b"test-chain".to_vec(),
        block_height: 1,
        block_time: 1_700_000_000,
        block_hash: ZERO_HASH,
        prev_state_root: ZERO_HASH,
        transactions,
        limits: ExecutionLimits::default(),
        execution_seed: None,
    }
}

/// Build an ExecutionRequest with custom limits and fuel.
pub fn make_request_with_limits(
    transactions: Vec<Vec<u8>>,
    gas_limit: u64,
) -> ExecutionRequest {
    ExecutionRequest {
        api_version: API_VERSION,
        chain_id: b"test-chain".to_vec(),
        block_height: 1,
        block_time: 1_700_000_000,
        block_hash: ZERO_HASH,
        prev_state_root: ZERO_HASH,
        transactions,
        limits: ExecutionLimits {
            gas_limit,
            ..ExecutionLimits::default()
        },
        execution_seed: None,
    }
}
