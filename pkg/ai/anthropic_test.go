package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// MockAnthropicHTTPClient is a mock implementation of HTTPClientInterface for testing.
type MockAnthropicHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
	Calls  []AnthropicMockCall
}

// AnthropicMockCall represents a recorded HTTP call for testing
type AnthropicMockCall struct {
	URL     string
	Method  string
	Headers http.Header
	Body    string
}

func (m *MockAnthropicHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Record the call
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))

	call := AnthropicMockCall{
		URL:     req.URL.String(),
		Method:  req.Method,
		Headers: req.Header,
		Body:    string(body),
	}
	m.Calls = append(m.Calls, call)

	return m.DoFunc(req)
}

func TestNewAnthropicClient(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		envAPIKey string
		wantErr   bool
	}{
		{
			name: "valid config with API key",
			config: &Config{
				APIKey:     "test-api-key",
				Model:      "claude-3-haiku-20240307",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			wantErr: false,
		},
		{
			name: "uses environment variable when config API key is empty",
			config: &Config{
				Model:      "claude-3-haiku-20240307",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			envAPIKey: "env-api-key",
			wantErr:   false,
		},
		{
			name:    "error when no API key provided",
			config:  &Config{},
			wantErr: true,
		},
		{
			name:      "uses default config when nil",
			config:    nil,
			envAPIKey: "env-api-key",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			if tt.envAPIKey != "" {
				t.Setenv("ANTHROPIC_API_KEY", tt.envAPIKey)
			} else {
				t.Setenv("ANTHROPIC_API_KEY", "")
			}

			client, err := NewAnthropicClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAnthropicClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && client == nil {
				t.Error("NewAnthropicClient() returned nil client without error")
			}

			// Check default values
			if !tt.wantErr && client != nil {
				if client.Config.Model == "" {
					t.Error("Model should have a default value")
				}
				if client.Config.MaxTokens == 0 {
					t.Error("MaxTokens should have a default value")
				}
			}
		})
	}
}

func TestAnthropicClient_CallBase(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		input      interface{}
		mockResp   *http.Response
		mockErr    error
		wantErr    bool
		wantResult string
	}{
		{
			name:   "successful API call",
			prompt: "Generate a variable name",
			input:  map[string]string{"key": "value"},
			mockResp: &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"id": "msg_123",
					"type": "message",
					"role": "assistant",
					"content": [{"type": "text", "text": "[{\"name\": \"testVariable\"}]"}],
					"model": "claude-3-haiku-20240307",
					"stop_reason": "end_turn",
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`)),
			},
			wantErr:    false,
			wantResult: `[{"name": "testVariable"}]`,
		},
		{
			name:   "API error response",
			prompt: "Generate a variable name",
			input:  map[string]string{"key": "value"},
			mockResp: &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error": {"message": "Invalid request"}}`)),
			},
			wantErr: true,
		},
		{
			name:   "empty content in response",
			prompt: "Generate a variable name",
			input:  map[string]string{"key": "value"},
			mockResp: &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"id": "msg_123",
					"type": "message",
					"role": "assistant",
					"content": [],
					"model": "claude-3-haiku-20240307"
				}`)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockAnthropicHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					if tt.mockErr != nil {
						return nil, tt.mockErr
					}
					return tt.mockResp, nil
				},
			}

			client := &AnthropicClient{
				Config: &Config{
					APIKey:     "test-key",
					Model:      "claude-3-haiku-20240307",
					MaxTokens:  4096,
					Timeout:    30 * time.Second,
					MaxRetries: 0, // No retries for testing
				},
				client: mockClient,
			}

			result, err := client.CallBase(tt.prompt, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CallBase() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result != tt.wantResult {
				t.Errorf("CallBase() result = %v, want %v", result, tt.wantResult)
			}

			// Verify request was made correctly
			if len(mockClient.Calls) > 0 {
				call := mockClient.Calls[0]
				if call.URL != "https://api.anthropic.com/v1/messages" {
					t.Errorf("Unexpected URL: %s", call.URL)
				}
				if call.Headers.Get("x-api-key") != "test-key" {
					t.Error("API key header not set correctly")
				}
				if call.Headers.Get("anthropic-version") != "2023-06-01" {
					t.Error("Anthropic version header not set correctly")
				}
			}
		})
	}
}

func TestAnthropicClient_CallArray(t *testing.T) {
	mockClient := &MockAnthropicHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"id": "msg_123",
					"type": "message",
					"role": "assistant",
					"content": [{"type": "text", "text": "[{\"name\": \"var1\"}, {\"name\": \"var2\"}]"}],
					"model": "claude-3-haiku-20240307",
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`)),
			}, nil
		},
	}

	client := &AnthropicClient{
		Config: &Config{
			APIKey:     "test-key",
			Model:      "claude-3-haiku-20240307",
			MaxTokens:  4096,
			Timeout:    30 * time.Second,
			MaxRetries: 0,
		},
		client: mockClient,
	}

	var result []map[string]string
	err := client.CallArray("test prompt", map[string]string{"test": "input"}, &result)
	if err != nil {
		t.Fatalf("CallArray() unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("CallArray() expected 2 results, got %d", len(result))
	}
	if result[0]["name"] != "var1" {
		t.Errorf("CallArray() first result = %v, want var1", result[0]["name"])
	}
	if result[1]["name"] != "var2" {
		t.Errorf("CallArray() second result = %v, want var2", result[1]["name"])
	}
}

func TestAnthropicClient_Retry(t *testing.T) {
	callCount := 0
	mockClient := &MockAnthropicHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount < 3 {
				// Fail the first two attempts
				return &http.Response{
					StatusCode: 500,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "Server error"}`)),
				}, nil
			}
			// Succeed on the third attempt
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"id": "msg_123",
					"type": "message",
					"role": "assistant",
					"content": [{"type": "text", "text": "Success after retry"}],
					"model": "claude-3-haiku-20240307",
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`)),
			}, nil
		},
	}

	client := &AnthropicClient{
		Config: &Config{
			APIKey:     "test-key",
			Model:      "claude-3-haiku-20240307",
			MaxTokens:  4096,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		client: mockClient,
	}

	result, err := client.CallBase("test prompt", map[string]string{"test": "input"})
	if err != nil {
		t.Fatalf("CallBase() unexpected error after retries: %v", err)
	}

	if result != "Success after retry" {
		t.Errorf("CallBase() result = %v, want 'Success after retry'", result)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 API calls (2 failures + 1 success), got %d", callCount)
	}
}

func TestAnthropicClient_RequestBody(t *testing.T) {
	var capturedBody AnthropicRequest

	mockClient := &MockAnthropicHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			json.Unmarshal(body, &capturedBody)

			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"id": "msg_123",
					"type": "message",
					"role": "assistant",
					"content": [{"type": "text", "text": "test response"}],
					"model": "claude-3-haiku-20240307",
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`)),
			}, nil
		},
	}

	client := &AnthropicClient{
		Config: &Config{
			APIKey:     "test-key",
			Model:      "claude-3-opus-20240229",
			MaxTokens:  2048,
			Timeout:    30 * time.Second,
			MaxRetries: 0,
		},
		client: mockClient,
	}

	inputData := map[string]string{"key": "value"}
	_, err := client.CallBase("Test prompt", inputData)
	if err != nil {
		t.Fatalf("CallBase() unexpected error: %v", err)
	}

	// Verify the request body
	if capturedBody.Model != "claude-3-opus-20240229" {
		t.Errorf("Expected model claude-3-opus-20240229, got %s", capturedBody.Model)
	}
	if capturedBody.MaxTokens != 2048 {
		t.Errorf("Expected max tokens 2048, got %d", capturedBody.MaxTokens)
	}
	if capturedBody.System == "" {
		t.Error("System prompt should not be empty")
	}
	if len(capturedBody.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(capturedBody.Messages))
	}
	if capturedBody.Messages[0].Role != "user" {
		t.Errorf("Expected role 'user', got %s", capturedBody.Messages[0].Role)
	}
}
