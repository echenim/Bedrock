package consensus

import (
	"context"
	"fmt"
	"sync"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// Engine is the BFT consensus state machine.
type Engine struct {
	state      *ConsensusState
	valSet     *types.ValidatorSet
	privKey    crypto.PrivateKey
	address    types.Address
	chainID    []byte
	store      storage.BlockStore
	executor   ExecutionAdapter
	transport  Transport
	txProvider TxProvider
	logger     *zap.Logger

	timeouts     *TimeoutScheduler
	evidencePool *EvidencePool

	// Channels for event processing.
	proposalCh  chan *types.Proposal
	voteCh      chan *types.Vote
	timeoutCh   chan timeoutEvent
	commitCh    chan CommitEvent
	nextHeightCh chan struct{} // signals that a new height should start

	// Lifecycle.
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
}

// NewEngine creates a new consensus engine from the given config.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.ValSet == nil {
		return nil, fmt.Errorf("consensus: validator set required")
	}
	if cfg.PrivKey == nil {
		return nil, fmt.Errorf("consensus: private key required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Derive address from private key.
	pubKey := cfg.PrivKey.Public().(crypto.PublicKey)
	address := crypto.AddressFromPubKey(pubKey)

	// Use configured address if provided, otherwise derived.
	if !cfg.Address.IsZero() {
		address = cfg.Address
	}

	startHeight := uint64(1)
	if cfg.Store != nil {
		if h, err := cfg.Store.GetLatestHeight(); err == nil {
			startHeight = h + 1
		}
	}

	return &Engine{
		state:        NewConsensusState(startHeight, cfg.ValSet),
		valSet:       cfg.ValSet,
		privKey:      cfg.PrivKey,
		address:      address,
		chainID:      cfg.ChainID,
		store:        cfg.Store,
		executor:     cfg.Executor,
		transport:    cfg.Transport,
		txProvider:   cfg.TxProvider,
		logger:       logger,
		timeouts:     NewTimeoutScheduler(cfg.BaseTimeoutMs, cfg.MaxTimeoutMs),
		evidencePool: NewEvidencePool(),
		proposalCh:   make(chan *types.Proposal, 16),
		voteCh:       make(chan *types.Vote, 64),
		timeoutCh:    make(chan timeoutEvent, 16),
		commitCh:     make(chan CommitEvent, 16),
		nextHeightCh: make(chan struct{}, 1),
	}, nil
}

// Start begins the consensus event loop.
func (e *Engine) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.eventLoop(ctx)
	}()

	// Kick off the first round.
	e.EnterPropose()

	return nil
}

// Stop gracefully shuts down the engine.
func (e *Engine) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.timeouts.Stop()
	e.wg.Wait()
	return nil
}

// SubscribeCommits returns a channel that receives committed blocks.
func (e *Engine) SubscribeCommits() <-chan CommitEvent {
	return e.commitCh
}

// State returns the current consensus state (for testing/inspection).
func (e *Engine) State() *ConsensusState {
	return e.state
}

// EvidencePool returns the evidence pool (for testing/inspection).
func (e *Engine) Evidence() *EvidencePool {
	return e.evidencePool
}

// Address returns the engine's validator address.
func (e *Engine) Address() types.Address {
	return e.address
}
