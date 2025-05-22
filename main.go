package main

import (
	"log"

	"github.com/gburgyan/chainer/cmd"
)

func main() {
	// Parse command-line flags
	config, err := cmd.ParseFlags()
	if err != nil {
		log.Fatalf("Error parsing flags: %v", err)
	}

	// Create and run the application
	app := cmd.NewApp(config)
	if err := app.Run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
