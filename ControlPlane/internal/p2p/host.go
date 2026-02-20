package p2p

import (
	"context"
	"fmt"

	libp2p "github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

// HostConfig holds configuration for creating a P2P Host.
type HostConfig struct {
	// PrivateKey is the Ed25519 private key (64 bytes, standard Go crypto/ed25519 format).
	PrivateKey []byte
	// ListenAddr is a multiaddr string (e.g. "/ip4/0.0.0.0/udp/26656/quic-v1").
	ListenAddr string
	// MaxPeers is the maximum number of connections.
	MaxPeers int
	// Seeds are multiaddr strings for seed nodes.
	Seeds []string
	// EnableScoring enables peer scoring.
	EnableScoring bool
	// Logger for the host.
	Logger *zap.Logger
	// Metrics for the P2P subsystem.
	Metrics *Metrics
}

// Host wraps a libp2p host with BedRock-specific peer management and gossip.
type Host struct {
	host        host.Host
	gossip      *GossipManager
	discovery   *Discovery
	peerMgr     *PeerManager
	scoring     *PeerScoring
	rateLimiter *RateLimiter
	metrics     *Metrics
	logger      *zap.Logger

	cancel context.CancelFunc
}

// NewHost creates a libp2p host with Ed25519 identity, QUIC transport, and
// integrated peer management.
func NewHost(ctx context.Context, cfg HostConfig) (*Host, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = NopMetrics()
	}

	// Convert Ed25519 private key to libp2p format.
	privKey, err := libp2pcrypto.UnmarshalEd25519PrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("p2p: unmarshal private key: %w", err)
	}

	// Parse listen address.
	listenAddr, err := multiaddr.NewMultiaddr(cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("p2p: invalid listen address %q: %w", cfg.ListenAddr, err)
	}

	// Create libp2p host.
	opts := []libp2p.Option{
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddr),
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("p2p: create host: %w", err)
	}

	// Scoring and rate limiting.
	scoring := NewPeerScoring()
	rateLimiter := NewRateLimiter(DefaultRateLimitConfig())

	// Peer manager.
	maxPeers := cfg.MaxPeers
	if maxPeers <= 0 {
		maxPeers = 50
	}
	peerMgr := NewPeerManager(maxPeers, scoring)

	// Parse seeds.
	seeds, err := ParseSeedAddrs(cfg.Seeds)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("p2p: parse seeds: %w", err)
	}

	// Discovery.
	disc := NewDiscovery(h, seeds, peerMgr, logger)

	// GossipSub.
	gossip, err := NewGossipManager(ctx, h, scoring, rateLimiter, logger)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("p2p: create gossip: %w", err)
	}

	bh := &Host{
		host:        h,
		gossip:      gossip,
		discovery:   disc,
		peerMgr:     peerMgr,
		scoring:     scoring,
		rateLimiter: rateLimiter,
		metrics:     metrics,
		logger:      logger,
	}

	// Register connection notifier for peer tracking.
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			pid := conn.RemotePeer()
			dir := Inbound
			if conn.Stat().Direction == network.DirOutbound {
				dir = Outbound
			}
			peerMgr.AddPeer(&PeerInfo{
				ID:        pid,
				Addrs:     []multiaddr.Multiaddr{conn.RemoteMultiaddr()},
				Direction: dir,
			})
			metrics.PeersConnected.Set(float64(peerMgr.PeerCount()))
			logger.Debug("peer connected",
				zap.String("peer", pid.String()),
			)
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			pid := conn.RemotePeer()
			peerMgr.RemovePeer(pid)
			metrics.PeersConnected.Set(float64(peerMgr.PeerCount()))
			logger.Debug("peer disconnected",
				zap.String("peer", pid.String()),
			)
		},
	})

	return bh, nil
}

// Start begins peer discovery and gossip.
func (bh *Host) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	bh.cancel = cancel

	// Join consensus topic.
	if _, err := bh.gossip.JoinTopic(TopicConsensus); err != nil {
		return fmt.Errorf("p2p: join consensus topic: %w", err)
	}

	// Register consensus validator.
	if err := bh.gossip.RegisterConsensusValidator(); err != nil {
		return fmt.Errorf("p2p: register consensus validator: %w", err)
	}

	// Start discovery.
	bh.discovery.Start(ctx)

	bh.logger.Info("p2p host started",
		zap.String("peer_id", bh.host.ID().String()),
		zap.Any("listen_addrs", bh.host.Addrs()),
	)

	return nil
}

// Stop shuts down the P2P host.
func (bh *Host) Stop() error {
	if bh.cancel != nil {
		bh.cancel()
	}
	bh.gossip.Close()
	return bh.host.Close()
}

// ID returns the host's peer ID.
func (bh *Host) ID() peer.ID {
	return bh.host.ID()
}

// Addrs returns the host's listen addresses.
func (bh *Host) Addrs() []multiaddr.Multiaddr {
	return bh.host.Addrs()
}

// LibP2PHost returns the underlying libp2p host.
func (bh *Host) LibP2PHost() host.Host {
	return bh.host
}

// Gossip returns the GossipManager.
func (bh *Host) Gossip() *GossipManager {
	return bh.gossip
}

// PeerManager returns the PeerManager.
func (bh *Host) PeerMgr() *PeerManager {
	return bh.peerMgr
}

// Scoring returns the PeerScoring instance.
func (bh *Host) Scoring() *PeerScoring {
	return bh.scoring
}
