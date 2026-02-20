package p2p

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds Prometheus metrics for the P2P subsystem.
type Metrics struct {
	PeersConnected   prometheus.Gauge
	PeersBanned      prometheus.Gauge
	MessagesReceived *prometheus.CounterVec
	MessagesSent     *prometheus.CounterVec
	MessagesRejected *prometheus.CounterVec
}

// NewMetrics creates registered Prometheus metrics for the P2P subsystem.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		PeersConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "bedrock",
			Subsystem: "p2p",
			Name:      "peers_connected",
			Help:      "Number of currently connected peers.",
		}),
		PeersBanned: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "bedrock",
			Subsystem: "p2p",
			Name:      "peers_banned",
			Help:      "Number of currently banned peers.",
		}),
		MessagesReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "bedrock",
			Subsystem: "p2p",
			Name:      "messages_received_total",
			Help:      "Total number of messages received by type.",
		}, []string{"type"}),
		MessagesSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "bedrock",
			Subsystem: "p2p",
			Name:      "messages_sent_total",
			Help:      "Total number of messages sent by type.",
		}, []string{"type"}),
		MessagesRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "bedrock",
			Subsystem: "p2p",
			Name:      "messages_rejected_total",
			Help:      "Total number of messages rejected by reason.",
		}, []string{"reason"}),
	}

	if registerer != nil {
		registerer.MustRegister(
			m.PeersConnected,
			m.PeersBanned,
			m.MessagesReceived,
			m.MessagesSent,
			m.MessagesRejected,
		)
	}

	return m
}

// NopMetrics returns no-op metrics for use in tests.
func NopMetrics() *Metrics {
	return NewMetrics(nil)
}
