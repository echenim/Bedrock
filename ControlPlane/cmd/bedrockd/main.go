package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "bedrockd",
		Short: "BedRock Protocol Node",
		Long:  "Byzantine Fault Tolerant protocol node with deterministic WASM execution",
	}

	rootCmd.AddCommand(newStartCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newKeysCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("bedrockd v%s\n", version)
		},
	}
}

// defaultHome returns the default node home directory.
func defaultHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".bedrockd"
	}
	return filepath.Join(home, ".bedrockd")
}
