package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// GenesisDoc defines the initial state of the chain.
type GenesisDoc struct {
	ChainID         string            `json:"chain_id"`
	GenesisTime     time.Time         `json:"genesis_time"`
	Validators      []GenesisValidator `json:"validators"`
	AppStateRoot    string            `json:"app_state_root"`
	ConsensusParams ConsensusParams   `json:"consensus_params"`
}

// GenesisValidator describes a validator in the genesis state.
type GenesisValidator struct {
	Address string `json:"address"`
	PubKey  string `json:"pub_key"`
	Power   uint64 `json:"power"`
	Name    string `json:"name"`
}

// ConsensusParams holds genesis-level consensus parameters.
type ConsensusParams struct {
	MaxBlockSize  int    `json:"max_block_size"`
	MaxBlockGas   uint64 `json:"max_block_gas"`
	MaxValidators int    `json:"max_validators"`
}

// LoadGenesis reads and validates a genesis file from the given path.
func LoadGenesis(path string) (*GenesisDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("genesis: read file: %w", err)
	}

	var gen GenesisDoc
	if err := json.Unmarshal(data, &gen); err != nil {
		return nil, fmt.Errorf("genesis: parse JSON: %w", err)
	}

	if err := gen.Validate(); err != nil {
		return nil, fmt.Errorf("genesis: %w", err)
	}

	return &gen, nil
}

// Validate checks the genesis document for structural validity.
func (g *GenesisDoc) Validate() error {
	if g.ChainID == "" {
		return errors.New("chain_id must not be empty")
	}
	if g.GenesisTime.IsZero() {
		return errors.New("genesis_time must not be zero")
	}
	if len(g.Validators) == 0 {
		return errors.New("must have at least one validator")
	}

	for i, v := range g.Validators {
		if v.Address == "" {
			return fmt.Errorf("validator %d: address must not be empty", i)
		}
		if v.PubKey == "" {
			return fmt.Errorf("validator %d: pub_key must not be empty", i)
		}
		if v.Power == 0 {
			return fmt.Errorf("validator %d: power must be > 0", i)
		}

		// Validate hex encoding.
		if _, err := hex.DecodeString(v.Address); err != nil {
			return fmt.Errorf("validator %d: invalid address hex: %w", i, err)
		}
		if _, err := hex.DecodeString(v.PubKey); err != nil {
			return fmt.Errorf("validator %d: invalid pub_key hex: %w", i, err)
		}
	}

	if g.ConsensusParams.MaxValidators <= 0 {
		return errors.New("consensus_params.max_validators must be > 0")
	}
	if len(g.Validators) > g.ConsensusParams.MaxValidators {
		return fmt.Errorf("too many validators: got %d, max %d",
			len(g.Validators), g.ConsensusParams.MaxValidators)
	}

	return nil
}

// ToValidatorSet converts the genesis validators to a runtime ValidatorSet.
func (g *GenesisDoc) ToValidatorSet() (*types.ValidatorSet, error) {
	validators := make([]types.Validator, len(g.Validators))
	for i, gv := range g.Validators {
		addrBytes, err := hex.DecodeString(gv.Address)
		if err != nil {
			return nil, fmt.Errorf("validator %d: invalid address hex: %w", i, err)
		}
		addr, err := types.AddressFromBytes(addrBytes)
		if err != nil {
			return nil, fmt.Errorf("validator %d: %w", i, err)
		}

		pubKeyBytes, err := hex.DecodeString(gv.PubKey)
		if err != nil {
			return nil, fmt.Errorf("validator %d: invalid pub_key hex: %w", i, err)
		}
		if len(pubKeyBytes) != 32 {
			return nil, fmt.Errorf("validator %d: pub_key must be 32 bytes, got %d", i, len(pubKeyBytes))
		}

		var pubKey [32]byte
		copy(pubKey[:], pubKeyBytes)

		validators[i] = types.Validator{
			Address:     addr,
			PublicKey:   pubKey,
			VotingPower: gv.Power,
		}
	}

	return types.NewValidatorSet(validators)
}

// AppStateRootHash parses the hex-encoded app state root into a Hash.
func (g *GenesisDoc) AppStateRootHash() (types.Hash, error) {
	if g.AppStateRoot == "" {
		return types.ZeroHash, nil
	}
	return types.HashFromHex(g.AppStateRoot)
}
