package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// PrivateKey is an Ed25519 private key (64 bytes).
type PrivateKey = ed25519.PrivateKey

// PublicKey is an Ed25519 public key (32 bytes).
type PublicKey = ed25519.PublicKey

// GenerateKeypair creates a new Ed25519 key pair.
func GenerateKeypair() (PublicKey, PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate keypair: %w", err)
	}
	return pub, priv, nil
}

// Sign signs a message with an Ed25519 private key.
func Sign(privKey PrivateKey, message []byte) []byte {
	return ed25519.Sign(privKey, message)
}

// Verify checks an Ed25519 signature against a public key and message.
func Verify(pubKey PublicKey, message, signature []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pubKey, message, signature)
}

// AddressFromPubKey derives a 32-byte address from a public key using SHA-256.
func AddressFromPubKey(pubKey PublicKey) types.Address {
	h := sha256.Sum256(pubKey)
	var addr types.Address
	copy(addr[:], h[:])
	return addr
}

// PubKeyTo32 converts a PublicKey to a [32]byte array.
func PubKeyTo32(pubKey PublicKey) [32]byte {
	var out [32]byte
	copy(out[:], pubKey)
	return out
}

// SigTo64 converts a signature slice to a [64]byte array.
func SigTo64(sig []byte) [64]byte {
	var out [64]byte
	copy(out[:], sig)
	return out
}
