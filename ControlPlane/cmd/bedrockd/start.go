package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/node"
	"github.com/echenim/Bedrock/controlplane/internal/telemetry"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the BedRock node",
		RunE:  runStart,
	}

	cmd.Flags().String("home", defaultHome(), "node home directory")
	cmd.Flags().String("config", "", "path to config file (default: <home>/config.toml)")
	cmd.Flags().String("genesis", "", "path to genesis file (default: <home>/genesis.json)")
	cmd.Flags().String("log-level", "development", "log level: development or production")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	homeDir, _ := cmd.Flags().GetString("home")
	logLevel, _ := cmd.Flags().GetString("log-level")

	// Setup logger.
	logger, err := telemetry.NewLogger(logLevel)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer logger.Sync()

	// Load config.
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = filepath.Join(homeDir, "config.toml")
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Resolve paths relative to home dir.
	if !filepath.IsAbs(cfg.Storage.DBPath) {
		cfg.Storage.DBPath = filepath.Join(homeDir, cfg.Storage.DBPath)
	}
	if !filepath.IsAbs(cfg.Execution.WASMPath) {
		cfg.Execution.WASMPath = filepath.Join(homeDir, cfg.Execution.WASMPath)
	}

	// Load node key.
	privKey, err := loadNodeKey(filepath.Join(homeDir, "node_key.json"))
	if err != nil {
		return fmt.Errorf("load node key: %w", err)
	}

	// Load genesis (for validator set).
	genesisPath, _ := cmd.Flags().GetString("genesis")
	if genesisPath == "" {
		genesisPath = filepath.Join(homeDir, "genesis.json")
	}

	valSet, err := loadGenesisValidators(genesisPath, privKey)
	if err != nil {
		return fmt.Errorf("load genesis: %w", err)
	}

	// Create and start node.
	n, err := node.NewNode(cfg, privKey, valSet, logger)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	// Handle OS signals for graceful shutdown.
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := n.Start(ctx); err != nil {
		return fmt.Errorf("start node: %w", err)
	}

	fmt.Println("BedRock node started. Press Ctrl+C to stop.")

	// Wait for shutdown signal.
	<-ctx.Done()
	fmt.Println("\nShutdown signal received...")

	return n.Stop()
}

func loadConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Use defaults.
			return config.DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := config.DefaultConfig()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// nodeKeyFile represents the JSON structure for storing node keys.
type nodeKeyFile struct {
	PrivateKey []byte `json:"private_key"`
	PublicKey  []byte `json:"public_key"`
}

func loadNodeKey(path string) (crypto.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read node key: %w", err)
	}

	var kf nodeKeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("parse node key: %w", err)
	}

	return crypto.PrivateKey(kf.PrivateKey), nil
}

// genesisDoc is a simplified genesis document.
type genesisDoc struct {
	ChainID    string `json:"chain_id"`
	Validators []struct {
		Address     string `json:"address"`
		PublicKey   string `json:"public_key"`
		VotingPower uint64 `json:"voting_power"`
	} `json:"validators"`
}

func loadGenesisValidators(path string, privKey crypto.PrivateKey) (*types.ValidatorSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Default single-validator genesis for dev mode.
			return createDevValidatorSet(privKey)
		}
		return nil, err
	}

	var gen genesisDoc
	if err := json.Unmarshal(data, &gen); err != nil {
		return nil, fmt.Errorf("parse genesis: %w", err)
	}

	if len(gen.Validators) == 0 {
		return createDevValidatorSet(privKey)
	}

	validators := make([]types.Validator, len(gen.Validators))
	for i, v := range gen.Validators {
		pubKey := crypto.PubKeyTo32([]byte(v.PublicKey))
		validators[i] = types.Validator{
			Address:     crypto.AddressFromPubKey(pubKey[:]),
			PublicKey:   pubKey,
			VotingPower: v.VotingPower,
		}
	}

	return types.NewValidatorSet(validators)
}

func createDevValidatorSet(privKey crypto.PrivateKey) (*types.ValidatorSet, error) {
	pubKey := privKey.Public().(crypto.PublicKey)
	addr := crypto.AddressFromPubKey(pubKey)

	return types.NewValidatorSet([]types.Validator{
		{
			Address:     addr,
			PublicKey:   crypto.PubKeyTo32(pubKey),
			VotingPower: 100,
		},
	})
}
