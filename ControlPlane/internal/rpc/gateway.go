package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	rpcv1 "github.com/echenim/Bedrock/controlplane/gen/proto/bedrock/rpc/v1"
	"go.uber.org/zap"
)

// Gateway provides an HTTP/JSON gateway that proxies to the gRPC NodeService.
type Gateway struct {
	server      *http.Server
	nodeService *NodeServiceImpl
	addr        string
	logger      *zap.Logger
	lis         net.Listener
}

// NewGateway creates an HTTP gateway.
func NewGateway(addr string, nodeService *NodeServiceImpl, logger *zap.Logger) *Gateway {
	if logger == nil {
		logger = zap.NewNop()
	}

	gw := &Gateway{
		nodeService: nodeService,
		addr:        addr,
		logger:      logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", gw.handleStatus)
	mux.HandleFunc("/tx", gw.handleSubmitTx)
	mux.HandleFunc("/block", gw.handleGetBlock)
	mux.HandleFunc("/state", gw.handleQueryState)
	mux.HandleFunc("/validators", gw.handleGetValidators)
	mux.HandleFunc("/health", gw.handleHealth)

	gw.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return gw
}

// Start begins serving HTTP requests.
func (gw *Gateway) Start(ctx context.Context) error {
	var err error
	gw.lis, err = net.Listen("tcp", gw.addr)
	if err != nil {
		return fmt.Errorf("gateway: listen on %s: %w", gw.addr, err)
	}

	gw.logger.Info("HTTP gateway starting", zap.String("addr", gw.lis.Addr().String()))

	go func() {
		if err := gw.server.Serve(gw.lis); err != nil && err != http.ErrServerClosed {
			gw.logger.Error("HTTP gateway error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the gateway.
func (gw *Gateway) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return gw.server.Shutdown(ctx)
}

// Name returns the service name.
func (gw *Gateway) Name() string {
	return "http-gateway"
}

// Addr returns the actual address the gateway is listening on.
func (gw *Gateway) Addr() string {
	if gw.lis != nil {
		return gw.lis.Addr().String()
	}
	return gw.addr
}

func (gw *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := gw.nodeService.GetStatus(r.Context(), &rpcv1.GetStatusRequest{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, resp)
}

func (gw *Gateway) handleSubmitTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Tx []byte `json:"tx"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := gw.nodeService.SubmitTransaction(r.Context(), &rpcv1.SubmitTransactionRequest{
		Tx: body.Tx,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, resp)
}

func (gw *Gateway) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	heightStr := r.URL.Query().Get("height")
	var height uint64
	if heightStr != "" {
		var err error
		height, err = strconv.ParseUint(heightStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid height parameter", http.StatusBadRequest)
			return
		}
	}

	resp, err := gw.nodeService.GetBlock(r.Context(), &rpcv1.GetBlockRequest{
		Height: height,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, resp)
}

func (gw *Gateway) handleQueryState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key parameter required", http.StatusBadRequest)
		return
	}

	resp, err := gw.nodeService.QueryState(r.Context(), &rpcv1.QueryStateRequest{
		Key: []byte(key),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, resp)
}

func (gw *Gateway) handleGetValidators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := gw.nodeService.GetValidators(r.Context(), &rpcv1.GetValidatorsRequest{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, resp)
}

func (gw *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
