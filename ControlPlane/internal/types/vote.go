package types

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"

	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
)

// Vote represents a validator's vote for a block.
type Vote struct {
	BlockHash Hash
	Height    uint64
	Round     uint64
	VoterID   Address
	Signature [64]byte
}

// SigningPayload returns the canonical bytes to sign for this vote.
// Format: block_hash(32) || height(8 LE) || round(8 LE)
func (v *Vote) SigningPayload() []byte {
	buf := make([]byte, 32+8+8)
	copy(buf[:32], v.BlockHash[:])
	binary.LittleEndian.PutUint64(buf[32:40], v.Height)
	binary.LittleEndian.PutUint64(buf[40:48], v.Round)
	return buf
}

// Verify checks the vote signature against the voter's public key.
func (v *Vote) Verify(pubKey [32]byte) bool {
	if v.Signature == [64]byte{} {
		return false
	}
	payload := v.SigningPayload()
	return ed25519.Verify(pubKey[:], payload, v.Signature[:])
}

// IsEquivocation checks if two votes from the same voter conflict.
// Per SPEC.md §7: same voter, same height, same round, different block hash.
func IsEquivocation(a, b *Vote) bool {
	return a.VoterID == b.VoterID &&
		a.Height == b.Height &&
		a.Round == b.Round &&
		a.BlockHash != b.BlockHash
}

// ToProto converts the Vote to its protobuf representation.
func (v *Vote) ToProto() *typesv1.Vote {
	return &typesv1.Vote{
		BlockHash: v.BlockHash[:],
		Height:    v.Height,
		Round:     v.Round,
		VoterId:   v.VoterID[:],
		Signature: v.Signature[:],
	}
}

// VoteFromProto converts a protobuf Vote to the domain type.
func VoteFromProto(pb *typesv1.Vote) (*Vote, error) {
	if pb == nil {
		return nil, errors.New("nil protobuf vote")
	}

	blockHash, err := HashFromBytes(pb.BlockHash)
	if err != nil {
		return nil, fmt.Errorf("block_hash: %w", err)
	}

	voterID, err := AddressFromBytes(pb.VoterId)
	if err != nil {
		return nil, fmt.Errorf("voter_id: %w", err)
	}

	var sig [64]byte
	if len(pb.Signature) == 64 {
		copy(sig[:], pb.Signature)
	} else if len(pb.Signature) > 0 {
		return nil, fmt.Errorf("invalid signature length: got %d, want 64", len(pb.Signature))
	}

	return &Vote{
		BlockHash: blockHash,
		Height:    pb.Height,
		Round:     pb.Round,
		VoterID:   voterID,
		Signature: sig,
	}, nil
}
