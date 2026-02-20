package executionv1_test

import (
	"bytes"
	"testing"

	executionv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/execution/v1"
	"google.golang.org/protobuf/proto"
)

func TestExecutionRequestRoundTrip(t *testing.T) {
	original := &executionv1.ExecutionRequest{
		ApiVersion:   1,
		ChainId:      []byte("bedrock-testnet"),
		BlockHeight:  42,
		BlockTime:    1700000000,
		BlockHash:    bytes.Repeat([]byte{0xaa}, 32),
		PrevStateRoot: bytes.Repeat([]byte{0xbb}, 32),
		Transactions: [][]byte{
			[]byte("tx1"),
			[]byte("tx2"),
		},
		Limits: &executionv1.ExecutionLimits{
			GasLimit:      10_000_000,
			MaxEvents:     1024,
			MaxWriteBytes: 65536,
		},
		ExecutionSeed: bytes.Repeat([]byte{0xcc}, 32),
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.ExecutionRequest{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for ExecutionRequest")
	}
}

func TestExecutionRequestDeterministicMarshal(t *testing.T) {
	req := &executionv1.ExecutionRequest{
		ApiVersion:   1,
		ChainId:      []byte("bedrock-testnet"),
		BlockHeight:  100,
		BlockTime:    1700000000,
		BlockHash:    bytes.Repeat([]byte{0x11}, 32),
		PrevStateRoot: bytes.Repeat([]byte{0x22}, 32),
		Transactions: [][]byte{
			[]byte("tx-a"),
			[]byte("tx-b"),
			[]byte("tx-c"),
		},
		Limits: &executionv1.ExecutionLimits{
			GasLimit:      50_000_000,
			MaxEvents:     2048,
			MaxWriteBytes: 131072,
		},
		ExecutionSeed: bytes.Repeat([]byte{0x33}, 32),
	}

	opts := proto.MarshalOptions{Deterministic: true}
	data1, err := opts.Marshal(req)
	if err != nil {
		t.Fatalf("marshal 1: %v", err)
	}

	for range 100 {
		data2, err := opts.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(data1, data2) {
			t.Fatal("deterministic marshal mismatch")
		}
	}
}

func TestExecutionResponseRoundTrip(t *testing.T) {
	original := &executionv1.ExecutionResponse{
		ApiVersion:   1,
		Status:       executionv1.ExecutionStatus_EXECUTION_STATUS_OK,
		NewStateRoot: bytes.Repeat([]byte{0xdd}, 32),
		GasUsed:      4970,
		Receipts: []*executionv1.Receipt{
			{
				TxIndex:    0,
				Success:    true,
				GasUsed:    4970,
				ResultCode: 0,
				ReturnData: nil,
			},
		},
		Events: []*executionv1.Event{
			{
				TxIndex:   0,
				EventType: "transfer",
				Attributes: []*executionv1.EventAttribute{
					{Key: "from", Value: bytes.Repeat([]byte{0x01}, 32)},
					{Key: "to", Value: bytes.Repeat([]byte{0x02}, 32)},
					{Key: "amount", Value: []byte("1000")},
				},
			},
		},
		Logs: []*executionv1.LogLine{
			{Level: 1, Message: "block executed successfully"},
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.ExecutionResponse{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for ExecutionResponse")
	}
}

func TestExecutionResponseDeterministicMarshal(t *testing.T) {
	resp := &executionv1.ExecutionResponse{
		ApiVersion:   1,
		Status:       executionv1.ExecutionStatus_EXECUTION_STATUS_OK,
		NewStateRoot: bytes.Repeat([]byte{0xaa}, 32),
		GasUsed:      14910,
		Receipts: []*executionv1.Receipt{
			{TxIndex: 0, Success: true, GasUsed: 4970, ResultCode: 0},
			{TxIndex: 1, Success: true, GasUsed: 4970, ResultCode: 0},
			{TxIndex: 2, Success: true, GasUsed: 4970, ResultCode: 0},
		},
		Events: []*executionv1.Event{
			{TxIndex: 0, EventType: "transfer"},
			{TxIndex: 1, EventType: "transfer"},
			{TxIndex: 2, EventType: "transfer"},
		},
		Logs: []*executionv1.LogLine{
			{Level: 0, Message: "processing block"},
			{Level: 1, Message: "block committed"},
		},
	}

	opts := proto.MarshalOptions{Deterministic: true}
	data1, err := opts.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal 1: %v", err)
	}

	for range 100 {
		data2, err := opts.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(data1, data2) {
			t.Fatal("deterministic marshal mismatch")
		}
	}
}

func TestReceiptRoundTrip(t *testing.T) {
	original := &executionv1.Receipt{
		TxIndex:    3,
		Success:    false,
		GasUsed:    1500,
		ResultCode: 8,
		ReturnData: []byte("insufficient balance"),
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.Receipt{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for Receipt")
	}
}

func TestEventRoundTrip(t *testing.T) {
	original := &executionv1.Event{
		TxIndex:   0,
		EventType: "transfer",
		Attributes: []*executionv1.EventAttribute{
			{Key: "sender", Value: []byte("alice")},
			{Key: "recipient", Value: []byte("bob")},
			{Key: "amount", Value: []byte("5000")},
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.Event{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for Event")
	}
}

func TestExecutionContextRoundTrip(t *testing.T) {
	original := &executionv1.ExecutionContext{
		ChainId:     []byte("bedrock-testnet"),
		BlockHeight: 100,
		BlockTime:   1700000000,
		BlockHash:   bytes.Repeat([]byte{0xaa}, 32),
		Limits: &executionv1.ExecutionLimits{
			GasLimit:      10_000_000,
			MaxEvents:     1024,
			MaxWriteBytes: 65536,
		},
		ProtocolParams: &executionv1.ProtocolParams{
			ApiVersion:  1,
			MaxTxSize:   1048576,
			MaxBlockGas: 100_000_000,
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.ExecutionContext{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for ExecutionContext")
	}
}

func TestLogLineRoundTrip(t *testing.T) {
	original := &executionv1.LogLine{
		Level:   2,
		Message: "execution error: out of gas at instruction 42",
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.LogLine{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for LogLine")
	}
}

func TestExecutionStatusEnumValues(t *testing.T) {
	tests := []struct {
		name     string
		status   executionv1.ExecutionStatus
		expected int32
	}{
		{"UNSPECIFIED", executionv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED, 0},
		{"OK", executionv1.ExecutionStatus_EXECUTION_STATUS_OK, 1},
		{"INVALID_BLOCK", executionv1.ExecutionStatus_EXECUTION_STATUS_INVALID_BLOCK, 2},
		{"EXECUTION_ERROR", executionv1.ExecutionStatus_EXECUTION_STATUS_EXECUTION_ERROR, 3},
		{"OUT_OF_GAS", executionv1.ExecutionStatus_EXECUTION_STATUS_OUT_OF_GAS, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int32(tt.status) != tt.expected {
				t.Errorf("expected %s=%d, got %d", tt.name, tt.expected, int32(tt.status))
			}
		})
	}
}

func TestEmptyExecutionResponseRoundTrip(t *testing.T) {
	original := &executionv1.ExecutionResponse{
		ApiVersion:   1,
		Status:       executionv1.ExecutionStatus_EXECUTION_STATUS_OK,
		NewStateRoot: bytes.Repeat([]byte{0x00}, 32),
		GasUsed:      0,
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &executionv1.ExecutionResponse{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for empty ExecutionResponse")
	}
	if len(decoded.Receipts) != 0 {
		t.Fatalf("expected 0 receipts, got %d", len(decoded.Receipts))
	}
	if len(decoded.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(decoded.Events))
	}
}
