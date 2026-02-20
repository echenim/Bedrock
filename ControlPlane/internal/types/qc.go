package types

import (
	"errors"
	"fmt"

	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
)

// QuorumCertificate proves that >= 2f+1 validators voted for a block.
// See SPEC.md §4.2.
type QuorumCertificate struct {
	BlockHash Hash
	Round     uint64
	Votes     []Vote
}

// Verify checks that the QC has >= quorum valid signatures from the given
// validator set. Returns an error if the QC is invalid.
func (qc *QuorumCertificate) Verify(valSet *ValidatorSet) error {
	if valSet == nil {
		return errors.New("nil validator set")
	}
	if len(qc.Votes) == 0 {
		return errors.New("QC has no votes")
	}

	// Track which validators have voted to prevent duplicates.
	seen := make(map[Address]bool)
	var votingPower uint64

	for i, vote := range qc.Votes {
		// All votes must be for the same block.
		if vote.BlockHash != qc.BlockHash {
			return fmt.Errorf("vote %d: block hash mismatch: want %s, got %s",
				i, qc.BlockHash, vote.BlockHash)
		}

		// Look up validator.
		val, ok := valSet.GetByAddress(vote.VoterID)
		if !ok {
			return fmt.Errorf("vote %d: unknown validator %s", i, vote.VoterID)
		}

		// Check for duplicate votes.
		if seen[vote.VoterID] {
			return fmt.Errorf("vote %d: duplicate vote from %s", i, vote.VoterID)
		}
		seen[vote.VoterID] = true

		// Verify signature.
		if !vote.Verify(val.PublicKey) {
			return fmt.Errorf("vote %d: invalid signature from %s", i, vote.VoterID)
		}

		votingPower += val.VotingPower
	}

	if !valSet.HasQuorum(votingPower) {
		return fmt.Errorf("insufficient voting power: got %d, need %d", votingPower, valSet.Quorum())
	}

	return nil
}

// VotingPower returns the total voting power of all signers in the QC
// that are present in the validator set.
func (qc *QuorumCertificate) VotingPower(valSet *ValidatorSet) uint64 {
	var power uint64
	for _, vote := range qc.Votes {
		if val, ok := valSet.GetByAddress(vote.VoterID); ok {
			power += val.VotingPower
		}
	}
	return power
}

// ToProto converts the QuorumCertificate to its protobuf representation.
func (qc *QuorumCertificate) ToProto() *typesv1.QuorumCertificate {
	pb := &typesv1.QuorumCertificate{
		BlockHash: qc.BlockHash[:],
		Round:     qc.Round,
	}
	pb.Votes = make([]*typesv1.VoteSignature, len(qc.Votes))
	for i, v := range qc.Votes {
		pb.Votes[i] = &typesv1.VoteSignature{
			VoterId:   v.VoterID[:],
			Signature: v.Signature[:],
		}
	}
	return pb
}

// QCFromProto converts a protobuf QuorumCertificate to the domain type.
func QCFromProto(pb *typesv1.QuorumCertificate) (*QuorumCertificate, error) {
	if pb == nil {
		return nil, errors.New("nil protobuf QC")
	}

	blockHash, err := HashFromBytes(pb.BlockHash)
	if err != nil && len(pb.BlockHash) > 0 {
		return nil, fmt.Errorf("block_hash: %w", err)
	}

	qc := &QuorumCertificate{
		BlockHash: blockHash,
		Round:     pb.Round,
	}

	qc.Votes = make([]Vote, len(pb.Votes))
	for i, vs := range pb.Votes {
		voterID, err := AddressFromBytes(vs.VoterId)
		if err != nil {
			return nil, fmt.Errorf("vote %d voter_id: %w", i, err)
		}

		var sig [64]byte
		if len(vs.Signature) == 64 {
			copy(sig[:], vs.Signature)
		}

		qc.Votes[i] = Vote{
			BlockHash: blockHash,
			Round:     pb.Round,
			VoterID:   voterID,
			Signature: sig,
		}
	}

	return qc, nil
}
