package ai

import (
	"strings"
	"testing"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/util"
)

func TestExtractPartialJSON(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		targetPath  string
		linesToKeep int
		wantErr     bool
		shouldFind  string
	}{
		{
			name: "simple object path",
			source: `{
				"data": {
					"user": {
						"id": "12345",
						"name": "John"
					}
				}
			}`,
			targetPath:  "data.user.id",
			linesToKeep: 2,
			shouldFind:  ">>NODE-TO-GET<<",
			wantErr:     false,
		},
		{
			name: "array path",
			source: `{
				"items": [
					{"id": "1", "name": "First"},
					{"id": "2", "name": "Second"}
				]
			}`,
			targetPath:  "items[1].id",
			linesToKeep: 2,
			shouldFind:  ">>NODE-TO-GET<<",
			wantErr:     false,
		},
		{
			name: "nested array path",
			source: `{
				"data": {
					"flights": [
						{
							"segments": [
								{"departure": "2025-01-01"},
								{"departure": "2025-01-02"}
							]
						}
					]
				}
			}`,
			targetPath:  "data.flights[0].segments[0].departure",
			linesToKeep: 3,
			shouldFind:  ">>NODE-TO-GET<<",
			wantErr:     false,
		},
		{
			name:        "invalid JSON",
			source:      `{invalid json`,
			targetPath:  "data.id",
			linesToKeep: 2,
			wantErr:     true,
		},
		{
			name: "path not found",
			source: `{
				"data": {"id": "123"}
			}`,
			targetPath:  "data.missing.field",
			linesToKeep: 2,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractPartialJSON(tt.source, tt.targetPath, tt.linesToKeep)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractPartialJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.shouldFind != "" {
				if !contains(result, tt.shouldFind) {
					t.Errorf("extractPartialJSON() result does not contain %q, got %q", tt.shouldFind, result)
				}
			}
		})
	}
}

func TestParseArrayKey(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantKey   string
		wantIdx   int
		wantArray bool
		wantErr   bool
	}{
		{
			name:      "simple key",
			token:     "key",
			wantKey:   "key",
			wantIdx:   -1,
			wantArray: false,
			wantErr:   false,
		},
		{
			name:      "array with key",
			token:     "items[2]",
			wantKey:   "items",
			wantIdx:   2,
			wantArray: true,
			wantErr:   false,
		},
		{
			name:      "array without key",
			token:     "[0]",
			wantKey:   "",
			wantIdx:   0,
			wantArray: true,
			wantErr:   false,
		},
		{
			name:      "invalid array syntax",
			token:     "items[",
			wantKey:   "",
			wantIdx:   -1,
			wantArray: false,
			wantErr:   true,
		},
		{
			name:      "empty array index",
			token:     "items[]",
			wantKey:   "items",
			wantIdx:   -1,
			wantArray: false,
			wantErr:   true,
		},
		{
			name:      "non-numeric index",
			token:     "items[abc]",
			wantKey:   "items",
			wantIdx:   -1,
			wantArray: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, idx, isArray, err := parseArrayKey(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseArrayKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if key != tt.wantKey {
				t.Errorf("parseArrayKey() key = %v, want %v", key, tt.wantKey)
			}
			if idx != tt.wantIdx {
				t.Errorf("parseArrayKey() idx = %v, want %v", idx, tt.wantIdx)
			}
			if isArray != tt.wantArray {
				t.Errorf("parseArrayKey() isArray = %v, want %v", isArray, tt.wantArray)
			}
		})
	}
}

func TestComplexPathProcessor_UpdateComplexPaths(t *testing.T) {
	// Create a mock client
	mockClient := NewMockOpenAIClient()
	mockClient.SetDefaultResponse("responseJson.data.flights[0].segments[0].departure_time")

	processor := NewComplexPathProcessor(mockClient)

	// Create test data
	harEntry := &har.Entry{
		Request: har.Request{
			URL: "https://api.example.com/flights",
		},
		Response: har.Response{
			Content: har.Content{
				Text: `{
					"data": {
						"flights": [
							{
								"segments": [
									{"departure_time": "2025-01-01T10:00:00Z"}
								]
							}
						]
					}
				}`,
			},
		},
	}

	callDetails := &util.CallDetails{
		Entry: harEntry,
	}

	valueRef := &util.ValueReference{
		Value:          "2025-01-01T10:00:00Z",
		ReferencePath:  "data.flights[0].segments[0].departure_time",
		Source:         callDetails,
		SourceType:     util.SourceTypeResponse,
		SourceLocation: util.SourceLocationBodyJson,
	}

	chainedValue := &util.ChainedValueContext{
		Value:       "2025-01-01T10:00:00Z",
		ValueSource: valueRef,
		AllUsages:   []*util.ValueReference{valueRef},
	}

	// Process the chained values
	processor.UpdateComplexPaths([]*util.ChainedValueContext{chainedValue})

	// Check that the path was updated
	if chainedValue.ValueSource.ReferencePath != "responseJson.data.flights[0].segments[0].departure_time" {
		t.Errorf("UpdateComplexPaths() path = %v, want responseJson.data.flights[0].segments[0].departure_time",
			chainedValue.ValueSource.ReferencePath)
	}

	// Verify the mock was called
	if mockClient.GetCallCount() != 1 {
		t.Errorf("UpdateComplexPaths() expected 1 AI call, got %d", mockClient.GetCallCount())
	}
}

func TestComplexPathProcessor_SkipsNonResponseValues(t *testing.T) {
	mockClient := NewMockOpenAIClient()
	processor := NewComplexPathProcessor(mockClient)

	// Create test data with a request value (should be skipped)
	valueRef := &util.ValueReference{
		Value:          "some-value",
		ReferencePath:  "headers.Authorization",
		SourceType:     util.SourceTypeRequest, // This should cause it to be skipped
		SourceLocation: util.SourceLocationHeader,
	}

	chainedValue := &util.ChainedValueContext{
		Value:       "some-value",
		ValueSource: valueRef,
	}

	// Process the chained values
	processor.UpdateComplexPaths([]*util.ChainedValueContext{chainedValue})

	// Verify no AI calls were made
	if mockClient.GetCallCount() != 0 {
		t.Errorf("UpdateComplexPaths() expected 0 AI calls for request values, got %d", mockClient.GetCallCount())
	}
}

// Helper function
func contains(s, substr string) bool {
	// Check both the plain string and the JSON-escaped version
	jsonEscaped := "\\u003e\\u003eNODE-TO-GET\\u003c\\u003c"
	return strings.Contains(s, substr) || strings.Contains(s, jsonEscaped)
}
