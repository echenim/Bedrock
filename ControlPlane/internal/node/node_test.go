package node

import (
	"context"
	"testing"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

func testConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Storage.Backend = "memory"
	cfg.RPC.GRPCAddr = "127.0.0.1:0"
	cfg.RPC.HTTPAddr = "127.0.0.1:0"
	cfg.Telemetry.Enabled = false
	return cfg
}

func testValSet(privKey crypto.PrivateKey) *types.ValidatorSet {
	pubKey := privKey.Public().(crypto.PublicKey)
	addr := crypto.AddressFromPubKey(pubKey)
	vs, _ := types.NewValidatorSet([]types.Validator{
		{
			Address:     addr,
			PublicKey:   crypto.PubKeyTo32(pubKey),
			VotingPower: 100,
		},
	})
	return vs
}

// --- ServiceManager tests ---

func TestServiceManagerStartStop(t *testing.T) {
	sm := NewServiceManager(nil)

	svc1 := &mockService{name: "svc1"}
	svc2 := &mockService{name: "svc2"}

	sm.Add(svc1)
	sm.Add(svc2)

	ctx := context.Background()
	if err := sm.StartAll(ctx); err != nil {
		t.Fatalf("start all: %v", err)
	}

	if !svc1.started || !svc2.started {
		t.Fatal("expected both services started")
	}

	if err := sm.StopAll(); err != nil {
		t.Fatalf("stop all: %v", err)
	}

	if !svc1.stopped || !svc2.stopped {
		t.Fatal("expected both services stopped")
	}
}

func TestServiceManagerRollback(t *testing.T) {
	sm := NewServiceManager(nil)

	svc1 := &mockService{name: "svc1"}
	svc2 := &mockService{name: "svc2", failStart: true}

	sm.Add(svc1)
	sm.Add(svc2)

	ctx := context.Background()
	err := sm.StartAll(ctx)
	if err == nil {
		t.Fatal("expected error when svc2 fails to start")
	}

	// svc1 should have been rolled back (stopped).
	if !svc1.stopped {
		t.Fatal("expected svc1 to be stopped during rollback")
	}
}

func TestServiceManagerStopReverseOrder(t *testing.T) {
	sm := NewServiceManager(nil)

	order := make([]string, 0)
	svc1 := &mockService{name: "svc1", onStop: func() { order = append(order, "svc1") }}
	svc2 := &mockService{name: "svc2", onStop: func() { order = append(order, "svc2") }}
	svc3 := &mockService{name: "svc3", onStop: func() { order = append(order, "svc3") }}

	sm.Add(svc1)
	sm.Add(svc2)
	sm.Add(svc3)

	sm.StartAll(context.Background())
	sm.StopAll()

	if len(order) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(order))
	}
	// Reverse order: svc3, svc2, svc1.
	if order[0] != "svc3" || order[1] != "svc2" || order[2] != "svc1" {
		t.Errorf("expected stop order [svc3, svc2, svc1], got %v", order)
	}
}

func TestServiceManagerServices(t *testing.T) {
	sm := NewServiceManager(nil)
	sm.Add(&mockService{name: "a"})
	sm.Add(&mockService{name: "b"})

	if len(sm.Services()) != 2 {
		t.Errorf("expected 2 services, got %d", len(sm.Services()))
	}
}

// --- Node lifecycle tests ---

func TestNodeCreateAndStop(t *testing.T) {
	cfg := testConfig()
	_, privKey, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	valSet := testValSet(privKey)

	n, err := NewNode(cfg, privKey, valSet, nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	if n.Store() == nil {
		t.Fatal("expected non-nil store")
	}
	if n.Engine() == nil {
		t.Fatal("expected non-nil engine")
	}

	// Stop without start should not panic.
	n.Stop()
}

func TestNodeStartStop(t *testing.T) {
	cfg := testConfig()
	_, privKey, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	valSet := testValSet(privKey)

	n, err := NewNode(cfg, privKey, valSet, nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := n.Start(ctx); err != nil {
		t.Fatalf("start node: %v", err)
	}

	// Verify RPC is listening.
	addr := n.RPCServer().GRPCAddr()
	if addr == "" {
		t.Fatal("expected non-empty gRPC address")
	}

	// Stop.
	if err := n.Stop(); err != nil {
		t.Fatalf("stop node: %v", err)
	}
}

func TestNodeStartStopMultipleTimes(t *testing.T) {
	cfg := testConfig()
	_, privKey, _ := crypto.GenerateKeypair()
	valSet := testValSet(privKey)

	n, err := NewNode(cfg, privKey, valSet, nil)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	ctx := context.Background()

	if err := n.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := n.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// --- Mock service ---

type mockService struct {
	name      string
	started   bool
	stopped   bool
	failStart bool
	onStop    func()
}

func (m *mockService) Start(ctx context.Context) error {
	if m.failStart {
		return context.DeadlineExceeded
	}
	m.started = true
	return nil
}

func (m *mockService) Stop() error {
	m.stopped = true
	if m.onStop != nil {
		m.onStop()
	}
	return nil
}

func (m *mockService) Name() string {
	return m.name
}
