package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/mempool"
	bsync "github.com/echenim/Bedrock/controlplane/internal/sync"
	"go.uber.org/zap"
)

// Server provides admin/debug endpoints.
// These are intended for operators, not exposed publicly.
type Server struct {
	httpServer *http.Server
	consensus  *consensus.Engine
	mempool    *mempool.Mempool
	syncer     *bsync.BlockSyncer
	logger     *zap.Logger
	lis        net.Listener
}

// NewServer creates an admin debug server.
func NewServer(
	addr string,
	consensus *consensus.Engine,
	mempool *mempool.Mempool,
	syncer *bsync.BlockSyncer,
	logger *zap.Logger,
) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &Server{
		consensus: consensus,
		mempool:   mempool,
		syncer:    syncer,
		logger:    logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/admin/consensus", s.handleConsensusState)
	mux.HandleFunc("/admin/mempool", s.handleMempoolStatus)
	mux.HandleFunc("/admin/sync", s.handleSyncStatus)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s
}

// Start begins serving admin endpoints.
func (s *Server) Start(ctx context.Context) error {
	var err error
	s.lis, err = net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("admin: listen on %s: %w", s.httpServer.Addr, err)
	}

	s.logger.Info("admin server starting", zap.String("addr", s.lis.Addr().String()))

	go func() {
		if err := s.httpServer.Serve(s.lis); err != nil && err != http.ErrServerClosed {
			s.logger.Error("admin server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the admin server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// Name returns the service name.
func (s *Server) Name() string {
	return "admin"
}

func (s *Server) handleConsensusState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := map[string]any{
		"available": s.consensus != nil,
	}

	if s.consensus != nil {
		state := s.consensus.State()
		if state != nil {
			result["height"] = state.Height
			result["round"] = state.Round
			result["step"] = state.Step.String()
		}
		result["address"] = s.consensus.Address().String()
	}

	writeJSON(w, result)
}

func (s *Server) handleMempoolStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := map[string]any{
		"available": s.mempool != nil,
	}

	if s.mempool != nil {
		result["size"] = s.mempool.Size()
	}

	writeJSON(w, result)
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := map[string]any{
		"available": s.syncer != nil,
	}

	if s.syncer != nil {
		result["state"] = s.syncer.State().String()
		result["synced"] = s.syncer.IsSynced()
		result["current_height"] = s.syncer.CurrentHeight()
		result["target_height"] = s.syncer.TargetHeight()
	}

	writeJSON(w, result)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
	}
}
