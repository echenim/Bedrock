package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Metrics tracks observable metrics per SPEC.md §18.
type Metrics struct {
	// Consensus.
	ConsensusHeight   prometheus.Gauge
	ConsensusRound    prometheus.Gauge
	BlockTime         prometheus.Histogram
	VotesReceived     prometheus.Counter
	TimeoutsTriggered prometheus.Counter

	// P2P.
	PeerCount        prometheus.Gauge
	MessagesSent     prometheus.Counter
	MessagesReceived prometheus.Counter

	// Mempool.
	MempoolSize prometheus.Gauge
	TxsAccepted prometheus.Counter
	TxsRejected prometheus.Counter

	// Execution.
	BlockGasUsed     prometheus.Histogram
	ExecutionLatency prometheus.Histogram

	// Sync.
	SyncStatus prometheus.Gauge // 0=synced, 1=syncing

	registry *prometheus.Registry
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics(namespace string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	m := &Metrics{
		registry: reg,

		ConsensusHeight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "height",
			Help:      "Current consensus block height.",
		}),
		ConsensusRound: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "round",
			Help:      "Current consensus round.",
		}),
		BlockTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "block_time_seconds",
			Help:      "Time between consecutive block commits.",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
		}),
		VotesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "votes_received_total",
			Help:      "Total number of votes received.",
		}),
		TimeoutsTriggered: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "timeouts_triggered_total",
			Help:      "Total number of consensus timeouts triggered.",
		}),

		PeerCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "p2p",
			Name:      "peers_connected",
			Help:      "Number of connected peers.",
		}),
		MessagesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "p2p",
			Name:      "messages_sent_total",
			Help:      "Total number of P2P messages sent.",
		}),
		MessagesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "p2p",
			Name:      "messages_received_total",
			Help:      "Total number of P2P messages received.",
		}),

		MempoolSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "mempool",
			Name:      "size",
			Help:      "Current number of transactions in the mempool.",
		}),
		TxsAccepted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "mempool",
			Name:      "txs_accepted_total",
			Help:      "Total transactions accepted into the mempool.",
		}),
		TxsRejected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "mempool",
			Name:      "txs_rejected_total",
			Help:      "Total transactions rejected from the mempool.",
		}),

		BlockGasUsed: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "execution",
			Name:      "block_gas_used",
			Help:      "Gas used per block.",
			Buckets:   prometheus.ExponentialBuckets(1000, 10, 8),
		}),
		ExecutionLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "execution",
			Name:      "latency_seconds",
			Help:      "Block execution latency in seconds.",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
		}),

		SyncStatus: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "sync",
			Name:      "status",
			Help:      "Sync status: 0=synced, 1=syncing.",
		}),
	}

	// Register all metrics with the registry.
	reg.MustRegister(
		m.ConsensusHeight, m.ConsensusRound, m.BlockTime,
		m.VotesReceived, m.TimeoutsTriggered,
		m.PeerCount, m.MessagesSent, m.MessagesReceived,
		m.MempoolSize, m.TxsAccepted, m.TxsRejected,
		m.BlockGasUsed, m.ExecutionLatency,
		m.SyncStatus,
	)

	return m
}

// NopMetrics returns a Metrics instance that discards all observations.
func NopMetrics() *Metrics {
	return &Metrics{
		ConsensusHeight:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "nop_ch"}),
		ConsensusRound:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "nop_cr"}),
		BlockTime:         prometheus.NewHistogram(prometheus.HistogramOpts{Name: "nop_bt"}),
		VotesReceived:     prometheus.NewCounter(prometheus.CounterOpts{Name: "nop_vr"}),
		TimeoutsTriggered: prometheus.NewCounter(prometheus.CounterOpts{Name: "nop_tt"}),
		PeerCount:         prometheus.NewGauge(prometheus.GaugeOpts{Name: "nop_pc"}),
		MessagesSent:      prometheus.NewCounter(prometheus.CounterOpts{Name: "nop_ms"}),
		MessagesReceived:  prometheus.NewCounter(prometheus.CounterOpts{Name: "nop_mr"}),
		MempoolSize:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "nop_mps"}),
		TxsAccepted:       prometheus.NewCounter(prometheus.CounterOpts{Name: "nop_ta"}),
		TxsRejected:       prometheus.NewCounter(prometheus.CounterOpts{Name: "nop_tr"}),
		BlockGasUsed:      prometheus.NewHistogram(prometheus.HistogramOpts{Name: "nop_bgu"}),
		ExecutionLatency:  prometheus.NewHistogram(prometheus.HistogramOpts{Name: "nop_el"}),
		SyncStatus:        prometheus.NewGauge(prometheus.GaugeOpts{Name: "nop_ss"}),
		registry:          prometheus.NewRegistry(),
	}
}

// Registry returns the Prometheus registry for this metrics instance.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// MetricsServer serves Prometheus metrics via HTTP.
type MetricsServer struct {
	server *http.Server
	logger *zap.Logger
}

// NewMetricsServer creates a metrics HTTP server.
func NewMetricsServer(addr string, metrics *Metrics, logger *zap.Logger) *MetricsServer {
	if logger == nil {
		logger = zap.NewNop()
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{}))

	return &MetricsServer{
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
		logger: logger,
	}
}

// Start begins serving metrics.
func (ms *MetricsServer) Start() error {
	ms.logger.Info("metrics server starting", zap.String("addr", ms.server.Addr))
	if err := ms.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the metrics server.
func (ms *MetricsServer) Stop() error {
	return ms.server.Close()
}
