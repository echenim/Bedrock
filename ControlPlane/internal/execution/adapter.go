package execution

import (
	"errors"
	"fmt"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/storage"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// Compile-time check that WASMAdapter implements consensus.ExecutionAdapter.
var _ consensus.ExecutionAdapter = (*WASMAdapter)(nil)

// WASMAdapter implements consensus.ExecutionAdapter using wasmtime-go.
// It loads a WASM artifact, creates sandbox instances, and invokes execution.
//
// Per EXECUTION_SPEC.md §2: The execution lifecycle is:
//  1. Construct ExecutionRequest protobuf from block
//  2. Create new Wasmtime instance with fuel/memory limits
//  3. Link host functions (state_get, state_set, etc.)
//  4. Call bedrock_init
//  5. Call bedrock_execute_block
//  6. Read response from guest memory
//  7. Call bedrock_free
//  8. Verify response (api_version, status, state root format)
//  9. Return ExecutionResult
type WASMAdapter struct {
	sandbox    *Sandbox
	cfg        config.ExecutionConfig
	stateStore storage.StateStore
	logger     *zap.Logger
}

// NewWASMAdapter creates a new WASM execution adapter.
// It loads the WASM module from the configured path.
func NewWASMAdapter(cfg config.ExecutionConfig, stateStore storage.StateStore, logger *zap.Logger) (*WASMAdapter, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	sandbox, err := NewSandbox(cfg)
	if err != nil {
		return nil, fmt.Errorf("execution: create sandbox: %w", err)
	}

	return &WASMAdapter{
		sandbox:    sandbox,
		cfg:        cfg,
		stateStore: stateStore,
		logger:     logger,
	}, nil
}

// ExecuteBlock implements consensus.ExecutionAdapter.
// It executes a block in the WASM sandbox and returns the resulting state root.
//
// Per EXECUTION_SPEC.md §2: execution is a pure function:
//
//	f(previous_state_root, block) → new_state_root
func (w *WASMAdapter) ExecuteBlock(block *types.Block, prevStateRoot types.Hash) (*consensus.ExecutionResult, error) {
	if block == nil {
		return nil, errors.New("execution: nil block")
	}

	w.logger.Debug("executing block",
		zap.Uint64("height", block.Header.Height),
		zap.Int("tx_count", len(block.Transactions)),
	)

	result, err := w.sandbox.Execute(block, prevStateRoot, w.stateStore)
	if err != nil {
		return nil, fmt.Errorf("execution: block %d: %w", block.Header.Height, err)
	}

	w.logger.Debug("block executed",
		zap.Uint64("height", block.Header.Height),
		zap.Uint64("gas_used", result.GasUsed),
		zap.String("state_root", result.StateRoot.String()),
	)

	return result, nil
}

// Close releases the WASM engine and module.
func (w *WASMAdapter) Close() error {
	if w.sandbox != nil {
		return w.sandbox.Close()
	}
	return nil
}
