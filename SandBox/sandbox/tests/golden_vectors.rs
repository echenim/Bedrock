//! Golden vector tests — load JSON vectors, execute, compare all fields.
//!
//! Golden vectors capture the exact output of the execution pipeline for
//! known inputs. Any change to the execution engine that alters these outputs
//! represents a potential consensus fork and must be reviewed carefully.

mod common;

use std::sync::Arc;

use bedrock_hostapi::MemStore;
use bedrock_primitives::types::ZERO_HASH;
use serde::Deserialize;

use common::*;

/// JSON representation of a golden vector test case.
#[derive(Deserialize)]
struct GoldenVector {
    name: String,
    /// Accounts to fund before execution: [[seed, balance], ...]
    funded_accounts: Vec<(u8, u64)>,
    /// Transfers to execute: [[sender_seed, nonce, recipient_seed, amount], ...]
    transfers: Vec<(u8, u64, u8, u64)>,
    /// Expected execution status (0=Ok, 1=InvalidBlock, 2=ExecutionError, 3=OutOfGas)
    expected_status: u8,
    /// Expected state root as hex string (64 chars, no 0x prefix)
    expected_state_root: String,
    /// Expected total gas used
    expected_gas_used: u64,
    /// Expected receipt count
    expected_receipt_count: usize,
    /// Per-receipt expectations: [[success, gas_used, result_code], ...]
    expected_receipts: Vec<(bool, u64, u32)>,
    /// Expected event count
    expected_event_count: usize,
}

fn hex_to_hash(hex: &str) -> [u8; 32] {
    assert_eq!(hex.len(), 64, "hex hash must be 64 chars");
    let mut hash = [0u8; 32];
    for i in 0..32 {
        hash[i] = u8::from_str_radix(&hex[i * 2..i * 2 + 2], 16)
            .unwrap_or_else(|_| panic!("invalid hex at position {}", i * 2));
    }
    hash
}

fn keypair_for_seed(seed: u8) -> (bedrock_primitives::Address, ed25519_dalek::SigningKey) {
    let (vk, sk) = deterministic_keypair(seed);
    (*vk.as_bytes(), sk)
}

fn execute_golden_vector(vector: &GoldenVector) {
    let sandbox = load_sandbox();

    // Build funded store
    let store = if vector.funded_accounts.is_empty() {
        Arc::new(MemStore::new())
    } else {
        let accounts: Vec<(bedrock_primitives::Address, u64)> = vector
            .funded_accounts
            .iter()
            .map(|(seed, balance)| {
                let (addr, _) = keypair_for_seed(*seed);
                (addr, *balance)
            })
            .collect();
        let refs: Vec<(&bedrock_primitives::Address, u64)> =
            accounts.iter().map(|(a, b)| (a, *b)).collect();
        funded_store_multi(&refs)
    };

    // Build transactions
    let txs: Vec<Vec<u8>> = vector
        .transfers
        .iter()
        .map(|(sender_seed, nonce, recipient_seed, amount)| {
            let (sender_addr, sender_sk) = keypair_for_seed(*sender_seed);
            let (recipient_addr, _) = keypair_for_seed(*recipient_seed);
            encode_transfer_tx(&sender_addr, *nonce, &recipient_addr, *amount, &sender_sk)
        })
        .collect();

    let request = make_request(txs);
    let response = sandbox.execute_block(&request, store).unwrap();

    // Assert status
    assert_eq!(
        response.status as u8, vector.expected_status,
        "[{}] status mismatch",
        vector.name
    );

    // Assert state root
    if vector.expected_state_root == "0".repeat(64) {
        assert_eq!(
            response.new_state_root, ZERO_HASH,
            "[{}] state root should be ZERO_HASH",
            vector.name
        );
    } else {
        let expected = hex_to_hash(&vector.expected_state_root);
        assert_eq!(
            response.new_state_root, expected,
            "[{}] state root mismatch",
            vector.name
        );
    }

    // Assert gas
    assert_eq!(
        response.gas_used, vector.expected_gas_used,
        "[{}] gas_used mismatch",
        vector.name
    );

    // Assert receipts
    assert_eq!(
        response.receipts.len(),
        vector.expected_receipt_count,
        "[{}] receipt count mismatch",
        vector.name
    );
    for (i, (exp_success, exp_gas, exp_code)) in vector.expected_receipts.iter().enumerate() {
        let receipt = &response.receipts[i];
        assert_eq!(
            receipt.success, *exp_success,
            "[{}] receipt {} success mismatch",
            vector.name, i
        );
        assert_eq!(
            receipt.gas_used, *exp_gas,
            "[{}] receipt {} gas_used mismatch",
            vector.name, i
        );
        assert_eq!(
            receipt.result_code, *exp_code,
            "[{}] receipt {} result_code mismatch",
            vector.name, i
        );
    }

    // Assert events
    assert_eq!(
        response.events.len(),
        vector.expected_event_count,
        "[{}] event count mismatch",
        vector.name
    );
}

// ── Test: empty block golden vector ──

#[test]
fn test_golden_empty_block() {
    let json = include_str!("vectors/empty_block.json");
    let vector: GoldenVector = serde_json::from_str(json).unwrap();
    execute_golden_vector(&vector);
}

// ── Test: single transfer golden vector ──

#[test]
fn test_golden_single_transfer() {
    let json = include_str!("vectors/single_transfer.json");
    let vector: GoldenVector = serde_json::from_str(json).unwrap();
    execute_golden_vector(&vector);
}

// ── Test: multi-tx golden vector ──

#[test]
fn test_golden_multi_tx() {
    let json = include_str!("vectors/multi_tx.json");
    let vector: GoldenVector = serde_json::from_str(json).unwrap();
    execute_golden_vector(&vector);
}

// ── Generator: run with `cargo test -p bedrock-sandbox --test golden_vectors generate_ -- --ignored` ──
// These tests generate the actual values to fill into JSON files.

#[test]
#[ignore]
fn generate_golden_values() {
    let sandbox = load_sandbox();

    // Empty block
    {
        let store = Arc::new(MemStore::new());
        let request = make_request(vec![]);
        let resp = sandbox.execute_block(&request, store).unwrap();
        println!("=== EMPTY BLOCK ===");
        println!("status: {}", resp.status as u8);
        println!("state_root: {}", hex::encode(resp.new_state_root));
        println!("gas_used: {}", resp.gas_used);
        println!("receipts: {}", resp.receipts.len());
        println!("events: {}", resp.events.len());
        println!();
    }

    // Single transfer: Alice(1) -> Bob(2), 3000
    {
        let (alice_addr, alice_sk) = alice();
        let (bob_addr, _) = bob();
        let store = funded_store(&alice_addr, 10_000);
        let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 3000, &alice_sk);
        let request = make_request(vec![tx]);
        let resp = sandbox.execute_block(&request, store).unwrap();
        println!("=== SINGLE TRANSFER ===");
        println!("status: {}", resp.status as u8);
        println!("state_root: {}", hex::encode(resp.new_state_root));
        println!("gas_used: {}", resp.gas_used);
        for (i, r) in resp.receipts.iter().enumerate() {
            println!(
                "receipt[{}]: success={}, gas={}, code={}",
                i, r.success, r.gas_used, r.result_code
            );
        }
        println!("events: {}", resp.events.len());
        println!();
    }

    // Multi-tx: Alice(1) -> Bob(2) x3
    {
        let (alice_addr, alice_sk) = alice();
        let (bob_addr, _) = bob();
        let store = funded_store(&alice_addr, 100_000);
        let tx0 = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
        let tx1 = encode_transfer_tx(&alice_addr, 1, &bob_addr, 2000, &alice_sk);
        let tx2 = encode_transfer_tx(&alice_addr, 2, &bob_addr, 3000, &alice_sk);
        let request = make_request(vec![tx0, tx1, tx2]);
        let resp = sandbox.execute_block(&request, store).unwrap();
        println!("=== MULTI TX ===");
        println!("status: {}", resp.status as u8);
        println!("state_root: {}", hex::encode(resp.new_state_root));
        println!("gas_used: {}", resp.gas_used);
        for (i, r) in resp.receipts.iter().enumerate() {
            println!(
                "receipt[{}]: success={}, gas={}, code={}",
                i, r.success, r.gas_used, r.result_code
            );
        }
        println!("events: {}", resp.events.len());
    }
}

/// Hex encoding helper (used only by the generator)
mod hex {
    pub fn encode(bytes: impl AsRef<[u8]>) -> String {
        bytes
            .as_ref()
            .iter()
            .map(|b| format!("{:02x}", b))
            .collect()
    }
}
