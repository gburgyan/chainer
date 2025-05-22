package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// MockOpenAIClient is a mock implementation of the OpenAI client for testing.
type MockOpenAIClient struct {
	// Responses maps prompts to pre-defined responses
	Responses map[string]string
	// DefaultResponse is returned if no specific response is found
	DefaultResponse string
	// CallHistory records all calls made to the client
	CallHistory []MockCall
}

// Do implements the HTTPClientInterface interface
func (m *MockOpenAIClient) Do(req *http.Request) (*http.Response, error) {
	// This is just a stub implementation, as this mock is primarily used for the OpenAIClientInterface methods
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"mock response"}}]}`)),
	}, nil
}

// MockCall represents a recorded call to the mock client.
type MockCall struct {
	Prompt string
	Input  interface{}
}

// NewMockOpenAIClient creates a new mock OpenAI client.
func NewMockOpenAIClient() *MockOpenAIClient {
	return &MockOpenAIClient{
		Responses:   make(map[string]string),
		CallHistory: []MockCall{},
	}
}

// AddMockResponse adds a response for a specific prompt.
func (m *MockOpenAIClient) AddMockResponse(prompt string, response string) {
	m.Responses[prompt] = response
}

// SetDefaultResponse sets the default response for any prompt.
func (m *MockOpenAIClient) SetDefaultResponse(response string) {
	m.DefaultResponse = response
}

// CallBase records the call and returns a pre-defined response.
func (m *MockOpenAIClient) CallBase(prompt string, input interface{}) (string, error) {
	// Record the call
	m.CallHistory = append(m.CallHistory, MockCall{
		Prompt: prompt,
		Input:  input,
	})

	// Check if there's a specific response for this prompt
	if response, ok := m.Responses[prompt]; ok {
		return response, nil
	}

	// Return the default response
	if m.DefaultResponse != "" {
		return m.DefaultResponse, nil
	}

	// If no response is configured, return a simple mock response
	return "mock_response", nil
}

// CallString calls the base method and returns the response.
func (m *MockOpenAIClient) CallString(prompt string, input interface{}) (string, error) {
	return m.CallBase(prompt, input)
}

// CallArray calls the base method and unmarshals the response into a slice.
func (m *MockOpenAIClient) CallArray(prompt string, input interface{}, result interface{}) error {
	response, err := m.CallBase(prompt, input)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(response), result)
}

// CallObject calls the base method and unmarshals the response into an object.
func (m *MockOpenAIClient) CallObject(prompt string, input interface{}, result interface{}) error {
	response, err := m.CallBase(prompt, input)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(response), result)
}

// GetCallCount returns the number of calls made to the client.
func (m *MockOpenAIClient) GetCallCount() int {
	return len(m.CallHistory)
}

// GetLastCall returns the last call made to the client.
func (m *MockOpenAIClient) GetLastCall() (MockCall, error) {
	if len(m.CallHistory) == 0 {
		return MockCall{}, fmt.Errorf("no calls made to the client")
	}

	return m.CallHistory[len(m.CallHistory)-1], nil
}

// GetCalls returns all calls made to the client.
func (m *MockOpenAIClient) GetCalls() []MockCall {
	return m.CallHistory
}

// ClearCalls clears the call history.
func (m *MockOpenAIClient) ClearCalls() {
	m.CallHistory = []MockCall{}
}
