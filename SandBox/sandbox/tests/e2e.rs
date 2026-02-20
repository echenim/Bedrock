//! End-to-end integration tests for the full execution pipeline:
//! ExecutionRequest → Sandbox → WASM guest → engine → host API → ExecutionResponse.

mod common;

use std::sync::Arc;

use bedrock_hostapi::MemStore;
use bedrock_primitives::types::{API_VERSION, ZERO_HASH};

use common::*;

// ── Test: empty block ──

#[test]
fn test_empty_block() {
    let sandbox = load_sandbox();
    let store = Arc::new(MemStore::new());
    let request = make_request(vec![]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok(), "empty block should succeed");
    assert_eq!(response.api_version, API_VERSION);
    assert!(response.receipts.is_empty());
    assert_eq!(response.gas_used, 0);
    assert_eq!(response.new_state_root, ZERO_HASH);
}

// ── Test: single transfer ──

#[test]
fn test_single_transfer() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 10_000);
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 3000, &alice_sk);
    let request = make_request(vec![tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 1);
    assert!(response.receipts[0].success);
    assert_eq!(response.receipts[0].tx_index, 0);
    assert!(response.receipts[0].gas_used > 0);
    assert_ne!(response.new_state_root, ZERO_HASH);

    // Should have emitted a transfer event
    assert!(!response.events.is_empty());
    let event = &response.events[0];
    assert_eq!(event.event_type, "transfer");
    assert_eq!(event.tx_index, 0);
}

// ── Test: multi-transaction block ──

#[test]
fn test_multi_transaction_block() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 100_000);
    let tx0 = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    let tx1 = encode_transfer_tx(&alice_addr, 1, &bob_addr, 2000, &alice_sk);
    let tx2 = encode_transfer_tx(&alice_addr, 2, &bob_addr, 3000, &alice_sk);
    let request = make_request(vec![tx0, tx1, tx2]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 3);
    for (i, receipt) in response.receipts.iter().enumerate() {
        assert!(receipt.success, "tx {} should succeed", i);
        assert_eq!(receipt.tx_index, i as u32);
    }

    // gas_used should equal sum of receipts
    let receipt_gas_sum: u64 = response.receipts.iter().map(|r| r.gas_used).sum();
    assert_eq!(response.gas_used, receipt_gas_sum);
}

// ── Test: sequential blocks (block 1 output feeds block 2 input) ──

#[test]
fn test_sequential_blocks() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    // Block 1: Alice sends 3000 to Bob
    let store1 = funded_store(&alice_addr, 10_000);
    let tx1 = encode_transfer_tx(&alice_addr, 0, &bob_addr, 3000, &alice_sk);
    let request1 = make_request(vec![tx1]);
    let response1 = sandbox.execute_block(&request1, store1).unwrap();
    assert!(response1.status.is_ok());

    // Block 2: simulate committed state after block 1
    // Alice: 10000-3000=7000 balance, nonce=1; Bob: 3000 balance
    let mut data = std::collections::BTreeMap::new();
    data.insert(balance_key(&alice_addr), bedrock_primitives::types::u64_to_le_bytes(7000).to_vec());
    data.insert(balance_key(&bob_addr), bedrock_primitives::types::u64_to_le_bytes(3000).to_vec());
    data.insert(nonce_key(&alice_addr), bedrock_primitives::types::u64_to_le_bytes(1).to_vec());
    let store2 = Arc::new(MemStore::with_data(data));

    let tx2 = encode_transfer_tx(&alice_addr, 1, &bob_addr, 2000, &alice_sk);
    let mut request2 = make_request(vec![tx2]);
    request2.block_height = 2;
    request2.prev_state_root = response1.new_state_root;

    let response2 = sandbox.execute_block(&request2, store2).unwrap();
    assert!(response2.status.is_ok());
    assert_eq!(response2.receipts.len(), 1);
    assert!(response2.receipts[0].success);
    // State root should differ from block 1
    assert_ne!(response2.new_state_root, response1.new_state_root);
}

// ── Test: malformed tx does not abort block ──

#[test]
fn test_malformed_tx_does_not_abort_block() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 10_000);
    let valid_tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    let malformed_tx = b"not-a-valid-transaction".to_vec();
    let request = make_request(vec![valid_tx, malformed_tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok(), "block should succeed despite malformed tx");
    assert_eq!(response.receipts.len(), 2);
    assert!(response.receipts[0].success, "valid tx should succeed");
    assert!(!response.receipts[1].success, "malformed tx should fail");
    assert_eq!(response.receipts[1].result_code, 2); // ERR_INVALID_ENCODING
}

// ── Test: invalid signature ──

#[test]
fn test_invalid_signature() {
    let sandbox = load_sandbox();
    let (alice_addr, _alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 10_000);

    // Manually build a tx with Alice's public key but a zeroed-out signature
    let mut raw = Vec::new();
    raw.extend_from_slice(&alice_addr); // sender
    raw.extend_from_slice(&0u64.to_le_bytes()); // nonce
    raw.push(0x01); // transfer payload type
    raw.extend_from_slice(&bob_addr); // to
    raw.extend_from_slice(&1000u64.to_le_bytes()); // amount
    raw.extend_from_slice(&alice_addr); // public_key (alice's vk == alice's address)
    raw.extend_from_slice(&[0u8; 64]); // invalid signature

    let request = make_request(vec![raw]);
    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok(), "block should succeed");
    assert_eq!(response.receipts.len(), 1);
    assert!(!response.receipts[0].success);
    assert_eq!(response.receipts[0].result_code, 8); // ERR_SIG_INVALID
}

// ── Test: insufficient balance ──

#[test]
fn test_insufficient_balance() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 500); // only 500
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk); // try to send 1000
    let request = make_request(vec![tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok(), "block should succeed");
    assert_eq!(response.receipts.len(), 1);
    assert!(!response.receipts[0].success, "overdraft should fail");
}
