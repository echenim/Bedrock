package types

import (
	"errors"
	"fmt"

	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
)

// Validator describes a validator in the active set.
type Validator struct {
	Address     Address
	PublicKey   [32]byte
	VotingPower uint64
}

// ValidatorSet manages the active validator set.
type ValidatorSet struct {
	Validators []Validator
	TotalPower uint64
}

// NewValidatorSet creates a ValidatorSet from a slice of validators,
// computing TotalPower automatically.
func NewValidatorSet(validators []Validator) (*ValidatorSet, error) {
	if len(validators) == 0 {
		return nil, errors.New("validator set must not be empty")
	}

	var total uint64
	for _, v := range validators {
		if v.VotingPower == 0 {
			return nil, fmt.Errorf("validator %s has zero voting power", v.Address)
		}
		total += v.VotingPower
	}

	return &ValidatorSet{
		Validators: validators,
		TotalPower: total,
	}, nil
}

// Quorum returns the quorum threshold: 2f+1 where f = (totalPower - 1) / 3.
// Per SPEC.md §17 and SPEC-v0.2.md §4.
func (vs *ValidatorSet) Quorum() uint64 {
	f := (vs.TotalPower - 1) / 3
	return 2*f + 1
}

// HasQuorum checks if votingPower >= Quorum().
func (vs *ValidatorSet) HasQuorum(votingPower uint64) bool {
	return votingPower >= vs.Quorum()
}

// GetProposer returns the proposer for (height, round).
// Deterministic rotation: proposer_index = (height + round) % len(validators).
// Per SPEC.md §6.
func (vs *ValidatorSet) GetProposer(height, round uint64) *Validator {
	if len(vs.Validators) == 0 {
		return nil
	}
	idx := (height + round) % uint64(len(vs.Validators))
	return &vs.Validators[idx]
}

// GetByAddress looks up a validator by address.
func (vs *ValidatorSet) GetByAddress(addr Address) (*Validator, bool) {
	for i := range vs.Validators {
		if vs.Validators[i].Address == addr {
			return &vs.Validators[i], true
		}
	}
	return nil, false
}

// Size returns the number of validators.
func (vs *ValidatorSet) Size() int {
	return len(vs.Validators)
}

// ToProto converts the ValidatorSet to its protobuf representation.
func (vs *ValidatorSet) ToProto() *typesv1.ValidatorSet {
	pb := &typesv1.ValidatorSet{
		TotalVotingPower: vs.TotalPower,
	}
	pb.Validators = make([]*typesv1.Validator, len(vs.Validators))
	for i, v := range vs.Validators {
		pb.Validators[i] = &typesv1.Validator{
			Address:     v.Address[:],
			PublicKey:   v.PublicKey[:],
			VotingPower: v.VotingPower,
		}
	}
	return pb
}

// ValidatorSetFromProto converts a protobuf ValidatorSet to the domain type.
func ValidatorSetFromProto(pb *typesv1.ValidatorSet) (*ValidatorSet, error) {
	if pb == nil {
		return nil, errors.New("nil protobuf validator set")
	}

	validators := make([]Validator, len(pb.Validators))
	for i, v := range pb.Validators {
		addr, err := AddressFromBytes(v.Address)
		if err != nil {
			return nil, fmt.Errorf("validator %d address: %w", i, err)
		}

		var pubKey [32]byte
		if len(v.PublicKey) == 32 {
			copy(pubKey[:], v.PublicKey)
		} else if len(v.PublicKey) > 0 {
			return nil, fmt.Errorf("validator %d: invalid public key length: got %d, want 32", i, len(v.PublicKey))
		}

		validators[i] = Validator{
			Address:     addr,
			PublicKey:   pubKey,
			VotingPower: v.VotingPower,
		}
	}

	return NewValidatorSet(validators)
}
