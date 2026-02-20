package crypto_test

import (
	"bytes"
	"testing"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

func TestGenerateKeypairAndSignVerify(t *testing.T) {
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	msg := []byte("hello bedrock")
	sig := crypto.Sign(priv, msg)

	if !crypto.Verify(pub, msg, sig) {
		t.Fatal("Verify failed for valid signature")
	}
}

func TestVerifyRejectsInvalidSignature(t *testing.T) {
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	msg := []byte("hello bedrock")
	sig := crypto.Sign(priv, msg)

	// Flip a byte in the signature.
	badSig := make([]byte, len(sig))
	copy(badSig, sig)
	badSig[0] ^= 0xff

	if crypto.Verify(pub, msg, badSig) {
		t.Fatal("Verify should reject corrupted signature")
	}

	// Wrong message.
	if crypto.Verify(pub, []byte("wrong message"), sig) {
		t.Fatal("Verify should reject wrong message")
	}

	// Wrong key.
	pub2, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	if crypto.Verify(pub2, msg, sig) {
		t.Fatal("Verify should reject wrong public key")
	}
}

func TestVerifyRejectsInvalidInputs(t *testing.T) {
	if crypto.Verify(nil, []byte("msg"), make([]byte, 64)) {
		t.Fatal("should reject nil public key")
	}
	if crypto.Verify(make([]byte, 32), []byte("msg"), nil) {
		t.Fatal("should reject nil signature")
	}
	if crypto.Verify(make([]byte, 32), []byte("msg"), make([]byte, 63)) {
		t.Fatal("should reject short signature")
	}
}

func TestAddressFromPubKey(t *testing.T) {
	pub, _, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	addr := crypto.AddressFromPubKey(pub)
	if addr.IsZero() {
		t.Fatal("address should not be zero")
	}

	// Same key -> same address.
	addr2 := crypto.AddressFromPubKey(pub)
	if addr != addr2 {
		t.Fatal("same public key should produce same address")
	}
}

func TestHashSHA256Deterministic(t *testing.T) {
	data := []byte("deterministic hashing test")
	h1 := crypto.HashSHA256(data)
	h2 := crypto.HashSHA256(data)
	if h1 != h2 {
		t.Fatal("SHA-256 should be deterministic")
	}
	if h1.IsZero() {
		t.Fatal("SHA-256 of non-empty data should not be zero")
	}
}

func TestHashSHA256EmptyInput(t *testing.T) {
	h := crypto.HashSHA256([]byte{})
	if h.IsZero() {
		t.Fatal("SHA-256 of empty data should not be zero hash")
	}
}

func TestComputeTxRootEmpty(t *testing.T) {
	root := crypto.ComputeTxRoot(nil)
	if root != types.ZeroHash {
		t.Fatal("tx root of empty list should be zero hash")
	}
}

func TestComputeTxRootSingle(t *testing.T) {
	root := crypto.ComputeTxRoot([][]byte{[]byte("tx1")})
	expected := crypto.HashSHA256([]byte("tx1"))
	if root != expected {
		t.Fatalf("single tx root mismatch: got %s, want %s", root, expected)
	}
}

func TestComputeTxRootDeterministic(t *testing.T) {
	txs := [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3")}
	r1 := crypto.ComputeTxRoot(txs)
	r2 := crypto.ComputeTxRoot(txs)
	if r1 != r2 {
		t.Fatal("tx root should be deterministic")
	}
	if r1.IsZero() {
		t.Fatal("tx root of non-empty list should not be zero")
	}
}

func TestComputeMerkleRootPowerOfTwo(t *testing.T) {
	hashes := []types.Hash{
		crypto.HashSHA256([]byte("a")),
		crypto.HashSHA256([]byte("b")),
		crypto.HashSHA256([]byte("c")),
		crypto.HashSHA256([]byte("d")),
	}
	root := crypto.ComputeMerkleRoot(hashes)
	if root.IsZero() {
		t.Fatal("merkle root should not be zero")
	}

	// Different order -> different root.
	swapped := []types.Hash{hashes[1], hashes[0], hashes[2], hashes[3]}
	root2 := crypto.ComputeMerkleRoot(swapped)
	if root == root2 {
		t.Fatal("swapping inputs should change merkle root")
	}
}

func TestPubKeyTo32AndSigTo64(t *testing.T) {
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}

	pk32 := crypto.PubKeyTo32(pub)
	if !bytes.Equal(pk32[:], pub) {
		t.Fatal("PubKeyTo32 mismatch")
	}

	sig := crypto.Sign(priv, []byte("test"))
	sig64 := crypto.SigTo64(sig)
	if !bytes.Equal(sig64[:], sig) {
		t.Fatal("SigTo64 mismatch")
	}
}
