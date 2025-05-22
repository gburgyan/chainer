package util

import (
	"testing"
)

func TestFindChainedValues(t *testing.T) {
	finder := NewChainFinder()

	// Create test data
	request1 := &CallDetails{
		RequestDetails: []*ValueReference{
			{
				Value:          "request-only-value",
				SourceType:     SourceTypeRequest,
				ReferencePath:  "requestPath",
				SourceLocation: SourceLocationUrl,
			},
		},
		ResponseDetails: []*ValueReference{
			{
				Value:          "chained-value",
				SourceType:     SourceTypeResponse,
				ReferencePath:  "responsePath",
				SourceLocation: SourceLocationBodyJson,
			},
		},
	}

	request2 := &CallDetails{
		RequestDetails: []*ValueReference{
			{
				Value:          "chained-value", // Same as response in request1
				SourceType:     SourceTypeRequest,
				ReferencePath:  "requestPath",
				SourceLocation: SourceLocationUrl,
			},
			{
				Value:          "unique-value",
				SourceType:     SourceTypeRequest,
				ReferencePath:  "anotherPath",
				SourceLocation: SourceLocationUrl,
			},
		},
		ResponseDetails: []*ValueReference{
			{
				Value:          "another-response",
				SourceType:     SourceTypeResponse,
				ReferencePath:  "anotherResponsePath",
				SourceLocation: SourceLocationBodyJson,
			},
		},
	}

	// Set up source references
	request1.RequestDetails[0].Source = request1
	request1.ResponseDetails[0].Source = request1
	request2.RequestDetails[0].Source = request2
	request2.RequestDetails[1].Source = request2
	request2.ResponseDetails[0].Source = request2

	callDetailsList := []*CallDetails{request1, request2}

	// Test finding chained values
	chainedValues := finder.FindChainedValues(callDetailsList)

	// We expect to find "chained-value" since it appears in request1's response and request2's request
	if len(chainedValues) != 1 {
		t.Errorf("Expected 1 chained value, got %d", len(chainedValues))
		return
	}

	if chainedValues[0].Value != "chained-value" {
		t.Errorf("Expected chained value to be 'chained-value', got '%s'", chainedValues[0].Value)
	}

	// Test that we have the correct usages
	if len(chainedValues[0].AllUsages) != 2 {
		t.Errorf("Expected 2 usages of chained value, got %d", len(chainedValues[0].AllUsages))
		return
	}

	// Test the RepopulateCallDetails function
	finder.RepopulateCallDetails(chainedValues)

	// Check that the Context field was set correctly
	if request1.ResponseDetails[0].Context != chainedValues[0] {
		t.Errorf("Context not set correctly for response in request1")
	}

	if request2.RequestDetails[0].Context != chainedValues[0] {
		t.Errorf("Context not set correctly for request in request2")
	}

	// Check that the ValueSource was set correctly
	if chainedValues[0].ValueSource != request1.ResponseDetails[0] {
		t.Errorf("ValueSource not set correctly for chained value")
	}

	// Check that the request/response chained values lists were populated
	if len(request1.ResponseChainedValues) != 1 || request1.ResponseChainedValues[0] != request1.ResponseDetails[0] {
		t.Errorf("ResponseChainedValues not populated correctly for request1")
	}

	if len(request2.RequestChainedValues) != 1 || request2.RequestChainedValues[0] != request2.RequestDetails[0] {
		t.Errorf("RequestChainedValues not populated correctly for request2")
	}
}

func TestExtractPredefinedVars(t *testing.T) {
	finder := NewChainFinder()

	// Create test data
	request := &CallDetails{
		RequestDetails: []*ValueReference{
			{
				Value:          "predefined-value",
				SourceType:     SourceTypeRequest,
				ReferencePath:  "requestPath",
				SourceLocation: SourceLocationUrl,
			},
			{
				Value:          "another-value",
				SourceType:     SourceTypeRequest,
				ReferencePath:  "anotherPath",
				SourceLocation: SourceLocationUrl,
			},
		},
	}

	request.RequestDetails[0].Source = request
	request.RequestDetails[1].Source = request

	callDetailsList := []*CallDetails{request}

	// Define predefined variables
	predefinedVars := []PredefinedVariable{
		{
			Name:              "testVar",
			SearchValue:       "predefined-value",
			InitializerPrompt: "// Initialize testVar",
		},
		{
			Name:              "unusedVar",
			SearchValue:       "unused-value",
			InitializerPrompt: "",
		},
	}

	// Test extracting predefined variables
	chainedValues := finder.ExtractPredefinedVars(callDetailsList, predefinedVars, []*ChainedValueContext{})

	// We expect to find "predefined-value" since it appears in the request
	if len(chainedValues) != 1 {
		t.Errorf("Expected 1 predefined variable, got %d", len(chainedValues))
		return
	}

	if chainedValues[0].Value != "predefined-value" {
		t.Errorf("Expected predefined value to be 'predefined-value', got '%s'", chainedValues[0].Value)
	}

	if chainedValues[0].VariableName != "testVar" {
		t.Errorf("Expected variable name to be 'testVar', got '%s'", chainedValues[0].VariableName)
	}

	if chainedValues[0].InitScript != "// Initialize testVar" {
		t.Errorf("Expected initializer to be '// Initialize testVar', got '%s'", chainedValues[0].InitScript)
	}

	if !chainedValues[0].ExternalSource {
		t.Errorf("Expected ExternalSource to be true")
	}

	// Check that the AllUsages field was populated correctly
	if len(chainedValues[0].AllUsages) != 1 || chainedValues[0].AllUsages[0] != request.RequestDetails[0] {
		t.Errorf("AllUsages not populated correctly for predefined variable")
	}
}
