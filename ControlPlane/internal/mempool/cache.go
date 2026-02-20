package mempool

import (
	"sync"

	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// EvictionCache tracks recently evicted and committed transaction hashes
// to avoid re-accepting them. Uses a fixed-size ring buffer.
type EvictionCache struct {
	mu       sync.RWMutex
	hashes   map[types.Hash]struct{}
	ring     []types.Hash
	pos      int
	capacity int
}

// NewEvictionCache creates a cache with the given capacity.
func NewEvictionCache(capacity int) *EvictionCache {
	if capacity <= 0 {
		capacity = 10000
	}
	return &EvictionCache{
		hashes:   make(map[types.Hash]struct{}, capacity),
		ring:     make([]types.Hash, capacity),
		capacity: capacity,
	}
}

// Add records a transaction hash in the cache.
// If the cache is full, the oldest entry is evicted.
func (c *EvictionCache) Add(hash types.Hash) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.hashes[hash]; ok {
		return
	}

	// Evict the oldest entry at this ring position.
	old := c.ring[c.pos]
	if old != types.ZeroHash {
		delete(c.hashes, old)
	}

	c.ring[c.pos] = hash
	c.hashes[hash] = struct{}{}
	c.pos = (c.pos + 1) % c.capacity
}

// Contains checks if a hash is in the cache.
func (c *EvictionCache) Contains(hash types.Hash) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.hashes[hash]
	return ok
}

// Size returns the current number of entries in the cache.
func (c *EvictionCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.hashes)
}
