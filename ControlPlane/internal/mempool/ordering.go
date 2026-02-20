package mempool

import (
	"bytes"
	"container/heap"
)

// PriorityQueue orders transactions by fee (highest first).
// Deterministic tiebreaker: lower hash sorts first.
type PriorityQueue struct {
	items []*MempoolTx
}

// NewPriorityQueue creates an empty priority queue.
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		items: make([]*MempoolTx, 0),
	}
	heap.Init(pq)
	return pq
}

// Push adds a transaction to the priority queue.
func (pq *PriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*MempoolTx))
}

// Pop removes and returns the highest-priority transaction.
func (pq *PriorityQueue) Pop() interface{} {
	old := pq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	pq.items = old[:n-1]
	return item
}

// Len returns the number of transactions in the queue.
func (pq *PriorityQueue) Len() int {
	return len(pq.items)
}

// Less reports whether element i should sort before element j.
// Higher fee = higher priority. Ties broken by lower hash (deterministic).
func (pq *PriorityQueue) Less(i, j int) bool {
	if pq.items[i].Fee != pq.items[j].Fee {
		return pq.items[i].Fee > pq.items[j].Fee
	}
	return bytes.Compare(pq.items[i].Hash[:], pq.items[j].Hash[:]) < 0
}

// Swap swaps elements i and j.
func (pq *PriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Peek returns the highest-priority transaction without removing it.
func (pq *PriorityQueue) Peek() *MempoolTx {
	if len(pq.items) == 0 {
		return nil
	}
	return pq.items[0]
}

// PopTx removes and returns the highest-priority transaction.
func (pq *PriorityQueue) PopTx() *MempoolTx {
	if len(pq.items) == 0 {
		return nil
	}
	return heap.Pop(pq).(*MempoolTx)
}

// PushTx adds a transaction to the queue.
func (pq *PriorityQueue) PushTx(tx *MempoolTx) {
	heap.Push(pq, tx)
}

// Remove removes a transaction by hash. Returns true if found.
func (pq *PriorityQueue) Remove(hash [32]byte) bool {
	for i, item := range pq.items {
		if item.Hash == hash {
			heap.Remove(pq, i)
			return true
		}
	}
	return false
}

// All returns all transactions in priority order (highest fee first).
func (pq *PriorityQueue) All() []*MempoolTx {
	// Clone and sort to return in priority order.
	result := make([]*MempoolTx, len(pq.items))
	copy(result, pq.items)

	// Re-sort to return in correct priority order.
	tmp := &PriorityQueue{items: result}
	sorted := make([]*MempoolTx, 0, len(result))
	for tmp.Len() > 0 {
		sorted = append(sorted, heap.Pop(tmp).(*MempoolTx))
	}
	return sorted
}

// LowestFee returns the lowest-fee transaction in the queue, or nil if empty.
func (pq *PriorityQueue) LowestFee() *MempoolTx {
	if len(pq.items) == 0 {
		return nil
	}
	lowest := pq.items[0]
	for _, item := range pq.items[1:] {
		if item.Fee < lowest.Fee {
			lowest = item
		}
	}
	return lowest
}
