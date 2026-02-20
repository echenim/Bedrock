//! Cryptographic operations for the BedRock execution layer.
//!
//! SPEC.md §3 defines the cryptographic primitives:
//! - Ed25519 for validator identity signatures
//! - SHA-256 for block hashing
//! - BLAKE3 for state commitments and general hashing
//!
//! EXECUTION_SPEC.md §8.3 defines the host API crypto functions.
//! All crypto operations must be deterministic with no randomization.

use crate::types::Hash;

/// Compute BLAKE3 hash of the input data.
///
/// Used for state commitments, Merkle tree nodes, and general-purpose
/// hashing within the execution engine.
pub fn hash_blake3(data: &[u8]) -> Hash {
    *blake3::hash(data).as_bytes()
}

/// Compute SHA-256 hash of the input data.
///
/// Used for block header hashing per SPEC.md §3.
pub fn hash_sha256(data: &[u8]) -> Hash {
    use sha2::Digest;
    let result = sha2::Sha256::digest(data);
    let mut hash = [0u8; 32];
    hash.copy_from_slice(&result);
    hash
}

/// Verify an Ed25519 signature.
///
/// Per EXECUTION_SPEC.md §8.3: verification must be deterministic,
/// no randomization.
///
/// Returns `true` if the signature is valid for the given message and
/// public key, `false` otherwise.
pub fn verify_ed25519(message: &[u8], signature: &[u8; 64], public_key: &[u8; 32]) -> bool {
    use ed25519_dalek::{Signature, VerifyingKey, Verifier};

    let Ok(verifying_key) = VerifyingKey::from_bytes(public_key) else {
        return false;
    };
    let sig = Signature::from_bytes(signature);
    verifying_key.verify(message, &sig).is_ok()
}

/// Sign a message with an Ed25519 private key.
///
/// Used in tests and by the control plane for validator signing.
/// Not available in `no_std` WASM guest (signing happens on the host side).
#[cfg(feature = "std")]
pub fn sign_ed25519(message: &[u8], secret_key: &ed25519_dalek::SigningKey) -> [u8; 64] {
    use ed25519_dalek::Signer;
    let sig = secret_key.sign(message);
    sig.to_bytes()
}

/// Generate an Ed25519 keypair for testing.
///
/// Uses OS randomness — only available with `std` feature.
/// NEVER used inside the execution engine (determinism violation).
#[cfg(feature = "std")]
pub fn generate_keypair() -> (ed25519_dalek::VerifyingKey, ed25519_dalek::SigningKey) {
    use ed25519_dalek::SigningKey;
    let mut rng = rand::rngs::OsRng;
    let signing_key = SigningKey::generate(&mut rng);
    let verifying_key = signing_key.verifying_key();
    (verifying_key, signing_key)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_blake3_deterministic() {
        let data = b"hello bedrock";
        let h1 = hash_blake3(data);
        let h2 = hash_blake3(data);
        assert_eq!(h1, h2);
        // Different input produces different hash
        let h3 = hash_blake3(b"hello bedrock!");
        assert_ne!(h1, h3);
    }

    #[test]
    fn test_blake3_empty() {
        let h = hash_blake3(b"");
        // Should produce a valid 32-byte hash
        assert_ne!(h, [0u8; 32]);
    }

    #[test]
    fn test_sha256_deterministic() {
        let data = b"hello bedrock";
        let h1 = hash_sha256(data);
        let h2 = hash_sha256(data);
        assert_eq!(h1, h2);
    }

    #[test]
    fn test_sha256_known_vector() {
        // SHA-256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
        let h = hash_sha256(b"");
        assert_eq!(h[0], 0xe3);
        assert_eq!(h[1], 0xb0);
        assert_eq!(h[31], 0x55);
    }

    #[test]
    fn test_ed25519_sign_verify_roundtrip() {
        let (verifying_key, signing_key) = generate_keypair();
        let message = b"bedrock block proposal";

        let signature = sign_ed25519(message, &signing_key);
        assert!(verify_ed25519(
            message,
            &signature,
            verifying_key.as_bytes()
        ));
    }

    #[test]
    fn test_ed25519_reject_wrong_message() {
        let (verifying_key, signing_key) = generate_keypair();
        let signature = sign_ed25519(b"correct message", &signing_key);
        assert!(!verify_ed25519(
            b"wrong message",
            &signature,
            verifying_key.as_bytes()
        ));
    }

    #[test]
    fn test_ed25519_reject_wrong_key() {
        let (_vk1, signing_key) = generate_keypair();
        let (vk2, _sk2) = generate_keypair();
        let message = b"test message";
        let signature = sign_ed25519(message, &signing_key);
        assert!(!verify_ed25519(message, &signature, vk2.as_bytes()));
    }

    #[test]
    fn test_ed25519_reject_invalid_public_key() {
        // All zeros is not a valid Ed25519 public key
        let invalid_pk = [0u8; 32];
        let sig = [0u8; 64];
        assert!(!verify_ed25519(b"test", &sig, &invalid_pk));
    }
}
