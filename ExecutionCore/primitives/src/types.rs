//! Core type aliases and constants for the BedRock execution layer.
//!
//! These types are used across the execution engine, WASM guest, and sandbox host.

use alloc::vec::Vec;

/// 32-byte hash used for block hashes, state roots, and transaction hashes.
pub type Hash = [u8; 32];

/// 32-byte address identifying validators and accounts.
pub type Address = [u8; 32];

/// Block height (monotonically increasing).
pub type BlockHeight = u64;

/// Round within a consensus height.
pub type Round = u64;

/// Current execution API version (EXECUTION_SPEC.md §10.1).
pub const API_VERSION: u32 = 1;

/// Maximum key length in the state key-value store (EXECUTION_SPEC.md §6.1).
pub const MAX_KEY_LEN: usize = 256;

/// Maximum value length in the state key-value store (EXECUTION_SPEC.md §6.1).
pub const MAX_VALUE_LEN: usize = 65_536; // 64 KiB

/// A zero-valued hash (32 zero bytes).
pub const ZERO_HASH: Hash = [0u8; 32];

/// A zero-valued address (32 zero bytes).
pub const ZERO_ADDRESS: Address = [0u8; 32];

/// Convert a `Hash` to a hex string for display purposes.
pub fn hash_to_hex(hash: &Hash) -> alloc::string::String {
    let mut s = alloc::string::String::with_capacity(66);
    s.push_str("0x");
    for byte in hash {
        use core::fmt::Write;
        let _ = write!(s, "{:02x}", byte);
    }
    s
}

/// Encode a u64 as little-endian bytes (EXECUTION_SPEC.md §5.2).
pub fn u64_to_le_bytes(v: u64) -> [u8; 8] {
    v.to_le_bytes()
}

/// Decode a u64 from little-endian bytes (EXECUTION_SPEC.md §5.2).
pub fn u64_from_le_bytes(bytes: &[u8]) -> Option<u64> {
    if bytes.len() < 8 {
        return None;
    }
    let mut buf = [0u8; 8];
    buf.copy_from_slice(&bytes[..8]);
    Some(u64::from_le_bytes(buf))
}

/// Encode a u32 as little-endian bytes.
pub fn u32_to_le_bytes(v: u32) -> [u8; 4] {
    v.to_le_bytes()
}

/// Decode a u32 from little-endian bytes.
pub fn u32_from_le_bytes(bytes: &[u8]) -> Option<u32> {
    if bytes.len() < 4 {
        return None;
    }
    let mut buf = [0u8; 4];
    buf.copy_from_slice(&bytes[..4]);
    Some(u32::from_le_bytes(buf))
}

/// Concatenate byte slices into a single Vec.
pub fn concat_bytes(slices: &[&[u8]]) -> Vec<u8> {
    let total: usize = slices.iter().map(|s| s.len()).sum();
    let mut out = Vec::with_capacity(total);
    for s in slices {
        out.extend_from_slice(s);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hash_to_hex() {
        let hash = [0xab; 32];
        let hex = hash_to_hex(&hash);
        assert_eq!(hex.len(), 66); // "0x" + 64 hex chars
        assert!(hex.starts_with("0x"));
        assert!(hex[2..].chars().all(|c| c == 'a' || c == 'b'));
    }

    #[test]
    fn test_u64_le_roundtrip() {
        let val = 0xDEAD_BEEF_CAFE_BABE_u64;
        let bytes = u64_to_le_bytes(val);
        assert_eq!(u64_from_le_bytes(&bytes), Some(val));
    }

    #[test]
    fn test_u32_le_roundtrip() {
        let val = 0xDEAD_BEEF_u32;
        let bytes = u32_to_le_bytes(val);
        assert_eq!(u32_from_le_bytes(&bytes), Some(val));
    }

    #[test]
    fn test_u64_from_short_slice() {
        assert_eq!(u64_from_le_bytes(&[0, 1, 2]), None);
    }

    #[test]
    fn test_concat_bytes() {
        let result = concat_bytes(&[b"hello", b" ", b"world"]);
        assert_eq!(result, b"hello world");
    }

    #[test]
    fn test_zero_constants() {
        assert_eq!(ZERO_HASH, [0u8; 32]);
        assert_eq!(ZERO_ADDRESS, [0u8; 32]);
    }

    #[test]
    fn test_api_version() {
        assert_eq!(API_VERSION, 1);
    }
}
