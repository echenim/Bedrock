package main

import (
	"fmt"
	"os"
)

var version = "0.1.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("bedrockd v%s\n", version)
		return
	}

	fmt.Println("bedrockd - BedRock Protocol Node")
	fmt.Println("Usage: bedrockd <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  start      Start the BedRock node")
	fmt.Println("  init       Initialize node configuration")
	fmt.Println("  version    Print version information")
}
