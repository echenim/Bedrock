package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// LoadFile reads and parses a TOML config file, applies environment variable
// overrides, and validates the result.
// Config precedence: File → Environment variables → Defaults.
func LoadFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse TOML: %w", err)
	}

	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides applies BEDROCK_* environment variable overrides.
// Env var format: BEDROCK_<SECTION>_<FIELD> (e.g., BEDROCK_P2P_LISTEN_ADDR).
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BEDROCK_MONIKER"); v != "" {
		cfg.Moniker = v
	}
	if v := os.Getenv("BEDROCK_CHAIN_ID"); v != "" {
		cfg.ChainID = v
	}

	// Consensus.
	if v := os.Getenv("BEDROCK_CONSENSUS_TIMEOUT_PROPOSE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Consensus.TimeoutPropose = Duration{d}
		}
	}
	if v := os.Getenv("BEDROCK_CONSENSUS_TIMEOUT_VOTE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Consensus.TimeoutVote = Duration{d}
		}
	}
	if v := os.Getenv("BEDROCK_CONSENSUS_TIMEOUT_COMMIT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Consensus.TimeoutCommit = Duration{d}
		}
	}
	if v := os.Getenv("BEDROCK_CONSENSUS_MAX_BLOCK_GAS"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Consensus.MaxBlockGas = n
		}
	}

	// P2P.
	if v := os.Getenv("BEDROCK_P2P_LISTEN_ADDR"); v != "" {
		cfg.P2P.ListenAddr = v
	}
	if v := os.Getenv("BEDROCK_P2P_SEEDS"); v != "" {
		cfg.P2P.Seeds = strings.Split(v, ",")
	}
	if v := os.Getenv("BEDROCK_P2P_MAX_PEERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.P2P.MaxPeers = n
		}
	}

	// Storage.
	if v := os.Getenv("BEDROCK_STORAGE_DB_PATH"); v != "" {
		cfg.Storage.DBPath = v
	}
	if v := os.Getenv("BEDROCK_STORAGE_BACKEND"); v != "" {
		cfg.Storage.Backend = v
	}

	// RPC.
	if v := os.Getenv("BEDROCK_RPC_GRPC_ADDR"); v != "" {
		cfg.RPC.GRPCAddr = v
	}
	if v := os.Getenv("BEDROCK_RPC_HTTP_ADDR"); v != "" {
		cfg.RPC.HTTPAddr = v
	}

	// Execution.
	if v := os.Getenv("BEDROCK_EXECUTION_WASM_PATH"); v != "" {
		cfg.Execution.WASMPath = v
	}
	if v := os.Getenv("BEDROCK_EXECUTION_GAS_LIMIT"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Execution.GasLimit = n
		}
	}
	if v := os.Getenv("BEDROCK_EXECUTION_FUEL_LIMIT"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.Execution.FuelLimit = n
		}
	}

	// Telemetry.
	if v := os.Getenv("BEDROCK_TELEMETRY_ENABLED"); v != "" {
		cfg.Telemetry.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("BEDROCK_TELEMETRY_ADDR"); v != "" {
		cfg.Telemetry.Addr = v
	}
}
