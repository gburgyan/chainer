package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

type VariableNameGenerator struct {
	OriginRequestUrl string `json:"origin_request_url"`
	ResponsePath     string `json:"response_path"`
}

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

func AssignVariableNames(chainedValues []*ChainedValueContext) {

	// Get the API key from the environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable is not set")
	}

	var variableNames []VariableNameGenerator
	for _, value := range chainedValues {
		variableNames = append(variableNames, VariableNameGenerator{
			OriginRequestUrl: value.ValueSource.Source.Entry.Request.URL,
			ResponsePath:     value.ValueSource.JavascriptReference,
		})
	}

	// Convert variable names to JSON
	jsonData, err := json.Marshal(variableNames)
	if err != nil {
		log.Fatalf("Error marshalling variable names: %v", err)
	}

	// Prepare the prompt in messages format
	messages := []OpenAIMessage{
		{
			Role:    "system",
			Content: "You are an assistant that generates variable names based on API request URLs and JSON response paths.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				`I want you to come up with good variable names for values retrieved from an API.
Please ensure each variable name is descriptive and follows best practices and is unique, but don't be overly verbose.
Don't include things like "identifier" or "value" in the name unless it's critical to the nameing, just the most descriptive part as a human would name it.

I'm giving you the URL that was called and the response JSON path that the value is retrieved from.

The API is: Travelport JSON API.

The response should be a JSON array of strings with each result corresponding to the name to
give the variable to hold the value. Please only return the JSON and nothing else with no additional
formatting or content.

Here's what I need names for: %s`, string(jsonData)),
		},
	}

	// Set up the OpenAI request
	reqBody := OpenAIRequest{
		Model:     "gpt-4o-mini",
		Messages:  messages,
		MaxTokens: 1000,
	}
	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatalf("Error marshalling request body: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBodyJson))
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error making request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}

	// Log the response
	log.Printf("Response: %s", respBody)

	// Parse OpenAI response
	var openAIResponse OpenAIResponse
	err = json.Unmarshal(respBody, &openAIResponse)
	if err != nil {
		log.Fatalf("Error unmarshalling response: %v", err)
	}

	if len(openAIResponse.Choices) == 0 {
		log.Fatalf("No choices returned from OpenAI")
	}

	// Extract variable names from the response
	choiceText := openAIResponse.Choices[0].Message.Content
	var variableNamesResponse []string
	err = json.Unmarshal([]byte(choiceText), &variableNamesResponse)
	if err != nil {
		log.Fatalf("Error unmarshalling variable names response: %v", err)
	}

	if len(variableNamesResponse) != len(chainedValues) {
		log.Fatalf("Mismatch between number of variable names and chained values")
	}

	// Assign variable names
	for i, value := range chainedValues {
		value.VariableName = variableNamesResponse[i]
	}
}
