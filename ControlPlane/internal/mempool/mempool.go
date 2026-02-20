package mempool

import (
	"errors"
	"sync"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// MempoolTx is a validated transaction in the mempool.
type MempoolTx struct {
	Hash    types.Hash
	Data    []byte
	Fee     uint64
	Nonce   uint64
	Sender  types.Address
	Size    int
	AddedAt time.Time

	// Internal fields not exported.
	sig     [64]byte
	payload []byte
}

// Mempool manages pending transactions before block inclusion.
// It implements the consensus.TxProvider interface.
type Mempool struct {
	mu         sync.RWMutex
	txs        *PriorityQueue
	txByHash   map[types.Hash]*MempoolTx
	cache      *EvictionCache
	cfg        config.MempoolConfig
	stateStore storage.StateStore
	logger     *zap.Logger
}

// NewMempool creates a new transaction mempool.
func NewMempool(cfg config.MempoolConfig, stateStore storage.StateStore, logger *zap.Logger) *Mempool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Mempool{
		txs:        NewPriorityQueue(),
		txByHash:   make(map[types.Hash]*MempoolTx),
		cache:      NewEvictionCache(cfg.CacheSize),
		cfg:        cfg,
		stateStore: stateStore,
		logger:     logger,
	}
}

// AddTx validates and adds a transaction to the mempool.
// Returns the tx hash on success or an error if validation fails or mempool is full.
// Per SPEC.md §15: stateless validation, then stateful validation.
func (m *Mempool) AddTx(tx []byte) (types.Hash, error) {
	// Phase 1: Stateless validation.
	mtx, err := ValidateStateless(tx, m.cfg)
	if err != nil {
		return types.ZeroHash, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Duplicate check (already in mempool).
	if _, exists := m.txByHash[mtx.Hash]; exists {
		return mtx.Hash, errors.New("mempool: duplicate transaction")
	}

	// Recently evicted/committed check.
	if m.cache.Contains(mtx.Hash) {
		return mtx.Hash, errors.New("mempool: transaction recently processed")
	}

	// Phase 2: Stateful validation.
	if err := ValidateStateful(mtx, m.stateStore); err != nil {
		return types.ZeroHash, err
	}

	// Mempool full — evict lowest fee.
	if len(m.txByHash) >= m.cfg.MaxSize {
		lowest := m.txs.LowestFee()
		if lowest == nil || mtx.Fee <= lowest.Fee {
			return types.ZeroHash, errors.New("mempool: full and tx fee too low")
		}
		m.removeTxLocked(lowest.Hash)
		m.cache.Add(lowest.Hash)
	}

	mtx.AddedAt = time.Now()
	m.txByHash[mtx.Hash] = mtx
	m.txs.PushTx(mtx)

	m.logger.Debug("transaction added to mempool",
		zap.String("hash", mtx.Hash.String()),
		zap.Uint64("fee", mtx.Fee),
		zap.Int("pool_size", len(m.txByHash)),
	)

	return mtx.Hash, nil
}

// ReapMaxTxs returns up to maxBytes worth of transactions ordered by fee.
// This implements the consensus.TxProvider interface.
// Per SPEC.md §15: deterministic ordering for block proposals.
func (m *Mempool) ReapMaxTxs(maxBytes int) [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.txs.Len() == 0 {
		return nil
	}

	// Get all transactions in priority order.
	sorted := m.txs.All()

	var (
		result    [][]byte
		totalSize int
	)

	for _, tx := range sorted {
		if totalSize+tx.Size > maxBytes {
			continue
		}
		result = append(result, tx.Data)
		totalSize += tx.Size
	}

	return result
}

// RemoveTxs removes committed transactions from the mempool.
func (m *Mempool) RemoveTxs(txHashes []types.Hash) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, hash := range txHashes {
		m.removeTxLocked(hash)
		m.cache.Add(hash)
	}
}

// removeTxLocked removes a single tx from the pool. Must be called with mu held.
func (m *Mempool) removeTxLocked(hash types.Hash) {
	if _, exists := m.txByHash[hash]; !exists {
		return
	}
	delete(m.txByHash, hash)
	m.txs.Remove(hash)
}

// Size returns the current number of transactions in the mempool.
func (m *Mempool) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.txByHash)
}

// Flush removes all transactions from the mempool.
func (m *Mempool) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.txByHash = make(map[types.Hash]*MempoolTx)
	m.txs = NewPriorityQueue()
}

// Has checks if a transaction hash is in the mempool.
func (m *Mempool) Has(hash types.Hash) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.txByHash[hash]
	return ok
}

// Get returns a transaction by its hash, if present.
func (m *Mempool) Get(hash types.Hash) *MempoolTx {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.txByHash[hash]
}
