package rpcv1_test

import (
	"bytes"
	"testing"

	executionv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/execution/v1"
	rpcv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/rpc/v1"
	typesv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/types/v1"
	"google.golang.org/protobuf/proto"
)

func TestGetStatusResponseRoundTrip(t *testing.T) {
	original := &rpcv1.GetStatusResponse{
		NodeId:             "node-abc123",
		Moniker:            "validator-1",
		LatestBlockHeight:  1000,
		LatestBlockHash:    bytes.Repeat([]byte{0xaa}, 32),
		LatestStateRoot:    bytes.Repeat([]byte{0xbb}, 32),
		Syncing:            false,
		EarliestBlockHeight: 1,
		Network:            "bedrock-testnet",
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &rpcv1.GetStatusResponse{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for GetStatusResponse")
	}
}

func TestSubmitTransactionRoundTrip(t *testing.T) {
	req := &rpcv1.SubmitTransactionRequest{
		Tx: bytes.Repeat([]byte{0xaa}, 177),
	}

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	decodedReq := &rpcv1.SubmitTransactionRequest{}
	if err := proto.Unmarshal(data, decodedReq); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !proto.Equal(req, decodedReq) {
		t.Fatal("round-trip mismatch for SubmitTransactionRequest")
	}

	resp := &rpcv1.SubmitTransactionResponse{
		TxHash: bytes.Repeat([]byte{0xbb}, 32),
		Code:   0,
		Log:    "ok",
	}

	data, err = proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	decodedResp := &rpcv1.SubmitTransactionResponse{}
	if err := proto.Unmarshal(data, decodedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !proto.Equal(resp, decodedResp) {
		t.Fatal("round-trip mismatch for SubmitTransactionResponse")
	}
}

func TestGetBlockResponseRoundTrip(t *testing.T) {
	original := &rpcv1.GetBlockResponse{
		Block: &typesv1.Block{
			Header: &typesv1.BlockHeader{
				Height:    10,
				Round:     1,
				BlockHash: bytes.Repeat([]byte{0xdd}, 32),
			},
			Transactions: []*typesv1.Transaction{
				{Data: []byte("tx1")},
			},
		},
		ExecutionResult: &executionv1.ExecutionResponse{
			ApiVersion:   1,
			Status:       executionv1.ExecutionStatus_EXECUTION_STATUS_OK,
			NewStateRoot: bytes.Repeat([]byte{0xee}, 32),
			GasUsed:      4970,
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &rpcv1.GetBlockResponse{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(original, decoded) {
		t.Fatal("round-trip mismatch for GetBlockResponse")
	}
}

func TestGetReceiptRoundTrip(t *testing.T) {
	req := &rpcv1.GetReceiptRequest{
		TxHash: bytes.Repeat([]byte{0xaa}, 32),
	}

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	decodedReq := &rpcv1.GetReceiptRequest{}
	if err := proto.Unmarshal(data, decodedReq); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !proto.Equal(req, decodedReq) {
		t.Fatal("round-trip mismatch for GetReceiptRequest")
	}

	resp := &rpcv1.GetReceiptResponse{
		Receipt: &executionv1.Receipt{
			TxIndex:    0,
			Success:    true,
			GasUsed:    4970,
			ResultCode: 0,
		},
		BlockHeight: 42,
		BlockHash:   bytes.Repeat([]byte{0xbb}, 32),
		TxIndex:     0,
	}

	data, err = proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	decodedResp := &rpcv1.GetReceiptResponse{}
	if err := proto.Unmarshal(data, decodedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !proto.Equal(resp, decodedResp) {
		t.Fatal("round-trip mismatch for GetReceiptResponse")
	}
}

func TestQueryStateRoundTrip(t *testing.T) {
	req := &rpcv1.QueryStateRequest{
		Key:    []byte("acct/alice/balance"),
		Height: 100,
		Prove:  true,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	decodedReq := &rpcv1.QueryStateRequest{}
	if err := proto.Unmarshal(data, decodedReq); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !proto.Equal(req, decodedReq) {
		t.Fatal("round-trip mismatch for QueryStateRequest")
	}

	resp := &rpcv1.QueryStateResponse{
		Key:    []byte("acct/alice/balance"),
		Value:  []byte{0xe8, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // 1000 LE
		Height: 100,
		Proof: &rpcv1.StateProof{
			RootHash: bytes.Repeat([]byte{0xaa}, 32),
			Key:      []byte("acct/alice/balance"),
			Value:    []byte{0xe8, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			Path:     [][]byte{bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)},
		},
	}

	data, err = proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	decodedResp := &rpcv1.QueryStateResponse{}
	if err := proto.Unmarshal(data, decodedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !proto.Equal(resp, decodedResp) {
		t.Fatal("round-trip mismatch for QueryStateResponse")
	}
}

func TestGetValidatorsRoundTrip(t *testing.T) {
	resp := &rpcv1.GetValidatorsResponse{
		ValidatorSet: &typesv1.ValidatorSet{
			Validators: []*typesv1.Validator{
				{Address: bytes.Repeat([]byte{0x01}, 32), PublicKey: bytes.Repeat([]byte{0xa1}, 32), VotingPower: 100},
				{Address: bytes.Repeat([]byte{0x02}, 32), PublicKey: bytes.Repeat([]byte{0xa2}, 32), VotingPower: 200},
			},
			Epoch:            3,
			TotalVotingPower: 300,
		},
		Height: 50,
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &rpcv1.GetValidatorsResponse{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(resp, decoded) {
		t.Fatal("round-trip mismatch for GetValidatorsResponse")
	}
}

func TestSubscribeBlocksRoundTrip(t *testing.T) {
	req := &rpcv1.SubscribeBlocksRequest{
		StartHeight: 100,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	decodedReq := &rpcv1.SubscribeBlocksRequest{}
	if err := proto.Unmarshal(data, decodedReq); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !proto.Equal(req, decodedReq) {
		t.Fatal("round-trip mismatch for SubscribeBlocksRequest")
	}

	resp := &rpcv1.SubscribeBlocksResponse{
		Block: &typesv1.Block{
			Header: &typesv1.BlockHeader{Height: 100, Round: 1},
		},
		ExecutionResult: &executionv1.ExecutionResponse{
			ApiVersion:   1,
			Status:       executionv1.ExecutionStatus_EXECUTION_STATUS_OK,
			NewStateRoot: bytes.Repeat([]byte{0xcc}, 32),
			GasUsed:      0,
		},
	}

	data, err = proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	decodedResp := &rpcv1.SubscribeBlocksResponse{}
	if err := proto.Unmarshal(data, decodedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !proto.Equal(resp, decodedResp) {
		t.Fatal("round-trip mismatch for SubscribeBlocksResponse")
	}
}

func TestCrossPackageImportsCompile(t *testing.T) {
	// Verify that the generated code correctly imports across packages.
	blockResp := &rpcv1.GetBlockResponse{
		Block: &typesv1.Block{
			Header: &typesv1.BlockHeader{Height: 1},
		},
		ExecutionResult: &executionv1.ExecutionResponse{
			Status: executionv1.ExecutionStatus_EXECUTION_STATUS_OK,
		},
	}

	validatorResp := &rpcv1.GetValidatorsResponse{
		ValidatorSet: &typesv1.ValidatorSet{
			Validators: []*typesv1.Validator{
				{Address: []byte("v1"), VotingPower: 100},
			},
		},
	}

	// Marshal and unmarshal to exercise the cross-package path.
	for _, msg := range []proto.Message{blockResp, validatorResp} {
		data, err := proto.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("expected non-empty marshal output")
		}
	}
}

func TestNodeServiceMethodNames(t *testing.T) {
	// Verify the generated gRPC method name constants exist.
	methods := []string{
		rpcv1.NodeService_GetStatus_FullMethodName,
		rpcv1.NodeService_SubmitTransaction_FullMethodName,
		rpcv1.NodeService_GetBlock_FullMethodName,
		rpcv1.NodeService_GetReceipt_FullMethodName,
		rpcv1.NodeService_SubscribeBlocks_FullMethodName,
		rpcv1.NodeService_QueryState_FullMethodName,
		rpcv1.NodeService_GetValidators_FullMethodName,
	}

	expectedPrefix := "/bedrock.rpc.v1.NodeService/"
	for _, m := range methods {
		if len(m) <= len(expectedPrefix) {
			t.Errorf("method name too short: %s", m)
		}
		if m[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("expected prefix %q, got %q", expectedPrefix, m)
		}
	}
}
