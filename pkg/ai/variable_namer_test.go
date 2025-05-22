package ai

import (
	"testing"

	"github.com/gburgyan/chainer/pkg/util"
)

func TestAssignVariableNames(t *testing.T) {
	// Create a mock OpenAI client
	mockClient := NewMockOpenAIClient()

	// Set up a mock response for variable names
	mockResponse := `[
		{"name": "mockVariable1"},
		{"name": "mockVariable2"}
	]`
	mockClient.SetDefaultResponse(mockResponse)

	// Create a variable namer with the mock client
	namer := NewVariableNamer(mockClient)

	// Create test data: two chained values
	chainedValue1 := &util.ChainedValueContext{
		Value: "test-value-1",
		AllUsages: []*util.ValueReference{
			{
				Value: "test-value-1",
				Source: &util.CallDetails{
					Entry: map[string]interface{}{
						"Request": map[string]interface{}{
							"URL": "https://api.example.com/test1",
						},
					},
				},
				ReferencePath: "responseJson.data.value1",
			},
		},
	}

	chainedValue2 := &util.ChainedValueContext{
		Value: "test-value-2",
		AllUsages: []*util.ValueReference{
			{
				Value: "test-value-2",
				Source: &util.CallDetails{
					Entry: map[string]interface{}{
						"Request": map[string]interface{}{
							"URL": "https://api.example.com/test2",
						},
					},
				},
				ReferencePath: "responseJson.data.value2",
			},
		},
	}

	chainedValues := []*util.ChainedValueContext{chainedValue1, chainedValue2}

	// Test assigning variable names
	err := namer.AssignVariableNames(chainedValues)

	// Check that there was no error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the OpenAI client was called
	if mockClient.GetCallCount() != 1 {
		t.Errorf("Expected 1 call to OpenAI client, got %d", mockClient.GetCallCount())
	}

	// Check that the variable names were assigned correctly
	if chainedValue1.VariableName != "mockVariable1" {
		t.Errorf("Expected variable name to be 'mockVariable1', got '%s'", chainedValue1.VariableName)
	}

	if chainedValue2.VariableName != "mockVariable2" {
		t.Errorf("Expected variable name to be 'mockVariable2', got '%s'", chainedValue2.VariableName)
	}

	// Test error handling with insufficient results
	mockClient.SetDefaultResponse(`[{"name": "singleVariable"}]`)

	err = namer.AssignVariableNames(chainedValues)

	// Check that there was an error
	if err == nil {
		t.Errorf("Expected error due to insufficient results, got nil")
	}
}
