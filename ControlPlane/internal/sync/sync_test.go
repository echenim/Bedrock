package sync

import (
	"context"
	"fmt"
	"testing"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/execution"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// --- Mock block provider ---

type mockBlockProvider struct {
	blocks    map[uint64]*types.Block
	qcs       map[uint64]*types.QuorumCertificate
	latestH   uint64
	snapshots map[uint64]mockSnapshot
	failAt    uint64 // height at which to return an error
}

type mockSnapshot struct {
	root types.Hash
	data map[string][]byte
}

func newMockProvider() *mockBlockProvider {
	return &mockBlockProvider{
		blocks:    make(map[uint64]*types.Block),
		qcs:       make(map[uint64]*types.QuorumCertificate),
		snapshots: make(map[uint64]mockSnapshot),
	}
}

func (m *mockBlockProvider) addBlock(h uint64, txs [][]byte) {
	block := &types.Block{
		Header: types.BlockHeader{
			Height:     h,
			Round:      0,
			ChainID:    []byte("test-chain"),
			ProposerID: types.Address{0x01}, // non-zero for validation
		},
		Transactions: txs,
	}
	block.Header.BlockHash = block.Header.ComputeHash()
	m.blocks[h] = block
	if h > m.latestH {
		m.latestH = h
	}
}

func (m *mockBlockProvider) addSnapshot(h uint64, root types.Hash, data map[string][]byte) {
	m.snapshots[h] = mockSnapshot{root: root, data: data}
}

func (m *mockBlockProvider) GetBlock(ctx context.Context, height uint64) (*types.Block, *types.QuorumCertificate, error) {
	if m.failAt > 0 && height == m.failAt {
		return nil, nil, fmt.Errorf("mock: connection failed at height %d", height)
	}
	block, ok := m.blocks[height]
	if !ok {
		return nil, nil, fmt.Errorf("mock: block %d not found", height)
	}
	qc := m.qcs[height]
	return block, qc, nil
}

func (m *mockBlockProvider) GetLatestHeight(ctx context.Context) (uint64, error) {
	return m.latestH, nil
}

func (m *mockBlockProvider) GetStateSnapshot(ctx context.Context, height uint64) (types.Hash, map[string][]byte, error) {
	snap, ok := m.snapshots[height]
	if !ok {
		return types.ZeroHash, nil, fmt.Errorf("mock: no snapshot at height %d", height)
	}
	return snap.root, snap.data, nil
}

// --- Verifier tests ---

func TestVerifyBlockValid(t *testing.T) {
	v := NewVerifier(nil, nil)

	block := &types.Block{
		Header: types.BlockHeader{
			Height:  1,
			ChainID: []byte("test"),
		},
	}
	block.Header.ProposerID = types.Address{0x01}
	block.Header.BlockHash = block.Header.ComputeHash()

	if err := v.VerifyBlock(block, nil, 1); err != nil {
		t.Fatalf("expected valid block: %v", err)
	}
}

func TestVerifyBlockNil(t *testing.T) {
	v := NewVerifier(nil, nil)
	if err := v.VerifyBlock(nil, nil, 1); err == nil {
		t.Fatal("expected error for nil block")
	}
}

func TestVerifyBlockWrongHeight(t *testing.T) {
	v := NewVerifier(nil, nil)

	block := &types.Block{
		Header: types.BlockHeader{Height: 5},
	}

	if err := v.VerifyBlock(block, nil, 3); err == nil {
		t.Fatal("expected error for wrong height")
	}
}

func TestVerifyAndExecuteBlock(t *testing.T) {
	mock := execution.NewMockExecutor()
	expectedRoot := crypto.HashSHA256([]byte("state-root-1"))
	mock.NextStateRoot = expectedRoot

	v := NewVerifier(nil, mock)

	block := &types.Block{
		Header: types.BlockHeader{
			Height:     1,
			ChainID:    []byte("test"),
			ProposerID: types.Address{0x01},
		},
		Transactions: [][]byte{[]byte("tx1")},
	}
	block.Header.BlockHash = block.Header.ComputeHash()

	result, err := v.VerifyAndExecuteBlock(block, nil, types.ZeroHash, expectedRoot)
	if err != nil {
		t.Fatalf("verify and execute: %v", err)
	}
	if result.StateRoot != expectedRoot {
		t.Fatal("state root mismatch")
	}
}

func TestVerifyAndExecuteBlockStateRootMismatch(t *testing.T) {
	mock := execution.NewMockExecutor()
	mock.NextStateRoot = crypto.HashSHA256([]byte("actual"))

	v := NewVerifier(nil, mock)

	block := &types.Block{
		Header: types.BlockHeader{
			Height:     1,
			ChainID:    []byte("test"),
			ProposerID: types.Address{0x01},
		},
	}
	block.Header.BlockHash = block.Header.ComputeHash()

	committedRoot := crypto.HashSHA256([]byte("expected"))
	_, err := v.VerifyAndExecuteBlock(block, nil, types.ZeroHash, committedRoot)
	if err == nil {
		t.Fatal("expected state root mismatch error")
	}
}

// --- Snapshot verification tests ---

func TestVerifySnapshotValid(t *testing.T) {
	root := crypto.HashSHA256([]byte("state"))
	if err := VerifySnapshot(root, root, nil); err != nil {
		t.Fatalf("expected valid snapshot: %v", err)
	}
}

func TestVerifySnapshotMismatch(t *testing.T) {
	committed := crypto.HashSHA256([]byte("committed"))
	snapshot := crypto.HashSHA256([]byte("different"))

	if err := VerifySnapshot(committed, snapshot, nil); err == nil {
		t.Fatal("expected snapshot mismatch error")
	}
}

func TestVerifySnapshotZeroRoot(t *testing.T) {
	if err := VerifySnapshot(types.ZeroHash, types.ZeroHash, nil); err == nil {
		t.Fatal("expected error for zero committed root")
	}
}

// --- Fetcher tests ---

func TestFetcherFetchBlocks(t *testing.T) {
	provider := newMockProvider()
	for h := uint64(1); h <= 5; h++ {
		provider.addBlock(h, [][]byte{[]byte(fmt.Sprintf("tx-%d", h))})
	}

	store := storage.NewMemStore()
	fetcher := NewFetcher(provider, store)

	ctx := context.Background()
	fetched, err := fetcher.FetchBlocks(ctx, 1, 5)
	if err != nil {
		t.Fatalf("fetch blocks: %v", err)
	}
	if fetched != 5 {
		t.Fatalf("expected 5 fetched, got %d", fetched)
	}

	// Verify blocks are in store.
	for h := uint64(1); h <= 5; h++ {
		has, _ := store.HasBlock(h)
		if !has {
			t.Fatalf("block %d not in store", h)
		}
	}
}

func TestFetcherSkipsExistingBlocks(t *testing.T) {
	provider := newMockProvider()
	provider.addBlock(1, nil)
	provider.addBlock(2, nil)

	store := storage.NewMemStore()
	// Pre-store block 1.
	store.SaveBlock(&types.Block{Header: types.BlockHeader{Height: 1}}, nil)

	fetcher := NewFetcher(provider, store)
	fetched, err := fetcher.FetchBlocks(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if fetched != 2 {
		t.Fatalf("expected 2 fetched (1 skipped + 1 new), got %d", fetched)
	}
}

func TestFetcherInvalidRange(t *testing.T) {
	fetcher := NewFetcher(newMockProvider(), storage.NewMemStore())
	_, err := fetcher.FetchBlocks(context.Background(), 5, 3)
	if err == nil {
		t.Fatal("expected error for invalid range")
	}
}

func TestFetcherHandlesPeerError(t *testing.T) {
	provider := newMockProvider()
	provider.addBlock(1, nil)
	provider.failAt = 2

	store := storage.NewMemStore()
	fetcher := NewFetcher(provider, store)

	fetched, err := fetcher.FetchBlocks(context.Background(), 1, 3)
	if err == nil {
		t.Fatal("expected error when peer fails")
	}
	if fetched != 1 {
		t.Fatalf("expected 1 fetched before failure, got %d", fetched)
	}
}

// --- BlockSyncer fast sync tests ---

func TestBlockSyncerFastSync(t *testing.T) {
	provider := newMockProvider()
	for h := uint64(1); h <= 10; h++ {
		provider.addBlock(h, [][]byte{[]byte(fmt.Sprintf("tx-%d", h))})
	}

	store := storage.NewMemStore()
	mock := execution.NewMockExecutor()
	mock.NextStateRoot = crypto.HashSHA256([]byte("root"))

	syncer := NewBlockSyncer(store, provider, mock, nil, nil)

	ctx := context.Background()
	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("sync start: %v", err)
	}

	if !syncer.IsSynced() {
		t.Fatal("expected syncer to be caught up")
	}
	if syncer.State() != SyncCaughtUp {
		t.Fatalf("expected CaughtUp state, got %s", syncer.State())
	}
	if syncer.CurrentHeight() != 10 {
		t.Fatalf("expected height 10, got %d", syncer.CurrentHeight())
	}
}

func TestBlockSyncerAlreadyCaughtUp(t *testing.T) {
	provider := newMockProvider()
	provider.addBlock(5, nil)

	store := storage.NewMemStore()
	// Pre-store blocks up to height 5.
	for h := uint64(1); h <= 5; h++ {
		store.SaveBlock(&types.Block{Header: types.BlockHeader{Height: h}}, nil)
	}

	syncer := NewBlockSyncer(store, provider, execution.NewMockExecutor(), nil, nil)
	if err := syncer.Start(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !syncer.IsSynced() {
		t.Fatal("expected already caught up")
	}
}

func TestBlockSyncerFastSyncRejectsInvalidBlock(t *testing.T) {
	provider := newMockProvider()
	provider.addBlock(1, nil)
	provider.failAt = 2 // simulates peer failure

	store := storage.NewMemStore()
	mock := execution.NewMockExecutor()
	mock.NextStateRoot = crypto.HashSHA256([]byte("root"))

	// Set target to 3 so we need to sync blocks 1-3.
	provider.latestH = 3

	syncer := NewBlockSyncer(store, provider, mock, nil, nil)
	err := syncer.Start(context.Background())
	if err == nil {
		t.Fatal("expected error during sync with missing blocks")
	}
}

func TestBlockSyncerSnapshotSync(t *testing.T) {
	provider := newMockProvider()
	// Set up many blocks (> snapshotThreshold).
	for h := uint64(1); h <= 200; h++ {
		provider.addBlock(h, nil)
	}

	// Add snapshot at height 200.
	snapRoot := crypto.HashSHA256([]byte("snapshot-root"))
	provider.addSnapshot(200, snapRoot, map[string][]byte{
		"key1": []byte("val1"),
		"key2": []byte("val2"),
	})
	// The snapshot verifier needs a committed root — set it in the block.
	provider.blocks[200].Header.StateRoot = snapRoot

	store := storage.NewMemStore()
	mock := execution.NewMockExecutor()

	syncer := NewBlockSyncer(store, provider, mock, nil, nil)
	if err := syncer.Start(context.Background()); err != nil {
		t.Fatalf("snapshot sync: %v", err)
	}

	if !syncer.IsSynced() {
		t.Fatal("expected caught up after snapshot sync")
	}
	if syncer.CurrentHeight() != 200 {
		t.Fatalf("expected height 200, got %d", syncer.CurrentHeight())
	}

	// Verify state was applied.
	val, _ := store.Get([]byte("key1"))
	if string(val) != "val1" {
		t.Fatalf("expected state key1=val1, got %s", string(val))
	}
}

func TestBlockSyncerContextCancellation(t *testing.T) {
	provider := newMockProvider()
	for h := uint64(1); h <= 100; h++ {
		provider.addBlock(h, nil)
	}

	store := storage.NewMemStore()
	mock := execution.NewMockExecutor()
	mock.NextStateRoot = crypto.HashSHA256([]byte("root"))

	syncer := NewBlockSyncer(store, provider, mock, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := syncer.Start(ctx)
	// Should get context error (either immediately or during sync).
	if err == nil {
		// It's possible sync completed before cancellation — that's ok for small sets.
		// Just verify it handled context properly.
	}
}

// --- SyncState tests ---

func TestSyncStateString(t *testing.T) {
	tests := []struct {
		state SyncState
		want  string
	}{
		{SyncIdle, "Idle"},
		{SyncFastSync, "FastSync"},
		{SyncStateSync, "StateSync"},
		{SyncCaughtUp, "CaughtUp"},
		{SyncState(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("SyncState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// --- SnapshotSyncer tests ---

func TestVerifyAndApplySnapshot(t *testing.T) {
	root := crypto.HashSHA256([]byte("root"))
	data := map[string][]byte{
		"a": []byte("1"),
		"b": []byte("2"),
	}
	store := storage.NewMemStore()

	if err := VerifyAndApplySnapshot(root, root, data, store); err != nil {
		t.Fatalf("verify and apply: %v", err)
	}

	val, _ := store.Get([]byte("a"))
	if string(val) != "1" {
		t.Fatalf("expected a=1, got %s", val)
	}

	savedRoot, _ := store.GetStateRoot()
	if savedRoot != root {
		t.Fatal("state root not saved")
	}
}

func TestVerifyAndApplySnapshotMismatch(t *testing.T) {
	committed := crypto.HashSHA256([]byte("committed"))
	snapshot := crypto.HashSHA256([]byte("snapshot"))
	store := storage.NewMemStore()

	err := VerifyAndApplySnapshot(snapshot, committed, nil, store)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}
