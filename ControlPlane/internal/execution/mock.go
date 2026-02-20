package execution

import (
	"errors"

	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// Compile-time check that MockExecutor implements consensus.ExecutionAdapter.
var _ consensus.ExecutionAdapter = (*MockExecutor)(nil)

// MockExecutor implements consensus.ExecutionAdapter for testing.
// It returns configurable results without actual WASM execution.
type MockExecutor struct {
	NextStateRoot types.Hash
	NextGasUsed   uint64
	ShouldFail    bool
	FailError     error

	// CallCount tracks how many times ExecuteBlock was called.
	CallCount int
	// LastBlock records the most recent block passed to ExecuteBlock.
	LastBlock *types.Block
	// LastPrevRoot records the most recent prevStateRoot.
	LastPrevRoot types.Hash
}

// NewMockExecutor creates a MockExecutor with default settings.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{}
}

// ExecuteBlock implements consensus.ExecutionAdapter.
func (m *MockExecutor) ExecuteBlock(block *types.Block, prevStateRoot types.Hash) (*consensus.ExecutionResult, error) {
	m.CallCount++
	m.LastBlock = block
	m.LastPrevRoot = prevStateRoot

	if m.ShouldFail {
		if m.FailError != nil {
			return nil, m.FailError
		}
		return nil, errors.New("mock: execution failed")
	}

	return &consensus.ExecutionResult{
		StateRoot: m.NextStateRoot,
		GasUsed:   m.NextGasUsed,
	}, nil
}
