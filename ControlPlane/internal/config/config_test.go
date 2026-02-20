package config_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig should be valid: %v", err)
	}
}

func TestDefaultConfigValues(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Moniker != "bedrock-node" {
		t.Errorf("expected moniker 'bedrock-node', got %q", cfg.Moniker)
	}
	if cfg.Consensus.TimeoutPropose.Duration.String() != "3s" {
		t.Errorf("expected timeout_propose 3s, got %v", cfg.Consensus.TimeoutPropose)
	}
	if cfg.P2P.MaxPeers != 50 {
		t.Errorf("expected max_peers 50, got %d", cfg.P2P.MaxPeers)
	}
	if cfg.Storage.Backend != "pebble" {
		t.Errorf("expected backend 'pebble', got %q", cfg.Storage.Backend)
	}
	if cfg.RPC.GRPCAddr != "0.0.0.0:26657" {
		t.Errorf("expected grpc_addr '0.0.0.0:26657', got %q", cfg.RPC.GRPCAddr)
	}
}

func TestValidateRejectsEmptyMoniker(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Moniker = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("should reject empty moniker")
	}
}

func TestValidateRejectsInvalidBackend(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Storage.Backend = "sqlite"
	if err := cfg.Validate(); err == nil {
		t.Fatal("should reject invalid storage backend")
	}
}

func TestValidateRejectsZeroTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Consensus.TimeoutPropose = config.Duration{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("should reject zero timeout_propose")
	}
}

func TestLoadFileFromTOML(t *testing.T) {
	tomlContent := `
moniker = "my-validator"
chain_id = "bedrock-main"

[consensus]
timeout_propose = "5s"
timeout_vote = "2s"
timeout_commit = "2s"
max_block_size = 4194304
max_block_gas = 200000000

[p2p]
listen_addr = "/ip4/0.0.0.0/tcp/26656"
max_peers = 100
peer_scoring = true

[mempool]
max_size = 5000
max_tx_bytes = 524288
cache_size = 5000

[storage]
db_path = "data/mystore"
backend = "pebble"

[rpc]
grpc_addr = "0.0.0.0:9090"
http_addr = "0.0.0.0:8080"

[execution]
wasm_path = "/opt/bedrock/execution.wasm"
gas_limit = 200000000
fuel_limit = 200000000
max_memory_mb = 512

[telemetry]
enabled = true
addr = "0.0.0.0:9100"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if cfg.Moniker != "my-validator" {
		t.Errorf("expected moniker 'my-validator', got %q", cfg.Moniker)
	}
	if cfg.ChainID != "bedrock-main" {
		t.Errorf("expected chain_id 'bedrock-main', got %q", cfg.ChainID)
	}
	if cfg.Consensus.TimeoutPropose.Duration.String() != "5s" {
		t.Errorf("expected timeout_propose 5s, got %v", cfg.Consensus.TimeoutPropose)
	}
	if cfg.P2P.MaxPeers != 100 {
		t.Errorf("expected max_peers 100, got %d", cfg.P2P.MaxPeers)
	}
	if cfg.Storage.DBPath != "data/mystore" {
		t.Errorf("expected db_path 'data/mystore', got %q", cfg.Storage.DBPath)
	}
	if cfg.RPC.GRPCAddr != "0.0.0.0:9090" {
		t.Errorf("expected grpc_addr '0.0.0.0:9090', got %q", cfg.RPC.GRPCAddr)
	}
	if cfg.Execution.WASMPath != "/opt/bedrock/execution.wasm" {
		t.Errorf("expected wasm_path, got %q", cfg.Execution.WASMPath)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("expected telemetry enabled")
	}
}

func TestLoadFileEnvOverrides(t *testing.T) {
	tomlContent := `
moniker = "original"
chain_id = "test"

[consensus]
timeout_propose = "3s"
timeout_vote = "1s"
timeout_commit = "1s"
max_block_size = 1048576

[p2p]
listen_addr = "/ip4/0.0.0.0/tcp/26656"
max_peers = 50
peer_scoring = true

[storage]
db_path = "data/blockstore"
backend = "pebble"

[rpc]
grpc_addr = "0.0.0.0:26657"

[execution]
wasm_path = "test.wasm"
max_memory_mb = 256
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set env vars.
	t.Setenv("BEDROCK_MONIKER", "env-override")
	t.Setenv("BEDROCK_P2P_MAX_PEERS", "200")
	t.Setenv("BEDROCK_TELEMETRY_ENABLED", "true")

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if cfg.Moniker != "env-override" {
		t.Errorf("env override failed for moniker: got %q", cfg.Moniker)
	}
	if cfg.P2P.MaxPeers != 200 {
		t.Errorf("env override failed for max_peers: got %d", cfg.P2P.MaxPeers)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("env override failed for telemetry.enabled")
	}
}

func TestLoadFileRejectsInvalid(t *testing.T) {
	// Missing file.
	_, err := config.LoadFile("/nonexistent/config.toml")
	if err == nil {
		t.Fatal("should reject missing file")
	}

	// Invalid TOML.
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("{{invalid toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = config.LoadFile(path)
	if err == nil {
		t.Fatal("should reject invalid TOML")
	}
}

// --- Genesis ---

func TestLoadGenesis(t *testing.T) {
	pub1, _, _ := crypto.GenerateKeypair()
	pub2, _, _ := crypto.GenerateKeypair()
	addr1 := crypto.AddressFromPubKey(pub1)
	addr2 := crypto.AddressFromPubKey(pub2)

	genesisJSON := `{
  "chain_id": "bedrock-test",
  "genesis_time": "2024-01-01T00:00:00Z",
  "validators": [
    {
      "address": "` + hex.EncodeToString(addr1[:]) + `",
      "pub_key": "` + hex.EncodeToString(pub1) + `",
      "power": 100,
      "name": "validator-1"
    },
    {
      "address": "` + hex.EncodeToString(addr2[:]) + `",
      "pub_key": "` + hex.EncodeToString(pub2) + `",
      "power": 200,
      "name": "validator-2"
    }
  ],
  "app_state_root": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "consensus_params": {
    "max_block_size": 2097152,
    "max_block_gas": 100000000,
    "max_validators": 100
  }
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(path, []byte(genesisJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	gen, err := config.LoadGenesis(path)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}

	if gen.ChainID != "bedrock-test" {
		t.Errorf("expected chain_id 'bedrock-test', got %q", gen.ChainID)
	}
	if len(gen.Validators) != 2 {
		t.Fatalf("expected 2 validators, got %d", len(gen.Validators))
	}
	if gen.Validators[0].Power != 100 {
		t.Errorf("expected power 100, got %d", gen.Validators[0].Power)
	}
}

func TestGenesisToValidatorSet(t *testing.T) {
	pub1, _, _ := crypto.GenerateKeypair()
	pub2, _, _ := crypto.GenerateKeypair()
	addr1 := crypto.AddressFromPubKey(pub1)
	addr2 := crypto.AddressFromPubKey(pub2)

	genesisJSON := `{
  "chain_id": "test",
  "genesis_time": "2024-01-01T00:00:00Z",
  "validators": [
    {
      "address": "` + hex.EncodeToString(addr1[:]) + `",
      "pub_key": "` + hex.EncodeToString(pub1) + `",
      "power": 100,
      "name": "v1"
    },
    {
      "address": "` + hex.EncodeToString(addr2[:]) + `",
      "pub_key": "` + hex.EncodeToString(pub2) + `",
      "power": 200,
      "name": "v2"
    }
  ],
  "consensus_params": {
    "max_block_size": 1048576,
    "max_block_gas": 50000000,
    "max_validators": 10
  }
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(path, []byte(genesisJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	gen, err := config.LoadGenesis(path)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}

	valSet, err := gen.ToValidatorSet()
	if err != nil {
		t.Fatalf("ToValidatorSet: %v", err)
	}

	if valSet.Size() != 2 {
		t.Fatalf("expected 2 validators, got %d", valSet.Size())
	}
	if valSet.TotalPower != 300 {
		t.Fatalf("expected total power 300, got %d", valSet.TotalPower)
	}
}

func TestGenesisAppStateRootHash(t *testing.T) {
	pub, _, _ := crypto.GenerateKeypair()
	addr := crypto.AddressFromPubKey(pub)

	genesisJSON := `{
  "chain_id": "test",
  "genesis_time": "2024-01-01T00:00:00Z",
  "validators": [{"address": "` + hex.EncodeToString(addr[:]) + `", "pub_key": "` + hex.EncodeToString(pub) + `", "power": 100, "name": "v"}],
  "app_state_root": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "consensus_params": {"max_block_size": 1048576, "max_block_gas": 50000000, "max_validators": 10}
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(path, []byte(genesisJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	gen, err := config.LoadGenesis(path)
	if err != nil {
		t.Fatalf("LoadGenesis: %v", err)
	}

	root, err := gen.AppStateRootHash()
	if err != nil {
		t.Fatalf("AppStateRootHash: %v", err)
	}
	if root.IsZero() {
		t.Fatal("app state root should not be zero")
	}
	if root.String() != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("unexpected app state root: %s", root.String())
	}
}

func TestGenesisValidateRejectsEmpty(t *testing.T) {
	_, err := config.LoadGenesis("/nonexistent/genesis.json")
	if err == nil {
		t.Fatal("should reject missing file")
	}
}

func TestGenesisValidateRejectsNoValidators(t *testing.T) {
	genesisJSON := `{
  "chain_id": "test",
  "genesis_time": "2024-01-01T00:00:00Z",
  "validators": [],
  "consensus_params": {"max_block_size": 1048576, "max_block_gas": 50000000, "max_validators": 10}
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(path, []byte(genesisJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.LoadGenesis(path)
	if err == nil {
		t.Fatal("should reject empty validator set")
	}
}
