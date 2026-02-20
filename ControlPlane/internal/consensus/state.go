package consensus

import "github.com/echenim/Bedrock/controlplane/internal/types"

// ConsensusState tracks the current consensus state per SPEC.md §9.
type ConsensusState struct {
	Height uint64
	Round  uint64
	Step   RoundStep

	// Locking state (SPEC.md §9).
	// A validator is "locked" on a block when it has seen a QC for that block.
	// Once locked, it only votes for blocks extending the locked block, unless
	// it sees a QC at a higher round justifying an unlock.
	LockedBlock *types.Block
	LockedRound uint64
	HighestQC   *types.QuorumCertificate

	// Current round state.
	Proposal *types.Proposal
	VoteSet  *VoteSet

	// Commit tracking (two-chain rule §8).
	LastCommitHeight uint64
	LastCommitQC     *types.QuorumCertificate
}

// NewConsensusState creates a new ConsensusState starting at the given height.
func NewConsensusState(height uint64, valSet *types.ValidatorSet) *ConsensusState {
	return &ConsensusState{
		Height:  height,
		Round:   0,
		Step:    StepPropose,
		VoteSet: NewVoteSet(height, 0, valSet),
	}
}

// ResetForNewRound resets per-round state while preserving locks and commit info.
func (cs *ConsensusState) ResetForNewRound(round uint64, valSet *types.ValidatorSet) {
	cs.Round = round
	cs.Step = StepPropose
	cs.Proposal = nil
	cs.VoteSet = NewVoteSet(cs.Height, round, valSet)
}

// ResetForNewHeight advances to a new height after commit.
func (cs *ConsensusState) ResetForNewHeight(height uint64, valSet *types.ValidatorSet) {
	cs.Height = height
	cs.Round = 0
	cs.Step = StepPropose
	cs.Proposal = nil
	cs.VoteSet = NewVoteSet(height, 0, valSet)
}

// IsLocked returns true if the validator is locked on a block.
func (cs *ConsensusState) IsLocked() bool {
	return cs.LockedBlock != nil
}

// Lock locks on the given block at the given round.
func (cs *ConsensusState) Lock(block *types.Block, round uint64) {
	cs.LockedBlock = block
	cs.LockedRound = round
}

// Unlock clears the lock (when justified by a higher QC).
func (cs *ConsensusState) Unlock() {
	cs.LockedBlock = nil
	cs.LockedRound = 0
}

// UpdateHighestQC updates the highest QC if the given one is at a higher round.
func (cs *ConsensusState) UpdateHighestQC(qc *types.QuorumCertificate) {
	if cs.HighestQC == nil || qc.Round > cs.HighestQC.Round {
		cs.HighestQC = qc
	}
}
