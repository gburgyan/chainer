package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

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

// callOpenAIBase sends the request to the OpenAI API and returns the raw response string.
// It serves as the common base for the higher-level helper functions.
func callOpenAIBase(prompt string, input interface{}) (string, error) {
	// Get the API key from the environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	// Convert input to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("error marshalling input: %v", err)
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

	// Log the request for debugging
	fmt.Println("Request to OpenAI:")
	fmt.Println(messages)

	// Create the OpenAI request body
	reqBody := OpenAIRequest{
		//Model: "o3-mini",
		//Model: "o1-mini",
		Model: "gpt-4o-mini",
		//Model:     "gpt-4o",
		Messages: messages,
		//MaxTokens: 16384,
	}
	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("error marshalling request body: %v", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBodyJSON))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body:", err)
		}
	}(resp.Body)

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	// Log the raw response
	fmt.Println("Response from OpenAI:")
	fmt.Println(string(respBody))

	// Parse the response into our struct
	var openAIResponse OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResponse); err != nil {
		return "", fmt.Errorf("error unmarshalling response: %v", err)
	}

	if len(openAIResponse.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from OpenAI")
	}

	// Return the raw content from the first choice
	return openAIResponse.Choices[0].Message.Content, nil
}

// CallOpenAIString calls the API and returns the raw string response.
func CallOpenAIString(prompt string, input interface{}) (string, error) {
	return callOpenAIBase(prompt, input)
}

// CallOpenAIArray calls the API and unmarshals the JSON response into a slice of type T.
func CallOpenAIArray[T any](prompt string, input interface{}) ([]T, error) {
	content, err := callOpenAIBase(prompt, input)
	if err != nil {
		return nil, err
	}

	var result []T
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling result into []T: %v", err)
	}

	return result, nil
}

// CallOpenAIObject calls the API and unmarshals the JSON response into an object of type T.
func CallOpenAIObject[T any](prompt string, input interface{}) (T, error) {
	var result T
	content, err := callOpenAIBase(prompt, input)
	if err != nil {
		return result, err
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return result, fmt.Errorf("error unmarshalling result into T: %v", err)
	}

	return result, nil
}
