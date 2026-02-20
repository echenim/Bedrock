package consensus

import (
	"fmt"
	"sync"

	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// EvidencePool collects and validates slashing evidence.
// Per SPEC-v0.2.md §6 (slashing conditions).
type EvidencePool struct {
	mu       sync.Mutex
	evidence map[types.Address]*types.SlashingEvidence
}

// NewEvidencePool creates a new EvidencePool.
func NewEvidencePool() *EvidencePool {
	return &EvidencePool{
		evidence: make(map[types.Address]*types.SlashingEvidence),
	}
}

// AddEvidence records equivocation evidence.
func (ep *EvidencePool) AddEvidence(ev *types.SlashingEvidence) error {
	if ev == nil {
		return fmt.Errorf("nil evidence")
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	var validatorID types.Address
	if ev.DoubleVote != nil {
		validatorID = ev.DoubleVote.ValidatorID
	} else if ev.DoubleProposal != nil {
		validatorID = ev.DoubleProposal.ValidatorID
	} else {
		return fmt.Errorf("evidence has no double vote or double proposal")
	}

	// Don't overwrite existing evidence for the same validator.
	if _, exists := ep.evidence[validatorID]; exists {
		return nil
	}

	ep.evidence[validatorID] = ev
	return nil
}

// GetPendingEvidence returns all evidence not yet included in a block.
func (ep *EvidencePool) GetPendingEvidence() []*types.SlashingEvidence {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	result := make([]*types.SlashingEvidence, 0, len(ep.evidence))
	for _, ev := range ep.evidence {
		result = append(result, ev)
	}
	return result
}

// HasEvidence returns true if evidence exists for the given validator.
func (ep *EvidencePool) HasEvidence(addr types.Address) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	_, ok := ep.evidence[addr]
	return ok
}

// Size returns the number of pending evidence items.
func (ep *EvidencePool) Size() int {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	return len(ep.evidence)
}
