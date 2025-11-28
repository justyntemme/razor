package main

import (
	"flag"
	"github.com/justyntemme/razor/internal/app"
)

func main() {
	debug := flag.Bool("debug", false, "Enable verbose debug logging")
	flag.Parse()

	// Handle OS-specific console visibility
	manageConsole(*debug)

	// Pass the debug flag to the application core
	app.Main(*debug)
}