package consensus

import (
	"errors"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// VoteSet collects votes for a specific (height, round).
type VoteSet struct {
	height    uint64
	round     uint64
	valSet    *types.ValidatorSet
	votes     map[types.Address]*types.Vote
	votePower uint64
}

// NewVoteSet creates a new VoteSet for the given height and round.
func NewVoteSet(height, round uint64, valSet *types.ValidatorSet) *VoteSet {
	return &VoteSet{
		height: height,
		round:  round,
		valSet: valSet,
		votes:  make(map[types.Address]*types.Vote),
	}
}

// AddVote adds a validated vote to the set.
// Returns (quorumReached, evidence, error).
// Per SPEC.md §7: detects equivocation (double voting).
func (vs *VoteSet) AddVote(vote *types.Vote) (bool, *types.SlashingEvidence, error) {
	if vote.Height != vs.height || vote.Round != vs.round {
		return false, nil, fmt.Errorf("vote for (h=%d, r=%d) does not match set (h=%d, r=%d)",
			vote.Height, vote.Round, vs.height, vs.round)
	}

	// Look up validator.
	val, ok := vs.valSet.GetByAddress(vote.VoterID)
	if !ok {
		return false, nil, fmt.Errorf("vote from unknown validator %s", vote.VoterID)
	}

	// Verify signature.
	if !vote.Verify(val.PublicKey) {
		return false, nil, errors.New("invalid vote signature")
	}

	// Check for equivocation.
	if existing, found := vs.votes[vote.VoterID]; found {
		if types.IsEquivocation(existing, vote) {
			evidence := &types.SlashingEvidence{
				DoubleVote: &types.DoubleVoteEvidence{
					VoteA:       existing,
					VoteB:       vote,
					ValidatorID: vote.VoterID,
				},
				Height: vote.Height,
			}
			return false, evidence, fmt.Errorf("equivocation detected from %s", vote.VoterID)
		}
		// Duplicate vote for same block — ignore.
		return vs.HasQuorum(), nil, nil
	}

	// Add the vote.
	vs.votes[vote.VoterID] = vote
	vs.votePower += val.VotingPower

	return vs.HasQuorum(), nil, nil
}

// HasQuorum returns true if collected votes have >= 2f+1 power.
func (vs *VoteSet) HasQuorum() bool {
	return vs.valSet.HasQuorum(vs.votePower)
}

// VotingPower returns the total power of collected votes.
func (vs *VoteSet) VotingPower() uint64 {
	return vs.votePower
}

// Size returns the number of votes collected.
func (vs *VoteSet) Size() int {
	return len(vs.votes)
}

// GetVotes returns all collected votes.
func (vs *VoteSet) GetVotes() []*types.Vote {
	votes := make([]*types.Vote, 0, len(vs.votes))
	for _, v := range vs.votes {
		votes = append(votes, v)
	}
	return votes
}
