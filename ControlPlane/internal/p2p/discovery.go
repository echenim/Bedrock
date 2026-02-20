package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

const (
	reconnectInterval = 30 * time.Second
	connectTimeout    = 10 * time.Second
)

// Discovery manages seed-based peer discovery and reconnection.
// No DHT — seed-based discovery is sufficient for BFT validator networks.
type Discovery struct {
	host    host.Host
	seeds   []peer.AddrInfo
	peerMgr *PeerManager
	logger  *zap.Logger
}

// NewDiscovery creates a Discovery instance with the given seed addresses.
func NewDiscovery(h host.Host, seeds []peer.AddrInfo, peerMgr *PeerManager, logger *zap.Logger) *Discovery {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Discovery{
		host:    h,
		seeds:   seeds,
		peerMgr: peerMgr,
		logger:  logger,
	}
}

// ParseSeedAddrs parses multiaddr strings into peer.AddrInfo structs.
// Each string must be a full multiaddr including the /p2p/<peer-id> component.
func ParseSeedAddrs(addrs []string) ([]peer.AddrInfo, error) {
	var infos []peer.AddrInfo
	for _, s := range addrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return nil, fmt.Errorf("p2p: invalid seed addr %q: %w", s, err)
		}
		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			return nil, fmt.Errorf("p2p: parse seed addr %q: %w", s, err)
		}
		infos = append(infos, *info)
	}
	return infos, nil
}

// Start begins the discovery loop: connect to seeds immediately, then
// periodically reconnect to any that have disconnected.
func (d *Discovery) Start(ctx context.Context) {
	// Initial connection to seeds.
	d.connectToSeeds(ctx)

	// Periodic reconnection loop.
	go d.reconnectLoop(ctx)
}

func (d *Discovery) connectToSeeds(ctx context.Context) {
	for _, seed := range d.seeds {
		// Don't connect to ourselves.
		if seed.ID == d.host.ID() {
			continue
		}

		connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
		if err := d.host.Connect(connectCtx, seed); err != nil {
			d.logger.Warn("failed to connect to seed",
				zap.String("peer", seed.ID.String()),
				zap.Error(err),
			)
		} else {
			d.logger.Info("connected to seed",
				zap.String("peer", seed.ID.String()),
			)
		}
		cancel()
	}
}

func (d *Discovery) reconnectLoop(ctx context.Context) {
	ticker := time.NewTicker(reconnectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.connectToSeeds(ctx)
		}
	}
}
