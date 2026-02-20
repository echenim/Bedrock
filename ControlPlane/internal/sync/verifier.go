package sync

import (
	"errors"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// Verifier validates blocks and state roots during sync.
type Verifier struct {
	valSet   *types.ValidatorSet
	executor consensus.ExecutionAdapter
}

// NewVerifier creates a block/state verifier.
func NewVerifier(valSet *types.ValidatorSet, executor consensus.ExecutionAdapter) *Verifier {
	return &Verifier{
		valSet:   valSet,
		executor: executor,
	}
}

// VerifyBlock validates a synced block:
//  1. Structural validity (Block.Validate)
//  2. QC signature validity (if QC provided)
//  3. Height consistency (must be sequential)
func (v *Verifier) VerifyBlock(block *types.Block, qc *types.QuorumCertificate, expectedHeight uint64) error {
	if block == nil {
		return errors.New("sync: nil block")
	}

	if block.Header.Height != expectedHeight {
		return fmt.Errorf("sync: height mismatch: got %d, want %d",
			block.Header.Height, expectedHeight)
	}

	if err := block.Validate(); err != nil {
		return fmt.Errorf("sync: invalid block at height %d: %w",
			block.Header.Height, err)
	}

	// Verify QC if provided.
	if qc != nil && v.valSet != nil {
		if err := qc.Verify(v.valSet); err != nil {
			return fmt.Errorf("sync: invalid QC at height %d: %w",
				block.Header.Height, err)
		}
	}

	return nil
}

// VerifyAndExecuteBlock validates the block and executes it to verify
// the state root matches the committed state root.
func (v *Verifier) VerifyAndExecuteBlock(
	block *types.Block,
	qc *types.QuorumCertificate,
	prevStateRoot types.Hash,
	committedRoot types.Hash,
) (*consensus.ExecutionResult, error) {
	// Structural + QC verification.
	if err := v.VerifyBlock(block, qc, block.Header.Height); err != nil {
		return nil, err
	}

	if v.executor == nil {
		return nil, errors.New("sync: no executor configured")
	}

	// Execute the block.
	result, err := v.executor.ExecuteBlock(block, prevStateRoot)
	if err != nil {
		return nil, fmt.Errorf("sync: execute block %d: %w", block.Header.Height, err)
	}

	// Verify state root matches the committed root.
	if committedRoot != types.ZeroHash && result.StateRoot != committedRoot {
		return nil, fmt.Errorf("sync: state root mismatch at height %d: got %s, want %s",
			block.Header.Height, result.StateRoot, committedRoot)
	}

	return result, nil
}

// VerifySnapshot validates a downloaded snapshot's state root against
// the committed state root at the given height.
// Per SPEC-v0.2.md §10: snapshot state_root must match committed state_root.
func VerifySnapshot(
	committedRoot types.Hash,
	snapshotRoot types.Hash,
	store storage.StateStore,
) error {
	if committedRoot == types.ZeroHash {
		return errors.New("sync: no committed root to verify against")
	}

	if snapshotRoot != committedRoot {
		return fmt.Errorf("sync: snapshot root mismatch: got %s, want %s",
			snapshotRoot, committedRoot)
	}

	return nil
}
