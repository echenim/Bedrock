package rpc

import (
	"context"
	"encoding/hex"

	rpcv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/rpc/v1"
	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/mempool"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	bsync "github.com/echenim/Bedrock/controlplane/internal/sync"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeServiceImpl implements the rpcv1.NodeServiceServer interface.
type NodeServiceImpl struct {
	rpcv1.UnimplementedNodeServiceServer

	store     storage.Store
	mempool   *mempool.Mempool
	consensus *consensus.Engine
	syncer    *bsync.BlockSyncer
	valSet    *types.ValidatorSet
	nodeID    string
	moniker   string
	chainID   string
	logger    *zap.Logger
}

// NodeServiceConfig holds configuration for the NodeService.
type NodeServiceConfig struct {
	Store     storage.Store
	Mempool   *mempool.Mempool
	Consensus *consensus.Engine
	Syncer    *bsync.BlockSyncer
	ValSet    *types.ValidatorSet
	NodeID    string
	Moniker   string
	ChainID   string
	Logger    *zap.Logger
}

// NewNodeService creates the gRPC node service implementation.
func NewNodeService(cfg NodeServiceConfig) *NodeServiceImpl {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	return &NodeServiceImpl{
		store:     cfg.Store,
		mempool:   cfg.Mempool,
		consensus: cfg.Consensus,
		syncer:    cfg.Syncer,
		valSet:    cfg.ValSet,
		nodeID:    cfg.NodeID,
		moniker:   cfg.Moniker,
		chainID:   cfg.ChainID,
		logger:    cfg.Logger,
	}
}

// GetStatus returns current node status.
func (s *NodeServiceImpl) GetStatus(
	ctx context.Context,
	req *rpcv1.GetStatusRequest,
) (*rpcv1.GetStatusResponse, error) {
	resp := &rpcv1.GetStatusResponse{
		NodeId:  s.nodeID,
		Moniker: s.moniker,
		Network: s.chainID,
	}

	// Sync status.
	if s.syncer != nil {
		resp.Syncing = !s.syncer.IsSynced()
	}

	// Latest block info from store.
	if s.store != nil {
		if height, err := s.store.GetLatestHeight(); err == nil {
			resp.LatestBlockHeight = height
			if block, err := s.store.GetBlock(height); err == nil {
				resp.LatestBlockHash = block.Header.BlockHash[:]
				resp.LatestStateRoot = block.Header.StateRoot[:]
			}
		}
	}

	return resp, nil
}

// SubmitTransaction validates and adds tx to mempool.
func (s *NodeServiceImpl) SubmitTransaction(
	ctx context.Context,
	req *rpcv1.SubmitTransactionRequest,
) (*rpcv1.SubmitTransactionResponse, error) {
	if len(req.GetTx()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "transaction data is required")
	}

	if s.mempool == nil {
		return nil, status.Error(codes.Unavailable, "mempool not available")
	}

	txHash, err := s.mempool.AddTx(req.GetTx())
	if err != nil {
		return &rpcv1.SubmitTransactionResponse{
			TxHash: txHash[:],
			Code:   1,
			Log:    err.Error(),
		}, nil
	}

	return &rpcv1.SubmitTransactionResponse{
		TxHash: txHash[:],
		Code:   0,
		Log:    "ok",
	}, nil
}

// GetBlock retrieves a block by height.
func (s *NodeServiceImpl) GetBlock(
	ctx context.Context,
	req *rpcv1.GetBlockRequest,
) (*rpcv1.GetBlockResponse, error) {
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "store not available")
	}

	height := req.GetHeight()
	if height == 0 {
		// Return latest block.
		h, err := s.store.GetLatestHeight()
		if err != nil {
			return nil, status.Error(codes.NotFound, "no blocks available")
		}
		height = h
	}

	block, err := s.store.GetBlock(height)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "block at height %d not found", height)
	}

	return &rpcv1.GetBlockResponse{
		Block: block.ToProto(),
	}, nil
}

// GetReceipt retrieves a transaction receipt.
func (s *NodeServiceImpl) GetReceipt(
	ctx context.Context,
	req *rpcv1.GetReceiptRequest,
) (*rpcv1.GetReceiptResponse, error) {
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "store not available")
	}

	txHashBytes := req.GetTxHash()
	if len(txHashBytes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "tx_hash is required")
	}

	var txHash types.Hash
	copy(txHash[:], txHashBytes)

	height, txIndex, err := s.store.GetTxLocation(txHash)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "transaction %s not found", hex.EncodeToString(txHashBytes))
	}

	block, err := s.store.GetBlock(height)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "block %d not found", height)
	}

	return &rpcv1.GetReceiptResponse{
		BlockHeight: height,
		BlockHash:   block.Header.BlockHash[:],
		TxIndex:     txIndex,
	}, nil
}

// SubscribeBlocks streams new blocks as they are committed.
func (s *NodeServiceImpl) SubscribeBlocks(
	req *rpcv1.SubscribeBlocksRequest,
	stream rpcv1.NodeService_SubscribeBlocksServer,
) error {
	if s.consensus == nil {
		return status.Error(codes.Unavailable, "consensus engine not available")
	}

	commitCh := s.consensus.SubscribeCommits()

	// If start_height > 0, replay historical blocks first.
	if req.GetStartHeight() > 0 && s.store != nil {
		latestH, _ := s.store.GetLatestHeight()
		for h := req.GetStartHeight(); h <= latestH; h++ {
			block, err := s.store.GetBlock(h)
			if err != nil {
				continue
			}
			if err := stream.Send(&rpcv1.SubscribeBlocksResponse{
				Block: block.ToProto(),
			}); err != nil {
				return err
			}
		}
	}

	// Stream live blocks.
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case evt, ok := <-commitCh:
			if !ok {
				return nil
			}
			if err := stream.Send(&rpcv1.SubscribeBlocksResponse{
				Block: evt.Block.ToProto(),
			}); err != nil {
				return err
			}
		}
	}
}

// QueryState reads application state at a given key.
func (s *NodeServiceImpl) QueryState(
	ctx context.Context,
	req *rpcv1.QueryStateRequest,
) (*rpcv1.QueryStateResponse, error) {
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "store not available")
	}

	key := req.GetKey()
	if len(key) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	value, err := s.store.Get(key)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "state query failed: %v", err)
	}

	height, _ := s.store.GetLatestHeight()
	stateRoot, _ := s.store.GetStateRoot()

	resp := &rpcv1.QueryStateResponse{
		Key:    key,
		Value:  value,
		Height: height,
	}

	if req.GetProve() {
		resp.Proof = &rpcv1.StateProof{
			RootHash: stateRoot[:],
			Key:      key,
			Value:    value,
		}
	}

	return resp, nil
}

// GetValidators returns the validator set.
func (s *NodeServiceImpl) GetValidators(
	ctx context.Context,
	req *rpcv1.GetValidatorsRequest,
) (*rpcv1.GetValidatorsResponse, error) {
	if s.valSet == nil {
		return nil, status.Error(codes.Unavailable, "validator set not available")
	}

	height := uint64(0)
	if s.store != nil {
		height, _ = s.store.GetLatestHeight()
	}

	return &rpcv1.GetValidatorsResponse{
		ValidatorSet: s.valSet.ToProto(),
		Height:       height,
	}, nil
}

// nodeIDFromKey derives a short node ID from a private key.
func nodeIDFromKey(privKey crypto.PrivateKey) string {
	if privKey == nil {
		return "unknown"
	}
	pubKey := privKey.Public().(crypto.PublicKey)
	addr := crypto.AddressFromPubKey(pubKey)
	return hex.EncodeToString(addr[:8])
}
