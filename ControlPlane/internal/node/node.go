package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/echenim/Bedrock/controlplane/internal/admin"
	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/execution"
	"github.com/echenim/Bedrock/controlplane/internal/mempool"
	"github.com/echenim/Bedrock/controlplane/internal/rpc"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	bsync "github.com/echenim/Bedrock/controlplane/internal/sync"
	"github.com/echenim/Bedrock/controlplane/internal/telemetry"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// Node is the top-level BedRock node that owns and manages all subsystems.
type Node struct {
	cfg     *config.Config
	privKey crypto.PrivateKey
	valSet  *types.ValidatorSet

	// Subsystems.
	store       storage.Store
	mempool     *mempool.Mempool
	executor    consensus.ExecutionAdapter
	engine      *consensus.Engine
	syncer      *bsync.BlockSyncer
	rpcServer   *rpc.Server
	gateway     *rpc.Gateway
	metrics     *telemetry.Metrics
	metricsSrv  *telemetry.MetricsServer
	adminServer *admin.Server

	svcMgr *ServiceManager
	logger *zap.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
	done   chan struct{}
}

// NewNode creates and wires all subsystems without starting them.
func NewNode(
	cfg *config.Config,
	privKey crypto.PrivateKey,
	valSet *types.ValidatorSet,
	logger *zap.Logger,
) (*Node, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	nodeID := nodeIDFromKey(privKey)
	logger = logger.With(zap.String("node_id", nodeID))

	// 1. Storage.
	store, err := storage.OpenStore(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("node: open store: %w", err)
	}

	// 2. Execution adapter.
	// NewWASMAdapter falls back to native execution if WASM artifact not found.
	wasmAdapter, err := execution.NewWASMAdapter(cfg.Execution, store, logger.Named("execution"))
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("node: create execution adapter: %w", err)
	}
	var executor consensus.ExecutionAdapter = wasmAdapter

	// 3. Mempool.
	mp := mempool.NewMempool(cfg.Mempool, store, logger.Named("mempool"))

	// 4. Metrics.
	metrics := telemetry.NopMetrics()
	var metricsSrv *telemetry.MetricsServer
	if cfg.Telemetry.Enabled {
		metrics = telemetry.NewMetrics("bedrock")
		metricsSrv = telemetry.NewMetricsServer(cfg.Telemetry.Addr, metrics, logger.Named("metrics"))
	}

	// 5. Consensus engine (transport is nil for now — P2P not wired here).
	ecfg := consensus.DefaultEngineConfig()
	ecfg.PrivKey = privKey
	ecfg.ValSet = valSet
	ecfg.ChainID = []byte(cfg.ChainID)
	ecfg.Store = store
	ecfg.StateStore = store
	ecfg.Executor = executor
	ecfg.TxProvider = mp
	ecfg.Logger = logger.Named("consensus")
	ecfg.BaseTimeoutMs = cfg.Consensus.TimeoutPropose.Milliseconds()
	if ecfg.BaseTimeoutMs == 0 {
		ecfg.BaseTimeoutMs = 3000
	}
	ecfg.MaxTimeoutMs = 60000

	engine, err := consensus.NewEngine(ecfg)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("node: create consensus engine: %w", err)
	}

	// 6. Block syncer (no real P2P provider — placeholder nil provider).
	// In production, this would be wired to the P2P transport.
	// For now, syncer is nil unless a provider is available.
	var syncer *bsync.BlockSyncer

	// 7. RPC server.
	rpcServer := rpc.NewServer(cfg.RPC, logger.Named("rpc"))
	nodeSvc := rpc.NewNodeService(rpc.NodeServiceConfig{
		Store:     store,
		Mempool:   mp,
		Consensus: engine,
		Syncer:    syncer,
		ValSet:    valSet,
		NodeID:    nodeID,
		Moniker:   cfg.Moniker,
		ChainID:   cfg.ChainID,
		Logger:    logger.Named("rpc"),
	})
	rpcServer.RegisterNodeService(nodeSvc)

	// 8. HTTP gateway.
	var gw *rpc.Gateway
	if cfg.RPC.HTTPAddr != "" {
		gw = rpc.NewGateway(cfg.RPC.HTTPAddr, nodeSvc, logger.Named("gateway"))
	}

	// 9. Admin server.
	adminSrv := admin.NewServer("127.0.0.1:26661", engine, mp, syncer, logger.Named("admin"))

	return &Node{
		cfg:         cfg,
		privKey:     privKey,
		valSet:      valSet,
		store:       store,
		mempool:     mp,
		executor:    executor,
		engine:      engine,
		syncer:      syncer,
		rpcServer:   rpcServer,
		gateway:     gw,
		metrics:     metrics,
		metricsSrv:  metricsSrv,
		adminServer: adminSrv,
		svcMgr:      NewServiceManager(logger),
		logger:      logger,
		done:        make(chan struct{}),
	}, nil
}

// Start boots all subsystems in dependency order.
func (n *Node) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	n.logger.Info("node starting",
		zap.String("moniker", n.cfg.Moniker),
		zap.String("chain_id", n.cfg.ChainID),
	)

	// Start consensus engine.
	if err := n.engine.Start(ctx); err != nil {
		cancel()
		return fmt.Errorf("node: start consensus: %w", err)
	}

	// Start RPC server.
	if err := n.rpcServer.Start(ctx); err != nil {
		n.engine.Stop()
		cancel()
		return fmt.Errorf("node: start rpc: %w", err)
	}

	// Start HTTP gateway.
	if n.gateway != nil {
		if err := n.gateway.Start(ctx); err != nil {
			n.rpcServer.Stop()
			n.engine.Stop()
			cancel()
			return fmt.Errorf("node: start gateway: %w", err)
		}
	}

	// Start metrics server.
	if n.metricsSrv != nil {
		go n.metricsSrv.Start()
	}

	// Start admin server.
	if err := n.adminServer.Start(ctx); err != nil {
		n.logger.Warn("admin server failed to start", zap.Error(err))
		// Non-fatal.
	}

	n.logger.Info("node started successfully",
		zap.String("grpc_addr", n.rpcServer.GRPCAddr()),
	)

	return nil
}

// Stop gracefully shuts down all subsystems in reverse order.
func (n *Node) Stop() error {
	n.logger.Info("node stopping")

	if n.cancel != nil {
		n.cancel()
	}

	// Stop in reverse dependency order.
	if n.adminServer != nil {
		n.adminServer.Stop()
	}

	if n.metricsSrv != nil {
		n.metricsSrv.Stop()
	}

	if n.gateway != nil {
		n.gateway.Stop()
	}

	if n.rpcServer != nil {
		n.rpcServer.Stop()
	}

	if n.engine != nil {
		n.engine.Stop()
	}

	if n.store != nil {
		n.store.Close()
	}

	// Close WASM adapter if applicable.
	if closer, ok := n.executor.(interface{ Close() error }); ok {
		closer.Close()
	}

	n.logger.Info("node stopped")
	close(n.done)
	return nil
}

// Wait blocks until the node is stopped.
func (n *Node) Wait() error {
	<-n.done
	return nil
}

// Store returns the node's storage (for testing).
func (n *Node) Store() storage.Store {
	return n.store
}

// Engine returns the consensus engine (for testing).
func (n *Node) Engine() *consensus.Engine {
	return n.engine
}

// RPCServer returns the RPC server (for testing).
func (n *Node) RPCServer() *rpc.Server {
	return n.rpcServer
}

func nodeIDFromKey(privKey crypto.PrivateKey) string {
	if privKey == nil {
		return "unknown"
	}
	pubKey := privKey.Public().(crypto.PublicKey)
	addr := crypto.AddressFromPubKey(pubKey)
	return hex.EncodeToString(addr[:8])
}
