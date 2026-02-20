package consensus

import "github.com/echenim/Bedrock/controlplane/internal/types"

// CheckCommitRule implements SPEC.md §8 (two-chain commit rule).
// A block B is committed if:
//  1. B has a QC (blockQC)
//  2. B's parent also has a QC (represented by state.HighestQC or the QC
//     embedded in B referencing the parent)
//  3. The QC rounds are consecutive or the lock conditions are satisfied
//
// In practice for our protocol:
//   - When we form a QC for the current block, we check if the current block's
//     embedded QC (the QC it references from its parent) proves the parent also
//     had a QC. If so, the parent block is ready to commit.
//
// Returns (shouldCommit, blockToCommit).
func CheckCommitRule(
	block *types.Block,
	blockQC *types.QuorumCertificate,
	state *ConsensusState,
) (bool, *types.Block) {
	if blockQC == nil {
		return false, nil
	}

	// Two-chain rule: the block being committed is the one whose QC is embedded
	// inside the current block (i.e., the parent).
	// If the current block has a QC, and the current block itself references
	// a valid QC for its parent (via block.QC), then the parent can be committed.

	// The block that's ready to commit is the locked block (or the parent of
	// the current proposal).
	if state.LockedBlock != nil && state.HighestQC != nil {
		// If we have a locked block with a QC (HighestQC), and now we have
		// a new QC (blockQC) extending that locked block, the locked block
		// can be committed.
		if blockQC.Round > state.HighestQC.Round {
			return true, state.LockedBlock
		}
	}

	// Direct commit: if block references a parent QC and we now have
	// a QC for this block, the parent is committed.
	if block.QC != nil {
		return true, nil // caller must look up parent block
	}

	return false, nil
}

// ShouldCommitOnQC is a simplified commit check:
// when a QC is formed, check if we should commit the predecessor.
// Returns true and the block to commit if the two-chain rule is satisfied.
func ShouldCommitOnQC(
	newQC *types.QuorumCertificate,
	currentBlock *types.Block,
	lockedBlock *types.Block,
) (bool, *types.Block) {
	// The two-chain rule means: when a QC forms for block at height H,
	// the block at height H-1 (which had its own QC embedded in block H)
	// is now committed.
	if lockedBlock != nil && currentBlock != nil {
		if currentBlock.QC != nil {
			return true, lockedBlock
		}
	}
	return false, nil
}
