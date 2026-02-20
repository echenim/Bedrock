//! Gas accounting integration tests.
//!
//! Verify that gas metering through the full WASM pipeline produces
//! consistent, correct results.

mod common;

use std::sync::Arc;

use bedrock_hostapi::MemStore;

use common::*;

// ── Test: receipt gas sum equals block gas_used ──

#[test]
fn test_receipt_gas_sum_equals_block_gas() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 100_000);
    let tx0 = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    let tx1 = encode_transfer_tx(&alice_addr, 1, &bob_addr, 2000, &alice_sk);
    let request = make_request(vec![tx0, tx1]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 2);

    let receipt_sum: u64 = response.receipts.iter().map(|r| r.gas_used).sum();
    assert_eq!(
        receipt_sum, response.gas_used,
        "sum of receipt gas_used must equal block gas_used"
    );
}

// ── Test: gas limit exceeded ──

#[test]
fn test_gas_limit_exceeded() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 100_000);
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    // Set a gas limit too low for even one transfer
    let request = make_request_with_limits(vec![tx], 100);

    let response = sandbox.execute_block(&request, store).unwrap();

    // Either the block reports OutOfGas or the tx fails with gas error
    let has_gas_failure = !response.status.is_ok()
        || response.receipts.iter().any(|r| !r.success && r.result_code == 7);
    assert!(has_gas_failure, "should fail due to gas limit");
}

// ── Test: zero gas for malformed tx ──

#[test]
fn test_zero_gas_for_malformed_tx() {
    let sandbox = load_sandbox();
    let store = Arc::new(MemStore::new());
    let request = make_request(vec![b"garbage".to_vec()]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 1);
    assert!(!response.receipts[0].success);
    assert_eq!(
        response.receipts[0].gas_used, 0,
        "malformed tx that fails decoding should use 0 gas"
    );
}

// ── Test: mixed success/fail gas accounting ──

#[test]
fn test_mixed_success_fail_gas_accounting() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 50_000);
    let valid_tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    let malformed_tx = b"invalid".to_vec();
    let request = make_request(vec![valid_tx, malformed_tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    assert!(response.status.is_ok());
    assert_eq!(response.receipts.len(), 2);

    // Valid tx should have gas > 0
    assert!(response.receipts[0].success);
    assert!(response.receipts[0].gas_used > 0);

    // Malformed tx should have gas == 0
    assert!(!response.receipts[1].success);
    assert_eq!(response.receipts[1].gas_used, 0);

    // Block total should still match
    let receipt_sum: u64 = response.receipts.iter().map(|r| r.gas_used).sum();
    assert_eq!(receipt_sum, response.gas_used);
}
