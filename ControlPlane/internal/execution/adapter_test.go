package execution

import (
	"testing"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// --- Test helpers ---

func testBlock(height uint64, txs [][]byte) *types.Block {
	block := &types.Block{
		Header: types.BlockHeader{
			Height:  height,
			Round:   0,
			ChainID: []byte("test-chain"),
		},
		Transactions: txs,
	}
	block.Header.BlockHash = block.Header.ComputeHash()
	return block
}

// --- MockExecutor tests ---

func TestMockExecutorImplementsInterface(t *testing.T) {
	var _ consensus.ExecutionAdapter = (*MockExecutor)(nil)
}

func TestMockExecutorSuccess(t *testing.T) {
	mock := NewMockExecutor()
	mock.NextStateRoot = crypto.HashSHA256([]byte("state-root"))
	mock.NextGasUsed = 5000

	block := testBlock(1, [][]byte{[]byte("tx1")})
	prevRoot := types.ZeroHash

	result, err := mock.ExecuteBlock(block, prevRoot)
	if err != nil {
		t.Fatalf("execute block: %v", err)
	}
	if result.StateRoot != mock.NextStateRoot {
		t.Fatal("state root mismatch")
	}
	if result.GasUsed != 5000 {
		t.Fatalf("gas used = %d, want 5000", result.GasUsed)
	}
	if mock.CallCount != 1 {
		t.Fatalf("call count = %d, want 1", mock.CallCount)
	}
	if mock.LastBlock != block {
		t.Fatal("last block mismatch")
	}
}

func TestMockExecutorFailure(t *testing.T) {
	mock := NewMockExecutor()
	mock.ShouldFail = true

	block := testBlock(1, nil)
	_, err := mock.ExecuteBlock(block, types.ZeroHash)
	if err == nil {
		t.Fatal("expected error from failed mock")
	}
}

// --- WASMAdapter tests ---

func TestWASMAdapterImplementsInterface(t *testing.T) {
	var _ consensus.ExecutionAdapter = (*WASMAdapter)(nil)
}

func TestNewWASMAdapterNoWASMFile(t *testing.T) {
	cfg := config.ExecutionConfig{
		WASMPath:    "/nonexistent/path.wasm",
		GasLimit:    100_000_000,
		FuelLimit:   100_000_000,
		MaxMemoryMB: 256,
	}

	adapter, err := NewWASMAdapter(cfg, storage.NewMemStore(), nil)
	if err != nil {
		t.Fatalf("expected adapter to be created (native mode): %v", err)
	}
	defer adapter.Close()
}

func TestWASMAdapterNilBlock(t *testing.T) {
	cfg := config.ExecutionConfig{
		WASMPath: "/nonexistent.wasm",
		GasLimit: 100_000_000,
	}
	adapter, _ := NewWASMAdapter(cfg, nil, nil)
	defer adapter.Close()

	_, err := adapter.ExecuteBlock(nil, types.ZeroHash)
	if err == nil {
		t.Fatal("expected error for nil block")
	}
}

func TestWASMAdapterExecuteBlock(t *testing.T) {
	cfg := config.ExecutionConfig{
		WASMPath: "/nonexistent.wasm", // triggers native executor
		GasLimit: 100_000_000,
	}
	store := storage.NewMemStore()
	adapter, err := NewWASMAdapter(cfg, store, nil)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	defer adapter.Close()

	block := testBlock(1, [][]byte{[]byte("tx1"), []byte("tx2")})
	prevRoot := types.ZeroHash

	result, err := adapter.ExecuteBlock(block, prevRoot)
	if err != nil {
		t.Fatalf("execute block: %v", err)
	}

	if result.StateRoot == types.ZeroHash {
		t.Fatal("expected non-zero state root")
	}
	if result.GasUsed == 0 {
		t.Fatal("expected non-zero gas used")
	}
}

// --- Sandbox (native executor) tests ---

func TestNativeExecutorDeterministic(t *testing.T) {
	cfg := config.ExecutionConfig{GasLimit: 100_000_000}
	s1, _ := NewSandbox(cfg)
	s2, _ := NewSandbox(cfg)

	txs := [][]byte{[]byte("tx-a"), []byte("tx-b"), []byte("tx-c")}
	block := testBlock(1, txs)
	prevRoot := types.ZeroHash

	store1 := storage.NewMemStore()
	store2 := storage.NewMemStore()

	r1, err := s1.Execute(block, prevRoot, store1)
	if err != nil {
		t.Fatalf("execute 1: %v", err)
	}
	r2, err := s2.Execute(block, prevRoot, store2)
	if err != nil {
		t.Fatalf("execute 2: %v", err)
	}

	if r1.StateRoot != r2.StateRoot {
		t.Fatal("state roots differ — execution is not deterministic")
	}
	if r1.GasUsed != r2.GasUsed {
		t.Fatal("gas used differs")
	}
}

func TestNativeExecutorDifferentBlocks(t *testing.T) {
	cfg := config.ExecutionConfig{GasLimit: 100_000_000}
	s, _ := NewSandbox(cfg)

	block1 := testBlock(1, [][]byte{[]byte("tx-a")})
	block2 := testBlock(1, [][]byte{[]byte("tx-b")})
	prevRoot := types.ZeroHash

	r1, _ := s.Execute(block1, prevRoot, nil)
	r2, _ := s.Execute(block2, prevRoot, nil)

	if r1.StateRoot == r2.StateRoot {
		t.Fatal("different txs should produce different state roots")
	}
}

func TestNativeExecutorEmptyBlock(t *testing.T) {
	cfg := config.ExecutionConfig{GasLimit: 100_000_000}
	s, _ := NewSandbox(cfg)

	block := testBlock(1, nil)
	prevRoot := crypto.HashSHA256([]byte("prev"))

	result, err := s.Execute(block, prevRoot, nil)
	if err != nil {
		t.Fatalf("execute empty block: %v", err)
	}

	// Empty block → state root = prevRoot (no changes).
	if result.StateRoot != prevRoot {
		t.Fatal("empty block should preserve previous state root")
	}
	if result.GasUsed != 0 {
		t.Fatalf("empty block gas = %d, want 0", result.GasUsed)
	}
}

func TestNativeExecutorGasLimit(t *testing.T) {
	cfg := config.ExecutionConfig{GasLimit: 500} // very low
	s, _ := NewSandbox(cfg)

	// Each tx uses 1000 base + payload bytes.
	block := testBlock(1, [][]byte{[]byte("tx-a")})

	_, err := s.Execute(block, types.ZeroHash, nil)
	if err == nil {
		t.Fatal("expected gas limit exceeded error")
	}
}

func TestNativeExecutorPersistsState(t *testing.T) {
	cfg := config.ExecutionConfig{GasLimit: 100_000_000}
	s, _ := NewSandbox(cfg)
	store := storage.NewMemStore()

	block := testBlock(1, [][]byte{[]byte("tx-data")})
	result, err := s.Execute(block, types.ZeroHash, store)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Verify state root was persisted.
	savedRoot, err := store.GetStateRoot()
	if err != nil {
		t.Fatalf("get state root: %v", err)
	}
	if savedRoot != result.StateRoot {
		t.Fatal("persisted state root doesn't match execution result")
	}
}

func TestNativeExecutorChainedBlocks(t *testing.T) {
	cfg := config.ExecutionConfig{GasLimit: 100_000_000}
	s, _ := NewSandbox(cfg)
	store := storage.NewMemStore()

	// Block 1
	block1 := testBlock(1, [][]byte{[]byte("tx1")})
	r1, err := s.Execute(block1, types.ZeroHash, store)
	if err != nil {
		t.Fatalf("execute block 1: %v", err)
	}

	// Block 2 builds on block 1's state root.
	block2 := testBlock(2, [][]byte{[]byte("tx2")})
	r2, err := s.Execute(block2, r1.StateRoot, store)
	if err != nil {
		t.Fatalf("execute block 2: %v", err)
	}

	// Different state roots for different history.
	if r1.StateRoot == r2.StateRoot {
		t.Fatal("chained blocks should produce different state roots")
	}
}

func TestComputeStateRootDeterministic(t *testing.T) {
	prevRoot := crypto.HashSHA256([]byte("root"))
	txs := [][]byte{[]byte("b"), []byte("a"), []byte("c")}

	root1 := computeStateRoot(prevRoot, txs)
	root2 := computeStateRoot(prevRoot, txs)

	if root1 != root2 {
		t.Fatal("computeStateRoot should be deterministic")
	}

	// Different order should give same result (txs are sorted internally).
	txsReversed := [][]byte{[]byte("c"), []byte("a"), []byte("b")}
	root3 := computeStateRoot(prevRoot, txsReversed)
	if root1 != root3 {
		t.Fatal("computeStateRoot should be order-independent")
	}
}
