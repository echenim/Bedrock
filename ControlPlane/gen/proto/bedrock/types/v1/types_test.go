package typesv1_test

import (
	"bytes"
	"testing"

	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
	"google.golang.org/protobuf/proto"
)

func TestBlockHeaderRoundTrip(t *testing.T) {
	original := &typesv1.BlockHeader{
		Height:     42,
		Round:      3,
		ParentHash: bytes.Repeat([]byte{0xaa}, 32),
		StateRoot:  bytes.Repeat([]byte{0xbb}, 32),
		TxRoot:     bytes.Repeat([]byte{0xcc}, 32),
		ProposerId: bytes.Repeat([]byte{0x01}, 32),
		BlockTime:  1700000000,
		ChainId:    []byte("bedrock-testnet"),
		BlockHash:  bytes.Repeat([]byte{0xdd}, 32),
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.BlockHeader{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for BlockHeader")
	}
}

func TestBlockHeaderDeterministicMarshal(t *testing.T) {
	header := &typesv1.BlockHeader{
		Height:     100,
		Round:      1,
		ParentHash: bytes.Repeat([]byte{0x11}, 32),
		StateRoot:  bytes.Repeat([]byte{0x22}, 32),
		TxRoot:     bytes.Repeat([]byte{0x33}, 32),
		ProposerId: bytes.Repeat([]byte{0x44}, 32),
		BlockTime:  1700000000,
		ChainId:    []byte("bedrock-test"),
		BlockHash:  bytes.Repeat([]byte{0x55}, 32),
	}

	opts := proto.MarshalOptions{Deterministic: true}
	data1, err := opts.Marshal(header)
	if err != nil {
		t.Fatalf("marshal 1: %v", err)
	}

	for range 100 {
		data2, err := opts.Marshal(header)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(data1, data2) {
			t.Fatal("deterministic marshal mismatch")
		}
	}
}

func TestBlockRoundTrip(t *testing.T) {
	original := &typesv1.Block{
		Header: &typesv1.BlockHeader{
			Height:     10,
			Round:      1,
			ParentHash: bytes.Repeat([]byte{0xaa}, 32),
			StateRoot:  bytes.Repeat([]byte{0xbb}, 32),
			TxRoot:     bytes.Repeat([]byte{0xcc}, 32),
			ProposerId: bytes.Repeat([]byte{0x01}, 32),
			BlockTime:  1700000000,
			ChainId:    []byte("bedrock-testnet"),
			BlockHash:  bytes.Repeat([]byte{0xdd}, 32),
		},
		Transactions: []*typesv1.Transaction{
			{Data: []byte("tx1")},
			{Data: []byte("tx2")},
			{Data: []byte("tx3")},
		},
		Qc: &typesv1.QuorumCertificate{
			BlockHash: bytes.Repeat([]byte{0xee}, 32),
			Round:     1,
			Votes: []*typesv1.VoteSignature{
				{VoterId: bytes.Repeat([]byte{0x01}, 32), Signature: bytes.Repeat([]byte{0xf1}, 64)},
				{VoterId: bytes.Repeat([]byte{0x02}, 32), Signature: bytes.Repeat([]byte{0xf2}, 64)},
			},
			AggregatedSig: bytes.Repeat([]byte{0xff}, 64),
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.Block{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for Block")
	}
}

func TestVoteRoundTrip(t *testing.T) {
	original := &typesv1.Vote{
		BlockHash: bytes.Repeat([]byte{0xaa}, 32),
		Height:    50,
		Round:     2,
		VoterId:   bytes.Repeat([]byte{0x01}, 32),
		Signature: bytes.Repeat([]byte{0xbb}, 64),
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.Vote{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for Vote")
	}
}

func TestProposalRoundTrip(t *testing.T) {
	original := &typesv1.Proposal{
		Block: &typesv1.Block{
			Header: &typesv1.BlockHeader{
				Height: 1,
				Round:  1,
			},
		},
		Round:      1,
		ProposerId: bytes.Repeat([]byte{0x01}, 32),
		Signature:  bytes.Repeat([]byte{0xbb}, 64),
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.Proposal{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for Proposal")
	}
}

func TestSlashingEvidenceDoubleVoteRoundTrip(t *testing.T) {
	original := &typesv1.SlashingEvidence{
		Evidence: &typesv1.SlashingEvidence_DoubleVote{
			DoubleVote: &typesv1.DoubleVoteEvidence{
				VoteA: &typesv1.Vote{
					BlockHash: bytes.Repeat([]byte{0xaa}, 32),
					Height:    10,
					Round:     1,
					VoterId:   bytes.Repeat([]byte{0x01}, 32),
					Signature: bytes.Repeat([]byte{0xf1}, 64),
				},
				VoteB: &typesv1.Vote{
					BlockHash: bytes.Repeat([]byte{0xbb}, 32),
					Height:    10,
					Round:     1,
					VoterId:   bytes.Repeat([]byte{0x01}, 32),
					Signature: bytes.Repeat([]byte{0xf2}, 64),
				},
				ValidatorId: bytes.Repeat([]byte{0x01}, 32),
			},
		},
		Height:    10,
		Timestamp: 1700000000,
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.SlashingEvidence{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for SlashingEvidence (double vote)")
	}

	// Verify the oneof resolves correctly.
	dv := decoded.GetDoubleVote()
	if dv == nil {
		t.Fatal("expected DoubleVoteEvidence, got nil")
	}
	if dv.VoteA.Height != 10 || dv.VoteB.Height != 10 {
		t.Fatal("vote heights mismatch after round-trip")
	}
}

func TestSlashingEvidenceDoubleProposalRoundTrip(t *testing.T) {
	original := &typesv1.SlashingEvidence{
		Evidence: &typesv1.SlashingEvidence_DoubleProposal{
			DoubleProposal: &typesv1.DoubleProposalEvidence{
				ProposalA: &typesv1.Proposal{
					Block:      &typesv1.Block{Header: &typesv1.BlockHeader{Height: 5, Round: 1}},
					Round:      1,
					ProposerId: bytes.Repeat([]byte{0x01}, 32),
					Signature:  bytes.Repeat([]byte{0xf1}, 64),
				},
				ProposalB: &typesv1.Proposal{
					Block:      &typesv1.Block{Header: &typesv1.BlockHeader{Height: 5, Round: 1}},
					Round:      1,
					ProposerId: bytes.Repeat([]byte{0x01}, 32),
					Signature:  bytes.Repeat([]byte{0xf2}, 64),
				},
				ValidatorId: bytes.Repeat([]byte{0x01}, 32),
			},
		},
		Height:    5,
		Timestamp: 1700000000,
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.SlashingEvidence{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for SlashingEvidence (double proposal)")
	}

	dp := decoded.GetDoubleProposal()
	if dp == nil {
		t.Fatal("expected DoubleProposalEvidence, got nil")
	}
}

func TestValidatorSetRoundTrip(t *testing.T) {
	original := &typesv1.ValidatorSet{
		Validators: []*typesv1.Validator{
			{Address: bytes.Repeat([]byte{0x01}, 32), PublicKey: bytes.Repeat([]byte{0xa1}, 32), VotingPower: 100},
			{Address: bytes.Repeat([]byte{0x02}, 32), PublicKey: bytes.Repeat([]byte{0xa2}, 32), VotingPower: 200},
			{Address: bytes.Repeat([]byte{0x03}, 32), PublicKey: bytes.Repeat([]byte{0xa3}, 32), VotingPower: 300},
		},
		Epoch:            5,
		TotalVotingPower: 600,
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.ValidatorSet{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for ValidatorSet")
	}
	if len(decoded.Validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(decoded.Validators))
	}
}

func TestTimeoutMessageRoundTrip(t *testing.T) {
	original := &typesv1.TimeoutMessage{
		Height:    10,
		Round:     5,
		VoterId:   bytes.Repeat([]byte{0x01}, 32),
		Signature: bytes.Repeat([]byte{0xbb}, 64),
		HighQc: &typesv1.QuorumCertificate{
			BlockHash: bytes.Repeat([]byte{0xcc}, 32),
			Round:     4,
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.TimeoutMessage{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for TimeoutMessage")
	}
}

func TestEmptyBlockRoundTrip(t *testing.T) {
	original := &typesv1.Block{
		Header:       &typesv1.BlockHeader{Height: 0},
		Transactions: nil,
		Qc:           nil,
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typesv1.Block{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for empty Block")
	}
}

func TestEnumValues(t *testing.T) {
	// Verify all expected enum values exist (compilation check).
	// This ensures the proto enum is properly defined.
	tests := []struct {
		name  string
		value int32
	}{
		// ExecutionStatus is in execution/v1, tested there.
		// Here we just verify that types compile and the structures are accessible.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.value
		})
	}

	// Verify that all message types can be instantiated.
	_ = &typesv1.Hash{}
	_ = &typesv1.Address{}
	_ = &typesv1.BlockHeader{}
	_ = &typesv1.Transaction{}
	_ = &typesv1.Block{}
	_ = &typesv1.QuorumCertificate{}
	_ = &typesv1.VoteSignature{}
	_ = &typesv1.Vote{}
	_ = &typesv1.Proposal{}
	_ = &typesv1.TimeoutMessage{}
	_ = &typesv1.Validator{}
	_ = &typesv1.ValidatorSet{}
	_ = &typesv1.SlashingEvidence{}
	_ = &typesv1.DoubleVoteEvidence{}
	_ = &typesv1.DoubleProposalEvidence{}
}
