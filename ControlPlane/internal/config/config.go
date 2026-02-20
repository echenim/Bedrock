package config

import (
	"errors"
	"fmt"
	"time"
)

// Duration wraps time.Duration to support TOML string unmarshaling (e.g. "3s").
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler for TOML duration strings.
func (d *Duration) UnmarshalText(text []byte) error {
	dur, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Config represents the full node configuration.
type Config struct {
	Moniker string `toml:"moniker"`
	ChainID string `toml:"chain_id"`

	Consensus ConsensusConfig `toml:"consensus"`
	P2P       P2PConfig       `toml:"p2p"`
	Mempool   MempoolConfig   `toml:"mempool"`
	Storage   StorageConfig   `toml:"storage"`
	RPC       RPCConfig       `toml:"rpc"`
	Execution ExecutionConfig `toml:"execution"`
	Telemetry TelemetryConfig `toml:"telemetry"`
}

// ConsensusConfig holds consensus protocol parameters.
type ConsensusConfig struct {
	TimeoutPropose Duration `toml:"timeout_propose"`
	TimeoutVote    Duration `toml:"timeout_vote"`
	TimeoutCommit  Duration `toml:"timeout_commit"`
	MaxBlockSize   int      `toml:"max_block_size"`
	MaxBlockGas    uint64   `toml:"max_block_gas"`
}

// P2PConfig holds peer-to-peer networking parameters.
type P2PConfig struct {
	ListenAddr  string   `toml:"listen_addr"`
	Seeds       []string `toml:"seeds"`
	MaxPeers    int      `toml:"max_peers"`
	PeerScoring bool     `toml:"peer_scoring"`
}

// MempoolConfig holds mempool parameters.
type MempoolConfig struct {
	MaxSize    int `toml:"max_size"`
	MaxTxBytes int `toml:"max_tx_bytes"`
	CacheSize  int `toml:"cache_size"`
}

// StorageConfig holds storage parameters.
type StorageConfig struct {
	DBPath  string `toml:"db_path"`
	Backend string `toml:"backend"`
}

// RPCConfig holds RPC server parameters.
type RPCConfig struct {
	GRPCAddr string `toml:"grpc_addr"`
	HTTPAddr string `toml:"http_addr"`
}

// ExecutionConfig holds execution engine parameters.
type ExecutionConfig struct {
	WASMPath    string `toml:"wasm_path"`
	GasLimit    uint64 `toml:"gas_limit"`
	FuelLimit   uint64 `toml:"fuel_limit"`
	MaxMemoryMB int    `toml:"max_memory_mb"`
}

// TelemetryConfig holds observability parameters.
type TelemetryConfig struct {
	Enabled bool   `toml:"enabled"`
	Addr    string `toml:"addr"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Moniker: "bedrock-node",
		ChainID: "bedrock-devnet",
		Consensus: ConsensusConfig{
			TimeoutPropose: Duration{3 * time.Second},
			TimeoutVote:    Duration{1 * time.Second},
			TimeoutCommit:  Duration{1 * time.Second},
			MaxBlockSize:   2 * 1024 * 1024, // 2 MB
			MaxBlockGas:    100_000_000,
		},
		P2P: P2PConfig{
			ListenAddr:  "/ip4/0.0.0.0/udp/26656/quic-v1",
			Seeds:       nil,
			MaxPeers:    50,
			PeerScoring: true,
		},
		Mempool: MempoolConfig{
			MaxSize:    10000,
			MaxTxBytes: 1024 * 1024, // 1 MB
			CacheSize:  10000,
		},
		Storage: StorageConfig{
			DBPath:  "data/blockstore",
			Backend: "pebble",
		},
		RPC: RPCConfig{
			GRPCAddr: "0.0.0.0:26657",
			HTTPAddr: "0.0.0.0:26658",
		},
		Execution: ExecutionConfig{
			WASMPath:    "bedrock-execution.wasm",
			GasLimit:    100_000_000,
			FuelLimit:   100_000_000,
			MaxMemoryMB: 256,
		},
		Telemetry: TelemetryConfig{
			Enabled: false,
			Addr:    "0.0.0.0:26660",
		},
	}
}

// Validate checks config for invalid values.
func (c *Config) Validate() error {
	if c.Moniker == "" {
		return errors.New("config: moniker must not be empty")
	}
	if c.ChainID == "" {
		return errors.New("config: chain_id must not be empty")
	}

	// Consensus.
	if c.Consensus.TimeoutPropose.Duration <= 0 {
		return errors.New("config: consensus.timeout_propose must be > 0")
	}
	if c.Consensus.TimeoutVote.Duration <= 0 {
		return errors.New("config: consensus.timeout_vote must be > 0")
	}
	if c.Consensus.TimeoutCommit.Duration <= 0 {
		return errors.New("config: consensus.timeout_commit must be > 0")
	}
	if c.Consensus.MaxBlockSize <= 0 {
		return errors.New("config: consensus.max_block_size must be > 0")
	}

	// P2P.
	if c.P2P.ListenAddr == "" {
		return errors.New("config: p2p.listen_addr must not be empty")
	}
	if c.P2P.MaxPeers <= 0 {
		return errors.New("config: p2p.max_peers must be > 0")
	}

	// Storage.
	if c.Storage.DBPath == "" {
		return errors.New("config: storage.db_path must not be empty")
	}
	validBackends := map[string]bool{"pebble": true, "memory": true}
	if !validBackends[c.Storage.Backend] {
		return fmt.Errorf("config: storage.backend must be 'pebble' or 'memory', got %q", c.Storage.Backend)
	}

	// RPC.
	if c.RPC.GRPCAddr == "" {
		return errors.New("config: rpc.grpc_addr must not be empty")
	}

	// Execution.
	if c.Execution.WASMPath == "" {
		return errors.New("config: execution.wasm_path must not be empty")
	}
	if c.Execution.MaxMemoryMB <= 0 {
		return errors.New("config: execution.max_memory_mb must be > 0")
	}

	return nil
}
