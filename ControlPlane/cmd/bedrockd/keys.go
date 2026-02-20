package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/spf13/cobra"
)

func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Key management commands",
	}

	cmd.AddCommand(keysGenerateCmd())
	cmd.AddCommand(keysShowCmd())

	return cmd
}

func keysGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new Ed25519 keypair",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")

			pubKey, privKey, err := crypto.GenerateKeypair()
			if err != nil {
				return fmt.Errorf("generate keypair: %w", err)
			}

			addr := crypto.AddressFromPubKey(pubKey)

			if output != "" {
				kf := nodeKeyFile{
					PrivateKey: []byte(privKey),
					PublicKey:  []byte(pubKey),
				}
				data, err := json.MarshalIndent(kf, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal key: %w", err)
				}
				if err := os.WriteFile(output, data, 0o600); err != nil {
					return fmt.Errorf("write key: %w", err)
				}
				fmt.Printf("Key saved to %s\n", output)
			}

			fmt.Printf("Address:     %s\n", hex.EncodeToString(addr[:]))
			fmt.Printf("Public Key:  %s\n", hex.EncodeToString(pubKey))

			return nil
		},
	}

	cmd.Flags().String("output", "", "file path to save the key (JSON format)")

	return cmd
}

func keysShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show node key information",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, _ := cmd.Flags().GetString("home")
			keyPath := filepath.Join(homeDir, "node_key.json")

			data, err := os.ReadFile(keyPath)
			if err != nil {
				return fmt.Errorf("read key file: %w", err)
			}

			var kf nodeKeyFile
			if err := json.Unmarshal(data, &kf); err != nil {
				return fmt.Errorf("parse key file: %w", err)
			}

			pubKey := crypto.PublicKey(kf.PublicKey)
			addr := crypto.AddressFromPubKey(pubKey)

			fmt.Printf("Address:     %s\n", hex.EncodeToString(addr[:]))
			fmt.Printf("Public Key:  %s\n", hex.EncodeToString(pubKey))
			fmt.Printf("Node ID:     %s\n", hex.EncodeToString(addr[:8]))

			return nil
		},
	}

	cmd.Flags().String("home", defaultHome(), "node home directory")

	return cmd
}
