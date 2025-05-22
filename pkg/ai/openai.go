package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Config holds configuration settings for OpenAI API calls.
type Config struct {
	Model      string
	MaxTokens  int
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
	Verbose    bool // Enable verbose logging of API calls
}

// DefaultConfig returns a default configuration for OpenAI API calls.
func DefaultConfig() *Config {
	return &Config{
		Model:      "gpt-4.1-nano",
		MaxTokens:  0, // Let the API decide
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		Verbose:    false,
	}
}

// HTTPClientInterface defines the interface for HTTP clients.
type HTTPClientInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

// OpenAIClient encapsulates the functionality for making calls to the OpenAI API.
type OpenAIClient struct {
	Config *Config
	client HTTPClientInterface
}

// NewOpenAIClient creates a new client for OpenAI API calls.
func NewOpenAIClient(config *Config) (*OpenAIClient, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		if config.APIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
		}
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}

	return &OpenAIClient{
		Config: config,
		client: client,
	}, nil
}

// OpenAIMessage represents a single message for the OpenAI API.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIRequest is the request body sent to the OpenAI API.
type OpenAIRequest struct {
	Model     string          `json:"model"`
	Messages  []OpenAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

// OpenAIChoice represents one of the choices in the OpenAI API response.
type OpenAIChoice struct {
	Message OpenAIMessage `json:"message"`
}

// OpenAIResponse represents the response from the OpenAI API.
type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
}

// CallBase sends the request to the OpenAI API and returns the raw response string.
// It serves as the common base for the higher-level helper functions.
func (c *OpenAIClient) CallBase(prompt string, input interface{}) (string, error) {
	// Prepare the request body
	reqBodyJSON, err := c.prepareRequestBody(prompt, input)
	if err != nil {
		return "", err
	}

	// Create and send the HTTP request with retries
	resp, err := c.sendRequestWithRetries(reqBodyJSON)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Process the response
	return c.processResponse(resp)
}

// prepareRequestBody creates the JSON request body for the OpenAI API.
func (c *OpenAIClient) prepareRequestBody(prompt string, input interface{}) ([]byte, error) {
	// Convert input to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("error marshalling input: %w", err)
	}

	// Prepare the messages
	messages := []OpenAIMessage{
		{
			Role:    "user",
			Content: "You are an assistant that takes the input request and performs a simple request.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
		{
			Role:    "user",
			Content: string(jsonData),
		},
	}

	// Create the OpenAI request body
	reqBody := OpenAIRequest{
		Model:     c.Config.Model,
		Messages:  messages,
		MaxTokens: c.Config.MaxTokens,
	}

	// Marshal the request body to JSON
	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request body: %w", err)
	}

	// Log the request if verbose mode is enabled
	if c.Config.Verbose {
		fmt.Println("\n--- OpenAI Request ---")
		fmt.Printf("Model: %s\n", c.Config.Model)
		fmt.Printf("Max Tokens: %d\n", c.Config.MaxTokens)
		fmt.Println("Prompt: ", prompt)
		fmt.Println("Input: ", string(jsonData))
		fmt.Println("----------------------")
	}

	return reqBodyJSON, nil
}

// createRequest creates the HTTP request for the OpenAI API.
func (c *OpenAIClient) createRequest(reqBodyJSON []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBodyJSON))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Config.APIKey)

	return req, nil
}

// sendRequestWithRetries sends the HTTP request with retries.
func (c *OpenAIClient) sendRequestWithRetries(reqBodyJSON []byte) (*http.Response, error) {
	// Create the HTTP request
	req, err := c.createRequest(reqBodyJSON)
	if err != nil {
		return nil, err
	}

	// Attempt the request with retries
	var resp *http.Response
	var retryErr error

	for attempt := 0; attempt <= c.Config.MaxRetries; attempt++ {
		if c.Config.Verbose {
			fmt.Printf("\nSending request (attempt %d/%d)...\n", attempt+1, c.Config.MaxRetries+1)
		}

		if attempt > 0 {
			// Calculate backoff duration
			backoffDuration := c.calculateBackoff(attempt)
			if c.Config.Verbose {
				fmt.Printf("Retrying after %v backoff...\n", backoffDuration)
			}
			time.Sleep(backoffDuration)
		}

		// Send the request
		resp, err = c.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			if c.Config.Verbose && attempt > 0 {
				fmt.Printf("Request succeeded after %d attempts\n", attempt+1)
			}
			return resp, nil
		}

		if resp != nil {
			if c.Config.Verbose {
				fmt.Printf("Request failed with status: %s\n", resp.Status)
			}
			resp.Body.Close()
		} else if err != nil && c.Config.Verbose {
			fmt.Printf("Request failed with error: %v\n", err)
		}

		retryErr = c.formatRetryError(err, resp)
	}

	if c.Config.Verbose {
		fmt.Printf("All %d retry attempts failed\n", c.Config.MaxRetries+1)
	}
	return nil, fmt.Errorf("all retry attempts failed: %w", retryErr)
}

// calculateBackoff calculates the backoff duration for a retry attempt.
func (c *OpenAIClient) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	jitter := time.Duration(100 * time.Millisecond)
	return backoff + jitter
}

// formatRetryError formats the error for a retry attempt.
func (c *OpenAIClient) formatRetryError(err error, resp *http.Response) error {
	if err != nil {
		return err
	}
	if resp != nil {
		return fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}
	return fmt.Errorf("unknown error during request")
}

// processResponse processes the HTTP response from the OpenAI API.
func (c *OpenAIClient) processResponse(resp *http.Response) (string, error) {
	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	// Log the raw response if verbose mode is enabled
	if c.Config.Verbose {
		fmt.Println("\n--- OpenAI Response ---")
		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Raw Response: %s\n", string(respBody))
	}

	// Parse the response into our struct
	var openAIResponse OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResponse); err != nil {
		if c.Config.Verbose {
			fmt.Printf("Error unmarshalling response: %v\n", err)
			fmt.Println("-------------------------")
		}
		return "", fmt.Errorf("error unmarshalling response: %w", err)
	}

	// Check if there are any choices in the response
	if len(openAIResponse.Choices) == 0 {
		if c.Config.Verbose {
			fmt.Println("Error: No choices returned from OpenAI")
			fmt.Println("-------------------------")
		}
		return "", fmt.Errorf("no choices returned from OpenAI")
	}

	// Get the content from the first choice
	content := openAIResponse.Choices[0].Message.Content

	// Log the content if verbose mode is enabled
	if c.Config.Verbose {
		fmt.Println("Content: ", content)
		fmt.Println("-------------------------")
	}

	// Return the raw content from the first choice
	return content, nil
}

// CallString calls the API and returns the raw string response.
func (c *OpenAIClient) CallString(prompt string, input interface{}) (string, error) {
	return c.CallBase(prompt, input)
}

// CallArray calls the API and unmarshals the JSON response into a slice of type T.
func (c *OpenAIClient) CallArray(prompt string, input interface{}, result interface{}) error {
	content, err := c.CallBase(prompt, input)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(content), result); err != nil {
		return fmt.Errorf("error unmarshalling result: %w", err)
	}

	return nil
}

// CallObject calls the API and unmarshals the JSON response into an object of type T.
func (c *OpenAIClient) CallObject(prompt string, input interface{}, result interface{}) error {
	content, err := c.CallBase(prompt, input)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(content), result); err != nil {
		return fmt.Errorf("error unmarshalling result: %w", err)
	}

	return nil
}
