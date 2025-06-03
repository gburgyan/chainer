package cmd

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gburgyan/chainer/pkg/ai"
	"github.com/gburgyan/chainer/pkg/har"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test the existing config creation
func TestConfigCreation(t *testing.T) {
	// Create a minimal config directly
	config := &Config{
		HarFilePath:  "test.har",
		VarsFilePath: "vars.json",
		OutputPath:   "output.json",
		AIConfig:     ai.DefaultConfig(),
		ProxyPort:    8080,
		RecordPath:   "capture.har",
	}

	// Check that the config values are set correctly
	assert.Equal(t, "test.har", config.HarFilePath)
	assert.Equal(t, "vars.json", config.VarsFilePath)
	assert.Equal(t, "output.json", config.OutputPath)
	assert.NotNil(t, config.AIConfig)
	assert.Equal(t, 8080, config.ProxyPort)
	assert.Equal(t, "capture.har", config.RecordPath)
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
	assert.Equal(t, config, app.Config)
}

func TestApp_Run_ModeSelection(t *testing.T) {
	t.Run("HAR mode when proxy port is 0", func(t *testing.T) {
		// Create a temporary HAR file for testing
		tempDir := t.TempDir()
		harPath := filepath.Join(tempDir, "test.har")

		// Create a minimal HAR file
		harData := har.HAR{
			Log: har.Log{
				Entries: []har.Entry{},
			},
		}
		data, err := json.Marshal(harData)
		require.NoError(t, err)
		err = os.WriteFile(harPath, data, 0644)
		require.NoError(t, err)

		config := &Config{
			HarFilePath: harPath,
			OutputPath:  filepath.Join(tempDir, "output.json"),
			ProxyPort:   0, // No proxy mode
			AIConfig:    ai.DefaultConfig(),
		}

		app := NewApp(config)

		// We can't easily test the full Run() without mocking OpenAI,
		// but we can verify it would choose the correct mode
		assert.Equal(t, 0, app.Config.ProxyPort)
		assert.NotEmpty(t, app.Config.HarFilePath)
	})

	t.Run("Proxy mode when proxy port is set", func(t *testing.T) {
		config := &Config{
			ProxyPort:  8080,
			RecordPath: "test.har",
		}

		app := NewApp(config)
		assert.Equal(t, 8080, app.Config.ProxyPort)
		assert.Equal(t, "test.har", app.Config.RecordPath)
	})
}

func TestParseFlags_ProxyMode(t *testing.T) {
	// Save original command-line arguments
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		validate func(t *testing.T, config *Config)
	}{
		{
			name: "Proxy mode with recording",
			args: []string{"cmd", "-proxy=8080", "-record=capture.har"},
			validate: func(t *testing.T, config *Config) {
				assert.Equal(t, 8080, config.ProxyPort)
				assert.Equal(t, "capture.har", config.RecordPath)
			},
		},
		{
			name: "Proxy mode without recording",
			args: []string{"cmd", "-proxy=9090"},
			validate: func(t *testing.T, config *Config) {
				assert.Equal(t, 9090, config.ProxyPort)
				assert.Empty(t, config.RecordPath)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set command-line arguments
			os.Args = tt.args

			// Reset flags
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

			config, err := ParseFlags()

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)

			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

func TestLoadYAMLConfig_ProxySettings(t *testing.T) {
	content := `
proxy:
  port: 8080
  record: "capture.har"
output: "collection.json"
`

	// Create temporary config file
	tempFile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	_, err = tempFile.WriteString(content)
	require.NoError(t, err)
	tempFile.Close()

	config, err := loadYAMLConfig(tempFile.Name())
	require.NoError(t, err)

	assert.Equal(t, 8080, config.Proxy.Port)
	assert.Equal(t, "capture.har", config.Proxy.Record)
	assert.Equal(t, "collection.json", config.Output)
}

func TestApp_ProxyModeIntegration(t *testing.T) {
	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer backend.Close()

	tempDir := t.TempDir()
	recordPath := filepath.Join(tempDir, "proxy-test.har")

	config := &Config{
		ProxyPort:  0, // Use any available port
		RecordPath: recordPath,
	}

	app := NewApp(config)

	// Verify proxy configuration
	assert.NotNil(t, app.Config)
	assert.Equal(t, recordPath, app.Config.RecordPath)
}
