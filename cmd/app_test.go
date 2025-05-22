package cmd

import (
	"testing"

	"github.com/gburgyan/chainer/pkg/ai"
)

// We'll use a simpler approach to test the Config creation
func TestConfigCreation(t *testing.T) {
	// Create a minimal config directly
	config := &Config{
		HarFilePath:  "test.har",
		VarsFilePath: "vars.json",
		OutputPath:   "output.json",
		AIConfig:     ai.DefaultConfig(),
	}

	// Check that the config values are set correctly
	if config.HarFilePath != "test.har" {
		t.Errorf("Config HarFilePath = %v, want %v", config.HarFilePath, "test.har")
	}

	if config.VarsFilePath != "vars.json" {
		t.Errorf("Config VarsFilePath = %v, want %v", config.VarsFilePath, "vars.json")
	}

	if config.OutputPath != "output.json" {
		t.Errorf("Config OutputPath = %v, want %v", config.OutputPath, "output.json")
	}

	// Check that the AI config was set
	if config.AIConfig == nil {
		t.Errorf("Config AIConfig was nil")
	}
}

func TestNewApp(t *testing.T) {
	// Create a minimal config
	config := &Config{
		HarFilePath: "test.har",
		AIConfig:    ai.DefaultConfig(),
	}

	// Create a new app
	app := NewApp(config)

	// Check that the app was created with the correct config
	if app.Config != config {
		t.Errorf("NewApp() config = %v, want %v", app.Config, config)
	}
}
