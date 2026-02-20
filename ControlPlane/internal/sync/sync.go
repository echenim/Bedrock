package sync

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// SyncState represents the current state of the block syncer.
type SyncState int32

const (
	SyncIdle      SyncState = iota // not syncing
	SyncFastSync                   // downloading and executing blocks
	SyncStateSync                  // downloading state snapshot
	SyncCaughtUp                   // caught up, ready for consensus
)

func (s SyncState) String() string {
	switch s {
	case SyncIdle:
		return "Idle"
	case SyncFastSync:
		return "FastSync"
	case SyncStateSync:
		return "StateSync"
	case SyncCaughtUp:
		return "CaughtUp"
	default:
		return "Unknown"
	}
}

// snapshotThreshold is the block difference threshold for choosing
// snapshot sync over fast sync. Per task spec: > 100 blocks behind.
const snapshotThreshold = 100

// BlockSyncer manages block synchronization for nodes catching up.
// Per SPEC.md §12: fast sync (download and execute) or snapshot sync.
type BlockSyncer struct {
	store    storage.Store
	provider BlockProvider
	executor consensus.ExecutionAdapter
	verifier *Verifier
	valSet   *types.ValidatorSet
	logger   *zap.Logger

	state   atomic.Int32
	targetH atomic.Uint64
	localH  atomic.Uint64
}

// NewBlockSyncer creates a new block syncer.
func NewBlockSyncer(
	store storage.Store,
	provider BlockProvider,
	executor consensus.ExecutionAdapter,
	valSet *types.ValidatorSet,
	logger *zap.Logger,
) *BlockSyncer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BlockSyncer{
		store:    store,
		provider: provider,
		executor: executor,
		verifier: NewVerifier(valSet, executor),
		valSet:   valSet,
		logger:   logger,
	}
}

// Start begins the sync process.
// Per SPEC.md §12:
//  1. Request latest height from peers
//  2. If far behind: use snapshot sync
//  3. If close: use fast sync (download and verify blocks)
//  4. When caught up: transition to CaughtUp state
func (bs *BlockSyncer) Start(ctx context.Context) error {
	// Determine local height.
	localHeight, err := bs.store.GetLatestHeight()
	if err != nil {
		localHeight = 0
	}
	bs.localH.Store(localHeight)

	// Query network for latest height.
	targetHeight, err := bs.provider.GetLatestHeight(ctx)
	if err != nil {
		return fmt.Errorf("sync: get latest height: %w", err)
	}
	bs.targetH.Store(targetHeight)

	bs.logger.Info("sync starting",
		zap.Uint64("local_height", localHeight),
		zap.Uint64("target_height", targetHeight),
	)

	if localHeight >= targetHeight {
		bs.setState(SyncCaughtUp)
		bs.logger.Info("already caught up")
		return nil
	}

	gap := targetHeight - localHeight

	if gap > snapshotThreshold {
		// Snapshot sync for large gaps.
		return bs.doSnapshotSync(ctx, targetHeight)
	}

	// Fast sync for small gaps.
	return bs.doFastSync(ctx, localHeight+1, targetHeight)
}

// doFastSync downloads and executes blocks sequentially.
func (bs *BlockSyncer) doFastSync(ctx context.Context, startHeight, endHeight uint64) error {
	bs.setState(SyncFastSync)
	bs.logger.Info("fast sync starting",
		zap.Uint64("start", startHeight),
		zap.Uint64("end", endHeight),
	)

	// Get current state root.
	prevRoot, err := bs.store.GetStateRoot()
	if err != nil {
		prevRoot = types.ZeroHash
	}

	for h := startHeight; h <= endHeight; h++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch block from peer.
		block, qc, err := bs.provider.GetBlock(ctx, h)
		if err != nil {
			return fmt.Errorf("sync: fetch block %d: %w", h, err)
		}

		// Verify block structure.
		if err := bs.verifier.VerifyBlock(block, qc, h); err != nil {
			return err
		}

		// Execute block to verify state root.
		result, err := bs.executor.ExecuteBlock(block, prevRoot)
		if err != nil {
			return fmt.Errorf("sync: execute block %d: %w", h, err)
		}

		// Save block and commit.
		if err := bs.store.SaveBlock(block, qc); err != nil {
			return fmt.Errorf("sync: save block %d: %w", h, err)
		}
		if err := bs.store.SaveCommit(h, result.StateRoot); err != nil {
			return fmt.Errorf("sync: save commit %d: %w", h, err)
		}
		if err := bs.store.SetStateRoot(result.StateRoot); err != nil {
			return fmt.Errorf("sync: set state root %d: %w", h, err)
		}

		prevRoot = result.StateRoot
		bs.localH.Store(h)

		bs.logger.Debug("synced block",
			zap.Uint64("height", h),
			zap.String("state_root", result.StateRoot.String()),
		)
	}

	bs.setState(SyncCaughtUp)
	bs.logger.Info("fast sync complete",
		zap.Uint64("height", endHeight),
	)

	return nil
}

// doSnapshotSync downloads a state snapshot and applies it.
func (bs *BlockSyncer) doSnapshotSync(ctx context.Context, targetHeight uint64) error {
	bs.setState(SyncStateSync)
	bs.logger.Info("snapshot sync starting",
		zap.Uint64("target", targetHeight),
	)

	ss := NewSnapshotSyncer(bs.provider, bs.store, bs.logger)
	if err := ss.SyncToHeight(ctx, targetHeight); err != nil {
		return err
	}

	bs.localH.Store(targetHeight)
	bs.setState(SyncCaughtUp)

	return nil
}

// IsSynced returns true if the node is caught up.
func (bs *BlockSyncer) IsSynced() bool {
	return bs.State() == SyncCaughtUp
}

// State returns the current sync state.
func (bs *BlockSyncer) State() SyncState {
	return SyncState(bs.state.Load())
}

func (bs *BlockSyncer) setState(s SyncState) {
	bs.state.Store(int32(s))
}

// CurrentHeight returns the latest synced height.
func (bs *BlockSyncer) CurrentHeight() uint64 {
	return bs.localH.Load()
}

// TargetHeight returns the target height being synced to.
func (bs *BlockSyncer) TargetHeight() uint64 {
	return bs.targetH.Load()
}
