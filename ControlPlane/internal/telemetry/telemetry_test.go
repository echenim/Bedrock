package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestNewMetricsRegistersAll(t *testing.T) {
	m := NewMetrics("test")
	if m.registry == nil {
		t.Fatal("expected non-nil registry")
	}

	// Verify all gauges/counters/histograms are non-nil.
	if m.ConsensusHeight == nil {
		t.Error("ConsensusHeight is nil")
	}
	if m.PeerCount == nil {
		t.Error("PeerCount is nil")
	}
	if m.MempoolSize == nil {
		t.Error("MempoolSize is nil")
	}
	if m.SyncStatus == nil {
		t.Error("SyncStatus is nil")
	}
}

func TestNopMetrics(t *testing.T) {
	m := NopMetrics()

	// NopMetrics should not panic when used.
	m.ConsensusHeight.Set(10)
	m.VotesReceived.Inc()
	m.BlockTime.Observe(1.5)
	m.MempoolSize.Set(100)
}

func TestMetricsEndpoint(t *testing.T) {
	m := NewMetrics("test")

	// Set some values.
	m.ConsensusHeight.Set(42)
	m.PeerCount.Set(5)

	handler := promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Fatal("expected non-empty metrics output")
	}
}

func TestNewLoggerDevelopment(t *testing.T) {
	logger, err := NewLogger("development")
	if err != nil {
		t.Fatalf("NewLogger(development): %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLoggerProduction(t *testing.T) {
	logger, err := NewLogger("production")
	if err != nil {
		t.Fatalf("NewLogger(production): %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLoggerInvalid(t *testing.T) {
	_, err := NewLogger("invalid")
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestNewNopLogger(t *testing.T) {
	logger := NewNopLogger()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	// Should not panic.
	logger.Info("test message")
}

func TestNewLoggerAliases(t *testing.T) {
	// Test short aliases.
	l1, err := NewLogger("dev")
	if err != nil {
		t.Fatalf("NewLogger(dev): %v", err)
	}
	if l1 == nil {
		t.Fatal("expected non-nil logger for 'dev'")
	}

	l2, err := NewLogger("prod")
	if err != nil {
		t.Fatalf("NewLogger(prod): %v", err)
	}
	if l2 == nil {
		t.Fatal("expected non-nil logger for 'prod'")
	}
}
