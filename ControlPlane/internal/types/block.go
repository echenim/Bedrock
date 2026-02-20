package types

import (
	"crypto/sha256"
	"errors"
	"fmt"

	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
	"google.golang.org/protobuf/proto"
)

// BlockHeader contains block metadata for consensus.
type BlockHeader struct {
	Height     uint64
	Round      uint64
	ParentHash Hash
	StateRoot  Hash
	TxRoot     Hash
	ProposerID Address
	BlockTime  uint64
	ChainID    []byte
	BlockHash  Hash
}

// Block wraps the protobuf Block with domain methods.
type Block struct {
	Header       BlockHeader
	Transactions [][]byte
	QC           *QuorumCertificate
}

// ComputeHash computes the canonical block hash: SHA-256 over the deterministic
// protobuf serialization of the header.
// Per SPEC.md §3 and implementation notes.
func (h *BlockHeader) ComputeHash() Hash {
	pb := &typesv1.BlockHeader{
		Height:     h.Height,
		Round:      h.Round,
		ParentHash: h.ParentHash[:],
		StateRoot:  h.StateRoot[:],
		TxRoot:     h.TxRoot[:],
		ProposerId: h.ProposerID[:],
		BlockTime:  h.BlockTime,
		ChainId:    h.ChainID,
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
	if err != nil {
		// proto.Marshal on a well-formed message should not fail.
		panic(fmt.Sprintf("types: failed to marshal block header: %v", err))
	}
	return sha256.Sum256(data)
}

// Validate checks structural validity of the block.
func (b *Block) Validate() error {
	if b.Header.Height == 0 && b.Header.Round == 0 && b.Header.ParentHash.IsZero() {
		// Genesis block — allow.
		return nil
	}
	if b.Header.Height == 0 {
		return errors.New("block height must be > 0 for non-genesis blocks")
	}
	if len(b.Header.ChainID) == 0 {
		return errors.New("block chain_id must not be empty")
	}
	if b.Header.ProposerID.IsZero() {
		return errors.New("block proposer_id must not be zero")
	}
	return nil
}

// ToProto converts the Block to its protobuf representation.
func (b *Block) ToProto() *typesv1.Block {
	pb := &typesv1.Block{
		Header: &typesv1.BlockHeader{
			Height:     b.Header.Height,
			Round:      b.Header.Round,
			ParentHash: b.Header.ParentHash[:],
			StateRoot:  b.Header.StateRoot[:],
			TxRoot:     b.Header.TxRoot[:],
			ProposerId: b.Header.ProposerID[:],
			BlockTime:  b.Header.BlockTime,
			ChainId:    b.Header.ChainID,
			BlockHash:  b.Header.BlockHash[:],
		},
	}

	pb.Transactions = make([]*typesv1.Transaction, len(b.Transactions))
	for i, tx := range b.Transactions {
		pb.Transactions[i] = &typesv1.Transaction{Data: tx}
	}

	if b.QC != nil {
		pb.Qc = b.QC.ToProto()
	}
	return pb
}

// BlockFromProto converts a protobuf Block to the domain type.
func BlockFromProto(pb *typesv1.Block) (*Block, error) {
	if pb == nil {
		return nil, errors.New("nil protobuf block")
	}
	if pb.Header == nil {
		return nil, errors.New("nil protobuf block header")
	}

	header, err := blockHeaderFromProto(pb.Header)
	if err != nil {
		return nil, fmt.Errorf("block header: %w", err)
	}

	b := &Block{Header: *header}

	b.Transactions = make([][]byte, len(pb.Transactions))
	for i, tx := range pb.Transactions {
		b.Transactions[i] = tx.Data
	}

	if pb.Qc != nil {
		qc, err := QCFromProto(pb.Qc)
		if err != nil {
			return nil, fmt.Errorf("qc: %w", err)
		}
		b.QC = qc
	}

	return b, nil
}

func blockHeaderFromProto(pb *typesv1.BlockHeader) (*BlockHeader, error) {
	parentHash, err := HashFromBytes(pb.ParentHash)
	if err != nil && len(pb.ParentHash) > 0 {
		return nil, fmt.Errorf("parent_hash: %w", err)
	}

	stateRoot, err := HashFromBytes(pb.StateRoot)
	if err != nil && len(pb.StateRoot) > 0 {
		return nil, fmt.Errorf("state_root: %w", err)
	}

	txRoot, err := HashFromBytes(pb.TxRoot)
	if err != nil && len(pb.TxRoot) > 0 {
		return nil, fmt.Errorf("tx_root: %w", err)
	}

	proposerID, err := AddressFromBytes(pb.ProposerId)
	if err != nil && len(pb.ProposerId) > 0 {
		return nil, fmt.Errorf("proposer_id: %w", err)
	}

	blockHash, err := HashFromBytes(pb.BlockHash)
	if err != nil && len(pb.BlockHash) > 0 {
		return nil, fmt.Errorf("block_hash: %w", err)
	}

	return &BlockHeader{
		Height:     pb.Height,
		Round:      pb.Round,
		ParentHash: parentHash,
		StateRoot:  stateRoot,
		TxRoot:     txRoot,
		ProposerID: proposerID,
		BlockTime:  pb.BlockTime,
		ChainID:    pb.ChainId,
		BlockHash:  blockHash,
	}, nil
}
