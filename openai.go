package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIRequest struct {
	Model     string          `json:"model"`
	Messages  []OpenAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type OpenAIChoice struct {
	Message OpenAIMessage `json:"message"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
}

func CallOpenAI[T any](prompt string, input []T) ([]string, error) {
	// Get the API key from the environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	// Convert input to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("error marshalling input: %v", err)
	}

	// Prepare the prompt in messages format
	messages := []OpenAIMessage{
		{
			Role:    "system",
			Content: "You are an assistant that takes the input request and performs a simple request.",
		},
		{
			Role:    "user",
			Content: prompt + "\n" + string(jsonData),
		},
	}

	// log the request
	fmt.Println("Request to OpenAI:")
	fmt.Println(messages)

	// Set up the OpenAI request
	reqBody := OpenAIRequest{
		Model:     "gpt-4o-mini",
		Messages:  messages,
		MaxTokens: 10000,
	}
	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request body: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBodyJson))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	// log the response
	fmt.Println("Response from OpenAI:")
	fmt.Println(string(respBody))

	// Parse OpenAI response
	var openAIResponse OpenAIResponse
	err = json.Unmarshal(respBody, &openAIResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}

	if len(openAIResponse.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from OpenAI")
	}

	// Extract variable names from the response
	choiceText := openAIResponse.Choices[0].Message.Content
	var result []string
	err = json.Unmarshal([]byte(choiceText), &result)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling result: %v", err)
	}

	if len(result) != len(input) {
		return nil, fmt.Errorf("number of results does not match number of inputs")
	}

	return result, nil
}
