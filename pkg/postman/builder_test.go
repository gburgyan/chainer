package postman

import (
	"strings"
	"testing"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/util"
)

func TestReplaceValuesInString(t *testing.T) {
	builder := NewBuilder()

	// Create test data
	chained := &util.ChainedValueContext{
		Value:        "replace-me",
		VariableName: "testVariable",
	}

	valueRef := &util.ValueReference{
		Value:   "replace-me",
		Context: chained,
	}

	valueRefs := []*util.ValueReference{valueRef}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Simple replacement",
			input: "This is replace-me in a string",
			want:  "This is {{testVariable}} in a string",
		},
		{
			name:  "Multiple replacements",
			input: "replace-me is here and replace-me is there",
			want:  "{{testVariable}} is here and {{testVariable}} is there",
		},
		{
			name:  "No replacement needed",
			input: "Nothing to replace here",
			want:  "Nothing to replace here",
		},
		{
			name:  "Empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := builder.ReplaceValuesInString(tt.input, valueRefs)
			if got != tt.want {
				t.Errorf("ReplaceValuesInString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateTestScript(t *testing.T) {
	builder := NewBuilder()

	// Create test data
	chained := &util.ChainedValueContext{
		Value:        "test-value",
		VariableName: "testVariable",
	}

	valueRef := &util.ValueReference{
		Value:          "test-value",
		SourceType:     util.SourceTypeResponse,
		ReferencePath:  "responseJson.data.value",
		SourceLocation: util.SourceLocationBodyJson,
		Context:        chained,
	}

	chainedValues := []*util.ValueReference{valueRef}

	// Test creating a test script
	script := builder.CreateTestScript(chainedValues)

	// Check that the script is a test script
	if script.Listen != "test" {
		t.Errorf("Expected script listen to be 'test', got '%s'", script.Listen)
	}

	// Check that the script contains the variable name and path
	scriptText := strings.Join(script.Script.Exec, "\n")

	// Check for variable extraction
	if !strings.Contains(scriptText, "var testVariable = responseJson.data.value;") {
		t.Errorf("Script doesn't contain variable extraction: %s", scriptText)
	}

	// Check for variable setting
	if !strings.Contains(scriptText, "pm.collectionVariables.set(\"testVariable\", testVariable);") {
		t.Errorf("Script doesn't contain variable setting: %s", scriptText)
	}

	// Check for error handling
	if !strings.Contains(scriptText, "try {") || !strings.Contains(scriptText, "catch (e) {") {
		t.Errorf("Script doesn't contain error handling: %s", scriptText)
	}
}

func TestCreateInitScript(t *testing.T) {
	builder := NewBuilder()

	// Create test data with an init script
	chainedValue := &util.ChainedValueContext{
		Value:        "init-value",
		VariableName: "initVariable",
		InitScript:   "result = 'initialized';",
	}

	chainedValues := []*util.ChainedValueContext{chainedValue}

	// Test creating an init script
	script := builder.CreateInitScript(chainedValues)

	// Check that the script is a prerequest script
	if script == nil {
		t.Fatalf("Expected script to be non-nil")
	}

	if script.Listen != "prerequest" {
		t.Errorf("Expected script listen to be 'prerequest', got '%s'", script.Listen)
	}

	// Check that the script contains the init code
	scriptText := strings.Join(script.Script.Exec, "\n")

	// Check for result variable initialization
	if !strings.Contains(scriptText, "var result = {};") {
		t.Errorf("Script doesn't contain result initialization: %s", scriptText)
	}

	// Check for custom init code
	if !strings.Contains(scriptText, "result = 'initialized';") {
		t.Errorf("Script doesn't contain custom init code: %s", scriptText)
	}

	// Check for variable setting
	if !strings.Contains(scriptText, "pm.collectionVariables.set(\"initVariable\", result);") {
		t.Errorf("Script doesn't contain variable setting: %s", scriptText)
	}

	// Test with no init script
	emptyChainedValue := &util.ChainedValueContext{
		Value:        "empty-value",
		VariableName: "emptyVariable",
		InitScript:   "",
	}

	emptyChainedValues := []*util.ChainedValueContext{emptyChainedValue}

	emptyScript := builder.CreateInitScript(emptyChainedValues)

	// There should be no script since there's no init code
	if emptyScript != nil {
		t.Errorf("Expected nil script for empty init code, got %v", emptyScript)
	}
}

func TestBuildPostmanURL(t *testing.T) {
	builder := NewBuilder()

	// Create a chained value for replacement
	chained := &util.ChainedValueContext{
		Value:        "param123",
		VariableName: "paramVariable",
	}

	valueRef := &util.ValueReference{
		Value:   "param123",
		Context: chained,
	}

	// Create a HAR entry with a URL that contains the chained value
	entry := &har.Entry{
		Request: har.Request{
			Method: "GET",
			URL:    "https://example.com/api/users/param123?filter=param123",
		},
	}

	// Create a call details with the entry and chained value
	callDetails := &util.CallDetails{
		Entry:                entry,
		RequestChainedValues: []*util.ValueReference{valueRef},
	}

	// Test building a Postman URL
	url := builder.BuildPostmanURL(callDetails)

	// Check the raw URL
	if url.Raw != "https://example.com/api/users/{{paramVariable}}?filter={{paramVariable}}" {
		t.Errorf("Expected raw URL with replacements, got '%s'", url.Raw)
	}

	// Check the host
	if len(url.Host) != 1 || url.Host[0] != "example.com" {
		t.Errorf("Expected host to be ['example.com'], got %v", url.Host)
	}

	// Check the path
	expectedPath := []string{"api", "users", "{{paramVariable}}"}
	if len(url.Path) != len(expectedPath) {
		t.Errorf("Expected path length %d, got %d", len(expectedPath), len(url.Path))
	} else {
		for i, segment := range expectedPath {
			if url.Path[i] != segment {
				t.Errorf("Expected path segment %d to be '%s', got '%s'", i, segment, url.Path[i])
			}
		}
	}

	// Check the query parameters
	if len(url.Query) != 1 {
		t.Errorf("Expected 1 query parameter, got %d", len(url.Query))
	} else {
		if url.Query[0].Key != "filter" || url.Query[0].Value != "{{paramVariable}}" {
			t.Errorf("Expected query parameter {filter: {{paramVariable}}}, got {%s: %s}",
				url.Query[0].Key, url.Query[0].Value)
		}
	}
}
