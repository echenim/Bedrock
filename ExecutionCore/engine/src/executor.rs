//! Block executor — the core deterministic state transition function.
//!
//! `BlockExecutor::execute_block` implements the execution lifecycle
//! from EXECUTION_SPEC.md §2.2:
//!
//! 1. Validate request (api_version, field validity)
//! 2. For each transaction:
//!    a. Decode and validate structure
//!    b. Process transaction (state reads/writes, signature verification)
//!    c. Produce receipt
//!    d. Check block-level gas
//! 3. Compute new state root from all buffered writes
//! 4. Return `ExecutionResponse`
//!
//! **Atomicity (§2.3):** Block execution is atomic. If a fatal block-level
//! error occurs (e.g., block gas exceeded), the entire response has error
//! status and no state commits. Individual transaction failures produce
//! failed receipts but do NOT abort the block.

use bedrock_primitives::{
    ExecutionRequest, ExecutionResponse, ExecutionStatus,
    Receipt, SparseMerkleTree,
};
use crate::host::HostInterface;
use crate::validation::validate_request;
use crate::transaction::{decode_transaction, process_transaction};
use alloc::vec::Vec;

/// The core block executor.
///
/// Stateless — all state is accessed through the `HostInterface`.
/// This ensures determinism: given the same request and state, the
/// executor always produces the same response.
pub struct BlockExecutor;

impl BlockExecutor {
    /// Execute a full block, producing an `ExecutionResponse`.
    ///
    /// This is the top-level entry point for the execution engine.
    /// The host provides state access, gas metering, and event collection.
    pub fn execute_block(
        request: &ExecutionRequest,
        host: &mut dyn HostInterface,
    ) -> ExecutionResponse {
        // Phase 1: Validate request
        if let Err(err) = validate_request(request) {
            let status = match err {
                bedrock_primitives::ExecError::InvalidApiVersion { .. } => {
                    ExecutionStatus::InvalidBlock
                }
                bedrock_primitives::ExecError::InvalidBlock(_) => {
                    ExecutionStatus::InvalidBlock
                }
                _ => ExecutionStatus::ExecutionError,
            };
            return ExecutionResponse::failure(
                request.api_version,
                status,
                request.prev_state_root,
            );
        }

        // Phase 2: Process transactions
        let mut receipts: Vec<Receipt> = Vec::with_capacity(request.transactions.len());
        let mut block_gas_exceeded = false;

        for (idx, raw_tx) in request.transactions.iter().enumerate() {
            let tx_index = idx as u32;

            // Decode the transaction
            let decoded = match decode_transaction(raw_tx) {
                Ok(tx) => tx,
                Err(_err) => {
                    // Malformed transaction → failed receipt, block continues
                    receipts.push(Receipt {
                        tx_index,
                        success: false,
                        gas_used: 0,
                        result_code: 2, // ERR_INVALID_ENCODING
                        return_data: Vec::new(),
                    });
                    continue;
                }
            };

            // Process the transaction (gas metered through host)
            let receipt = process_transaction(tx_index, &decoded, host);

            // Check if gas is exhausted at the block level
            if host.gas_meter().is_exhausted() {
                receipts.push(receipt);
                block_gas_exceeded = true;
                break;
            }

            receipts.push(receipt);
        }

        // Phase 3: Determine status and compute state root
        if block_gas_exceeded {
            // Block gas exceeded — fail entire block, discard writes
            return ExecutionResponse {
                api_version: request.api_version,
                status: ExecutionStatus::OutOfGas,
                new_state_root: request.prev_state_root,
                gas_used: host.gas_meter().consumed(),
                receipts,
                events: Vec::new(),
                logs: host.logs().to_vec(),
            };
        }

        // Phase 4: Compute new state root
        let new_state_root = compute_state_root(
            &request.prev_state_root,
            host,
        );

        ExecutionResponse {
            api_version: request.api_version,
            status: ExecutionStatus::Ok,
            new_state_root,
            gas_used: host.gas_meter().consumed(),
            receipts,
            events: host.events().to_vec(),
            logs: host.logs().to_vec(),
        }
    }
}

/// Compute the new state root by applying all buffered writes to a
/// Merkle tree seeded with the previous state root.
///
/// For the initial version, we build a fresh Merkle tree from the
/// overlay writes. In production, this would be an incremental update
/// to a persistent Merkle structure.
fn compute_state_root(
    prev_state_root: &bedrock_primitives::Hash,
    host: &dyn HostInterface,
) -> bedrock_primitives::Hash {
    let overlay = host.overlay();
    let writes = overlay.writes();

    if writes.is_empty() {
        return *prev_state_root;
    }

    let mut tree = SparseMerkleTree::new();
    tree.apply_writes(writes);
    tree.root()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::host::MockHost;
    use crate::transaction::encode_transfer_tx;
    use alloc::collections::BTreeMap;
    use bedrock_primitives::{
        ExecutionLimits, ExecutionContext,
        crypto::generate_keypair,
        types::{ZERO_HASH, API_VERSION},
    };

    fn test_context() -> ExecutionContext {
        ExecutionContext {
            chain_id: b"test-chain".to_vec(),
            block_height: 1,
            block_time: 1_700_000_000,
            block_hash: ZERO_HASH,
            gas_limit: 10_000_000,
            max_events: 1024,
            max_write_bytes: 4 * 1024 * 1024,
            api_version: API_VERSION,
            execution_seed: None,
        }
    }

    fn make_request(transactions: Vec<Vec<u8>>) -> ExecutionRequest {
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

    fn setup_funded_host(addr: &bedrock_primitives::Address, balance: u64) -> MockHost {
        let mut committed = BTreeMap::new();
        let mut key = Vec::new();
        key.extend_from_slice(b"acct/");
        key.extend_from_slice(addr);
        key.extend_from_slice(b"/balance");
        committed.insert(key, balance.to_le_bytes().to_vec());
        MockHost::new(committed, test_context())
    }

    // ── Test: empty block ──

    #[test]
    fn test_execute_empty_block() {
        let request = make_request(Vec::new());
        let mut host = MockHost::new(BTreeMap::new(), test_context());

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::Ok);
        assert_eq!(response.new_state_root, ZERO_HASH);
        assert!(response.receipts.is_empty());
        assert_eq!(response.gas_used, 0);
    }

    // ── Test: invalid API version ──

    #[test]
    fn test_execute_invalid_api_version() {
        let mut request = make_request(Vec::new());
        request.api_version = 999;
        let mut host = MockHost::new(BTreeMap::new(), test_context());

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::InvalidBlock);
        assert_eq!(response.new_state_root, ZERO_HASH);
        assert_eq!(response.gas_used, 0);
    }

    // ── Test: zero block height ──

    #[test]
    fn test_execute_zero_block_height() {
        let mut request = make_request(Vec::new());
        request.block_height = 0;
        let mut host = MockHost::new(BTreeMap::new(), test_context());

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::InvalidBlock);
    }

    // ── Test: valid transfer ──

    #[test]
    fn test_execute_valid_transfer() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_funded_host(&sender, 5000);
        let raw_tx = encode_transfer_tx(&sender, 0, &to, 1000, &sk);
        let request = make_request(vec![raw_tx]);

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::Ok);
        assert_eq!(response.receipts.len(), 1);
        assert!(response.receipts[0].success);
        assert!(response.gas_used > 0);
        assert_ne!(response.new_state_root, ZERO_HASH);
    }

    // ── Test: invalid signature produces failed receipt ──

    #[test]
    fn test_execute_invalid_signature_receipt() {
        let (vk, _sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_funded_host(&sender, 5000);

        // Build raw tx with bad signature
        let mut raw = Vec::new();
        raw.extend_from_slice(&sender);
        raw.extend_from_slice(&0u64.to_le_bytes());
        raw.push(0x01); // transfer
        raw.extend_from_slice(&to);
        raw.extend_from_slice(&1000u64.to_le_bytes());
        raw.extend_from_slice(vk.as_bytes());
        raw.extend_from_slice(&[0u8; 64]); // bad sig

        let request = make_request(vec![raw]);
        let response = BlockExecutor::execute_block(&request, &mut host);

        // Block succeeds but transaction fails
        assert_eq!(response.status, ExecutionStatus::Ok);
        assert_eq!(response.receipts.len(), 1);
        assert!(!response.receipts[0].success);
        assert_eq!(response.receipts[0].result_code, 8); // ERR_SIG_INVALID
    }

    // ── Test: malformed transaction ──

    #[test]
    fn test_execute_malformed_transaction() {
        let mut host = MockHost::new(BTreeMap::new(), test_context());
        let request = make_request(vec![vec![0u8; 10]]); // too short

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::Ok);
        assert_eq!(response.receipts.len(), 1);
        assert!(!response.receipts[0].success);
        assert_eq!(response.receipts[0].result_code, 2); // ERR_INVALID_ENCODING
    }

    // ── Test: gas accounting ──

    #[test]
    fn test_gas_accounting_sum() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_funded_host(&sender, 100_000);

        let raw_tx0 = encode_transfer_tx(&sender, 0, &to, 100, &sk);
        let raw_tx1 = encode_transfer_tx(&sender, 1, &to, 200, &sk);
        let request = make_request(vec![raw_tx0, raw_tx1]);

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::Ok);
        assert_eq!(response.receipts.len(), 2);

        // Sum of receipt gas_used should equal block gas_used
        let receipt_gas: u64 = response.receipts.iter().map(|r| r.gas_used).sum();
        assert_eq!(receipt_gas, response.gas_used);
    }

    // ── Test: block gas limit exceeded ──

    #[test]
    fn test_execute_block_gas_exceeded() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let ctx = ExecutionContext {
            gas_limit: 3000, // too low for 2 full txs
            ..test_context()
        };
        let mut committed = BTreeMap::new();
        let mut key = Vec::new();
        key.extend_from_slice(b"acct/");
        key.extend_from_slice(&sender);
        key.extend_from_slice(b"/balance");
        committed.insert(key, 100_000u64.to_le_bytes().to_vec());
        let mut host = MockHost::new(committed, ctx);

        let raw_tx0 = encode_transfer_tx(&sender, 0, &to, 100, &sk);
        let raw_tx1 = encode_transfer_tx(&sender, 1, &to, 200, &sk);
        let mut request = make_request(vec![raw_tx0, raw_tx1]);
        request.limits.gas_limit = 3000;

        let response = BlockExecutor::execute_block(&request, &mut host);

        // At least one tx should fail with gas-related error
        let has_gas_failure = response.status == ExecutionStatus::OutOfGas
            || response.receipts.iter().any(|r| !r.success && r.result_code == 7);
        assert!(has_gas_failure);
    }

    // ── Test: determinism ──

    #[test]
    fn test_determinism_same_input_same_output() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let raw_tx = encode_transfer_tx(&sender, 0, &to, 1000, &sk);

        // Execute twice with identical setup
        let response1 = {
            let mut host = setup_funded_host(&sender, 5000);
            BlockExecutor::execute_block(&make_request(vec![raw_tx.clone()]), &mut host)
        };

        let response2 = {
            let mut host = setup_funded_host(&sender, 5000);
            BlockExecutor::execute_block(&make_request(vec![raw_tx]), &mut host)
        };

        assert_eq!(response1.status, response2.status);
        assert_eq!(response1.new_state_root, response2.new_state_root);
        assert_eq!(response1.gas_used, response2.gas_used);
        assert_eq!(response1.receipts.len(), response2.receipts.len());
        for (r1, r2) in response1.receipts.iter().zip(response2.receipts.iter()) {
            assert_eq!(r1.success, r2.success);
            assert_eq!(r1.gas_used, r2.gas_used);
        }
    }

    // ── Test: cumulative state within block ──

    #[test]
    fn test_cumulative_state_within_block() {
        let (vk, sk) = generate_keypair();
        let sender = *vk.as_bytes();
        let to = [2u8; 32];

        let mut host = setup_funded_host(&sender, 10_000);

        // Two transfers: tx0 sends 3000, tx1 sends 2000
        let raw_tx0 = encode_transfer_tx(&sender, 0, &to, 3000, &sk);
        let raw_tx1 = encode_transfer_tx(&sender, 1, &to, 2000, &sk);
        let request = make_request(vec![raw_tx0, raw_tx1]);

        let response = BlockExecutor::execute_block(&request, &mut host);

        assert_eq!(response.status, ExecutionStatus::Ok);
        assert_eq!(response.receipts.len(), 2);
        assert!(response.receipts[0].success);
        assert!(response.receipts[1].success);
    }

    // ── Test: empty chain_id rejected ──

    #[test]
    fn test_execute_empty_chain_id() {
        let mut request = make_request(Vec::new());
        request.chain_id = Vec::new();
        let mut host = MockHost::new(BTreeMap::new(), test_context());

        let response = BlockExecutor::execute_block(&request, &mut host);
        assert_eq!(response.status, ExecutionStatus::InvalidBlock);
    }

    // ── Test: zero gas_limit rejected ──

    #[test]
    fn test_execute_zero_gas_limit() {
        let mut request = make_request(Vec::new());
        request.limits.gas_limit = 0;
        let mut host = MockHost::new(BTreeMap::new(), test_context());

        let response = BlockExecutor::execute_block(&request, &mut host);
        assert_eq!(response.status, ExecutionStatus::InvalidBlock);
    }
}
