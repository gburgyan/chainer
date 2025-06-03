package ai

import (
	"fmt"
	"os"
	"strings"

	"github.com/gburgyan/chainer/pkg/template"
)

// Provider represents the AI provider type
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
)

// NewClient creates a new AI client based on the provider specified in the config.
// If no provider is specified, it attempts to auto-detect based on available API keys.
func NewClient(config *Config) (OpenAIClientInterface, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Determine provider
	provider := determineProvider(config)

	switch provider {
	case ProviderOpenAI:
		return NewOpenAIClient(config)
	case ProviderAnthropic:
		return NewAnthropicClient(config)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// NewTemplatedClientWithProvider creates a new templated client with the specified provider.
func NewTemplatedClientWithProvider(config *Config) (*TemplatedClient, error) {
	// Create the base client based on provider
	client, err := NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating AI client: %w", err)
	}

	// Wrap it in a templated client
	templates, err := template.DefaultManager()
	if err != nil {
		return nil, fmt.Errorf("error loading templates: %w", err)
	}

	return &TemplatedClient{
		OpenAIClient: client,
		Templates:    templates,
	}, nil
}

// determineProvider determines which AI provider to use based on configuration and environment.
func determineProvider(config *Config) Provider {
	// Check if provider is explicitly set in config
	if config.Provider != "" {
		return Provider(strings.ToLower(config.Provider))
	}

	// Auto-detect based on model name
	modelLower := strings.ToLower(config.Model)
	if strings.Contains(modelLower, "gpt") || strings.Contains(modelLower, "davinci") || strings.Contains(modelLower, "turbo") {
		return ProviderOpenAI
	}
	if strings.Contains(modelLower, "claude") {
		return ProviderAnthropic
	}

	// Auto-detect based on API key availability
	if config.APIKey != "" {
		// If API key is explicitly set, we can't auto-detect
		// Default to OpenAI for backward compatibility
		return ProviderOpenAI
	}

	// Check environment variables
	if os.Getenv("OPENAI_API_KEY") != "" {
		return ProviderOpenAI
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return ProviderAnthropic
	}

	// Default to OpenAI for backward compatibility
	return ProviderOpenAI
}
