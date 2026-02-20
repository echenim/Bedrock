//! Transactional state overlay for the BedRock execution layer.
//!
//! The state overlay buffers writes during block execution and makes them
//! visible to subsequent reads within the same execution. On success, the
//! buffered writes are committed atomically. On failure, they are discarded.
//!
//! See EXECUTION_SPEC.md §6 (State Model):
//! - §6.2: Reads reflect committed state + buffered writes
//! - §6.3: Writes are buffered, committed atomically on success

use alloc::collections::BTreeMap;
use alloc::vec::Vec;

/// Transactional write buffer overlaying committed state.
///
/// Uses `BTreeMap` for deterministic iteration order
/// (EXECUTION_SPEC.md §4 — determinism rules).
#[derive(Debug, Clone)]
pub struct StateOverlay {
    /// Buffered writes: key → Some(value) for sets, key → None for deletions.
    writes: BTreeMap<Vec<u8>, Option<Vec<u8>>>,
    /// Total bytes written (keys + values) for enforcing `max_write_bytes`.
    total_write_bytes: u64,
}

/// Result of looking up a key in the overlay.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OverlayResult {
    /// Key was found in the overlay with this value.
    Found(Vec<u8>),
    /// Key was explicitly deleted in this overlay.
    Deleted,
    /// Key is not in the overlay — caller must check committed state.
    NotInOverlay,
}

impl StateOverlay {
    /// Create a new empty overlay.
    pub fn new() -> Self {
        Self {
            writes: BTreeMap::new(),
            total_write_bytes: 0,
        }
    }

    /// Set a key-value pair in the overlay.
    ///
    /// If the key was previously written or deleted in this overlay,
    /// the previous entry is replaced.
    pub fn set(&mut self, key: Vec<u8>, value: Vec<u8>) {
        let new_bytes = (key.len() + value.len()) as u64;
        // Subtract bytes from any previous write for this key
        if let Some(prev) = self.writes.get(&key) {
            let prev_bytes = key.len() as u64
                + match prev {
                    Some(v) => v.len() as u64,
                    None => 0,
                };
            self.total_write_bytes = self.total_write_bytes.saturating_sub(prev_bytes);
        }
        self.total_write_bytes = self.total_write_bytes.saturating_add(new_bytes);
        self.writes.insert(key, Some(value));
    }

    /// Mark a key as deleted in the overlay.
    ///
    /// Subsequent reads for this key will return `Deleted` rather than
    /// falling through to committed state.
    pub fn delete(&mut self, key: Vec<u8>) {
        // Subtract bytes from any previous write for this key
        if let Some(prev) = self.writes.get(&key) {
            let prev_bytes = key.len() as u64
                + match prev {
                    Some(v) => v.len() as u64,
                    None => 0,
                };
            self.total_write_bytes = self.total_write_bytes.saturating_sub(prev_bytes);
        }
        // Deletion still counts the key bytes toward the write budget
        self.total_write_bytes = self.total_write_bytes.saturating_add(key.len() as u64);
        self.writes.insert(key, None);
    }

    /// Look up a key in the overlay.
    ///
    /// Returns:
    /// - `Found(value)` if the key was set in this overlay
    /// - `Deleted` if the key was deleted in this overlay
    /// - `NotInOverlay` if the key has not been touched — caller must
    ///   check committed state
    pub fn get(&self, key: &[u8]) -> OverlayResult {
        match self.writes.get(key) {
            Some(Some(value)) => OverlayResult::Found(value.clone()),
            Some(None) => OverlayResult::Deleted,
            None => OverlayResult::NotInOverlay,
        }
    }

    /// Returns true if the overlay contains any entry (set or delete) for this key.
    pub fn contains_key(&self, key: &[u8]) -> bool {
        self.writes.contains_key(key)
    }

    /// Consume the overlay and return all buffered writes.
    ///
    /// The returned map contains `Some(value)` for sets and `None` for deletions.
    /// Iteration order is deterministic (sorted by key).
    pub fn drain(self) -> BTreeMap<Vec<u8>, Option<Vec<u8>>> {
        self.writes
    }

    /// Returns a reference to the writes for iteration without consuming.
    pub fn writes(&self) -> &BTreeMap<Vec<u8>, Option<Vec<u8>>> {
        &self.writes
    }

    /// Clear all buffered writes. Used when discarding on failure.
    pub fn clear(&mut self) {
        self.writes.clear();
        self.total_write_bytes = 0;
    }

    /// Returns the number of keys touched (set or deleted) in this overlay.
    pub fn len(&self) -> usize {
        self.writes.len()
    }

    /// Returns true if no writes have been buffered.
    pub fn is_empty(&self) -> bool {
        self.writes.is_empty()
    }

    /// Returns the total bytes written (keys + values) for limit enforcement.
    pub fn total_write_bytes(&self) -> u64 {
        self.total_write_bytes
    }
}

impl Default for StateOverlay {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_overlay_set_and_get() {
        let mut overlay = StateOverlay::new();
        overlay.set(b"key1".to_vec(), b"value1".to_vec());

        assert_eq!(
            overlay.get(b"key1"),
            OverlayResult::Found(b"value1".to_vec())
        );
    }

    #[test]
    fn test_overlay_not_in_overlay() {
        let overlay = StateOverlay::new();
        assert_eq!(overlay.get(b"missing"), OverlayResult::NotInOverlay);
    }

    #[test]
    fn test_overlay_delete() {
        let mut overlay = StateOverlay::new();
        overlay.set(b"key1".to_vec(), b"value1".to_vec());
        overlay.delete(b"key1".to_vec());
        assert_eq!(overlay.get(b"key1"), OverlayResult::Deleted);
    }

    #[test]
    fn test_overlay_delete_then_set() {
        let mut overlay = StateOverlay::new();
        overlay.delete(b"key1".to_vec());
        assert_eq!(overlay.get(b"key1"), OverlayResult::Deleted);

        // Re-setting after delete should return the new value
        overlay.set(b"key1".to_vec(), b"new_value".to_vec());
        assert_eq!(
            overlay.get(b"key1"),
            OverlayResult::Found(b"new_value".to_vec())
        );
    }

    #[test]
    fn test_overlay_overwrite() {
        let mut overlay = StateOverlay::new();
        overlay.set(b"key1".to_vec(), b"v1".to_vec());
        overlay.set(b"key1".to_vec(), b"v2".to_vec());
        assert_eq!(overlay.get(b"key1"), OverlayResult::Found(b"v2".to_vec()));
    }

    #[test]
    fn test_overlay_drain_order() {
        let mut overlay = StateOverlay::new();
        // Insert in non-sorted order
        overlay.set(b"c".to_vec(), b"3".to_vec());
        overlay.set(b"a".to_vec(), b"1".to_vec());
        overlay.set(b"b".to_vec(), b"2".to_vec());
        overlay.delete(b"d".to_vec());

        let writes = overlay.drain();
        let keys: Vec<&Vec<u8>> = writes.keys().collect();
        assert_eq!(keys, vec![b"a", b"b", b"c", b"d"]);

        // Verify values
        assert_eq!(writes[&b"a".to_vec()], Some(b"1".to_vec()));
        assert_eq!(writes[&b"d".to_vec()], None); // deletion
    }

    #[test]
    fn test_overlay_clear() {
        let mut overlay = StateOverlay::new();
        overlay.set(b"key1".to_vec(), b"value1".to_vec());
        assert!(!overlay.is_empty());

        overlay.clear();
        assert!(overlay.is_empty());
        assert_eq!(overlay.len(), 0);
        assert_eq!(overlay.total_write_bytes(), 0);
    }

    #[test]
    fn test_overlay_total_write_bytes() {
        let mut overlay = StateOverlay::new();
        // "key1" (4) + "value1" (6) = 10
        overlay.set(b"key1".to_vec(), b"value1".to_vec());
        assert_eq!(overlay.total_write_bytes(), 10);

        // "key2" (4) + "val2" (4) = 8, total = 18
        overlay.set(b"key2".to_vec(), b"val2".to_vec());
        assert_eq!(overlay.total_write_bytes(), 18);

        // Overwrite key1: subtract old (10), add new "key1" (4) + "v" (1) = 5
        overlay.set(b"key1".to_vec(), b"v".to_vec());
        assert_eq!(overlay.total_write_bytes(), 13); // 5 + 8
    }

    #[test]
    fn test_overlay_contains_key() {
        let mut overlay = StateOverlay::new();
        overlay.set(b"key1".to_vec(), b"value1".to_vec());
        overlay.delete(b"key2".to_vec());

        assert!(overlay.contains_key(b"key1"));
        assert!(overlay.contains_key(b"key2")); // deleted keys are still "contained"
        assert!(!overlay.contains_key(b"key3"));
    }

    #[test]
    fn test_overlay_len() {
        let mut overlay = StateOverlay::new();
        assert_eq!(overlay.len(), 0);

        overlay.set(b"a".to_vec(), b"1".to_vec());
        assert_eq!(overlay.len(), 1);

        overlay.delete(b"b".to_vec());
        assert_eq!(overlay.len(), 2);

        overlay.set(b"a".to_vec(), b"2".to_vec()); // overwrite, same count
        assert_eq!(overlay.len(), 2);
    }
}
