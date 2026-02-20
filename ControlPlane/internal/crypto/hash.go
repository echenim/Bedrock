package crypto

import (
	"crypto/sha256"

	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// HashSHA256 computes the SHA-256 hash of data.
// Per SPEC.md §3, SHA-256 is used for block hashing.
func HashSHA256(data []byte) types.Hash {
	return sha256.Sum256(data)
}

// ComputeTxRoot computes the Merkle root of a list of transactions.
// If the list is empty, returns the zero hash.
func ComputeTxRoot(txs [][]byte) types.Hash {
	if len(txs) == 0 {
		return types.ZeroHash
	}
	hashes := make([]types.Hash, len(txs))
	for i, tx := range txs {
		hashes[i] = HashSHA256(tx)
	}
	return ComputeMerkleRoot(hashes)
}

// ComputeMerkleRoot computes a binary Merkle tree root from a list of hashes.
// Uses a simple iterative pairing approach. If the number of hashes at any
// level is odd, the last hash is duplicated.
func ComputeMerkleRoot(hashes []types.Hash) types.Hash {
	if len(hashes) == 0 {
		return types.ZeroHash
	}
	if len(hashes) == 1 {
		return hashes[0]
	}

	for len(hashes) > 1 {
		// If odd, duplicate the last element.
		if len(hashes)%2 != 0 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}
		next := make([]types.Hash, 0, len(hashes)/2)
		for i := 0; i < len(hashes); i += 2 {
			var combined [64]byte
			copy(combined[:32], hashes[i][:])
			copy(combined[32:], hashes[i+1][:])
			next = append(next, HashSHA256(combined[:]))
		}
		hashes = next
	}
	return hashes[0]
}
