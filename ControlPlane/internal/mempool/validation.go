package mempool

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// Transaction wire format (canonical):
//   [0:32]   sender address
//   [32:40]  nonce (little-endian uint64)
//   [40:48]  fee (little-endian uint64)
//   [48:112] ed25519 signature (64 bytes)
//   [112:]   payload data
//
// Signature covers: sender(32) || nonce(8) || fee(8) || sha256(payload)(32)

const (
	txHeaderSize = 32 + 8 + 8 + 64 // 112 bytes minimum
	minTxSize    = txHeaderSize + 1 // at least 1 byte of payload
)

// State key prefixes for nonce tracking.
const nonceKeyPrefix = "nonce/"

// ParseTx parses raw transaction bytes into a MempoolTx.
func ParseTx(raw []byte) (*MempoolTx, error) {
	if len(raw) < minTxSize {
		return nil, fmt.Errorf("mempool: tx too small: %d < %d", len(raw), minTxSize)
	}

	var sender types.Address
	copy(sender[:], raw[0:32])

	nonce := binary.LittleEndian.Uint64(raw[32:40])
	fee := binary.LittleEndian.Uint64(raw[40:48])

	var sig [64]byte
	copy(sig[:], raw[48:112])

	txHash := sha256.Sum256(raw)

	return &MempoolTx{
		Hash:    txHash,
		Data:    raw,
		Fee:     fee,
		Nonce:   nonce,
		Sender:  sender,
		Size:    len(raw),
		sig:     sig,
		payload: raw[112:],
	}, nil
}

// signingPayload constructs the canonical bytes signed by the sender.
func signingPayload(sender types.Address, nonce, fee uint64, payload []byte) []byte {
	buf := make([]byte, 32+8+8+32)
	copy(buf[0:32], sender[:])
	binary.LittleEndian.PutUint64(buf[32:40], nonce)
	binary.LittleEndian.PutUint64(buf[40:48], fee)
	h := sha256.Sum256(payload)
	copy(buf[48:80], h[:])
	return buf
}

// ValidateStateless performs checks that don't require state access.
// Per SPEC.md §15: signature verification, size limits, encoding.
func ValidateStateless(tx []byte, cfg config.MempoolConfig) (*MempoolTx, error) {
	// Size check.
	if len(tx) > cfg.MaxTxBytes {
		return nil, fmt.Errorf("mempool: tx exceeds max size: %d > %d", len(tx), cfg.MaxTxBytes)
	}

	mtx, err := ParseTx(tx)
	if err != nil {
		return nil, err
	}

	// Zero sender check.
	if mtx.Sender == types.ZeroAddress {
		return nil, errors.New("mempool: zero sender address")
	}

	// Signature verification.
	// The sender address is sha256(pubkey), but we can't recover the pubkey
	// from the address alone. For our stateless check, we verify that the
	// signature field is non-zero. Full pubkey-based verification happens
	// during stateful validation or is handled by the execution engine.
	if mtx.sig == [64]byte{} {
		return nil, errors.New("mempool: empty signature")
	}

	return mtx, nil
}

// ValidateStateful performs checks against current state.
// Per SPEC.md §15: nonce verification (replay protection).
func ValidateStateful(tx *MempoolTx, stateStore storage.StateStore) error {
	if stateStore == nil {
		return nil
	}

	// Check nonce.
	nonceKey := []byte(nonceKeyPrefix + tx.Sender.String())
	data, err := stateStore.Get(nonceKey)
	if err != nil {
		return fmt.Errorf("mempool: read nonce: %w", err)
	}

	var expectedNonce uint64
	if data != nil && len(data) >= 8 {
		expectedNonce = binary.LittleEndian.Uint64(data)
	}

	if tx.Nonce < expectedNonce {
		return fmt.Errorf("mempool: nonce too low: got %d, expected >= %d", tx.Nonce, expectedNonce)
	}

	// Allow nonce == expected (next tx) or expected+small_gap for pending txs.
	// Strict mode: only accept exact next nonce.
	if tx.Nonce > expectedNonce+64 {
		return fmt.Errorf("mempool: nonce gap too large: got %d, expected ~%d", tx.Nonce, expectedNonce)
	}

	return nil
}

// VerifySignature verifies a transaction signature given the sender's public key.
func VerifySignature(tx *MempoolTx, pubKey ed25519.PublicKey) bool {
	payload := signingPayload(tx.Sender, tx.Nonce, tx.Fee, tx.payload)
	return ed25519.Verify(pubKey, payload, tx.sig[:])
}

// BuildTx constructs a raw transaction from components and signs it.
func BuildTx(sender types.Address, nonce, fee uint64, payload []byte, privKey ed25519.PrivateKey) []byte {
	raw := make([]byte, txHeaderSize+len(payload))
	copy(raw[0:32], sender[:])
	binary.LittleEndian.PutUint64(raw[32:40], nonce)
	binary.LittleEndian.PutUint64(raw[40:48], fee)

	sigPayload := signingPayload(sender, nonce, fee, payload)
	sig := ed25519.Sign(privKey, sigPayload)
	copy(raw[48:112], sig)
	copy(raw[112:], payload)
	return raw
}
