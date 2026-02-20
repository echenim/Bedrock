package types

import (
	"encoding/hex"
	"fmt"
)

// HashSize is the length of a Hash in bytes (SHA-256).
const HashSize = 32

// AddressSize is the length of an Address in bytes.
const AddressSize = 32

// Hash is a 32-byte SHA-256 hash.
type Hash [HashSize]byte

// Address is a 32-byte validator/account identifier.
type Address [AddressSize]byte

// ZeroHash is the zero-value hash.
var ZeroHash Hash

// ZeroAddress is the zero-value address.
var ZeroAddress Address

// Bytes returns the hash as a byte slice.
func (h Hash) Bytes() []byte { return h[:] }

// IsZero returns true if the hash is all zeros.
func (h Hash) IsZero() bool { return h == ZeroHash }

// String returns the hex-encoded hash.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// HashFromBytes creates a Hash from a byte slice, returning an error if
// the slice is not exactly 32 bytes.
func HashFromBytes(b []byte) (Hash, error) {
	if len(b) != HashSize {
		return ZeroHash, fmt.Errorf("invalid hash length: got %d, want %d", len(b), HashSize)
	}
	var h Hash
	copy(h[:], b)
	return h, nil
}

// HashFromHex decodes a hex string into a Hash.
func HashFromHex(s string) (Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return ZeroHash, fmt.Errorf("invalid hex: %w", err)
	}
	return HashFromBytes(b)
}

// Bytes returns the address as a byte slice.
func (a Address) Bytes() []byte { return a[:] }

// IsZero returns true if the address is all zeros.
func (a Address) IsZero() bool { return a == ZeroAddress }

// String returns the hex-encoded address.
func (a Address) String() string { return hex.EncodeToString(a[:]) }

// AddressFromBytes creates an Address from a byte slice, returning an error if
// the slice is not exactly 32 bytes.
func AddressFromBytes(b []byte) (Address, error) {
	if len(b) != AddressSize {
		return ZeroAddress, fmt.Errorf("invalid address length: got %d, want %d", len(b), AddressSize)
	}
	var a Address
	copy(a[:], b)
	return a, nil
}

// AddressFromHex decodes a hex string into an Address.
func AddressFromHex(s string) (Address, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return ZeroAddress, fmt.Errorf("invalid hex: %w", err)
	}
	return AddressFromBytes(b)
}
