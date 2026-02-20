package p2p

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	validMessageScore   = 1.0
	invalidMessageScore = -10.0
	banThreshold        = -100.0
	defaultBanDuration  = 10 * time.Minute
)

// BanEntry records a peer ban with expiry.
type BanEntry struct {
	Reason  string
	Expires time.Time
}

// PeerScoring tracks peer reputation and bans.
type PeerScoring struct {
	mu     sync.RWMutex
	scores map[peer.ID]float64
	bans   map[peer.ID]BanEntry
}

// NewPeerScoring creates a new PeerScoring instance.
func NewPeerScoring() *PeerScoring {
	return &PeerScoring{
		scores: make(map[peer.ID]float64),
		bans:   make(map[peer.ID]BanEntry),
	}
}

// RecordValidMessage increases a peer's score.
func (ps *PeerScoring) RecordValidMessage(pid peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.scores[pid] += validMessageScore
}

// RecordInvalidMessage decreases a peer's score and auto-bans if below threshold.
func (ps *PeerScoring) RecordInvalidMessage(pid peer.ID, reason string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.scores[pid] += invalidMessageScore
	if ps.scores[pid] <= banThreshold {
		ps.bans[pid] = BanEntry{
			Reason:  reason,
			Expires: time.Now().Add(defaultBanDuration),
		}
	}
}

// Score returns the current score for a peer.
func (ps *PeerScoring) Score(pid peer.ID) float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.scores[pid]
}

// IsBanned returns true if the peer is currently banned.
func (ps *PeerScoring) IsBanned(pid peer.ID) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	entry, ok := ps.bans[pid]
	if !ok {
		return false
	}
	return time.Now().Before(entry.Expires)
}

// Ban explicitly bans a peer for the given duration.
func (ps *PeerScoring) Ban(pid peer.ID, reason string, duration time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.bans[pid] = BanEntry{
		Reason:  reason,
		Expires: time.Now().Add(duration),
	}
}

// Unban removes a peer's ban entry.
func (ps *PeerScoring) Unban(pid peer.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.bans, pid)
	// Reset score on unban so they start fresh.
	ps.scores[pid] = 0
}

// CleanupExpiredBans removes expired ban entries. Returns the number removed.
func (ps *PeerScoring) CleanupExpiredBans() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	now := time.Now()
	removed := 0
	for pid, entry := range ps.bans {
		if now.After(entry.Expires) {
			delete(ps.bans, pid)
			removed++
		}
	}
	return removed
}

// BannedCount returns the number of currently banned peers.
func (ps *PeerScoring) BannedCount() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	now := time.Now()
	count := 0
	for _, entry := range ps.bans {
		if now.Before(entry.Expires) {
			count++
		}
	}
	return count
}
