//! Sparse Merkle Tree for deterministic state root computation.
//!
//! The Merkle tree produces a deterministic 32-byte root hash from a set
//! of key-value pairs. Given the same set of entries, `root()` must always
//! return the same `Hash` regardless of insertion order.
//!
//! This implementation uses a sorted-key approach: all key-value pairs
//! are sorted by key, then hashed pairwise into a binary Merkle tree.
//! This is simple, deterministic, and sufficient for the initial version.
//!
//! See EXECUTION_SPEC.md §6.4 (Merkle Commitment).

use alloc::collections::BTreeMap;
use alloc::vec::Vec;
use crate::crypto::hash_blake3;
use crate::types::Hash;

/// Domain separator for leaf nodes (prevents second-preimage attacks).
const LEAF_PREFIX: u8 = 0x00;
/// Domain separator for internal nodes.
const INTERNAL_PREFIX: u8 = 0x01;

/// Sparse Merkle Tree for state root computation.
///
/// Produces a deterministic root from key-value pairs. Uses `BTreeMap`
/// internally for sorted iteration (determinism guarantee).
#[derive(Debug, Clone)]
pub struct SparseMerkleTree {
    /// All entries, sorted by key (BTreeMap guarantees this).
    entries: BTreeMap<Vec<u8>, Vec<u8>>,
}

/// A Merkle proof for a single key.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MerkleProof {
    /// Sibling hashes along the path from leaf to root.
    pub siblings: Vec<Hash>,
    /// Indicates whether each sibling is on the left (false) or right (true).
    pub path_bits: Vec<bool>,
    /// The leaf hash (hash of key-value), or None if proving absence.
    pub leaf_hash: Option<Hash>,
}

impl SparseMerkleTree {
    /// Create a new empty Merkle tree.
    pub fn new() -> Self {
        Self {
            entries: BTreeMap::new(),
        }
    }

    /// Insert or update a key-value pair.
    pub fn insert(&mut self, key: &[u8], value: &[u8]) {
        self.entries.insert(key.to_vec(), value.to_vec());
    }

    /// Delete a key from the tree.
    pub fn delete(&mut self, key: &[u8]) {
        self.entries.remove(key);
    }

    /// Look up a value by key.
    pub fn get(&self, key: &[u8]) -> Option<&[u8]> {
        self.entries.get(key).map(|v| v.as_slice())
    }

    /// Returns true if the key exists in the tree.
    pub fn contains_key(&self, key: &[u8]) -> bool {
        self.entries.contains_key(key)
    }

    /// Returns the number of entries.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Returns true if the tree is empty.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    /// Compute the deterministic Merkle root.
    ///
    /// Empty tree returns `ZERO_HASH`.
    /// Single entry returns its leaf hash.
    /// Multiple entries are hashed pairwise into a binary tree.
    ///
    /// **Determinism guarantee:** Because `BTreeMap` iterates in sorted
    /// key order, the same set of entries always produces the same root.
    pub fn root(&self) -> Hash {
        if self.entries.is_empty() {
            return [0u8; 32];
        }

        // Compute leaf hashes in sorted key order
        let leaf_hashes: Vec<Hash> = self
            .entries
            .iter()
            .map(|(k, v)| hash_leaf(k, v))
            .collect();

        compute_root_from_leaves(&leaf_hashes)
    }

    /// Apply a batch of writes from a `StateOverlay`.
    ///
    /// `Some(value)` = set, `None` = delete.
    pub fn apply_writes(&mut self, writes: &BTreeMap<Vec<u8>, Option<Vec<u8>>>) {
        for (key, value) in writes {
            match value {
                Some(v) => {
                    self.entries.insert(key.clone(), v.clone());
                }
                None => {
                    self.entries.remove(key);
                }
            }
        }
    }

    /// Generate a Merkle proof for a key.
    pub fn prove(&self, key: &[u8]) -> MerkleProof {
        if self.entries.is_empty() {
            return MerkleProof {
                siblings: Vec::new(),
                path_bits: Vec::new(),
                leaf_hash: None,
            };
        }

        let leaf_hashes: Vec<Hash> = self
            .entries
            .iter()
            .map(|(k, v)| hash_leaf(k, v))
            .collect();

        // Find the index of the key in sorted order
        let keys: Vec<&Vec<u8>> = self.entries.keys().collect();
        let target_idx = keys.iter().position(|k| k.as_slice() == key);

        let leaf_hash = target_idx.map(|idx| leaf_hashes[idx]);

        // For a simple tree, compute the proof path
        let (siblings, path_bits) = match target_idx {
            Some(idx) => compute_proof_path(&leaf_hashes, idx),
            None => (Vec::new(), Vec::new()),
        };

        MerkleProof {
            siblings,
            path_bits,
            leaf_hash,
        }
    }

    /// Verify a Merkle proof against a root hash.
    pub fn verify_proof(
        root: &Hash,
        proof: &MerkleProof,
        key: &[u8],
        value: Option<&[u8]>,
    ) -> bool {
        let leaf_hash = match (&proof.leaf_hash, value) {
            (Some(lh), Some(v)) => {
                let expected = hash_leaf(key, v);
                if *lh != expected {
                    return false;
                }
                *lh
            }
            (None, None) => return proof.siblings.is_empty() && *root == [0u8; 32],
            _ => return false,
        };

        // Walk up the proof path
        let mut current = leaf_hash;
        for (sibling, is_right) in proof.siblings.iter().zip(proof.path_bits.iter()) {
            current = if *is_right {
                hash_internal(&current, sibling)
            } else {
                hash_internal(sibling, &current)
            };
        }

        current == *root
    }

    /// Returns a reference to all entries.
    pub fn entries(&self) -> &BTreeMap<Vec<u8>, Vec<u8>> {
        &self.entries
    }
}

impl Default for SparseMerkleTree {
    fn default() -> Self {
        Self::new()
    }
}

/// Hash a leaf node: H(LEAF_PREFIX || key_len_le32 || key || value)
fn hash_leaf(key: &[u8], value: &[u8]) -> Hash {
    let key_len = (key.len() as u32).to_le_bytes();
    let mut data = Vec::with_capacity(1 + 4 + key.len() + value.len());
    data.push(LEAF_PREFIX);
    data.extend_from_slice(&key_len);
    data.extend_from_slice(key);
    data.extend_from_slice(value);
    hash_blake3(&data)
}

/// Hash an internal node: H(INTERNAL_PREFIX || left || right)
fn hash_internal(left: &Hash, right: &Hash) -> Hash {
    let mut data = [0u8; 1 + 32 + 32];
    data[0] = INTERNAL_PREFIX;
    data[1..33].copy_from_slice(left);
    data[33..65].copy_from_slice(right);
    hash_blake3(&data)
}

/// Compute the Merkle root from a list of leaf hashes.
///
/// Uses a standard binary tree construction: hashes are paired and
/// combined level by level. If the number of leaves is odd, the last
/// leaf is promoted to the next level.
fn compute_root_from_leaves(leaves: &[Hash]) -> Hash {
    if leaves.is_empty() {
        return [0u8; 32];
    }
    if leaves.len() == 1 {
        return leaves[0];
    }

    let mut current_level: Vec<Hash> = leaves.to_vec();

    while current_level.len() > 1 {
        let mut next_level = Vec::with_capacity(current_level.len().div_ceil(2));

        let mut i = 0;
        while i < current_level.len() {
            if i + 1 < current_level.len() {
                next_level.push(hash_internal(&current_level[i], &current_level[i + 1]));
            } else {
                // Odd element: promote to next level
                next_level.push(current_level[i]);
            }
            i += 2;
        }

        current_level = next_level;
    }

    current_level[0]
}

/// Compute the proof path (siblings and direction bits) for a leaf at `index`.
fn compute_proof_path(leaves: &[Hash], index: usize) -> (Vec<Hash>, Vec<bool>) {
    if leaves.len() <= 1 {
        return (Vec::new(), Vec::new());
    }

    let mut siblings = Vec::new();
    let mut path_bits = Vec::new();
    let mut current_level: Vec<Hash> = leaves.to_vec();
    let mut idx = index;

    while current_level.len() > 1 {
        let sibling_idx = if idx.is_multiple_of(2) { idx + 1 } else { idx - 1 };

        if sibling_idx < current_level.len() {
            siblings.push(current_level[sibling_idx]);
            // true = our node is on the left (sibling is right)
            path_bits.push(idx.is_multiple_of(2));
        }
        // else: odd element, no sibling at this level

        // Build next level
        let mut next_level = Vec::with_capacity(current_level.len().div_ceil(2));
        let mut i = 0;
        while i < current_level.len() {
            if i + 1 < current_level.len() {
                next_level.push(hash_internal(&current_level[i], &current_level[i + 1]));
            } else {
                next_level.push(current_level[i]);
            }
            i += 2;
        }

        idx /= 2;
        current_level = next_level;
    }

    (siblings, path_bits)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::ZERO_HASH;

    #[test]
    fn test_empty_tree_root() {
        let tree = SparseMerkleTree::new();
        assert_eq!(tree.root(), ZERO_HASH);
    }

    #[test]
    fn test_single_entry_root() {
        let mut tree = SparseMerkleTree::new();
        tree.insert(b"key", b"value");
        let root = tree.root();
        assert_ne!(root, ZERO_HASH);

        // Same entry again should produce same root
        let mut tree2 = SparseMerkleTree::new();
        tree2.insert(b"key", b"value");
        assert_eq!(tree.root(), tree2.root());
    }

    #[test]
    fn test_insertion_order_independence() {
        // Critical determinism test: same keys in different order → same root
        let mut tree1 = SparseMerkleTree::new();
        tree1.insert(b"apple", b"1");
        tree1.insert(b"banana", b"2");
        tree1.insert(b"cherry", b"3");

        let mut tree2 = SparseMerkleTree::new();
        tree2.insert(b"cherry", b"3");
        tree2.insert(b"apple", b"1");
        tree2.insert(b"banana", b"2");

        assert_eq!(tree1.root(), tree2.root());
    }

    #[test]
    fn test_different_values_different_roots() {
        let mut tree1 = SparseMerkleTree::new();
        tree1.insert(b"key", b"value1");

        let mut tree2 = SparseMerkleTree::new();
        tree2.insert(b"key", b"value2");

        assert_ne!(tree1.root(), tree2.root());
    }

    #[test]
    fn test_delete_affects_root() {
        let mut tree = SparseMerkleTree::new();
        tree.insert(b"a", b"1");
        tree.insert(b"b", b"2");
        let root_with_both = tree.root();

        tree.delete(b"b");
        let root_after_delete = tree.root();

        assert_ne!(root_with_both, root_after_delete);

        // Root after delete should match a tree with only "a"
        let mut tree_only_a = SparseMerkleTree::new();
        tree_only_a.insert(b"a", b"1");
        assert_eq!(root_after_delete, tree_only_a.root());
    }

    #[test]
    fn test_get() {
        let mut tree = SparseMerkleTree::new();
        tree.insert(b"key", b"value");
        assert_eq!(tree.get(b"key"), Some(b"value".as_slice()));
        assert_eq!(tree.get(b"missing"), None);
    }

    #[test]
    fn test_apply_writes() {
        let mut tree = SparseMerkleTree::new();
        tree.insert(b"existing", b"old");

        let mut writes = BTreeMap::new();
        writes.insert(b"existing".to_vec(), Some(b"new".to_vec()));
        writes.insert(b"added".to_vec(), Some(b"val".to_vec()));
        writes.insert(b"to_delete".to_vec(), None);

        tree.apply_writes(&writes);

        assert_eq!(tree.get(b"existing"), Some(b"new".as_slice()));
        assert_eq!(tree.get(b"added"), Some(b"val".as_slice()));
        assert_eq!(tree.get(b"to_delete"), None);
    }

    #[test]
    fn test_proof_and_verify() {
        let mut tree = SparseMerkleTree::new();
        tree.insert(b"alice", b"100");
        tree.insert(b"bob", b"200");
        tree.insert(b"charlie", b"300");

        let root = tree.root();

        // Prove and verify "bob"
        let proof = tree.prove(b"bob");
        assert!(SparseMerkleTree::verify_proof(
            &root,
            &proof,
            b"bob",
            Some(b"200")
        ));

        // Wrong value should fail
        assert!(!SparseMerkleTree::verify_proof(
            &root,
            &proof,
            b"bob",
            Some(b"999")
        ));
    }

    #[test]
    fn test_proof_single_entry() {
        let mut tree = SparseMerkleTree::new();
        tree.insert(b"only", b"one");
        let root = tree.root();

        let proof = tree.prove(b"only");
        assert!(SparseMerkleTree::verify_proof(
            &root,
            &proof,
            b"only",
            Some(b"one")
        ));
    }

    #[test]
    fn test_large_tree_determinism() {
        let mut tree = SparseMerkleTree::new();
        for i in 0..100u32 {
            let key = alloc::format!("key_{:05}", i);
            let val = alloc::format!("val_{}", i);
            tree.insert(key.as_bytes(), val.as_bytes());
        }
        let root1 = tree.root();

        // Build same tree in reverse order
        let mut tree2 = SparseMerkleTree::new();
        for i in (0..100u32).rev() {
            let key = alloc::format!("key_{:05}", i);
            let val = alloc::format!("val_{}", i);
            tree2.insert(key.as_bytes(), val.as_bytes());
        }
        assert_eq!(root1, tree2.root());
    }

    #[test]
    fn test_leaf_hash_domain_separation() {
        // Leaf and internal hashes should use different prefixes
        let leaf = hash_leaf(b"key", b"value");
        let internal = hash_internal(&leaf, &leaf);
        assert_ne!(leaf, internal);
    }
}
