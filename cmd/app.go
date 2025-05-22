package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gburgyan/chainer/pkg/ai"
	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/postman"
	"github.com/gburgyan/chainer/pkg/util"
)

// Config holds the application configuration.
type Config struct {
	HarFilePath  string
	VarsFilePath string
	OutputPath   string
	AIConfig     *ai.Config
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
func (a *App) Run() error {
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

// AppComponents holds the main application components
type AppComponents struct {
	HarProcessor    *har.Processor
	ChainFinder     *util.ChainFinder
	OpenAIClient    *ai.OpenAIClient
	TemplatedClient *ai.TemplatedClient
	VariableNamer   *ai.VariableNamer
	PostmanBuilder  *postman.Builder
}

// initializeComponents initializes all the application components.
func (a *App) initializeComponents() (*AppComponents, error) {
	// Create basic components
	harProcessor := har.NewProcessor()
	chainFinder := util.NewChainFinder()
	postmanBuilder := postman.NewBuilder()

	// Setup OpenAI client
	openAIClient, err := ai.NewOpenAIClient(a.Config.AIConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating OpenAI client: %w", err)
	}

	// Try to create a templated client, but fall back to regular client if there's an issue
	var templatedClient *ai.TemplatedClient
	templatedClient, err = ai.NewTemplatedClient(a.Config.AIConfig)
	if err != nil {
		log.Printf("Warning: Unable to initialize templated client: %v", err)
		log.Println("Falling back to standard OpenAI client")
		templatedClient = nil
	}

	// Create variable namer with the best available client
	var aiClient ai.OpenAIClientInterface = openAIClient
	if templatedClient != nil {
		aiClient = templatedClient
	}
	variableNamer := ai.NewVariableNamer(aiClient)

	return &AppComponents{
		HarProcessor:    harProcessor,
		ChainFinder:     chainFinder,
		OpenAIClient:    openAIClient,
		TemplatedClient: templatedClient,
		VariableNamer:   variableNamer,
		PostmanBuilder:  postmanBuilder,
	}, nil
}

// processHAR processes the HAR file and identifies chained values.
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
func (a *App) processChainedValues(components *AppComponents, callDetailsList []*util.CallDetails, chainedValues []*util.ChainedValueContext) error {
	// Log initial chained values for debugging
	components.ChainFinder.LogChainedValues(chainedValues)

	// Repopulate call details with chained values
	components.ChainFinder.RepopulateCallDetails(chainedValues)

	// Assign variable names using OpenAI
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

// ParseFlags parses command-line flags and returns a Config.
func ParseFlags() (*Config, error) {
	harFilePath := flag.String("file", "", "Path to the HAR file")
	varsFilePath := flag.String("vars", "", "Path to the JSON file with pre-defined variables")
	outputPath := flag.String("output", "collection.json", "Output path for the generated Postman collection")
	verbose := flag.Bool("verbose", false, "Enable verbose logging of AI API calls")

	flag.Parse()

	if *harFilePath == "" {
		usage := "Usage: chainer -file=<path_to_har_file> [-vars=<path_to_vars_file>] [-output=collection.json] [-verbose]"
		fmt.Println(usage)
		return nil, errors.New("missing HAR file path")
	}

	// Create default OpenAI config
	aiConfig := ai.DefaultConfig()
	aiConfig.Verbose = *verbose

	return &Config{
		HarFilePath:  *harFilePath,
		VarsFilePath: *varsFilePath,
		OutputPath:   *outputPath,
		AIConfig:     aiConfig,
	}, nil
}
