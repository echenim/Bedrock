//! Determinism tests — verify that identical inputs always produce identical outputs.
//!
//! Determinism is non-negotiable for BFT consensus: all validators must compute
//! the same state root for the same block.

mod common;

use bedrock_primitives::codec::{
    decode_execution_response, encode_execution_request, encode_execution_response,
};

use common::*;

// ── Test: 5-run identical output ──

#[test]
fn test_five_run_identical_output() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 50_000);
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 5000, &alice_sk);
    let request = make_request(vec![tx]);

    let mut responses = Vec::new();
    for _ in 0..5 {
        let resp = sandbox.execute_block(&request, store.clone()).unwrap();
        responses.push(resp);
    }

    let first = &responses[0];
    for (i, resp) in responses.iter().enumerate().skip(1) {
        assert_eq!(first.status, resp.status, "run {} status mismatch", i);
        assert_eq!(
            first.new_state_root, resp.new_state_root,
            "run {} state root mismatch",
            i
        );
        assert_eq!(first.gas_used, resp.gas_used, "run {} gas mismatch", i);
        assert_eq!(
            first.receipts.len(),
            resp.receipts.len(),
            "run {} receipt count mismatch",
            i
        );
        for (j, (r1, r2)) in first.receipts.iter().zip(resp.receipts.iter()).enumerate() {
            assert_eq!(r1.success, r2.success, "run {} receipt {} success", i, j);
            assert_eq!(r1.gas_used, r2.gas_used, "run {} receipt {} gas", i, j);
            assert_eq!(
                r1.result_code, r2.result_code,
                "run {} receipt {} code",
                i, j
            );
        }
        assert_eq!(
            first.events.len(),
            resp.events.len(),
            "run {} event count mismatch",
            i
        );
    }
}

// ── Test: multi-tx determinism ──

#[test]
fn test_multi_tx_determinism() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();
    let (charlie_addr, _) = charlie();

    let store = funded_store(&alice_addr, 100_000);
    let tx0 = encode_transfer_tx(&alice_addr, 0, &bob_addr, 1000, &alice_sk);
    let tx1 = encode_transfer_tx(&alice_addr, 1, &charlie_addr, 2000, &alice_sk);
    let tx2 = encode_transfer_tx(&alice_addr, 2, &bob_addr, 500, &alice_sk);
    let request = make_request(vec![tx0, tx1, tx2]);

    let resp1 = sandbox.execute_block(&request, store.clone()).unwrap();
    let resp2 = sandbox.execute_block(&request, store).unwrap();

    assert_eq!(resp1.new_state_root, resp2.new_state_root);
    assert_eq!(resp1.gas_used, resp2.gas_used);
    assert_eq!(resp1.receipts.len(), resp2.receipts.len());
    assert_eq!(resp1.events.len(), resp2.events.len());
}

// ── Test: response serialization roundtrip ──

#[test]
fn test_response_serialization_roundtrip() {
    let sandbox = load_sandbox();
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let store = funded_store(&alice_addr, 10_000);
    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 3000, &alice_sk);
    let request = make_request(vec![tx]);

    let response = sandbox.execute_block(&request, store).unwrap();

    // Encode and decode the response — must be identical
    let encoded = encode_execution_response(&response);
    let decoded = decode_execution_response(&encoded).unwrap();

    assert_eq!(response.api_version, decoded.api_version);
    assert_eq!(response.status, decoded.status);
    assert_eq!(response.new_state_root, decoded.new_state_root);
    assert_eq!(response.gas_used, decoded.gas_used);
    assert_eq!(response.receipts, decoded.receipts);
    assert_eq!(response.events, decoded.events);
}

// ── Test: request encode/decode roundtrip ──

#[test]
fn test_request_encode_decode_roundtrip() {
    let (alice_addr, alice_sk) = alice();
    let (bob_addr, _) = bob();

    let tx = encode_transfer_tx(&alice_addr, 0, &bob_addr, 5000, &alice_sk);
    let request = make_request(vec![tx]);

    let encoded = encode_execution_request(&request);
    let decoded =
        bedrock_primitives::codec::decode_execution_request(&encoded).unwrap();

    assert_eq!(request, decoded);

    // Encode again — must be byte-identical
    let re_encoded = encode_execution_request(&decoded);
    assert_eq!(encoded, re_encoded);
}
