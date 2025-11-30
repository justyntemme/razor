package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/justyntemme/razor/internal/app"
	"github.com/justyntemme/razor/internal/config"
)

func main() {
	startPath := flag.String("path", "", "Initial directory path (defaults to user home)")
	generateConfig := flag.Bool("generate-config", false, "Generate fresh config.json, backing up existing config with timestamp")
	flag.Parse()

	// Handle --generate-config flag
	if *generateConfig {
		backupPath, err := config.GenerateConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if backupPath != "" {
			fmt.Printf("Existing config backed up to: %s\n", backupPath)
		}
		fmt.Printf("Fresh config generated at: %s\n", config.ConfigPath())
		os.Exit(0)
	}

	// Handle OS-specific console visibility based on build tags
	manageConsole()

	app.Main(*startPath)
}
