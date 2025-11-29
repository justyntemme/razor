package main

import (
	"flag"
	"github.com/justyntemme/razor/internal/app"
)

func main() {
	startPath := flag.String("path", "", "Initial directory path (defaults to user home)")
	flag.Parse()

	// Handle OS-specific console visibility based on build tags
	manageConsole()

	app.Main(*startPath)
}
