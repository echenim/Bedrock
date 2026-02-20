package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "bedrockd",
		Short: "BedRock Protocol Node",
		Long:  "Byzantine Fault Tolerant protocol node with deterministic WASM execution",
	}

	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the BedRock node",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Placeholder — wired up in task 015
			fmt.Println("bedrockd starting...")
			return nil
		},
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [moniker]",
		Short: "Initialize node configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Initializing node: %s\n", args[0])
			return nil
		},
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
