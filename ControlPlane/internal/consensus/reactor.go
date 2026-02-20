package consensus

import (
	"context"

	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// HandleProposal processes a received proposal message.
func (e *Engine) HandleProposal(proposal *types.Proposal) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if proposal == nil || proposal.Block == nil {
		return
	}

	// Ignore proposals for wrong height/round.
	if proposal.Block.Header.Height != e.state.Height {
		e.logger.Debug("ignoring proposal for wrong height",
			zap.Uint64("got", proposal.Block.Header.Height),
			zap.Uint64("want", e.state.Height),
		)
		return
	}
	if proposal.Round != e.state.Round {
		e.logger.Debug("ignoring proposal for wrong round",
			zap.Uint64("got", proposal.Round),
			zap.Uint64("want", e.state.Round),
		)
		return
	}

	// Already have a proposal for this round.
	if e.state.Proposal != nil {
		return
	}

	// Validate proposal.
	if err := e.ValidateProposal(proposal); err != nil {
		e.logger.Warn("invalid proposal", zap.Error(err))
		return
	}

	e.state.Proposal = proposal

	// If we got the proposal and were waiting, move to vote.
	if e.state.Step == StepPropose {
		e.EnterVote()
	}
}

// HandleVote processes a received vote message.
func (e *Engine) HandleVote(vote *types.Vote) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if vote == nil {
		return
	}

	// Ignore votes for wrong height/round.
	if vote.Height != e.state.Height || vote.Round != e.state.Round {
		return
	}

	quorum, evidence, err := e.state.VoteSet.AddVote(vote)
	if err != nil {
		e.logger.Debug("failed to add vote", zap.Error(err))
		return
	}

	if evidence != nil {
		e.logger.Warn("equivocation detected",
			zap.String("validator", vote.VoterID.String()),
		)
		e.evidencePool.AddEvidence(evidence)
	}

	if quorum && e.state.Step == StepVote {
		e.onQuorumReached()
	}
}

// HandleTimeoutMsg processes a received timeout message from a peer.
// If we receive f+1 timeout messages for a round, we also advance.
func (e *Engine) HandleTimeoutMsg(msg *types.TimeoutMessage) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if msg == nil {
		return
	}

	// If the message carries a higher QC, update ours.
	if msg.HighQC != nil {
		if e.state.HighestQC == nil || msg.HighQC.Round > e.state.HighestQC.Round {
			e.state.UpdateHighestQC(msg.HighQC)
		}
	}

	// If this timeout is for our current or future round, consider advancing.
	if msg.Height == e.state.Height && msg.Round > e.state.Round {
		e.logger.Info("received timeout for future round, advancing",
			zap.Uint64("from_round", e.state.Round),
			zap.Uint64("to_round", msg.Round),
		)
		e.EnterNewRound(msg.Round)
	}
}

// eventLoop is the main consensus event loop.
// All state mutations happen through this goroutine to prevent races.
func (e *Engine) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case proposal := <-e.proposalCh:
			e.HandleProposal(proposal)

		case vote := <-e.voteCh:
			e.HandleVote(vote)

		case te := <-e.timeoutCh:
			e.mu.Lock()
			e.HandleTimeout(te.Height, te.Round)
			e.mu.Unlock()

		case <-e.nextHeightCh:
			e.mu.Lock()
			e.EnterPropose()
			e.mu.Unlock()
		}
	}
}

// DrainNextHeight processes a pending next-height signal synchronously.
// Used in tests to step through the two-chain commit rule.
func (e *Engine) DrainNextHeight() bool {
	select {
	case <-e.nextHeightCh:
		e.EnterPropose()
		return true
	default:
		return false
	}
}

// SubmitProposal queues a proposal for processing.
func (e *Engine) SubmitProposal(proposal *types.Proposal) {
	select {
	case e.proposalCh <- proposal:
	default:
		e.logger.Warn("proposal channel full, dropping")
	}
}

// SubmitVote queues a vote for processing.
func (e *Engine) SubmitVote(vote *types.Vote) {
	select {
	case e.voteCh <- vote:
	default:
		e.logger.Warn("vote channel full, dropping")
	}
}
