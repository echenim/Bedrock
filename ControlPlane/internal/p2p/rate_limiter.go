package p2p

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RateLimitConfig defines rate limits per message type.
type RateLimitConfig struct {
	ProposalRate    float64 // proposals per second
	VoteRate        float64 // votes per second
	TimeoutRate     float64 // timeouts per second
	GlobalRate      float64 // total messages per second per peer
	BurstMultiplier float64 // burst capacity = rate * multiplier
}

// DefaultRateLimitConfig returns sensible defaults.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		ProposalRate:    2,
		VoteRate:        20,
		TimeoutRate:     5,
		GlobalRate:      50,
		BurstMultiplier: 3,
	}
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	tokens    float64
	maxTokens float64
	rate      float64 // tokens per second
	lastFill  time.Time
}

func newTokenBucket(rate, burstMultiplier float64) *tokenBucket {
	maxTokens := rate * burstMultiplier
	return &tokenBucket{
		tokens:    maxTokens,
		maxTokens: maxTokens,
		rate:      rate,
		lastFill:  time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastFill).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastFill = now

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// peerBuckets holds all rate limit buckets for a single peer.
type peerBuckets struct {
	global   *tokenBucket
	proposal *tokenBucket
	vote     *tokenBucket
	timeout  *tokenBucket
	lastSeen time.Time
}

// RateLimiter tracks per-peer, per-type rate limits.
type RateLimiter struct {
	mu     sync.Mutex
	peers  map[peer.ID]*peerBuckets
	config RateLimitConfig
}

// NewRateLimiter creates a RateLimiter with the given config.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		peers:  make(map[peer.ID]*peerBuckets),
		config: cfg,
	}
}

func (rl *RateLimiter) getOrCreate(pid peer.ID) *peerBuckets {
	pb, ok := rl.peers[pid]
	if !ok {
		pb = &peerBuckets{
			global:   newTokenBucket(rl.config.GlobalRate, rl.config.BurstMultiplier),
			proposal: newTokenBucket(rl.config.ProposalRate, rl.config.BurstMultiplier),
			vote:     newTokenBucket(rl.config.VoteRate, rl.config.BurstMultiplier),
			timeout:  newTokenBucket(rl.config.TimeoutRate, rl.config.BurstMultiplier),
			lastSeen: time.Now(),
		}
		rl.peers[pid] = pb
	}
	pb.lastSeen = time.Now()
	return pb
}

// Allow checks whether a message from the given peer of the given type is allowed.
func (rl *RateLimiter) Allow(pid peer.ID, msgType MessageType) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	pb := rl.getOrCreate(pid)

	// Check global limit first.
	if !pb.global.allow() {
		return false
	}

	// Check type-specific limit.
	switch msgType {
	case MsgProposal:
		return pb.proposal.allow()
	case MsgVote:
		return pb.vote.allow()
	case MsgTimeout:
		return pb.timeout.allow()
	default:
		return true
	}
}

// Cleanup removes buckets for peers not seen in the given duration.
func (rl *RateLimiter) Cleanup(staleAfter time.Duration) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-staleAfter)
	removed := 0
	for pid, pb := range rl.peers {
		if pb.lastSeen.Before(cutoff) {
			delete(rl.peers, pid)
			removed++
		}
	}
	return removed
}
