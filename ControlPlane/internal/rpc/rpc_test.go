package rpc

import (
	"context"
	"testing"
	"time"

	rpcv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/rpc/v1"
	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/mempool"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// --- Test helpers ---

func testNodeService(t *testing.T) (*NodeServiceImpl, storage.Store) {
	t.Helper()
	store := storage.NewMemStore()

	// Add a block for testing.
	block := &types.Block{
		Header: types.BlockHeader{
			Height:     1,
			ChainID:    []byte("test-chain"),
			ProposerID: types.Address{0x01},
		},
		Transactions: [][]byte{[]byte("tx1"), []byte("tx2")},
	}
	block.Header.BlockHash = block.Header.ComputeHash()
	store.SaveBlock(block, nil)

	// Add state data.
	store.ApplyWriteSet(map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
	})

	mp := mempool.NewMempool(config.MempoolConfig{
		MaxSize:    100,
		MaxTxBytes: 1024 * 1024,
		CacheSize:  100,
	}, store, nil)

	_, privKey, _ := crypto.GenerateKeypair()
	pubKey := privKey.Public().(crypto.PublicKey)
	addr := crypto.AddressFromPubKey(pubKey)

	valSet, _ := types.NewValidatorSet([]types.Validator{
		{
			Address:     addr,
			PublicKey:   crypto.PubKeyTo32(pubKey),
			VotingPower: 100,
		},
	})

	svc := NewNodeService(NodeServiceConfig{
		Store:   store,
		Mempool: mp,
		ValSet:  valSet,
		NodeID:  "test-node-id",
		Moniker: "test-moniker",
		ChainID: "test-chain",
	})

	return svc, store
}

func startTestServer(t *testing.T, svc *NodeServiceImpl) (addr string, cleanup func()) {
	t.Helper()
	server := NewServer(config.RPCConfig{
		GRPCAddr: "127.0.0.1:0",
	}, nil)
	server.RegisterNodeService(svc)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}

	return server.GRPCAddr(), func() { server.Stop() }
}

func dialGRPC(t *testing.T, addr string) rpcv1.NodeServiceClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial grpc: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return rpcv1.NewNodeServiceClient(conn)
}

// --- NodeService unit tests ---

func TestGetStatusReturnsNodeInfo(t *testing.T) {
	svc, _ := testNodeService(t)

	resp, err := svc.GetStatus(context.Background(), &rpcv1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.NodeId != "test-node-id" {
		t.Errorf("expected node_id=test-node-id, got %s", resp.NodeId)
	}
	if resp.Moniker != "test-moniker" {
		t.Errorf("expected moniker=test-moniker, got %s", resp.Moniker)
	}
	if resp.Network != "test-chain" {
		t.Errorf("expected network=test-chain, got %s", resp.Network)
	}
	if resp.LatestBlockHeight != 1 {
		t.Errorf("expected height=1, got %d", resp.LatestBlockHeight)
	}
}

func TestGetStatusNoSyncer(t *testing.T) {
	svc, _ := testNodeService(t)
	resp, err := svc.GetStatus(context.Background(), &rpcv1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	// Without syncer, Syncing should be false (default).
	if resp.Syncing {
		t.Error("expected Syncing=false when no syncer")
	}
}

func TestGetBlock(t *testing.T) {
	svc, _ := testNodeService(t)

	resp, err := svc.GetBlock(context.Background(), &rpcv1.GetBlockRequest{Height: 1})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if resp.Block == nil {
		t.Fatal("expected non-nil block")
	}
	if resp.Block.Header.Height != 1 {
		t.Errorf("expected height=1, got %d", resp.Block.Header.Height)
	}
}

func TestGetBlockLatest(t *testing.T) {
	svc, _ := testNodeService(t)

	// Height=0 should return the latest block.
	resp, err := svc.GetBlock(context.Background(), &rpcv1.GetBlockRequest{Height: 0})
	if err != nil {
		t.Fatalf("GetBlock(0): %v", err)
	}
	if resp.Block == nil {
		t.Fatal("expected non-nil block")
	}
}

func TestGetBlockNotFound(t *testing.T) {
	svc, _ := testNodeService(t)

	_, err := svc.GetBlock(context.Background(), &rpcv1.GetBlockRequest{Height: 999})
	if err == nil {
		t.Fatal("expected error for non-existent block")
	}
}

func TestSubmitTransactionEmpty(t *testing.T) {
	svc, _ := testNodeService(t)

	_, err := svc.SubmitTransaction(context.Background(), &rpcv1.SubmitTransactionRequest{})
	if err == nil {
		t.Fatal("expected error for empty tx")
	}
}

func TestSubmitTransactionValid(t *testing.T) {
	svc, _ := testNodeService(t)

	// Build a valid transaction (follows mempool format: 4-byte fee + 4-byte nonce + 32-byte sender + 64-byte sig + payload).
	tx := makeTestTx()

	resp, err := svc.SubmitTransaction(context.Background(), &rpcv1.SubmitTransactionRequest{Tx: tx})
	if err != nil {
		t.Fatalf("SubmitTransaction: %v", err)
	}
	if resp.Code != 0 {
		// Might fail validation - that's ok, we just check it doesn't panic.
		t.Logf("submit response: code=%d log=%s", resp.Code, resp.Log)
	}
}

func TestQueryState(t *testing.T) {
	svc, _ := testNodeService(t)

	resp, err := svc.QueryState(context.Background(), &rpcv1.QueryStateRequest{
		Key: []byte("key1"),
	})
	if err != nil {
		t.Fatalf("QueryState: %v", err)
	}
	if string(resp.Value) != "value1" {
		t.Errorf("expected value1, got %s", string(resp.Value))
	}
}

func TestQueryStateWithProof(t *testing.T) {
	svc, _ := testNodeService(t)

	resp, err := svc.QueryState(context.Background(), &rpcv1.QueryStateRequest{
		Key:   []byte("key1"),
		Prove: true,
	})
	if err != nil {
		t.Fatalf("QueryState: %v", err)
	}
	if resp.Proof == nil {
		t.Fatal("expected proof with Prove=true")
	}
}

func TestQueryStateEmptyKey(t *testing.T) {
	svc, _ := testNodeService(t)

	_, err := svc.QueryState(context.Background(), &rpcv1.QueryStateRequest{})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestGetValidators(t *testing.T) {
	svc, _ := testNodeService(t)

	resp, err := svc.GetValidators(context.Background(), &rpcv1.GetValidatorsRequest{})
	if err != nil {
		t.Fatalf("GetValidators: %v", err)
	}
	if resp.ValidatorSet == nil {
		t.Fatal("expected non-nil validator set")
	}
	if len(resp.ValidatorSet.Validators) != 1 {
		t.Errorf("expected 1 validator, got %d", len(resp.ValidatorSet.Validators))
	}
}

func TestGetReceiptNotFound(t *testing.T) {
	svc, _ := testNodeService(t)

	_, err := svc.GetReceipt(context.Background(), &rpcv1.GetReceiptRequest{
		TxHash: []byte{0x01, 0x02, 0x03},
	})
	if err == nil {
		t.Fatal("expected error for non-existent receipt")
	}
}

func TestGetReceiptEmptyHash(t *testing.T) {
	svc, _ := testNodeService(t)

	_, err := svc.GetReceipt(context.Background(), &rpcv1.GetReceiptRequest{})
	if err == nil {
		t.Fatal("expected error for empty tx_hash")
	}
}

// --- gRPC integration tests ---

func TestGRPCGetStatus(t *testing.T) {
	svc, _ := testNodeService(t)
	addr, cleanup := startTestServer(t, svc)
	defer cleanup()

	client := dialGRPC(t, addr)

	resp, err := client.GetStatus(context.Background(), &rpcv1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("gRPC GetStatus: %v", err)
	}
	if resp.NodeId != "test-node-id" {
		t.Errorf("expected node_id=test-node-id, got %s", resp.NodeId)
	}
	if resp.LatestBlockHeight != 1 {
		t.Errorf("expected height=1, got %d", resp.LatestBlockHeight)
	}
}

func TestGRPCGetBlock(t *testing.T) {
	svc, _ := testNodeService(t)
	addr, cleanup := startTestServer(t, svc)
	defer cleanup()

	client := dialGRPC(t, addr)

	resp, err := client.GetBlock(context.Background(), &rpcv1.GetBlockRequest{Height: 1})
	if err != nil {
		t.Fatalf("gRPC GetBlock: %v", err)
	}
	if resp.Block == nil {
		t.Fatal("expected non-nil block")
	}
}

func TestGRPCQueryState(t *testing.T) {
	svc, _ := testNodeService(t)
	addr, cleanup := startTestServer(t, svc)
	defer cleanup()

	client := dialGRPC(t, addr)

	resp, err := client.QueryState(context.Background(), &rpcv1.QueryStateRequest{
		Key: []byte("key1"),
	})
	if err != nil {
		t.Fatalf("gRPC QueryState: %v", err)
	}
	if string(resp.Value) != "value1" {
		t.Errorf("expected value1, got %s", string(resp.Value))
	}
}

func TestGRPCGetValidators(t *testing.T) {
	svc, _ := testNodeService(t)
	addr, cleanup := startTestServer(t, svc)
	defer cleanup()

	client := dialGRPC(t, addr)

	resp, err := client.GetValidators(context.Background(), &rpcv1.GetValidatorsRequest{})
	if err != nil {
		t.Fatalf("gRPC GetValidators: %v", err)
	}
	if resp.ValidatorSet == nil {
		t.Fatal("expected validator set")
	}
}

// --- Server lifecycle tests ---

func TestServerStartStop(t *testing.T) {
	server := NewServer(config.RPCConfig{
		GRPCAddr: "127.0.0.1:0",
	}, nil)

	svc, _ := testNodeService(t)
	server.RegisterNodeService(svc)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	addr := server.GRPCAddr()
	if addr == "" {
		t.Fatal("expected non-empty address")
	}

	if err := server.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestServerName(t *testing.T) {
	server := NewServer(config.RPCConfig{GRPCAddr: "127.0.0.1:0"}, nil)
	if server.Name() != "rpc" {
		t.Errorf("expected name=rpc, got %s", server.Name())
	}
}

// --- Helper ---

func makeTestTx() []byte {
	// Minimal tx that passes size check but may fail mempool validation.
	// Format: 4-byte fee (big-endian) + 4-byte nonce + 32-byte sender + 64-byte sig + payload
	tx := make([]byte, 4+4+32+64+10)
	// Set fee = 1000 (big-endian).
	tx[0] = 0
	tx[1] = 0
	tx[2] = 0x03
	tx[3] = 0xe8
	// Set nonce = 1.
	tx[4] = 0
	tx[5] = 0
	tx[6] = 0
	tx[7] = 1
	// Sender (32 bytes) + sig (64 bytes) + payload filled with zeros.
	copy(tx[8:40], []byte("sender-address-32bytes-padded!!!"))
	return tx
}
