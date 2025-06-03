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

// AnthropicMessage represents a single message for the Anthropic API.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicRequest is the request body sent to the Anthropic API.
type AnthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
}

// AnthropicResponse represents the response from the Anthropic API.
type AnthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// AnthropicClient encapsulates the functionality for making calls to the Anthropic API.
type AnthropicClient struct {
	Config *Config
	client HTTPClientInterface
}

// NewAnthropicClient creates a new client for Anthropic API calls.
func NewAnthropicClient(config *Config) (*AnthropicClient, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Check for API key in config or environment
	if config.APIKey == "" {
		config.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		if config.APIKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
		}
	}

	// Set default model if not specified
	if config.Model == "" {
		config.Model = "claude-3-haiku-20240307" // Fast and cost-effective
	}

	// Set default max tokens if not specified
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}

	return &AnthropicClient{
		Config: config,
		client: client,
	}, nil
}

// CallBase sends the request to the Anthropic API and returns the raw response string.
func (c *AnthropicClient) CallBase(prompt string, input interface{}) (string, error) {
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

// prepareRequestBody creates the JSON request body for the Anthropic API.
func (c *AnthropicClient) prepareRequestBody(prompt string, input interface{}) ([]byte, error) {
	// Convert input to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("error marshalling input: %w", err)
	}

	// Prepare the messages - Anthropic uses a different format than OpenAI
	messages := []AnthropicMessage{
		{
			Role:    "user",
			Content: fmt.Sprintf("%s\n\nInput:\n%s", prompt, string(jsonData)),
		},
	}

	// Create the Anthropic request body
	reqBody := AnthropicRequest{
		Model:     c.Config.Model,
		Messages:  messages,
		MaxTokens: c.Config.MaxTokens,
		System:    "You are an assistant that takes the input request and performs a simple request. Return only raw JSON without any explanations or decorations.",
	}

	// Marshal the request body to JSON
	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request body: %w", err)
	}

	// Log the request if verbose mode is enabled
	if c.Config.Verbose {
		fmt.Println("\n--- Anthropic Request ---")
		fmt.Printf("Model: %s\n", c.Config.Model)
		fmt.Printf("Max Tokens: %d\n", c.Config.MaxTokens)
		fmt.Println("Prompt: ", prompt)
		fmt.Println("Input: ", string(jsonData))
		fmt.Println("----------------------")
	}

	return reqBodyJSON, nil
}

// createRequest creates the HTTP request for the Anthropic API.
func (c *AnthropicClient) createRequest(reqBodyJSON []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(reqBodyJSON))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers - Anthropic requires different headers than OpenAI
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.Config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	return req, nil
}

// sendRequestWithRetries sends the HTTP request with retries.
func (c *AnthropicClient) sendRequestWithRetries(reqBodyJSON []byte) (*http.Response, error) {
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
			fmt.Printf("\nSending request to Anthropic (attempt %d/%d)...\n", attempt+1, c.Config.MaxRetries+1)
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
			// Read error body for better debugging
			if resp.StatusCode >= 400 {
				errBody, _ := io.ReadAll(resp.Body)
				if c.Config.Verbose {
					fmt.Printf("Error response: %s\n", string(errBody))
				}
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
func (c *AnthropicClient) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	jitter := time.Duration(100 * time.Millisecond)
	return backoff + jitter
}

// formatRetryError formats the error for a retry attempt.
func (c *AnthropicClient) formatRetryError(err error, resp *http.Response) error {
	if err != nil {
		return err
	}
	if resp != nil {
		return fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}
	return fmt.Errorf("unknown error during request")
}

// processResponse processes the HTTP response from the Anthropic API.
func (c *AnthropicClient) processResponse(resp *http.Response) (string, error) {
	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	// Log the raw response if verbose mode is enabled
	if c.Config.Verbose {
		fmt.Println("\n--- Anthropic Response ---")
		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Raw Response: %s\n", string(respBody))
	}

	// Parse the response into our struct
	var anthropicResponse AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResponse); err != nil {
		if c.Config.Verbose {
			fmt.Printf("Error unmarshalling response: %v\n", err)
			fmt.Println("-------------------------")
		}
		return "", fmt.Errorf("error unmarshalling response: %w", err)
	}

	// Check if there is content in the response
	if len(anthropicResponse.Content) == 0 {
		if c.Config.Verbose {
			fmt.Println("Error: No content returned from Anthropic")
			fmt.Println("-------------------------")
		}
		return "", fmt.Errorf("no content returned from Anthropic")
	}

	// Get the text content from the first content item
	content := ""
	for _, c := range anthropicResponse.Content {
		if c.Type == "text" {
			content = c.Text
			break
		}
	}

	if content == "" {
		return "", fmt.Errorf("no text content found in Anthropic response")
	}

	// Log the content if verbose mode is enabled
	if c.Config.Verbose {
		fmt.Println("Content: ", content)
		fmt.Printf("Input tokens: %d, Output tokens: %d\n",
			anthropicResponse.Usage.InputTokens,
			anthropicResponse.Usage.OutputTokens)
		fmt.Println("-------------------------")
	}

	// Return the raw content
	return content, nil
}

// CallString calls the API and returns the raw string response.
func (c *AnthropicClient) CallString(prompt string, input interface{}) (string, error) {
	return c.CallBase(prompt, input)
}

// CallArray calls the API and unmarshals the JSON response into a slice of type T.
func (c *AnthropicClient) CallArray(prompt string, input interface{}, result interface{}) error {
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
func (c *AnthropicClient) CallObject(prompt string, input interface{}, result interface{}) error {
	content, err := c.CallBase(prompt, input)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(content), result); err != nil {
		return fmt.Errorf("error unmarshalling result: %w", err)
	}

	return nil
}
