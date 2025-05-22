package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// MockHTTPClient is a mock implementation of http.Client for testing.
type MockHTTPClient struct {
	// DoFunc will be executed when the client's Do method is called.
	DoFunc func(req *http.Request) (*http.Response, error)
}

// Do implements the HTTPClientInterface interface.
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

// TestOpenAIClient_PrepareRequestBody tests the prepareRequestBody function.
func TestOpenAIClient_PrepareRequestBody(t *testing.T) {
	client := &OpenAIClient{
		Config: &Config{
			Model:     "test-model",
			MaxTokens: 100,
		},
	}

	prompt := "Test prompt"
	input := map[string]string{"key": "value"}

	reqBodyJSON, err := client.prepareRequestBody(prompt, input)
	if err != nil {
		t.Fatalf("prepareRequestBody failed: %v", err)
	}

	// Decode the request body to verify its contents
	var reqBody OpenAIRequest
	if err := json.Unmarshal(reqBodyJSON, &reqBody); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Check the model
	if reqBody.Model != "test-model" {
		t.Errorf("Expected model to be 'test-model', got '%s'", reqBody.Model)
	}

	// Check the max tokens
	if reqBody.MaxTokens != 100 {
		t.Errorf("Expected max_tokens to be 100, got %d", reqBody.MaxTokens)
	}

	// Check the messages
	if len(reqBody.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(reqBody.Messages))
	}

	// Check the first message
	if reqBody.Messages[0].Role != "user" || reqBody.Messages[0].Content != "You are an assistant that takes the input request and performs a simple request." {
		t.Errorf("First message does not match expected content")
	}

	// Check the second message
	if reqBody.Messages[1].Role != "user" || reqBody.Messages[1].Content != "Test prompt" {
		t.Errorf("Second message does not match expected content")
	}

	// Check the third message
	expectedInput := `{"key":"value"}`
	if reqBody.Messages[2].Role != "user" || reqBody.Messages[2].Content != expectedInput {
		t.Errorf("Third message does not match expected content. Expected: %s, Got: %s", expectedInput, reqBody.Messages[2].Content)
	}
}

// TestOpenAIClient_CreateRequest tests the createRequest function.
func TestOpenAIClient_CreateRequest(t *testing.T) {
	client := &OpenAIClient{
		Config: &Config{
			APIKey: "test-api-key",
		},
	}

	reqBodyJSON := []byte(`{"test":"value"}`)
	req, err := client.createRequest(reqBodyJSON)
	if err != nil {
		t.Fatalf("createRequest failed: %v", err)
	}

	// Check the URL
	if req.URL.String() != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("Expected URL to be 'https://api.openai.com/v1/chat/completions', got '%s'", req.URL.String())
	}

	// Check the method
	if req.Method != "POST" {
		t.Errorf("Expected method to be 'POST', got '%s'", req.Method)
	}

	// Check the headers
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be 'application/json', got '%s'", req.Header.Get("Content-Type"))
	}

	if req.Header.Get("Authorization") != "Bearer test-api-key" {
		t.Errorf("Expected Authorization to be 'Bearer test-api-key', got '%s'", req.Header.Get("Authorization"))
	}

	// Check the body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	if string(body) != `{"test":"value"}` {
		t.Errorf("Expected body to be '{\"test\":\"value\"}', got '%s'", string(body))
	}
}

// TestOpenAIClient_CalculateBackoff tests the calculateBackoff function.
func TestOpenAIClient_CalculateBackoff(t *testing.T) {
	client := &OpenAIClient{}

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1100 * time.Millisecond}, // 1s + 100ms jitter
		{2, 2100 * time.Millisecond}, // 2s + 100ms jitter
		{3, 4100 * time.Millisecond}, // 4s + 100ms jitter
	}

	for _, tc := range testCases {
		backoff := client.calculateBackoff(tc.attempt)
		// Check if the backoff is within 10ms of the expected value to account for minor differences
		if backoff < tc.expected-10*time.Millisecond || backoff > tc.expected+10*time.Millisecond {
			t.Errorf("For attempt %d, expected backoff around %v, got %v", tc.attempt, tc.expected, backoff)
		}
	}
}

// TestOpenAIClient_FormatRetryError tests the formatRetryError function.
func TestOpenAIClient_FormatRetryError(t *testing.T) {
	client := &OpenAIClient{}

	// Test with a normal error
	originalErr := io.EOF
	err := client.formatRetryError(originalErr, nil)
	if err != originalErr {
		t.Errorf("Expected error to be %v, got %v", originalErr, err)
	}

	// Test with a response error
	resp := &http.Response{
		StatusCode: 429,
		Status:     "429 Too Many Requests",
	}
	err = client.formatRetryError(nil, resp)
	if err == nil || err.Error() != "HTTP error: 429 429 Too Many Requests" {
		t.Errorf("Expected error to be 'HTTP error: 429 429 Too Many Requests', got '%v'", err)
	}

	// Test with neither error nor response
	err = client.formatRetryError(nil, nil)
	if err == nil || err.Error() != "unknown error during request" {
		t.Errorf("Expected error to be 'unknown error during request', got '%v'", err)
	}
}

// TestOpenAIClient_ProcessResponse tests the processResponse function.
func TestOpenAIClient_ProcessResponse(t *testing.T) {
	client := &OpenAIClient{
		Config: DefaultConfig(),
	}

	// Create a mock response with valid data
	validResponseBody := `{"choices":[{"message":{"role":"assistant","content":"Test response"}}]}`
	validResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(validResponseBody)),
	}

	content, err := client.processResponse(validResp)
	if err != nil {
		t.Fatalf("processResponse failed: %v", err)
	}

	if content != "Test response" {
		t.Errorf("Expected content to be 'Test response', got '%s'", content)
	}

	// Create a mock response with invalid JSON
	invalidResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString("invalid json")),
	}

	_, err = client.processResponse(invalidResp)
	if err == nil {
		t.Errorf("Expected error for invalid JSON, got nil")
	}

	// Create a mock response with no choices
	noChoicesResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[]}`)),
	}

	_, err = client.processResponse(noChoicesResp)
	if err == nil || err.Error() != "no choices returned from OpenAI" {
		t.Errorf("Expected error to be 'no choices returned from OpenAI', got '%v'", err)
	}
}

// TestOpenAIClient_SendRequestWithRetries tests the sendRequestWithRetries function.
func TestOpenAIClient_SendRequestWithRetries(t *testing.T) {
	// Create a client with a mock HTTP client that succeeds on the second attempt
	attemptCount := 0
	mockHTTPClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			attemptCount++
			if attemptCount == 1 {
				// First attempt fails with status 429
				return &http.Response{
					StatusCode: 429,
					Status:     "429 Too Many Requests",
					Body:       io.NopCloser(bytes.NewBufferString("")),
				}, nil
			}
			// Second attempt succeeds
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"success"}}]}`)),
			}, nil
		},
	}

	client := &OpenAIClient{
		Config: &Config{
			MaxRetries: 3,
			Timeout:    1 * time.Second,
		},
		client: mockHTTPClient,
	}

	reqBodyJSON := []byte(`{"test":"value"}`)
	resp, err := client.sendRequestWithRetries(reqBodyJSON)
	if err != nil {
		t.Fatalf("sendRequestWithRetries failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code to be 200, got %d", resp.StatusCode)
	}

	if attemptCount != 2 {
		t.Errorf("Expected 2 attempts, got %d", attemptCount)
	}

	// Test with a client that always fails
	mockHTTPClient = &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Status:     "500 Internal Server Error",
				Body:       io.NopCloser(bytes.NewBufferString("")),
			}, nil
		},
	}

	client = &OpenAIClient{
		Config: &Config{
			MaxRetries: 2,
			Timeout:    1 * time.Second,
		},
		client: mockHTTPClient,
	}

	_, err = client.sendRequestWithRetries(reqBodyJSON)
	if err == nil {
		t.Errorf("Expected error for all failed attempts, got nil")
	}
}

// TestOpenAIClient_CallBase tests the CallBase function.
func TestOpenAIClient_CallBase(t *testing.T) {
	// Create a client with a mock HTTP client
	mockHTTPClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"success"}}]}`)),
			}, nil
		},
	}

	client := &OpenAIClient{
		Config: &Config{
			Model:      "test-model",
			MaxTokens:  100,
			APIKey:     "test-api-key",
			MaxRetries: 1,
			Timeout:    1 * time.Second,
		},
		client: mockHTTPClient,
	}

	content, err := client.CallBase("Test prompt", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("CallBase failed: %v", err)
	}

	if content != "success" {
		t.Errorf("Expected content to be 'success', got '%s'", content)
	}
}
