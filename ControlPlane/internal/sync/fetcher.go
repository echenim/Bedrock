package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// BlockProvider abstracts block retrieval from peers.
// This allows sync to work with both P2P transports and mock providers.
type BlockProvider interface {
	// GetBlock requests a block at the given height from a peer.
	GetBlock(ctx context.Context, height uint64) (*types.Block, *types.QuorumCertificate, error)

	// GetLatestHeight queries the network for the latest known height.
	GetLatestHeight(ctx context.Context) (uint64, error)

	// GetStateSnapshot requests a state snapshot at the given height.
	GetStateSnapshot(ctx context.Context, height uint64) (stateRoot types.Hash, data map[string][]byte, err error)
}

// Fetcher downloads blocks from peers and stores them.
type Fetcher struct {
	provider BlockProvider
	store    storage.Store
}

// NewFetcher creates a block fetcher.
func NewFetcher(provider BlockProvider, store storage.Store) *Fetcher {
	return &Fetcher{
		provider: provider,
		store:    store,
	}
}

// FetchBlocks downloads blocks from startHeight to endHeight (inclusive)
// and stores them. Returns the number of blocks fetched.
func (f *Fetcher) FetchBlocks(ctx context.Context, startHeight, endHeight uint64) (int, error) {
	if startHeight > endHeight {
		return 0, fmt.Errorf("sync: invalid range: start %d > end %d", startHeight, endHeight)
	}

	fetched := 0
	for h := startHeight; h <= endHeight; h++ {
		select {
		case <-ctx.Done():
			return fetched, ctx.Err()
		default:
		}

		// Check if we already have this block.
		has, _ := f.store.HasBlock(h)
		if has {
			fetched++
			continue
		}

		block, qc, err := f.provider.GetBlock(ctx, h)
		if err != nil {
			return fetched, fmt.Errorf("sync: fetch block %d: %w", h, err)
		}

		if err := f.store.SaveBlock(block, qc); err != nil {
			return fetched, fmt.Errorf("sync: save block %d: %w", h, err)
		}

		fetched++
	}

	return fetched, nil
}

// FetchLatestHeight queries the network for the latest block height.
func (f *Fetcher) FetchLatestHeight(ctx context.Context) (uint64, error) {
	if f.provider == nil {
		return 0, errors.New("sync: no block provider")
	}
	return f.provider.GetLatestHeight(ctx)
}
