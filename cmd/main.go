package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

func main() {
	// Parse command line flags
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Initialize logger
	logger.Init(*debug)

	logger.Info("Starting YA8HODA bot...")

	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		logger.Error("Failed to get working directory: %v", err)
		os.Exit(1)
	}

	// Construct path to the bot executable
	botPath := filepath.Join(wd, "cmd", "bot")

	// Create command to run the bot
	cmd := exec.Command(botPath)
	if *debug {
		cmd.Args = append(cmd.Args, "-debug")
	}

	// Set up command output to use current stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the bot
	logger.Info("Executing bot from: %s", botPath)
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to run bot: %v", err)
		os.Exit(1)
	}
}
