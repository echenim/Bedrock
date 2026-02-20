package rpc

import (
	"context"
	"fmt"
	"net"

	rpcv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/rpc/v1"
	"github.com/echenim/Bedrock/controlplane/internal/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server hosts the gRPC and HTTP gateway servers.
type Server struct {
	grpcServer  *grpc.Server
	nodeService *NodeServiceImpl
	cfg         config.RPCConfig
	logger      *zap.Logger

	grpcLis net.Listener
}

// NewServer creates a new RPC server.
func NewServer(cfg config.RPCConfig, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			RecoveryUnaryInterceptor(logger),
			LoggingUnaryInterceptor(logger),
		),
		grpc.ChainStreamInterceptor(
			LoggingStreamInterceptor(logger),
		),
	)

	// Enable gRPC server reflection for debugging with grpcurl.
	reflection.Register(grpcServer)

	return &Server{
		grpcServer: grpcServer,
		cfg:        cfg,
		logger:     logger,
	}
}

// RegisterNodeService registers the node service implementation.
func (s *Server) RegisterNodeService(svc *NodeServiceImpl) {
	s.nodeService = svc
	rpcv1.RegisterNodeServiceServer(s.grpcServer, svc)
}

// Start begins serving gRPC requests.
func (s *Server) Start(ctx context.Context) error {
	var err error
	s.grpcLis, err = net.Listen("tcp", s.cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("rpc: listen on %s: %w", s.cfg.GRPCAddr, err)
	}

	s.logger.Info("gRPC server starting",
		zap.String("addr", s.cfg.GRPCAddr),
	)

	go func() {
		if err := s.grpcServer.Serve(s.grpcLis); err != nil {
			s.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.logger.Info("gRPC server stopping")
	s.grpcServer.GracefulStop()
	return nil
}

// Name returns the service name.
func (s *Server) Name() string {
	return "rpc"
}

// GRPCServer returns the underlying gRPC server (for testing).
func (s *Server) GRPCServer() *grpc.Server {
	return s.grpcServer
}

// GRPCAddr returns the actual address the gRPC server is listening on.
// Useful when configured with port 0 for tests.
func (s *Server) GRPCAddr() string {
	if s.grpcLis != nil {
		return s.grpcLis.Addr().String()
	}
	return s.cfg.GRPCAddr
}
