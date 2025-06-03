package ai

import (
	"fmt"
	"testing"
)

func TestDetermineProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		envVars map[string]string
		want    Provider
	}{
		{
			name: "explicit provider in config",
			config: &Config{
				Provider: "anthropic",
			},
			want: ProviderAnthropic,
		},
		{
			name: "detect by GPT model name",
			config: &Config{
				Model: "gpt-4-turbo",
			},
			want: ProviderOpenAI,
		},
		{
			name: "detect by Claude model name",
			config: &Config{
				Model: "claude-3-opus-20240229",
			},
			want: ProviderAnthropic,
		},
		{
			name:   "detect by OpenAI API key in env",
			config: &Config{},
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test",
			},
			want: ProviderOpenAI,
		},
		{
			name:   "detect by Anthropic API key in env",
			config: &Config{},
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
			},
			want: ProviderAnthropic,
		},
		{
			name:   "default to OpenAI when no hints",
			config: &Config{},
			want:   ProviderOpenAI,
		},
		{
			name: "case insensitive provider",
			config: &Config{
				Provider: "OPENAI",
			},
			want: ProviderOpenAI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got := determineProvider(tt.config)
			if got != tt.want {
				t.Errorf("determineProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		envVars  map[string]string
		wantType string
		wantErr  bool
	}{
		{
			name: "creates OpenAI client",
			config: &Config{
				Provider: "openai",
				APIKey:   "test-key",
			},
			wantType: "*ai.OpenAIClient",
			wantErr:  false,
		},
		{
			name: "creates Anthropic client",
			config: &Config{
				Provider: "anthropic",
				APIKey:   "test-key",
			},
			wantType: "*ai.AnthropicClient",
			wantErr:  false,
		},
		{
			name:   "auto-detects OpenAI from env",
			config: nil,
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-test",
			},
			wantType: "*ai.OpenAIClient",
			wantErr:  false,
		},
		{
			name:   "auto-detects Anthropic from env",
			config: &Config{}, // Empty config without default model
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
			},
			wantType: "*ai.AnthropicClient",
			wantErr:  false,
		},
		{
			name: "error when no API key",
			config: &Config{
				Provider: "openai",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client != nil {
				// Check the type of client created
				clientType := fmt.Sprintf("%T", client)
				if clientType != tt.wantType {
					t.Errorf("NewClient() created %v, want %v", clientType, tt.wantType)
				}
			}
		})
	}
}

func TestNewTemplatedClientWithProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "creates templated client with OpenAI",
			config: &Config{
				Provider: "openai",
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "creates templated client with Anthropic",
			config: &Config{
				Provider: "anthropic",
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "error when client creation fails",
			config: &Config{
				Provider: "openai",
				// Missing API key
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			client, err := NewTemplatedClientWithProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTemplatedClientWithProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client != nil {
				if client.OpenAIClient == nil {
					t.Error("TemplatedClient should have a non-nil OpenAIClient")
				}
				if client.Templates == nil {
					t.Error("TemplatedClient should have non-nil Templates")
				}
			}
		})
	}
}
