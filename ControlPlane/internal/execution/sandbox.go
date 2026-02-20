package execution

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// Sandbox wraps WASM execution. When a real WASM artifact is available,
// this uses wasmtime-go. Otherwise, it falls back to a deterministic
// Go-native executor that computes state roots from transactions.
type Sandbox struct {
	cfg      config.ExecutionConfig
	wasmCode []byte // loaded WASM bytes, nil if no artifact available
}

// NewSandbox creates a new execution sandbox.
// If the WASM artifact exists, it loads it for future execution.
// If not, it operates in native mode using a deterministic Go executor.
func NewSandbox(cfg config.ExecutionConfig) (*Sandbox, error) {
	s := &Sandbox{cfg: cfg}

	// Try to load WASM artifact.
	if cfg.WASMPath != "" {
		data, err := os.ReadFile(cfg.WASMPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("execution: read wasm: %w", err)
			}
			// WASM file not found — will use native executor.
		} else {
			s.wasmCode = data
		}
	}

	return s, nil
}

// Execute runs block execution in the sandbox.
// If a WASM artifact is loaded, it uses wasmtime-go.
// Otherwise, it uses the deterministic native executor.
func (s *Sandbox) Execute(block *types.Block, prevStateRoot types.Hash, stateStore storage.StateStore) (*consensus.ExecutionResult, error) {
	if s.wasmCode != nil {
		return s.executeWASM(block, prevStateRoot, stateStore)
	}
	return s.executeNative(block, prevStateRoot, stateStore)
}

// executeWASM runs execution through the WASM sandbox.
// This requires the wasmtime-go bindings and a compiled WASM artifact.
func (s *Sandbox) executeWASM(block *types.Block, prevStateRoot types.Hash, stateStore storage.StateStore) (*consensus.ExecutionResult, error) {
	// WASM execution via wasmtime-go.
	// The wasmtime-go dependency is already in go.mod.
	// Full implementation requires:
	//   1. Create wasmtime.Engine with fuel metering
	//   2. Compile module from s.wasmCode
	//   3. Create Store with fuel limits (s.cfg.FuelLimit)
	//   4. Create Linker and register host functions
	//   5. Instantiate module
	//   6. Call bedrock_init export
	//   7. Write ExecutionRequest to guest memory
	//   8. Call bedrock_execute_block export
	//   9. Read ExecutionResponse from guest memory
	//   10. Call bedrock_free export
	//
	// For now, return an error directing to native mode until the WASM
	// artifact (from ExecutionCore build) is available.
	return nil, errors.New("execution: WASM execution not yet implemented — use native executor or provide mock")
}

// executeNative is a deterministic Go-native executor.
// It computes state transitions and a new state root without WASM.
//
// State root computation:
//  1. Apply each transaction to state (key = sha256(tx), value = tx)
//  2. Compute new state root from the ordered state entries
//
// This is deterministic: same (prevStateRoot, block) → same result.
func (s *Sandbox) executeNative(block *types.Block, prevStateRoot types.Hash, stateStore storage.StateStore) (*consensus.ExecutionResult, error) {
	var gasUsed uint64
	writes := make(map[string][]byte)

	for _, tx := range block.Transactions {
		// Per-tx gas: 1000 base + 1 per byte.
		txGas := uint64(1000) + uint64(len(tx))
		gasUsed += txGas

		if s.cfg.GasLimit > 0 && gasUsed > s.cfg.GasLimit {
			return nil, fmt.Errorf("execution: gas limit exceeded: %d > %d", gasUsed, s.cfg.GasLimit)
		}

		// Apply tx to state: key = sha256(tx_data), value = tx_data.
		txKey := sha256.Sum256(tx)
		writes[string(txKey[:])] = tx
	}

	// Apply writes to state store.
	if stateStore != nil && len(writes) > 0 {
		if err := stateStore.ApplyWriteSet(writes); err != nil {
			return nil, fmt.Errorf("execution: apply writes: %w", err)
		}
	}

	// Compute new state root deterministically.
	newRoot := computeStateRoot(prevStateRoot, block.Transactions)

	// Persist state root.
	if stateStore != nil {
		if err := stateStore.SetStateRoot(newRoot); err != nil {
			return nil, fmt.Errorf("execution: set state root: %w", err)
		}
	}

	return &consensus.ExecutionResult{
		StateRoot: newRoot,
		GasUsed:   gasUsed,
	}, nil
}

// computeStateRoot computes a deterministic state root from the previous root
// and the list of transactions. This is a simplified version that
// hashes: prevStateRoot || sorted(sha256(tx_i)).
func computeStateRoot(prevRoot types.Hash, txs [][]byte) types.Hash {
	if len(txs) == 0 {
		return prevRoot
	}

	// Collect sorted tx hashes for determinism.
	txHashes := make([][32]byte, len(txs))
	for i, tx := range txs {
		txHashes[i] = sha256.Sum256(tx)
	}
	sort.Slice(txHashes, func(i, j int) bool {
		for k := range 32 {
			if txHashes[i][k] != txHashes[j][k] {
				return txHashes[i][k] < txHashes[j][k]
			}
		}
		return false
	})

	// Hash: prevRoot(32) || numTxs(8) || txHash_0(32) || txHash_1(32) || ...
	buf := make([]byte, 32+8+32*len(txHashes))
	copy(buf[0:32], prevRoot[:])
	binary.BigEndian.PutUint64(buf[32:40], uint64(len(txHashes)))
	for i, h := range txHashes {
		copy(buf[40+32*i:40+32*(i+1)], h[:])
	}

	return sha256.Sum256(buf)
}

// Close releases sandbox resources.
func (s *Sandbox) Close() error {
	s.wasmCode = nil
	return nil
}
