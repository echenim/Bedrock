package types

import (
	"encoding/binary"
	"errors"
	"fmt"

	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
)

// Proposal is broadcast by the round leader containing a block and signature.
type Proposal struct {
	Block      *Block
	Round      uint64
	ProposerID Address
	Signature  [64]byte
}

// SigningPayload returns the canonical bytes to sign for this proposal.
// Format: block_hash(32) || round(8 LE)
func (p *Proposal) SigningPayload() []byte {
	buf := make([]byte, 32+8)
	blockHash := p.Block.Header.BlockHash
	if blockHash.IsZero() {
		blockHash = p.Block.Header.ComputeHash()
	}
	copy(buf[:32], blockHash[:])
	binary.LittleEndian.PutUint64(buf[32:40], p.Round)
	return buf
}

// ToProto converts the Proposal to its protobuf representation.
func (p *Proposal) ToProto() *typesv1.Proposal {
	return &typesv1.Proposal{
		Block:      p.Block.ToProto(),
		Round:      p.Round,
		ProposerId: p.ProposerID[:],
		Signature:  p.Signature[:],
	}
}

// ProposalFromProto converts a protobuf Proposal to the domain type.
func ProposalFromProto(pb *typesv1.Proposal) (*Proposal, error) {
	if pb == nil {
		return nil, errors.New("nil protobuf proposal")
	}
	block, err := BlockFromProto(pb.Block)
	if err != nil {
		return nil, fmt.Errorf("proposal block: %w", err)
	}
	proposerID, err := AddressFromBytes(pb.ProposerId)
	if err != nil {
		return nil, fmt.Errorf("proposal proposer_id: %w", err)
	}
	var sig [64]byte
	if len(pb.Signature) == 64 {
		copy(sig[:], pb.Signature)
	}
	return &Proposal{
		Block:      block,
		Round:      pb.Round,
		ProposerID: proposerID,
		Signature:  sig,
	}, nil
}

// TimeoutMessage is sent when a validator's round timer expires.
type TimeoutMessage struct {
	Height    uint64
	Round     uint64
	VoterID   Address
	Signature [64]byte
	HighQC    *QuorumCertificate
}

// SigningPayload returns the canonical bytes to sign for this timeout message.
// Format: height(8 LE) || round(8 LE)
func (tm *TimeoutMessage) SigningPayload() []byte {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[:8], tm.Height)
	binary.LittleEndian.PutUint64(buf[8:16], tm.Round)
	return buf
}

// ToProto converts the TimeoutMessage to its protobuf representation.
func (tm *TimeoutMessage) ToProto() *typesv1.TimeoutMessage {
	pb := &typesv1.TimeoutMessage{
		Height:    tm.Height,
		Round:     tm.Round,
		VoterId:   tm.VoterID[:],
		Signature: tm.Signature[:],
	}
	if tm.HighQC != nil {
		pb.HighQc = tm.HighQC.ToProto()
	}
	return pb
}

// TimeoutMessageFromProto converts a protobuf TimeoutMessage to the domain type.
func TimeoutMessageFromProto(pb *typesv1.TimeoutMessage) (*TimeoutMessage, error) {
	if pb == nil {
		return nil, errors.New("nil protobuf timeout message")
	}
	voterID, err := AddressFromBytes(pb.VoterId)
	if err != nil {
		return nil, fmt.Errorf("timeout voter_id: %w", err)
	}
	var sig [64]byte
	if len(pb.Signature) == 64 {
		copy(sig[:], pb.Signature)
	}
	tm := &TimeoutMessage{
		Height:    pb.Height,
		Round:     pb.Round,
		VoterID:   voterID,
		Signature: sig,
	}
	if pb.HighQc != nil {
		qc, err := QCFromProto(pb.HighQc)
		if err != nil {
			return nil, fmt.Errorf("timeout high_qc: %w", err)
		}
		tm.HighQC = qc
	}
	return tm, nil
}

// SlashingEvidence wraps evidence of validator misbehaviour.
type SlashingEvidence struct {
	DoubleVote     *DoubleVoteEvidence
	DoubleProposal *DoubleProposalEvidence
	Height         uint64
	Timestamp      uint64
}

// DoubleVoteEvidence proves a validator voted for two different blocks
// in the same round.
type DoubleVoteEvidence struct {
	VoteA       *Vote
	VoteB       *Vote
	ValidatorID Address
}

// DoubleProposalEvidence proves a validator proposed two different blocks
// in the same round.
type DoubleProposalEvidence struct {
	ProposalA   *Proposal
	ProposalB   *Proposal
	ValidatorID Address
}
