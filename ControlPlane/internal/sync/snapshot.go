package sync

import (
	"context"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// SnapshotSyncer handles snapshot-based state synchronization.
// Per SPEC-v0.2.md §10: snapshot sync for nodes far behind.
type SnapshotSyncer struct {
	provider BlockProvider
	store    storage.Store
	logger   *zap.Logger
}

// NewSnapshotSyncer creates a snapshot syncer.
func NewSnapshotSyncer(provider BlockProvider, store storage.Store, logger *zap.Logger) *SnapshotSyncer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SnapshotSyncer{
		provider: provider,
		store:    store,
		logger:   logger,
	}
}

// SyncToHeight downloads and applies a state snapshot at the given height.
// Per SPEC-v0.2.md §10:
//   - Snapshot state_root must match committed state_root
//   - State is verified via the committed root in the QC
func (ss *SnapshotSyncer) SyncToHeight(ctx context.Context, targetHeight uint64) error {
	ss.logger.Info("starting snapshot sync",
		zap.Uint64("target_height", targetHeight),
	)

	// Get snapshot from peers.
	snapshotRoot, stateData, err := ss.provider.GetStateSnapshot(ctx, targetHeight)
	if err != nil {
		return fmt.Errorf("sync: get snapshot at height %d: %w", targetHeight, err)
	}

	// Get the committed state root to verify against.
	committedRoot, err := ss.store.GetCommitStateRoot(targetHeight)
	if err != nil {
		// If we don't have the committed root yet, fetch the block.
		block, qc, fetchErr := ss.provider.GetBlock(ctx, targetHeight)
		if fetchErr != nil {
			return fmt.Errorf("sync: fetch block %d for verification: %w", targetHeight, fetchErr)
		}
		if err := ss.store.SaveBlock(block, qc); err != nil {
			return fmt.Errorf("sync: save block %d: %w", targetHeight, err)
		}
		// Use the block's state root.
		committedRoot = block.Header.StateRoot
	}

	// Verify snapshot root matches committed root.
	if err := VerifySnapshot(committedRoot, snapshotRoot, ss.store); err != nil {
		return err
	}

	// Apply state data to store.
	if err := ss.store.ApplyWriteSet(stateData); err != nil {
		return fmt.Errorf("sync: apply snapshot state: %w", err)
	}

	// Set the verified state root.
	if err := ss.store.SetStateRoot(snapshotRoot); err != nil {
		return fmt.Errorf("sync: set state root: %w", err)
	}

	// Record commit.
	if err := ss.store.SaveCommit(targetHeight, snapshotRoot); err != nil {
		return fmt.Errorf("sync: save commit: %w", err)
	}

	ss.logger.Info("snapshot sync complete",
		zap.Uint64("height", targetHeight),
		zap.String("state_root", snapshotRoot.String()),
	)

	return nil
}

// VerifyAndApplySnapshot verifies a snapshot against a known state root
// and applies it to the store.
func VerifyAndApplySnapshot(
	snapshotRoot types.Hash,
	committedRoot types.Hash,
	stateData map[string][]byte,
	store storage.Store,
) error {
	if err := VerifySnapshot(committedRoot, snapshotRoot, store); err != nil {
		return err
	}

	if err := store.ApplyWriteSet(stateData); err != nil {
		return fmt.Errorf("sync: apply snapshot: %w", err)
	}

	if err := store.SetStateRoot(snapshotRoot); err != nil {
		return fmt.Errorf("sync: set root: %w", err)
	}

	return nil
}
