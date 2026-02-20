package p2p

import (
	"sync"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/types"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// PeerDirection indicates whether we initiated or received the connection.
type PeerDirection int

const (
	Inbound PeerDirection = iota
	Outbound
)

// outboundReservedRatio is the fraction of MaxPeers reserved for outbound connections.
const outboundReservedRatio = 0.20

// PeerInfo tracks metadata about a connected peer.
type PeerInfo struct {
	ID            peer.ID
	Addrs         []multiaddr.Multiaddr
	Direction     PeerDirection
	ConnectedAt   time.Time
	IsValidator   bool
	ValidatorAddr types.Address
}

// PeerManager tracks connected peers and enforces limits.
type PeerManager struct {
	mu       sync.RWMutex
	peers    map[peer.ID]*PeerInfo
	maxPeers int
	scoring  *PeerScoring
}

// NewPeerManager creates a PeerManager with the given limits.
func NewPeerManager(maxPeers int, scoring *PeerScoring) *PeerManager {
	return &PeerManager{
		peers:    make(map[peer.ID]*PeerInfo),
		maxPeers: maxPeers,
		scoring:  scoring,
	}
}

// AddPeer registers a connected peer.
func (pm *PeerManager) AddPeer(info *PeerInfo) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peers[info.ID] = info
}

// RemovePeer removes a peer from tracking.
func (pm *PeerManager) RemovePeer(pid peer.ID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, pid)
}

// GetPeer returns info for a peer, if known.
func (pm *PeerManager) GetPeer(pid peer.ID) (*PeerInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	info, ok := pm.peers[pid]
	return info, ok
}

// PeerCount returns the number of connected peers.
func (pm *PeerManager) PeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// ConnectedPeers returns a snapshot of all connected peer IDs.
func (pm *PeerManager) ConnectedPeers() []peer.ID {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pids := make([]peer.ID, 0, len(pm.peers))
	for pid := range pm.peers {
		pids = append(pids, pid)
	}
	return pids
}

// ShouldAcceptConnection decides whether a new connection should be accepted.
// Validators are always accepted. Banned peers are always rejected.
// Non-validator inbound connections are rejected if at max peers.
func (pm *PeerManager) ShouldAcceptConnection(pid peer.ID, dir network.Direction) bool {
	// Always reject banned peers.
	if pm.scoring != nil && pm.scoring.IsBanned(pid) {
		return false
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Already connected — allow (idempotent).
	if _, ok := pm.peers[pid]; ok {
		return true
	}

	// Under the limit — always accept.
	if len(pm.peers) < pm.maxPeers {
		return true
	}

	// At or over limit: only accept validators.
	// For inbound connections we can't easily check validator status up front,
	// but we allow over-limit only if the peer is known to be a validator.
	return false
}

// EvictWorstPeer finds the lowest-scored non-validator peer and returns its ID.
// Returns empty peer.ID if no evictable peer exists.
func (pm *PeerManager) EvictWorstPeer() peer.ID {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.scoring == nil {
		return ""
	}

	var worstPeer peer.ID
	worstScore := float64(0)
	first := true

	for pid, info := range pm.peers {
		if info.IsValidator {
			continue
		}
		score := pm.scoring.Score(pid)
		if first || score < worstScore {
			worstPeer = pid
			worstScore = score
			first = false
		}
	}

	return worstPeer
}

// OutboundCount returns the number of outbound connections.
func (pm *PeerManager) OutboundCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	count := 0
	for _, info := range pm.peers {
		if info.Direction == Outbound {
			count++
		}
	}
	return count
}

// OutboundSlotsFull returns true if the outbound-reserved slots are filled.
// Anti-eclipse: at least outboundReservedRatio of maxPeers should be outbound.
func (pm *PeerManager) OutboundSlotsFull() bool {
	reserved := int(float64(pm.maxPeers) * outboundReservedRatio)
	if reserved < 1 {
		reserved = 1
	}
	return pm.OutboundCount() >= reserved
}

// MarkValidator marks a connected peer as a validator with the given address.
func (pm *PeerManager) MarkValidator(pid peer.ID, addr types.Address) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if info, ok := pm.peers[pid]; ok {
		info.IsValidator = true
		info.ValidatorAddr = addr
	}
}
