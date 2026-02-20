package consensus

import (
	"fmt"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// CreateProposal builds a new block proposal.
// Per SPEC.md §6:
// 1. Select transactions from mempool
// 2. Build block referencing highest QC
// 3. Execute block to compute state root
// 4. Sign proposal
func (e *Engine) CreateProposal() (*types.Proposal, error) {
	// Gather transactions from mempool.
	var txs [][]byte
	if e.txProvider != nil {
		txs = e.txProvider.ReapMaxTxs(1 << 20) // 1MB max
	}

	// Determine parent hash from highest QC or last committed block.
	parentHash := types.ZeroHash
	if e.state.HighestQC != nil {
		parentHash = e.state.HighestQC.BlockHash
	}

	// Compute the previous state root for execution.
	prevStateRoot := types.ZeroHash
	if e.store != nil {
		if sr, err := e.store.GetCommitStateRoot(e.state.Height - 1); err == nil {
			prevStateRoot = sr
		}
	}

	// Compute tx root.
	txRoot := crypto.ComputeTxRoot(txs)

	// Build the block header.
	header := types.BlockHeader{
		Height:     e.state.Height,
		Round:      e.state.Round,
		ParentHash: parentHash,
		TxRoot:     txRoot,
		ProposerID: e.address,
		BlockTime:  uint64(time.Now().UnixMilli()),
		ChainID:    e.chainID,
	}

	block := &types.Block{
		Header:       header,
		Transactions: txs,
		QC:           e.state.HighestQC,
	}

	// Execute the block to compute the state root.
	if e.executor != nil {
		result, err := e.executor.ExecuteBlock(block, prevStateRoot)
		if err != nil {
			return nil, fmt.Errorf("execute block: %w", err)
		}
		block.Header.StateRoot = result.StateRoot
	}

	// Compute block hash after all fields are set.
	block.Header.BlockHash = block.Header.ComputeHash()

	// Sign the proposal.
	payload := (&types.Proposal{Block: block, Round: e.state.Round}).SigningPayload()
	sig := crypto.Sign(e.privKey, payload)

	proposal := &types.Proposal{
		Block:      block,
		Round:      e.state.Round,
		ProposerID: e.address,
		Signature:  crypto.SigTo64(sig),
	}

	return proposal, nil
}

// ValidateProposal checks a received proposal.
// Per SPEC.md §6 (block validity rules):
// - Correct height and round
// - Proposer matches expected for this round
// - QC valid (if present)
// - Block structural validity
func (e *Engine) ValidateProposal(proposal *types.Proposal) error {
	if proposal == nil || proposal.Block == nil {
		return fmt.Errorf("nil proposal or block")
	}

	block := proposal.Block

	// Check height matches.
	if block.Header.Height != e.state.Height {
		return fmt.Errorf("proposal height %d != expected %d", block.Header.Height, e.state.Height)
	}

	// Check round matches.
	if proposal.Round != e.state.Round {
		return fmt.Errorf("proposal round %d != expected %d", proposal.Round, e.state.Round)
	}

	// Verify proposer is the expected one for this round.
	expectedProposer := e.valSet.GetProposer(e.state.Height, e.state.Round)
	if expectedProposer == nil {
		return fmt.Errorf("no proposer for (h=%d, r=%d)", e.state.Height, e.state.Round)
	}
	if proposal.ProposerID != expectedProposer.Address {
		return fmt.Errorf("wrong proposer: got %s, expected %s",
			proposal.ProposerID, expectedProposer.Address)
	}

	// Verify proposal signature.
	payload := proposal.SigningPayload()
	if !crypto.Verify(expectedProposer.PublicKey[:], payload, proposal.Signature[:]) {
		return fmt.Errorf("invalid proposal signature")
	}

	// Verify embedded QC if present.
	if block.QC != nil {
		if err := block.QC.Verify(e.valSet); err != nil {
			return fmt.Errorf("embedded QC invalid: %w", err)
		}
	}

	// Check locking rule (SPEC.md §9):
	// A locked validator only votes for blocks that:
	// 1. Extend the locked block, OR
	// 2. Have a QC at a round higher than LockedRound (justifying unlock)
	if e.state.IsLocked() {
		lockedHash := e.state.LockedBlock.Header.BlockHash
		if lockedHash.IsZero() {
			lockedHash = e.state.LockedBlock.Header.ComputeHash()
		}

		extendsLocked := block.Header.ParentHash == lockedHash
		hasHigherQC := block.QC != nil && block.QC.Round > e.state.LockedRound

		if !extendsLocked && !hasHigherQC {
			return fmt.Errorf("proposal does not extend locked block and has no higher QC (locked_round=%d)",
				e.state.LockedRound)
		}
	}

	return nil
}
