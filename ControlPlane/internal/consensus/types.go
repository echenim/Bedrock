package consensus

import (
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// RoundStep represents the current step in a consensus round.
type RoundStep int

const (
	StepPropose RoundStep = iota
	StepVote
	StepCommit
)

func (s RoundStep) String() string {
	switch s {
	case StepPropose:
		return "Propose"
	case StepVote:
		return "Vote"
	case StepCommit:
		return "Commit"
	default:
		return "Unknown"
	}
}

// ExecutionAdapter invokes deterministic execution for a proposed block.
type ExecutionAdapter interface {
	ExecuteBlock(block *types.Block, prevStateRoot types.Hash) (*ExecutionResult, error)
}

// ExecutionResult holds the output of block execution.
type ExecutionResult struct {
	StateRoot types.Hash
	GasUsed   uint64
}

// Transport abstracts P2P message sending.
type Transport interface {
	BroadcastProposal(proposal *types.Proposal) error
	BroadcastVote(vote *types.Vote) error
	BroadcastTimeout(msg *types.TimeoutMessage) error
}

// TxProvider supplies transactions for block building.
type TxProvider interface {
	ReapMaxTxs(maxBytes int) [][]byte
}

// timeoutEvent is fired when a round timer expires.
type timeoutEvent struct {
	Height uint64
	Round  uint64
}

// CommitEvent signals that a block has been committed.
type CommitEvent struct {
	Block     *types.Block
	QC        *types.QuorumCertificate
	StateRoot types.Hash
	Height    uint64
}

// EngineConfig holds configuration for the consensus engine.
type EngineConfig struct {
	PrivKey    crypto.PrivateKey
	Address    types.Address
	ValSet     *types.ValidatorSet
	ChainID    []byte
	Store      storage.BlockStore
	StateStore storage.StateStore
	Executor   ExecutionAdapter
	Transport  Transport
	TxProvider TxProvider
	Logger     *zap.Logger

	// Timeout settings.
	BaseTimeoutMs  int64 // base propose timeout in milliseconds (default: 3000)
	MaxTimeoutMs   int64 // max timeout cap in milliseconds (default: 60000)
	TimeoutStepMs  int64 // vote/commit step timeout in milliseconds (default: 1000)
}

// DefaultEngineConfig returns an EngineConfig with sensible timeout defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		BaseTimeoutMs: 3000,
		MaxTimeoutMs:  60000,
		TimeoutStepMs: 1000,
	}
}
