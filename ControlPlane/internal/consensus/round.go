package consensus

import (
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// EnterNewRound transitions to a new round.
// Per SPEC.md §10 (view change): reset per-round state, schedule timeout.
func (e *Engine) EnterNewRound(round uint64) {
	e.logger.Info("entering new round",
		zap.Uint64("height", e.state.Height),
		zap.Uint64("round", round),
	)

	e.state.ResetForNewRound(round, e.valSet)
	e.EnterPropose()
}

// EnterPropose begins the proposal phase.
// If we are the proposer: build block, broadcast proposal.
// If not: wait for proposal with timeout.
func (e *Engine) EnterPropose() {
	e.state.Step = StepPropose

	proposer := e.valSet.GetProposer(e.state.Height, e.state.Round)
	if proposer == nil {
		e.logger.Error("no proposer for round",
			zap.Uint64("height", e.state.Height),
			zap.Uint64("round", e.state.Round),
		)
		return
	}

	if proposer.Address == e.address {
		// We are the proposer.
		e.logger.Info("we are proposer, creating proposal",
			zap.Uint64("height", e.state.Height),
			zap.Uint64("round", e.state.Round),
		)

		proposal, err := e.CreateProposal()
		if err != nil {
			e.logger.Error("failed to create proposal", zap.Error(err))
			// Schedule timeout so we don't get stuck.
			e.scheduleRoundTimeout()
			return
		}

		// Set our own proposal.
		e.state.Proposal = proposal

		// Broadcast to peers.
		if e.transport != nil {
			if err := e.transport.BroadcastProposal(proposal); err != nil {
				e.logger.Error("failed to broadcast proposal", zap.Error(err))
			}
		}

		// Move to vote step immediately since we have our own proposal.
		e.EnterVote()
	} else {
		// Not the proposer — schedule timeout waiting for proposal.
		e.logger.Debug("waiting for proposal",
			zap.String("proposer", proposer.Address.String()),
			zap.Uint64("height", e.state.Height),
			zap.Uint64("round", e.state.Round),
		)
		e.scheduleRoundTimeout()
	}
}

// EnterVote begins the vote phase.
// Verify proposal, check lock rules, sign vote, broadcast.
func (e *Engine) EnterVote() {
	e.state.Step = StepVote

	proposal := e.state.Proposal
	if proposal == nil {
		e.logger.Debug("no proposal to vote on")
		return
	}

	block := proposal.Block
	blockHash := block.Header.BlockHash
	if blockHash.IsZero() {
		blockHash = block.Header.ComputeHash()
	}

	// Sign vote.
	vote := &types.Vote{
		BlockHash: blockHash,
		Height:    e.state.Height,
		Round:     e.state.Round,
		VoterID:   e.address,
	}
	payload := vote.SigningPayload()
	sig := crypto.Sign(e.privKey, payload)
	vote.Signature = crypto.SigTo64(sig)

	// Add our own vote to the vote set.
	quorum, evidence, err := e.state.VoteSet.AddVote(vote)
	if err != nil {
		e.logger.Error("failed to add own vote", zap.Error(err))
	}
	if evidence != nil {
		e.evidencePool.AddEvidence(evidence)
	}

	// Broadcast vote.
	if e.transport != nil {
		if err := e.transport.BroadcastVote(vote); err != nil {
			e.logger.Error("failed to broadcast vote", zap.Error(err))
		}
	}

	// Check if quorum reached with our vote.
	if quorum {
		e.onQuorumReached()
	}
}

// HandleTimeout processes a round timeout.
// Increment round, broadcast timeout message, enter new round.
func (e *Engine) HandleTimeout(height, round uint64) {
	// Ignore stale timeouts.
	if height != e.state.Height || round != e.state.Round {
		return
	}

	e.logger.Info("round timed out",
		zap.Uint64("height", height),
		zap.Uint64("round", round),
	)

	// Build and broadcast timeout message.
	tm := &types.TimeoutMessage{
		Height:  height,
		Round:   round,
		VoterID: e.address,
		HighQC:  e.state.HighestQC,
	}
	payload := tm.SigningPayload()
	sig := crypto.Sign(e.privKey, payload)
	tm.Signature = crypto.SigTo64(sig)

	if e.transport != nil {
		if err := e.transport.BroadcastTimeout(tm); err != nil {
			e.logger.Error("failed to broadcast timeout", zap.Error(err))
		}
	}

	// Advance to next round.
	e.EnterNewRound(round + 1)
}

// onQuorumReached is called when a QC forms from collected votes.
// Two-chain commit rule: when a QC forms for block B at height H, and B embeds
// a QC for its parent (block at height H-1), the parent is committed.
// After processing, the engine always advances to the next height.
func (e *Engine) onQuorumReached() {
	qc, err := e.state.VoteSet.MakeQC()
	if err != nil {
		e.logger.Error("failed to make QC", zap.Error(err))
		return
	}

	block := e.state.Proposal.Block

	e.logger.Info("quorum reached, QC formed",
		zap.Uint64("height", e.state.Height),
		zap.Uint64("round", e.state.Round),
	)

	// Two-chain commit check: if the current block has an embedded QC
	// (for its parent), the parent block is now committed.
	previousLockedBlock := e.state.LockedBlock
	if block.QC != nil && previousLockedBlock != nil {
		e.persistCommit(previousLockedBlock, block.QC)
	}

	// Lock on the current block and update highest QC.
	e.state.UpdateHighestQC(qc)
	e.state.Lock(block, e.state.Round)

	// Reset timeout backoff on progress.
	e.timeouts.Reset(e.state.Round)

	// Advance to next height — the QC certifies this height is done.
	e.advanceHeight()
}

// persistCommit finalizes a committed block: persists to store, notifies subscribers.
func (e *Engine) persistCommit(block *types.Block, qc *types.QuorumCertificate) {
	blockHash := block.Header.BlockHash
	if blockHash.IsZero() {
		blockHash = block.Header.ComputeHash()
	}

	e.logger.Info("committing block",
		zap.Uint64("height", block.Header.Height),
		zap.String("hash", blockHash.String()),
	)

	stateRoot := block.Header.StateRoot

	// Persist to store.
	if e.store != nil {
		if err := e.store.SaveBlock(block, qc); err != nil {
			e.logger.Error("failed to save block", zap.Error(err))
		}
		if err := e.store.SaveCommit(block.Header.Height, stateRoot); err != nil {
			e.logger.Error("failed to save commit", zap.Error(err))
		}
	}

	// Update commit tracking.
	e.state.LastCommitHeight = block.Header.Height
	e.state.LastCommitQC = qc

	// Notify subscribers.
	evt := CommitEvent{
		Block:     block,
		QC:        qc,
		StateRoot: stateRoot,
		Height:    block.Header.Height,
	}
	select {
	case e.commitCh <- evt:
	default:
		// Don't block if no one is listening.
	}
}

// advanceHeight moves the engine to the next height while preserving
// lock state and highest QC, then signals the event loop to start the new round.
func (e *Engine) advanceHeight() {
	nextHeight := e.state.Height + 1
	lockedBlock := e.state.LockedBlock
	lockedRound := e.state.LockedRound
	highestQC := e.state.HighestQC
	lastCommitHeight := e.state.LastCommitHeight
	lastCommitQC := e.state.LastCommitQC

	e.state.ResetForNewHeight(nextHeight, e.valSet)

	// Preserve locks and QC state across height advancement.
	e.state.LockedBlock = lockedBlock
	e.state.LockedRound = lockedRound
	e.state.HighestQC = highestQC
	e.state.LastCommitHeight = lastCommitHeight
	e.state.LastCommitQC = lastCommitQC

	// Signal event loop to start the next height asynchronously.
	// This breaks the synchronous recursion that would otherwise occur
	// when a single validator can form QCs alone.
	select {
	case e.nextHeightCh <- struct{}{}:
	default:
	}
}

// scheduleRoundTimeout starts the round timer.
func (e *Engine) scheduleRoundTimeout() {
	ch := e.timeouts.ScheduleTimeout(e.state.Round)
	height := e.state.Height
	round := e.state.Round

	go func() {
		<-ch
		select {
		case e.timeoutCh <- timeoutEvent{Height: height, Round: round}:
		default:
		}
	}()
}
