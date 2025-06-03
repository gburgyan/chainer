package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gburgyan/chainer/pkg/ai"
	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/postman"
	"github.com/gburgyan/chainer/pkg/proxy"
	"github.com/gburgyan/chainer/pkg/util"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	HarFilePath  string
	VarsFilePath string
	OutputPath   string
	AIConfig     *ai.Config
	// Proxy-related fields
	ProxyPort  int
	RecordPath string
}

// App is the main application struct.
type App struct {
	Config *Config
}

// NewApp creates a new application instance.
func NewApp(config *Config) *App {
	return &App{
		Config: config,
	}
}

// Run executes the main application flow.
// The application can run in two modes:
// 1. HAR Processing Mode: Processes an existing HAR file to create a Postman collection
// 2. Proxy Mode: Starts a proxy server to capture HTTP traffic and optionally create a collection
func (a *App) Run() error {
	// Check if we're in proxy mode
	if a.Config.ProxyPort > 0 {
		return a.runProxyMode()
	}

	// Otherwise, run in HAR processing mode
	return a.runHARMode()
}

// runHARMode executes the HAR processing flow.
// The process follows these steps:
//  1. Initialize all required components (HAR processor, chain finder, AI clients, etc.)
//  2. Parse the HAR file to extract HTTP calls and their request/response data
//  3. Analyze the extracted data to find "chained values" - values that appear in one response
//     and are then used in subsequent requests (e.g., authentication tokens, IDs, etc.)
//  4. Optionally incorporate pre-defined variables from a user-provided JSON file
//  5. Use AI to generate meaningful names for both the variables and API calls
//  6. Build a Postman collection with automatic variable extraction and substitution
//  7. Write the collection to the specified output file
func (a *App) runHARMode() error {
	// Initialize components
	components, err := a.initializeComponents()
	if err != nil {
		return err
	}

	// Process the HAR file and find chained values
	callDetailsList, chainedValues, err := a.processHAR(components)
	if err != nil {
		return err
	}

	// Process predefined variables if specified
	if a.Config.VarsFilePath != "" {
		chainedValues, err = a.processPredefinedVariables(components, callDetailsList, chainedValues)
		if err != nil {
			return err
		}
	}

	// Process chained values and assign names
	if err := a.processChainedValues(components, callDetailsList, chainedValues); err != nil {
		return err
	}

	// Build and write Postman collection
	if err := a.buildPostmanCollection(components, callDetailsList, chainedValues); err != nil {
		return err
	}

	fmt.Println("Postman collection generated successfully at:", a.Config.OutputPath)
	return nil
}

// runProxyMode starts the proxy server and optionally records HAR data
func (a *App) runProxyMode() error {
	// Create proxy server
	proxyServer, err := proxy.New(proxy.Options{
		Port:       a.Config.ProxyPort,
		RecordPath: a.Config.RecordPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create proxy server: %w", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start proxy in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- proxyServer.Start()
	}()

	// Wait for either error or interrupt signal
	select {
	case err := <-errChan:
		return fmt.Errorf("proxy server error: %w", err)
	case <-sigChan:
		log.Println("\nShutting down proxy server...")
		if err := proxyServer.Stop(); err != nil {
			return fmt.Errorf("error stopping proxy: %w", err)
		}

		// If we recorded a HAR file and have an output path, process it
		if a.Config.RecordPath != "" && a.Config.OutputPath != "" {
			log.Println("Processing recorded HAR file...")
			// Update config to process the recorded HAR
			a.Config.HarFilePath = a.Config.RecordPath
			return a.runHARMode()
		}
	}

	return nil
}

// AppComponents holds the main application components
type AppComponents struct {
	HarProcessor         *har.Processor
	ChainFinder          *util.ChainFinder
	OpenAIClient         *ai.OpenAIClient
	TemplatedClient      *ai.TemplatedClient
	VariableNamer        *ai.VariableNamer
	ComplexPathProcessor *ai.ComplexPathProcessor
	PostmanBuilder       *postman.Builder
}

// initializeComponents initializes all the application components.
// This creates instances of:
// - HAR Processor: Parses HAR files and extracts HTTP request/response data
// - Chain Finder: Identifies values that are used across multiple requests
// - OpenAI Client: Direct API client for AI-powered naming
// - Templated Client: Template-based AI client for more structured prompts (optional)
// - Variable Namer: Uses AI to generate meaningful names for extracted variables
// - Postman Builder: Constructs the final Postman collection with all the data
func (a *App) initializeComponents() (*AppComponents, error) {
	// Create basic components
	harProcessor := har.NewProcessor()
	chainFinder := util.NewChainFinder()
	postmanBuilder := postman.NewBuilder()

	// Setup AI client using the factory
	aiClient, err := ai.NewClient(a.Config.AIConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating AI client: %w", err)
	}

	// Try to create a templated client, but fall back to regular client if there's an issue
	var templatedClient *ai.TemplatedClient
	templatedClient, err = ai.NewTemplatedClientWithProvider(a.Config.AIConfig)
	if err != nil {
		log.Printf("Warning: Unable to initialize templated client: %v", err)
		log.Println("Falling back to standard AI client")
		templatedClient = nil
	}

	// Create variable namer with the best available client
	var clientForNamer ai.OpenAIClientInterface = aiClient
	if templatedClient != nil {
		clientForNamer = templatedClient
	}
	variableNamer := ai.NewVariableNamer(clientForNamer)

	// Create complex path processor
	complexPathProcessor := ai.NewComplexPathProcessor(clientForNamer)

	return &AppComponents{
		HarProcessor:         harProcessor,
		ChainFinder:          chainFinder,
		OpenAIClient:         nil, // Deprecated - kept for backward compatibility
		TemplatedClient:      templatedClient,
		VariableNamer:        variableNamer,
		ComplexPathProcessor: complexPathProcessor,
		PostmanBuilder:       postmanBuilder,
	}, nil
}

// processHAR processes the HAR file and identifies chained values.
// This function:
//  1. Reads and parses the HAR file to extract all HTTP calls (request/response pairs)
//  2. Analyzes the calls to find "chained values" - values that appear in one response
//     and are then used in subsequent requests. Common examples include:
//     - Authentication tokens (OAuth, JWT, session IDs)
//     - Resource IDs (user IDs, order IDs, etc.)
//     - Timestamps or nonces used across requests
//  3. Returns both the raw call details and the identified chained values
func (a *App) processHAR(components *AppComponents) ([]*util.CallDetails, []*util.ChainedValueContext, error) {
	log.Println("Processing HAR file:", a.Config.HarFilePath)
	callDetailsList, err := components.HarProcessor.Process(a.Config.HarFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("error processing HAR file: %w", err)
	}

	log.Println("Identifying chained values...")
	chainedValues := components.ChainFinder.FindChainedValues(callDetailsList)

	return callDetailsList, chainedValues, nil
}

// processPredefinedVariables processes predefined variables from a file.
// This allows users to specify variables that should be extracted and used in the collection,
// even if they wouldn't be automatically detected as chained values. This is useful for:
// - Environment-specific values (API keys, base URLs, etc.)
// - Values that appear only once but should be parameterized
// - Values that the automatic detection might miss due to complex patterns
// The variables file should be a JSON array of objects with "name" and "value" properties.
func (a *App) processPredefinedVariables(components *AppComponents, callDetailsList []*util.CallDetails, chainedValues []*util.ChainedValueContext) ([]*util.ChainedValueContext, error) {
	log.Println("Processing predefined variables from:", a.Config.VarsFilePath)
	predefinedVars, err := a.loadPredefinedVars(a.Config.VarsFilePath)
	if err != nil {
		return nil, fmt.Errorf("error loading predefined variables: %w", err)
	}

	updatedChainedValues := components.ChainFinder.ExtractPredefinedVars(callDetailsList, predefinedVars, chainedValues)
	return updatedChainedValues, nil
}

// processChainedValues processes chained values, assigns names, and updates call details.
// This function performs several important steps:
//  1. Logs the detected chained values for debugging purposes
//  2. Re-associates the chained values with their respective HTTP calls
//  3. Uses AI to generate meaningful variable names based on context
//     (e.g., "auth_token" instead of "var1", "user_id" instead of "var2")
//  4. Uses AI to generate descriptive names for each API call in the sequence
//     (e.g., "Login User", "Get User Profile", "Update Account Settings")
func (a *App) processChainedValues(components *AppComponents, callDetailsList []*util.CallDetails, chainedValues []*util.ChainedValueContext) error {
	// Log initial chained values for debugging
	components.ChainFinder.LogChainedValues(chainedValues)

	// Repopulate call details with chained values
	components.ChainFinder.RepopulateCallDetails(chainedValues)

	// Update complex paths to make them more stable (if enabled)
	if a.Config.AIConfig.RefineComplexPaths {
		log.Println("Refining complex JSON paths...")
		components.ComplexPathProcessor.UpdateComplexPaths(chainedValues)
	}

	// Assign variable names using AI
	log.Println("Assigning variable names...")
	if err := components.VariableNamer.AssignVariableNames(chainedValues); err != nil {
		return fmt.Errorf("error assigning variable names: %w", err)
	}

	// Assign call names using OpenAI
	log.Println("Assigning call names...")
	if err := a.assignCallNames(components, callDetailsList); err != nil {
		return fmt.Errorf("error assigning call names: %w", err)
	}

	return nil
}

// buildPostmanCollection builds and writes the Postman collection.
// This final step creates a complete Postman collection that includes:
// - All HTTP requests from the HAR file with proper formatting
// - Variable substitutions in URLs, headers, and request bodies (e.g., {{auth_token}})
// - Test scripts that automatically extract values from responses and save them as variables
// - Pre-request scripts if needed for dynamic value generation
// The resulting collection can be imported into Postman and run without manual modification.
func (a *App) buildPostmanCollection(components *AppComponents, callDetailsList []*util.CallDetails, chainedValues []*util.ChainedValueContext) error {
	log.Println("Building Postman collection...")
	collection := components.PostmanBuilder.BuildCollection(callDetailsList, chainedValues)

	log.Println("Writing collection to:", a.Config.OutputPath)
	if err := components.PostmanBuilder.WriteToFile(collection, a.Config.OutputPath); err != nil {
		return fmt.Errorf("error writing Postman collection: %w", err)
	}

	return nil
}

// CallNameRequest holds the URL and sequence number for naming a call.
type CallNameRequest struct {
	URL      string `json:"url"`
	Sequence int    `json:"sequence"`
}

// CallNameResponse represents the AI-generated name.
type CallNameResponse struct {
	Name string `json:"name"`
}

// assignCallNames uses OpenAI to generate meaningful names for API calls.
// This function sends all the API URLs and their sequence numbers to OpenAI,
// which returns human-readable names for each endpoint. The AI considers:
// - The URL structure and path components
// - The sequence of calls to understand the workflow
// - Common API patterns and naming conventions
// The function includes retry logic for resilience against API failures,
// and supports both templated prompts (if available) and legacy direct prompts.
func (a *App) assignCallNames(components *AppComponents, callDetailsList []*util.CallDetails) error {
	var requests []CallNameRequest
	for i, callDetails := range callDetailsList {
		entry := callDetails.Entry.(*har.Entry)
		requests = append(requests, CallNameRequest{
			URL:      entry.Request.URL,
			Sequence: i + 1,
		})
	}

	var responses []CallNameResponse
	var err error

	// Try using templated client first, if available
	if components.TemplatedClient != nil {
		retryableCall := util.WithRetries(components.TemplatedClient.CallArrayWithTemplate, 3)
		err = retryableCall("call_naming", nil, requests, &responses)
	} else {
		// Fall back to legacy approach with retries
		retryableCall := util.WithRetries(components.OpenAIClient.CallArray, 3)
		err = retryableCall(legacyCallNamingPrompt, requests, &responses)
	}

	if err != nil {
		return fmt.Errorf("error calling OpenAI for call names: %w", err)
	}

	// Ensure there is a 1:1 correspondence between the input and the response
	if len(responses) != len(requests) {
		return errors.New("mismatched response count from OpenAI")
	}

	// Assign the AI-generated names to the respective call details
	for i, callDetails := range callDetailsList {
		callDetails.Name = responses[i].Name
	}

	return nil
}

// Legacy prompt for backward compatibility
const legacyCallNamingPrompt = `
# Role and Objective
You are an API endpoint naming specialist for Postman collections. Your task is to generate concise, descriptive names for API calls based on their URLs and sequence in a workflow.

# Instructions
For each API call in the provided list, create a clear, user-friendly name that accurately reflects the endpoint's purpose and its position in the sequence of API operations.

## Naming Guidelines
- Create concise yet descriptive names
- Reflect both the endpoint's purpose and its order in the sequence
- Ensure names are intuitive for users viewing the collection
- Use consistent naming patterns for similar endpoints
- For repeated calls to the same endpoint, you may use the same name if appropriate

# Reasoning Steps
1. Analyze the URL structure to identify the API resource or action
2. Consider the sequence number to understand where this call fits in the workflow
3. Extract meaningful parts from the URL path that indicate purpose
4. Use domain knowledge of the API to inform naming choices
5. Format the name to be concise but clear for end-users

# Output Format
Return an array of objects, with each object containing a "name" property. The response must be a raw JSON array with no commentary or additional formatting.

# Examples
## Example 1
Input:
[
  {
    "url": "https://api.example.com/v1/air/search",
    "sequence": 1
  },
  {
    "url": "https://api.example.com/v1/air/price",
    "sequence": 2
  }
]

Output:
[
  {
    "name": "Air Search"
  },
  {
    "name": "Air Price"
  }
]

# Context
This naming is part of a HAR to Postman Collection converter. The names you generate will be displayed in the Postman collection
sidebar and will help users understand the purpose of each request in the workflow.

# Final instructions
Always provide a name for every call in the input list. There must be a 1:1 correspondence between the input array and output
array and the ordering MUST be preserved. Return only the raw JSON array without any explanations or decorations.
`

// loadPredefinedVars loads predefined variables from a JSON file.
// The file should contain a JSON array of objects with the following structure:
// [
//
//	{
//	  "name": "api_key",
//	  "value": "sk-1234567890abcdef"
//	},
//	{
//	  "name": "base_url",
//	  "value": "https://api.example.com"
//	}
//
// ]
// These variables will be incorporated into the Postman collection alongside
// the automatically detected chained values.
func (a *App) loadPredefinedVars(filePath string) ([]util.PredefinedVariable, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading variables file: %w", err)
	}

	var vars []util.PredefinedVariable
	if err := json.Unmarshal(data, &vars); err != nil {
		return nil, fmt.Errorf("error parsing variables file: %w", err)
	}

	return vars, nil
}

// YAMLConfig represents the structure of the YAML configuration file
type YAMLConfig struct {
	HarFile  string `yaml:"har_file"`
	VarsFile string `yaml:"vars_file"`
	Output   string `yaml:"output"`
	AI       struct {
		Provider           string  `yaml:"provider"`
		APIKey             string  `yaml:"api_key"`
		Model              string  `yaml:"model"`
		MaxTokens          int     `yaml:"max_tokens"`
		Temperature        float64 `yaml:"temperature"`
		Verbose            bool    `yaml:"verbose"`
		RefineComplexPaths bool    `yaml:"refine_complex_paths"`
	} `yaml:"ai"`
	Proxy struct {
		Port   int    `yaml:"port"`
		Record string `yaml:"record"`
	} `yaml:"proxy"`
}

// ParseFlags parses command-line flags and returns a Config.
// Now supports both command-line flags and config file.
func ParseFlags() (*Config, error) {
	configFile := flag.String("config", "", "Path to the YAML configuration file")
	harFilePath := flag.String("file", "", "Path to the HAR file (overrides config file)")
	varsFilePath := flag.String("vars", "", "Path to the JSON file with pre-defined variables (overrides config file)")
	outputPath := flag.String("output", "", "Output path for the generated Postman collection (overrides config file)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging of AI API calls (overrides config file)")
	// AI-related flags
	provider := flag.String("provider", "", "AI provider: 'openai' or 'anthropic' (auto-detects if not specified)")
	model := flag.String("model", "", "AI model to use (e.g., 'gpt-4', 'claude-3-haiku-20240307')")
	refineComplexPaths := flag.Bool("refine-paths", false, "Enable complex JSON path refinement for more stable extraction")
	// Proxy-related flags
	proxyPort := flag.Int("proxy", 0, "Start proxy server on specified port (e.g., 8080)")
	recordPath := flag.String("record", "", "Record proxy traffic to HAR file at specified path")

	flag.Parse()

	var config *Config

	if *configFile != "" {
		// Load from config file
		yamlConfig, err := loadYAMLConfig(*configFile)
		if err != nil {
			return nil, fmt.Errorf("error loading config file: %w", err)
		}

		// Create AI config from YAML
		aiConfig := ai.DefaultConfig()
		if yamlConfig.AI.Provider != "" {
			aiConfig.Provider = yamlConfig.AI.Provider
		}
		if yamlConfig.AI.APIKey != "" {
			aiConfig.APIKey = yamlConfig.AI.APIKey
		}
		if yamlConfig.AI.Model != "" {
			aiConfig.Model = yamlConfig.AI.Model
		}
		if yamlConfig.AI.MaxTokens > 0 {
			aiConfig.MaxTokens = yamlConfig.AI.MaxTokens
		}
		// Note: Temperature is not currently supported in ai.Config
		// This would need to be added to the ai package if needed
		aiConfig.Verbose = yamlConfig.AI.Verbose
		aiConfig.RefineComplexPaths = yamlConfig.AI.RefineComplexPaths

		config = &Config{
			HarFilePath:  yamlConfig.HarFile,
			VarsFilePath: yamlConfig.VarsFile,
			OutputPath:   yamlConfig.Output,
			AIConfig:     aiConfig,
			ProxyPort:    yamlConfig.Proxy.Port,
			RecordPath:   yamlConfig.Proxy.Record,
		}
	} else {
		// Create default config
		config = &Config{
			OutputPath: "collection.json",
			AIConfig:   ai.DefaultConfig(),
		}
	}

	// Override with command-line flags if provided
	if *harFilePath != "" {
		config.HarFilePath = *harFilePath
	}
	if *varsFilePath != "" {
		config.VarsFilePath = *varsFilePath
	}
	if *outputPath != "" {
		config.OutputPath = *outputPath
	}
	if *verbose {
		config.AIConfig.Verbose = true
	}
	if *provider != "" {
		config.AIConfig.Provider = *provider
	}
	if *model != "" {
		config.AIConfig.Model = *model
	}
	if *refineComplexPaths {
		config.AIConfig.RefineComplexPaths = true
	}
	if *proxyPort > 0 {
		config.ProxyPort = *proxyPort
	}
	if *recordPath != "" {
		config.RecordPath = *recordPath
	}

	// Validate based on mode
	if config.ProxyPort > 0 {
		// Proxy mode - no additional validation needed
	} else {
		// HAR processing mode validation
		if config.HarFilePath == "" {
			usage := `Usage:
  # HAR Processing Mode:
  chainer -config=<path_to_config_file>
  chainer -file=<path_to_har_file> [-vars=<path_to_vars_file>] [-output=collection.json] [-verbose]
          [-provider=openai|anthropic] [-model=<model_name>]

  # Proxy Mode (acts as a general HTTP/HTTPS forward proxy):
  chainer -proxy=<port> [-record=<har_file>] [-output=collection.json]

  AI Providers:
    OpenAI: Set OPENAI_API_KEY environment variable
    Anthropic: Set ANTHROPIC_API_KEY environment variable
    Auto-detection: If provider not specified, will detect based on available API keys

Example config file:
  har_file: "path/to/your.har"
  output: "collection.json"
  ai:
    provider: "anthropic"  # or "openai"
    api_key: "sk-your-api-key"  # Or use environment variables
    model: "claude-3-haiku-20240307"  # or "gpt-4", etc.
  
  # Or for proxy mode:
  proxy:
    port: 8080
    record: "capture.har"`
			fmt.Println(usage)
			return nil, errors.New("missing HAR file path or proxy configuration")
		}
	}

	return config, nil
}

// loadYAMLConfig loads configuration from a YAML file
func loadYAMLConfig(filePath string) (*YAMLConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config YAMLConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Set defaults if not specified
	if config.Output == "" {
		config.Output = "collection.json"
	}

	return &config, nil
}
