package ai

import (
	"testing"
	"testing/fstest"

	"github.com/gburgyan/chainer/pkg/template"
)

func TestTemplatedClient(t *testing.T) {
	// Create a mock template manager
	mockFS := fstest.MapFS{
		"templates/test.tmpl": &fstest.MapFile{
			Data: []byte("Test template with {{.Param}}"),
		},
		"templates/array.tmpl": &fstest.MapFile{
			Data: []byte("Array template"),
		},
		"templates/object.tmpl": &fstest.MapFile{
			Data: []byte("Object template"),
		},
	}

	tm := template.NewManager(mockFS)
	err := tm.Load()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Create a mock OpenAI client
	mockClient := NewMockOpenAIClient()
	mockClient.SetDefaultResponse(`{"test": "response"}`)

	// Create a templated client with the mock components
	client := &TemplatedClient{
		OpenAIClient: mockClient,
		Templates:    tm,
	}

	// Test CallWithTemplate
	result, err := client.CallWithTemplate("test", struct{ Param string }{"value"}, nil)
	if err != nil {
		t.Fatalf("CallWithTemplate failed: %v", err)
	}
	if result != `{"test": "response"}` {
		t.Errorf("Expected response to be '{\"test\": \"response\"}', got '%s'", result)
	}

	// Check that the mock client was called with the rendered template
	calls := mockClient.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Prompt != "Test template with value" {
		t.Errorf("Expected prompt to be 'Test template with value', got '%s'", calls[0].Prompt)
	}

	// Test CallArrayWithTemplate
	mockClient.SetDefaultResponse(`[{"name": "test1"}, {"name": "test2"}]`)
	var arrayResult []struct{ Name string }
	err = client.CallArrayWithTemplate("array", nil, nil, &arrayResult)
	if err != nil {
		t.Fatalf("CallArrayWithTemplate failed: %v", err)
	}
	if len(arrayResult) != 2 || arrayResult[0].Name != "test1" || arrayResult[1].Name != "test2" {
		t.Errorf("Unexpected array result: %+v", arrayResult)
	}

	// Test CallObjectWithTemplate
	mockClient.SetDefaultResponse(`{"name": "testObject", "value": 123}`)
	var objectResult struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	err = client.CallObjectWithTemplate("object", nil, nil, &objectResult)
	if err != nil {
		t.Fatalf("CallObjectWithTemplate failed: %v", err)
	}
	if objectResult.Name != "testObject" || objectResult.Value != 123 {
		t.Errorf("Unexpected object result: %+v", objectResult)
	}

	// Test error handling for non-existent template
	_, err = client.CallWithTemplate("nonexistent", nil, nil)
	if err == nil {
		t.Errorf("Expected error for non-existent template")
	}
}
