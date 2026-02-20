//! Block and transaction types for the BedRock protocol.
//!
//! These types represent the consensus-layer data structures defined in
//! SPEC.md §4 (Block, QC, Vote). The execution engine receives blocks
//! as part of `ExecutionRequest` and processes transactions sequentially.

use alloc::vec::Vec;
use crate::types::{Address, BlockHeight, Hash, Round};

/// Block header — matches SPEC.md §4.1.
///
/// Contains all metadata needed to identify and validate a block.
/// The block hash is computed over the canonical serialization of this header.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BlockHeader {
    /// Block height (monotonically increasing).
    pub height: BlockHeight,
    /// Round within the consensus height.
    pub round: Round,
    /// Hash of the parent block's header.
    pub parent_hash: Hash,
    /// Merkle root of the application state after executing this block.
    pub state_root: Hash,
    /// Merkle root of the transaction list.
    pub tx_root: Hash,
    /// Address of the block proposer.
    pub proposer_id: Address,
    /// Logical time from consensus header (NOT OS clock).
    /// Per EXECUTION_SPEC.md §3.2: guest must not read OS time.
    pub block_time: u64,
    /// Chain identifier (e.g., "bedrock-testnet-1").
    pub chain_id: Vec<u8>,
}

/// Full block with ordered transactions.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Block {
    /// Block header containing metadata.
    pub header: BlockHeader,
    /// Ordered list of transactions in this block.
    pub transactions: Vec<Transaction>,
}

/// An opaque transaction payload.
///
/// The execution engine receives transactions as raw bytes and is responsible
/// for decoding and validating them internally.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Transaction {
    /// Raw transaction bytes.
    pub data: Vec<u8>,
}

impl Transaction {
    /// Create a new transaction from raw bytes.
    pub fn new(data: Vec<u8>) -> Self {
        Self { data }
    }

    /// Returns the length of the transaction data in bytes.
    pub fn len(&self) -> usize {
        self.data.len()
    }

    /// Returns true if the transaction data is empty.
    pub fn is_empty(&self) -> bool {
        self.data.is_empty()
    }
}

impl Block {
    /// Returns the number of transactions in this block.
    pub fn tx_count(&self) -> usize {
        self.transactions.len()
    }

    /// Returns true if this block has no transactions.
    pub fn is_empty(&self) -> bool {
        self.transactions.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::{ZERO_HASH, ZERO_ADDRESS};

    fn sample_header() -> BlockHeader {
        BlockHeader {
            height: 1,
            round: 0,
            parent_hash: ZERO_HASH,
            state_root: ZERO_HASH,
            tx_root: ZERO_HASH,
            proposer_id: ZERO_ADDRESS,
            block_time: 1_700_000_000,
            chain_id: b"bedrock-test".to_vec(),
        }
    }

    #[test]
    fn test_block_with_transactions() {
        let block = Block {
            header: sample_header(),
            transactions: vec![
                Transaction::new(b"tx1".to_vec()),
                Transaction::new(b"tx2".to_vec()),
            ],
        };
        assert_eq!(block.tx_count(), 2);
        assert!(!block.is_empty());
    }

    #[test]
    fn test_empty_block() {
        let block = Block {
            header: sample_header(),
            transactions: vec![],
        };
        assert_eq!(block.tx_count(), 0);
        assert!(block.is_empty());
    }

    #[test]
    fn test_transaction_len() {
        let tx = Transaction::new(b"hello".to_vec());
        assert_eq!(tx.len(), 5);
        assert!(!tx.is_empty());

        let empty = Transaction::new(vec![]);
        assert!(empty.is_empty());
    }
}
