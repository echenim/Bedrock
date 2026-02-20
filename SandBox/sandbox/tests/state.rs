//! State transition integration tests.
//!
//! Verify balance/nonce changes and state root computation through the
//! full WASM pipeline. State roots are verified by independently building
//! a SparseMerkleTree from the expected overlay writes.

mod common;

use std::sync::Arc;

use bedrock_hostapi::MemStore;
use bedrock_primitives::types::{u64_to_le_bytes, ZERO_HASH};
use bedrock_primitives::SparseMerkleTree;

use common::*;

// ── Test: balance debited and credited ──

#[test]
fn test_balance_debited_and_credited() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 10_000);
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 3000, &alice_sk);
    let request = make_request(vec![tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert!(response.receipts[0].success);

    // Independently compute expected state root
    let mut tree = SparseMerkleTree::new();
    // After transfer: alice balance=7000, bob balance=3000, alice nonce=1
    tree.insert(&balance_key(&alice_addr), &u64_to_le_bytes(7000));
    tree.insert(&balance_key(&bob_addr), &u64_to_le_bytes(3000));
    tree.insert(&nonce_key(&alice_addr), &u64_to_le_bytes(1));
    let expected_root = tree.root();

    assert_eq!(
        response.new_state_root, expected_root,
        "state root should match independently computed Merkle tree"
    );
}

// ── Test: nonce increments ──

#[test]
fn test_nonce_increments() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 50_000);
    let tx0 = encode_transfer_tx(&alice_addr, 0, &bob_addr, 100, &alice_sk);
    let tx1 = encode_transfer_tx(&alice_addr, 1, &bob_addr, 200, &alice_sk);
    let request = make_request(vec![tx0, tx1]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 2);
    assert!(response.receipts[0].success);
    assert!(response.receipts[1].success);

    // After 2 txs: nonce should be 2
    let mut tree = SparseMerkleTree::new();
    tree.insert(&balance_key(&alice_addr), &u64_to_le_bytes(50_000 - 300));
    tree.insert(&balance_key(&bob_addr), &u64_to_le_bytes(300));
    tree.insert(&nonce_key(&alice_addr), &u64_to_le_bytes(2));
    let expected_root = tree.root();

    assert_eq!(response.new_state_root, expected_root);
}

// ── Test: empty block preserves state root ──

#[test]
fn test_empty_block_preserves_root() {
    let sandbox = load_sandbox();
    let store = Arc::new(MemStore::new());
    let request = make_request(vec![]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(
        response.new_state_root, ZERO_HASH,
        "empty block with ZERO_HASH input should return ZERO_HASH"
    );
}

// ── Test: state root changes on writes ──

#[test]
fn test_state_root_changes_on_writes() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 10_000);
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    let request = make_request(vec![tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_ne!(
        response.new_state_root, ZERO_HASH,
        "state root should be non-zero after successful transfer"
    );
}

// ── Test: failed tx minimal state impact ──

#[test]
fn test_failed_tx_minimal_state_impact() {
    let sandbox = load_sandbox();
    let store = Arc::new(MemStore::new());

    // Only malformed txs — should produce no state changes
    let request = make_request(vec![b"bad1".to_vec(), b"bad2".to_vec()]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 2);
    assert!(!response.receipts[0].success);
    assert!(!response.receipts[1].success);
    assert_eq!(
        response.new_state_root, ZERO_HASH,
        "failed txs should not change state root"
    );
}
