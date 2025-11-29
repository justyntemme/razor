package main

import (
	"flag"
	"github.com/justyntemme/razor/internal/app"
)

func main() {
	debug := flag.Bool("debug", false, "Enable verbose debug logging")
	startPath := flag.String("path", "", "Initial directory path (defaults to user home)")
	flag.Parse()

	// Handle OS-specific console visibility
	manageConsole(*debug)

	// Pass flags to the application core
	app.Main(*debug, *startPath)
}