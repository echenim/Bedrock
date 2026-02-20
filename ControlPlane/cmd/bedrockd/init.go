package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/echenim/Bedrock/controlplane/internal/config"
	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [moniker]",
		Short: "Initialize a new BedRock node",
		Args:  cobra.ExactArgs(1),
		RunE:  runInit,
	}

	cmd.Flags().String("home", defaultHome(), "node home directory")
	cmd.Flags().String("chain-id", "bedrock-devnet", "chain ID")

	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	moniker := args[0]
	homeDir, _ := cmd.Flags().GetString("home")
	chainID, _ := cmd.Flags().GetString("chain-id")

	// Create home directory structure.
	dirs := []string{
		homeDir,
		filepath.Join(homeDir, "data"),
		filepath.Join(homeDir, "wasm"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Generate node key.
	pubKey, privKey, err := crypto.GenerateKeypair()
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}

	keyPath := filepath.Join(homeDir, "node_key.json")
	if err := writeNodeKey(keyPath, privKey, pubKey); err != nil {
		return err
	}

	// Write default config.
	cfg := config.DefaultConfig()
	cfg.Moniker = moniker
	cfg.ChainID = chainID
	configPath := filepath.Join(homeDir, "config.toml")
	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}

	// Write genesis.
	addr := crypto.AddressFromPubKey(pubKey)
	genesisPath := filepath.Join(homeDir, "genesis.json")
	if err := writeGenesis(genesisPath, chainID, pubKey, addr); err != nil {
		return err
	}

	nodeID := hex.EncodeToString(addr[:8])
	fmt.Printf("Initialized BedRock node\n")
	fmt.Printf("  Home:     %s\n", homeDir)
	fmt.Printf("  Node ID:  %s\n", nodeID)
	fmt.Printf("  Chain:    %s\n", chainID)
	fmt.Printf("  Moniker:  %s\n", moniker)
	fmt.Printf("\nStart with: bedrockd start --home %s\n", homeDir)

	return nil
}

func writeNodeKey(path string, privKey crypto.PrivateKey, pubKey crypto.PublicKey) error {
	kf := nodeKeyFile{
		PrivateKey: []byte(privKey),
		PublicKey:  []byte(pubKey),
	}

	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal node key: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write node key: %w", err)
	}

	return nil
}

func writeConfig(path string, cfg *config.Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func writeGenesis(path string, chainID string, pubKey crypto.PublicKey, addr [32]byte) error {
	gen := genesisDoc{
		ChainID: chainID,
		Validators: []struct {
			Address     string `json:"address"`
			PublicKey   string `json:"public_key"`
			VotingPower uint64 `json:"voting_power"`
		}{
			{
				Address:     hex.EncodeToString(addr[:]),
				PublicKey:   hex.EncodeToString(pubKey),
				VotingPower: 100,
			},
		},
	}

	data, err := json.MarshalIndent(gen, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write genesis: %w", err)
	}

	return nil
}
