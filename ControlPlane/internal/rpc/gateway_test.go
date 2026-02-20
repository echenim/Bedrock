package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayHealth(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", resp["status"])
	}
}

func TestGatewayStatus(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["moniker"] != "test-moniker" {
		t.Errorf("expected moniker=test-moniker, got %v", resp["moniker"])
	}
}

func TestGatewayGetBlock(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/block?height=1", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGatewayGetBlockInvalidHeight(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/block?height=abc", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGatewayQueryState(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/state?key=key1", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGatewayQueryStateNoKey(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/state", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGatewayValidators(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/validators", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGatewayMethodNotAllowed(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	// POST to GET-only endpoint.
	req := httptest.NewRequest(http.MethodPost, "/status", nil)
	w := httptest.NewRecorder()
	gw.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestGatewayStartStop(t *testing.T) {
	svc, _ := testNodeService(t)
	gw := NewGateway("127.0.0.1:0", svc, nil)

	if err := gw.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	addr := gw.Addr()
	if addr == "" {
		t.Fatal("expected non-empty address")
	}

	if err := gw.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestGatewayName(t *testing.T) {
	gw := NewGateway("127.0.0.1:0", nil, nil)
	if gw.Name() != "http-gateway" {
		t.Errorf("expected name=http-gateway, got %s", gw.Name())
	}
}
