//! In-memory state store for testing.
//!
//! `MemStore` implements `StateStore` using a `BTreeMap` for deterministic
//! key ordering. Useful for unit tests and integration tests where a real
//! storage backend is not needed.

use std::collections::BTreeMap;
use crate::error::HostError;
use crate::state_store::StateStore;

/// In-memory state store backed by `BTreeMap`.
///
/// BTreeMap is used instead of HashMap for deterministic iteration order,
/// consistent with the BedRock determinism requirements.
#[derive(Debug, Clone, Default)]
pub struct MemStore {
    data: BTreeMap<Vec<u8>, Vec<u8>>,
}

impl MemStore {
    /// Create a new empty store.
    pub fn new() -> Self {
        Self {
            data: BTreeMap::new(),
        }
    }

    /// Create a store pre-populated with data.
    pub fn with_data(data: BTreeMap<Vec<u8>, Vec<u8>>) -> Self {
        Self { data }
    }

    /// Insert a key-value pair into the store.
    pub fn insert(&mut self, key: Vec<u8>, value: Vec<u8>) {
        self.data.insert(key, value);
    }

    /// Remove a key from the store.
    pub fn remove(&mut self, key: &[u8]) {
        self.data.remove(key);
    }

    /// Returns the number of entries in the store.
    pub fn len(&self) -> usize {
        self.data.len()
    }

    /// Returns true if the store is empty.
    pub fn is_empty(&self) -> bool {
        self.data.is_empty()
    }
}

impl StateStore for MemStore {
    fn get(&self, key: &[u8]) -> Result<Option<Vec<u8>>, HostError> {
        Ok(self.data.get(key).cloned())
    }

    fn contains(&self, key: &[u8]) -> Result<bool, HostError> {
        Ok(self.data.contains_key(key))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_store() {
        let store = MemStore::new();
        assert!(store.is_empty());
        assert_eq!(store.len(), 0);
        assert_eq!(store.get(b"missing").unwrap(), None);
        assert!(!store.contains(b"missing").unwrap());
    }

    #[test]
    fn test_insert_and_get() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"value1".to_vec());

        assert_eq!(store.get(b"key1").unwrap(), Some(b"value1".to_vec()));
        assert!(store.contains(b"key1").unwrap());
        assert_eq!(store.len(), 1);
    }

    #[test]
    fn test_missing_key_returns_none() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"value1".to_vec());

        assert_eq!(store.get(b"key2").unwrap(), None);
        assert!(!store.contains(b"key2").unwrap());
    }

    #[test]
    fn test_overwrite() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"v1".to_vec());
        store.insert(b"key1".to_vec(), b"v2".to_vec());

        assert_eq!(store.get(b"key1").unwrap(), Some(b"v2".to_vec()));
        assert_eq!(store.len(), 1);
    }

    #[test]
    fn test_remove() {
        let mut store = MemStore::new();
        store.insert(b"key1".to_vec(), b"value1".to_vec());
        store.remove(b"key1");

        assert_eq!(store.get(b"key1").unwrap(), None);
        assert!(store.is_empty());
    }

    #[test]
    fn test_with_data() {
        let mut data = BTreeMap::new();
        data.insert(b"a".to_vec(), b"1".to_vec());
        data.insert(b"b".to_vec(), b"2".to_vec());

        let store = MemStore::with_data(data);
        assert_eq!(store.len(), 2);
        assert_eq!(store.get(b"a").unwrap(), Some(b"1".to_vec()));
        assert_eq!(store.get(b"b").unwrap(), Some(b"2".to_vec()));
    }

    #[test]
    fn test_empty_key_and_value() {
        let mut store = MemStore::new();
        store.insert(vec![], b"empty_key".to_vec());
        store.insert(b"empty_val".to_vec(), vec![]);

        assert_eq!(store.get(b"").unwrap(), Some(b"empty_key".to_vec()));
        assert_eq!(store.get(b"empty_val").unwrap(), Some(vec![]));
    }
}
