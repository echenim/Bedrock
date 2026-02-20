package mempool

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// --- Test helpers ---

func testConfig() config.MempoolConfig {
	return config.MempoolConfig{
		MaxSize:    100,
		MaxTxBytes: 1024 * 1024,
		CacheSize:  1000,
	}
}

func makeTestTx(t *testing.T, nonce, fee uint64) ([]byte, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	addr := sha256.Sum256(pub)
	var sender types.Address
	copy(sender[:], addr[:])

	payload := []byte("test-payload")
	raw := BuildTx(sender, nonce, fee, payload, priv)
	return raw, pub
}

func makeTestTxFromKey(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, nonce, fee uint64) []byte {
	t.Helper()
	addr := sha256.Sum256(pub)
	var sender types.Address
	copy(sender[:], addr[:])
	payload := []byte("test-payload")
	return BuildTx(sender, nonce, fee, payload, priv)
}

// --- Validation tests ---

func TestValidateStatelessValid(t *testing.T) {
	cfg := testConfig()
	tx, _ := makeTestTx(t, 0, 100)

	mtx, err := ValidateStateless(tx, cfg)
	if err != nil {
		t.Fatalf("expected valid tx: %v", err)
	}
	if mtx.Fee != 100 {
		t.Fatalf("fee = %d, want 100", mtx.Fee)
	}
	if mtx.Nonce != 0 {
		t.Fatalf("nonce = %d, want 0", mtx.Nonce)
	}
}

func TestValidateStatelessTooSmall(t *testing.T) {
	cfg := testConfig()
	_, err := ValidateStateless([]byte{0x01, 0x02, 0x03}, cfg)
	if err == nil {
		t.Fatal("expected error for too-small tx")
	}
}

func TestValidateStatelessTooLarge(t *testing.T) {
	cfg := testConfig()
	cfg.MaxTxBytes = 100 // very small limit
	tx, _ := makeTestTx(t, 0, 100)

	_, err := ValidateStateless(tx, cfg)
	if err == nil {
		t.Fatal("expected error for oversized tx")
	}
}

func TestValidateStatelessEmptySignature(t *testing.T) {
	cfg := testConfig()
	// Build a tx with zeroed signature.
	raw := make([]byte, txHeaderSize+10)
	copy(raw[0:32], make([]byte, 32))
	raw[0] = 0x01 // non-zero sender
	copy(raw[112:], []byte("test-data!"))

	_, err := ValidateStateless(raw, cfg)
	if err == nil {
		t.Fatal("expected error for empty signature")
	}
}

func TestValidateStatelessZeroSender(t *testing.T) {
	cfg := testConfig()
	// Build a tx with zero sender.
	raw := make([]byte, txHeaderSize+10)
	// sender is all zeros
	raw[48] = 0x01 // non-zero sig to pass sig check first
	copy(raw[112:], []byte("test-data!"))

	_, err := ValidateStateless(raw, cfg)
	if err == nil {
		t.Fatal("expected error for zero sender")
	}
}

func TestValidateStatefulNonceTooLow(t *testing.T) {
	store := storage.NewMemStore()
	// Set nonce to 5 in state.
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr := sha256.Sum256(pub)
	var sender types.Address
	copy(sender[:], addr[:])

	nonceKey := []byte(nonceKeyPrefix + sender.String())
	nonceBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBuf, 5)
	store.ApplyWriteSet(map[string][]byte{string(nonceKey): nonceBuf})

	tx := BuildTx(sender, 3, 100, []byte("payload"), priv)
	mtx, _ := ParseTx(tx)

	err := ValidateStateful(mtx, store)
	if err == nil {
		t.Fatal("expected error for low nonce")
	}
}

func TestValidateStatefulNonceOk(t *testing.T) {
	store := storage.NewMemStore()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr := sha256.Sum256(pub)
	var sender types.Address
	copy(sender[:], addr[:])

	nonceKey := []byte(nonceKeyPrefix + sender.String())
	nonceBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBuf, 5)
	store.ApplyWriteSet(map[string][]byte{string(nonceKey): nonceBuf})

	tx := BuildTx(sender, 5, 100, []byte("payload"), priv)
	mtx, _ := ParseTx(tx)

	err := ValidateStateful(mtx, store)
	if err != nil {
		t.Fatalf("expected valid nonce: %v", err)
	}
}

func TestSignatureVerification(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr := sha256.Sum256(pub)
	var sender types.Address
	copy(sender[:], addr[:])

	tx := BuildTx(sender, 0, 100, []byte("payload"), priv)
	mtx, err := ParseTx(tx)
	if err != nil {
		t.Fatalf("parse tx: %v", err)
	}

	if !VerifySignature(mtx, pub) {
		t.Fatal("expected valid signature")
	}

	// Tamper with fee and verify signature fails.
	binary.LittleEndian.PutUint64(tx[40:48], 999)
	mtx2, _ := ParseTx(tx)
	if VerifySignature(mtx2, pub) {
		t.Fatal("expected invalid signature after tampering")
	}
}

// --- Priority queue tests ---

func TestPriorityQueueOrdering(t *testing.T) {
	pq := NewPriorityQueue()

	pq.PushTx(&MempoolTx{Hash: types.Hash{0x01}, Fee: 100})
	pq.PushTx(&MempoolTx{Hash: types.Hash{0x02}, Fee: 300})
	pq.PushTx(&MempoolTx{Hash: types.Hash{0x03}, Fee: 200})

	// Should pop in order: 300, 200, 100.
	tx1 := pq.PopTx()
	if tx1.Fee != 300 {
		t.Fatalf("expected fee 300, got %d", tx1.Fee)
	}
	tx2 := pq.PopTx()
	if tx2.Fee != 200 {
		t.Fatalf("expected fee 200, got %d", tx2.Fee)
	}
	tx3 := pq.PopTx()
	if tx3.Fee != 100 {
		t.Fatalf("expected fee 100, got %d", tx3.Fee)
	}
}

func TestPriorityQueueDeterministicTiebreaker(t *testing.T) {
	pq := NewPriorityQueue()

	hashA := types.Hash{0x01}
	hashB := types.Hash{0x02}

	pq.PushTx(&MempoolTx{Hash: hashB, Fee: 100})
	pq.PushTx(&MempoolTx{Hash: hashA, Fee: 100})

	// Same fee — lower hash first (deterministic).
	tx1 := pq.PopTx()
	if tx1.Hash != hashA {
		t.Fatalf("expected lower hash first, got %s", tx1.Hash)
	}
}

func TestPriorityQueueRemove(t *testing.T) {
	pq := NewPriorityQueue()

	pq.PushTx(&MempoolTx{Hash: types.Hash{0x01}, Fee: 100})
	pq.PushTx(&MempoolTx{Hash: types.Hash{0x02}, Fee: 200})

	ok := pq.Remove(types.Hash{0x02})
	if !ok {
		t.Fatal("expected Remove to return true")
	}
	if pq.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", pq.Len())
	}
}

func TestPriorityQueueLowestFee(t *testing.T) {
	pq := NewPriorityQueue()

	pq.PushTx(&MempoolTx{Hash: types.Hash{0x01}, Fee: 100})
	pq.PushTx(&MempoolTx{Hash: types.Hash{0x02}, Fee: 50})
	pq.PushTx(&MempoolTx{Hash: types.Hash{0x03}, Fee: 200})

	lowest := pq.LowestFee()
	if lowest.Fee != 50 {
		t.Fatalf("expected lowest fee 50, got %d", lowest.Fee)
	}
}

// --- Eviction cache tests ---

func TestEvictionCacheAddContains(t *testing.T) {
	c := NewEvictionCache(10)

	h := types.Hash{0x01}
	c.Add(h)

	if !c.Contains(h) {
		t.Fatal("expected cache to contain hash")
	}
	if c.Contains(types.Hash{0x02}) {
		t.Fatal("expected cache to not contain other hash")
	}
}

func TestEvictionCacheCapacity(t *testing.T) {
	c := NewEvictionCache(3)

	h1 := types.Hash{0x01}
	h2 := types.Hash{0x02}
	h3 := types.Hash{0x03}
	h4 := types.Hash{0x04}

	c.Add(h1)
	c.Add(h2)
	c.Add(h3)

	if c.Size() != 3 {
		t.Fatalf("expected size 3, got %d", c.Size())
	}

	// Adding h4 should evict h1 (oldest).
	c.Add(h4)

	if c.Contains(h1) {
		t.Fatal("h1 should have been evicted")
	}
	if !c.Contains(h4) {
		t.Fatal("h4 should be present")
	}
	if c.Size() != 3 {
		t.Fatalf("expected size 3 after eviction, got %d", c.Size())
	}
}

func TestEvictionCacheDuplicate(t *testing.T) {
	c := NewEvictionCache(10)

	h := types.Hash{0x01}
	c.Add(h)
	c.Add(h) // duplicate

	if c.Size() != 1 {
		t.Fatalf("expected size 1 after duplicate add, got %d", c.Size())
	}
}

// --- Mempool tests ---

func TestMempoolAddTxValid(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx, _ := makeTestTx(t, 0, 100)
	hash, err := m.AddTx(tx)
	if err != nil {
		t.Fatalf("add tx: %v", err)
	}
	if hash == types.ZeroHash {
		t.Fatal("expected non-zero hash")
	}
	if m.Size() != 1 {
		t.Fatalf("expected size 1, got %d", m.Size())
	}
	if !m.Has(hash) {
		t.Fatal("expected mempool to contain tx")
	}
}

func TestMempoolDuplicateRejected(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx, _ := makeTestTx(t, 0, 100)
	m.AddTx(tx)
	_, err := m.AddTx(tx)
	if err == nil {
		t.Fatal("expected error for duplicate tx")
	}
}

func TestMempoolFullEvictsLowestFee(t *testing.T) {
	cfg := testConfig()
	cfg.MaxSize = 2
	m := NewMempool(cfg, nil, nil)

	tx1, _ := makeTestTx(t, 0, 50)
	tx2, _ := makeTestTx(t, 0, 100)
	tx3, _ := makeTestTx(t, 0, 200)

	m.AddTx(tx1)
	m.AddTx(tx2)

	// Pool is full. tx3 with higher fee should evict tx1 (lowest fee).
	hash3, err := m.AddTx(tx3)
	if err != nil {
		t.Fatalf("add tx3: %v", err)
	}
	if m.Size() != 2 {
		t.Fatalf("expected size 2, got %d", m.Size())
	}
	if !m.Has(hash3) {
		t.Fatal("expected tx3 in mempool")
	}
}

func TestMempoolFullRejectsLowFee(t *testing.T) {
	cfg := testConfig()
	cfg.MaxSize = 2
	m := NewMempool(cfg, nil, nil)

	tx1, _ := makeTestTx(t, 0, 100)
	tx2, _ := makeTestTx(t, 0, 200)
	tx3, _ := makeTestTx(t, 0, 50) // lower than both

	m.AddTx(tx1)
	m.AddTx(tx2)

	_, err := m.AddTx(tx3)
	if err == nil {
		t.Fatal("expected rejection for low-fee tx when pool is full")
	}
}

func TestReapMaxTxsFeeOrder(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx1, _ := makeTestTx(t, 0, 50)
	tx2, _ := makeTestTx(t, 0, 300)
	tx3, _ := makeTestTx(t, 0, 100)

	m.AddTx(tx1)
	m.AddTx(tx2)
	m.AddTx(tx3)

	reaped := m.ReapMaxTxs(1024 * 1024)
	if len(reaped) != 3 {
		t.Fatalf("expected 3 reaped txs, got %d", len(reaped))
	}

	// Verify fee ordering: tx2 (300) > tx3 (100) > tx1 (50).
	mtx1, _ := ParseTx(reaped[0])
	mtx2, _ := ParseTx(reaped[1])
	mtx3, _ := ParseTx(reaped[2])

	if mtx1.Fee < mtx2.Fee || mtx2.Fee < mtx3.Fee {
		t.Fatalf("expected descending fee order: %d, %d, %d",
			mtx1.Fee, mtx2.Fee, mtx3.Fee)
	}
}

func TestReapMaxTxsRespectsMaxBytes(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx1, _ := makeTestTx(t, 0, 100)
	tx2, _ := makeTestTx(t, 0, 200)

	m.AddTx(tx1)
	m.AddTx(tx2)

	// Limit to only one tx's worth of bytes.
	reaped := m.ReapMaxTxs(len(tx1))
	if len(reaped) != 1 {
		t.Fatalf("expected 1 reaped tx, got %d", len(reaped))
	}
}

func TestRemoveTxs(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx1, _ := makeTestTx(t, 0, 100)
	tx2, _ := makeTestTx(t, 0, 200)

	hash1, _ := m.AddTx(tx1)
	hash2, _ := m.AddTx(tx2)

	m.RemoveTxs([]types.Hash{hash1})

	if m.Has(hash1) {
		t.Fatal("tx1 should have been removed")
	}
	if !m.Has(hash2) {
		t.Fatal("tx2 should still be present")
	}
	if m.Size() != 1 {
		t.Fatalf("expected size 1, got %d", m.Size())
	}
}

func TestMempoolFlush(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx, _ := makeTestTx(t, 0, 100)
	m.AddTx(tx)
	m.Flush()

	if m.Size() != 0 {
		t.Fatalf("expected size 0 after flush, got %d", m.Size())
	}
}

func TestDeterministicOrdering(t *testing.T) {
	// Two mempools with same transactions should produce same ReapMaxTxs output.
	cfg := testConfig()
	m1 := NewMempool(cfg, nil, nil)
	m2 := NewMempool(cfg, nil, nil)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	var txs [][]byte
	for i := range 5 {
		tx := makeTestTxFromKey(t, pub, priv, uint64(i), uint64(100+i*10))
		txs = append(txs, tx)
	}

	// Add in different orders.
	for _, tx := range txs {
		m1.AddTx(tx)
	}
	for i := len(txs) - 1; i >= 0; i-- {
		m2.AddTx(txs[i])
	}

	reaped1 := m1.ReapMaxTxs(1024 * 1024)
	reaped2 := m2.ReapMaxTxs(1024 * 1024)

	if len(reaped1) != len(reaped2) {
		t.Fatalf("reap count mismatch: %d vs %d", len(reaped1), len(reaped2))
	}

	for i := range reaped1 {
		h1 := sha256.Sum256(reaped1[i])
		h2 := sha256.Sum256(reaped2[i])
		if h1 != h2 {
			t.Fatalf("reap order mismatch at index %d", i)
		}
	}
}

func TestMempoolRecentlyProcessedRejected(t *testing.T) {
	m := NewMempool(testConfig(), nil, nil)

	tx, _ := makeTestTx(t, 0, 100)
	hash, _ := m.AddTx(tx)

	// Remove tx (simulating commit).
	m.RemoveTxs([]types.Hash{hash})

	// Try to re-add — should be rejected (in eviction cache).
	_, err := m.AddTx(tx)
	if err == nil {
		t.Fatal("expected rejection for recently processed tx")
	}
}

func TestMempoolNonceReplayRejected(t *testing.T) {
	store := storage.NewMemStore()
	m := NewMempool(testConfig(), store, nil)

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	addr := sha256.Sum256(pub)
	var sender types.Address
	copy(sender[:], addr[:])

	// Set nonce to 5 in state.
	nonceKey := nonceKeyPrefix + sender.String()
	nonceBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBuf, 5)
	store.ApplyWriteSet(map[string][]byte{nonceKey: nonceBuf})

	// Try to add tx with nonce 3 (replay).
	tx := BuildTx(sender, 3, 100, []byte("payload"), priv)
	_, err := m.AddTx(tx)
	if err == nil {
		t.Fatal("expected rejection for replayed nonce")
	}
}
