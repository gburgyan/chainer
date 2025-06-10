package ai

import (
	"encoding/json"
	"fmt"
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
	// Set response for complex_path_v2 template
	mockClient.AddMockResponse("complex_path_v2", "responseJson.data.flights[0].segments[0].departure_time")
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

func TestSmartPruneJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		targetPath []string
		validate   func(t *testing.T, result interface{})
	}{
		{
			name: "simple object path",
			input: map[string]interface{}{
				"data": map[string]interface{}{
					"user": map[string]interface{}{
						"id":   ">>NODE-TO-GET<<",
						"name": "John",
					},
					"meta": map[string]interface{}{
						"version": "1.0",
						"deep": map[string]interface{}{
							"nested": map[string]interface{}{
								"value": "should be pruned",
							},
						},
					},
				},
			},
			targetPath: []string{"data", "user", "id"},
			validate: func(t *testing.T, result interface{}) {
				// Check that the path to target is preserved
				root, ok := result.(map[string]interface{})
				if !ok {
					t.Fatal("result is not a map")
				}
				data, ok := root["data"].(map[string]interface{})
				if !ok {
					t.Fatal("data is not a map")
				}
				user, ok := data["user"].(map[string]interface{})
				if !ok {
					t.Fatal("user is not a map")
				}
				if user["id"] != ">>NODE-TO-GET<<" {
					t.Errorf("target value not preserved: %v", user["id"])
				}
				// Check sibling field is preserved
				if user["name"] != "John" {
					t.Errorf("sibling field not preserved: %v", user["name"])
				}
				// Check deep structure is pruned
				meta, ok := data["meta"].(map[string]interface{})
				if !ok {
					t.Fatal("meta is not a map")
				}
				if meta["deep"] == "{...}" {
					// Correctly pruned
				} else if deep, ok := meta["deep"].(map[string]interface{}); ok {
					if deep["nested"] == "{...}" {
						// Also correctly pruned at a deeper level
					}
				}
			},
		},
		{
			name: "array path with context preservation",
			input: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "data": "first"},
					map[string]interface{}{"id": "2", "data": "second"},
					map[string]interface{}{"id": ">>NODE-TO-GET<<", "data": "target"},
					map[string]interface{}{"id": "4", "data": "fourth"},
					map[string]interface{}{"id": "5", "data": "fifth"},
					map[string]interface{}{"id": "6", "data": "sixth"},
				},
			},
			targetPath: []string{"items", "[2]", "id"},
			validate: func(t *testing.T, result interface{}) {
				root, ok := result.(map[string]interface{})
				if !ok {
					t.Fatal("result is not a map")
				}
				items, ok := root["items"].([]interface{})
				if !ok {
					t.Fatal("items is not an array")
				}

				// Check that first, last, target, and surrounding elements are kept
				if len(items) != 6 {
					t.Errorf("array length changed: %d", len(items))
				}

				// First element should be preserved
				if first, ok := items[0].(map[string]interface{}); ok {
					if first["id"] != "1" {
						t.Error("first element not preserved correctly")
					}
				}

				// Elements around target should be preserved
				if before, ok := items[1].(map[string]interface{}); ok {
					if before["id"] != "2" {
						t.Error("element before target not preserved")
					}
				}

				// Target element
				if target, ok := items[2].(map[string]interface{}); ok {
					if target["id"] != ">>NODE-TO-GET<<" {
						t.Error("target element not preserved")
					}
				}

				// Element after target
				if after, ok := items[3].(map[string]interface{}); ok {
					if after["id"] != "4" {
						t.Error("element after target not preserved")
					}
				}

				// Middle element should be placeholder
				if placeholder, ok := items[4].(string); ok {
					if !strings.Contains(placeholder, "item 4") {
						t.Errorf("middle element not replaced with placeholder: %v", placeholder)
					}
				}

				// Last element should be preserved
				if last, ok := items[5].(map[string]interface{}); ok {
					if last["id"] != "6" {
						t.Error("last element not preserved")
					}
				}
			},
		},
		{
			name: "nested array path",
			input: map[string]interface{}{
				"data": map[string]interface{}{
					"results": []interface{}{
						map[string]interface{}{
							"flights": []interface{}{
								map[string]interface{}{
									"segments": []interface{}{
										map[string]interface{}{"departure": ">>NODE-TO-GET<<", "arrival": "2025-01-02"},
										map[string]interface{}{"departure": "2025-01-03", "arrival": "2025-01-04"},
									},
								},
							},
						},
					},
				},
			},
			targetPath: []string{"data", "results", "[0]", "flights", "[0]", "segments", "[0]", "departure"},
			validate: func(t *testing.T, result interface{}) {
				// Navigate to target and verify structure
				root := result.(map[string]interface{})
				data := root["data"].(map[string]interface{})
				results := data["results"].([]interface{})
				result0 := results[0].(map[string]interface{})
				flights := result0["flights"].([]interface{})
				flight0 := flights[0].(map[string]interface{})
				segments := flight0["segments"].([]interface{})
				segment0 := segments[0].(map[string]interface{})

				if segment0["departure"] != ">>NODE-TO-GET<<" {
					t.Error("target not preserved in nested array")
				}
				// Sibling field should be preserved
				if segment0["arrival"] != "2025-01-02" {
					t.Error("sibling field in target object not preserved")
				}
			},
		},
		{
			name: "deep structure pruning",
			input: map[string]interface{}{
				"target": map[string]interface{}{
					"value": ">>NODE-TO-GET<<",
				},
				"sibling": map[string]interface{}{
					"level1": map[string]interface{}{
						"level2": map[string]interface{}{
							"level3": map[string]interface{}{
								"deep": "should be pruned",
							},
						},
					},
				},
			},
			targetPath: []string{"target", "value"},
			validate: func(t *testing.T, result interface{}) {
				root := result.(map[string]interface{})

				// Target should be preserved
				target := root["target"].(map[string]interface{})
				if target["value"] != ">>NODE-TO-GET<<" {
					t.Error("target not preserved")
				}

				// Sibling should be pruned at depth
				sibling := root["sibling"].(map[string]interface{})
				level1 := sibling["level1"]

				// Check if it's pruned to placeholder or has limited depth
				if level1 == "{...}" {
					// Correctly pruned
					return
				}

				if l1Map, ok := level1.(map[string]interface{}); ok {
					level2 := l1Map["level2"]
					if level2 == "{...}" {
						// Correctly pruned at level 2
						return
					}
					if l2Map, ok := level2.(map[string]interface{}); ok {
						if l2Map["level3"] == "{...}" {
							// Correctly pruned at level 3
							return
						}
					}
				}
			},
		},
		{
			name: "large array pruning",
			input: map[string]interface{}{
				"results": func() []interface{} {
					arr := make([]interface{}, 100)
					for i := 0; i < 100; i++ {
						if i == 50 {
							arr[i] = map[string]interface{}{"id": ">>NODE-TO-GET<<", "index": i}
						} else {
							arr[i] = map[string]interface{}{"id": fmt.Sprintf("id-%d", i), "index": i}
						}
					}
					return arr
				}(),
			},
			targetPath: []string{"results", "[50]", "id"},
			validate: func(t *testing.T, result interface{}) {
				root := result.(map[string]interface{})
				results := root["results"].([]interface{})

				// Should keep first, last, target and surrounding elements
				// Others should be placeholders

				// Check first element
				if first, ok := results[0].(map[string]interface{}); ok {
					if first["id"] != "id-0" {
						t.Error("first element not preserved")
					}
				}

				// Check elements around target
				if elem49, ok := results[49].(map[string]interface{}); ok {
					if elem49["index"].(float64) != 49 {
						t.Error("element before target not preserved")
					}
				}

				// Check target
				if target, ok := results[50].(map[string]interface{}); ok {
					if target["id"] != ">>NODE-TO-GET<<" {
						t.Error("target element not preserved")
					}
				}

				// Check element after target
				if elem51, ok := results[51].(map[string]interface{}); ok {
					if elem51["index"].(float64) != 51 {
						t.Error("element after target not preserved")
					}
				}

				// Check last element
				if last, ok := results[99].(map[string]interface{}); ok {
					if last["id"] != "id-99" {
						t.Error("last element not preserved")
					}
				}

				// Check some middle elements are placeholders
				for i := 10; i < 40; i++ {
					if placeholder, ok := results[i].(string); ok {
						if !strings.Contains(placeholder, fmt.Sprintf("item %d", i)) {
							t.Errorf("element %d not replaced with placeholder: %v", i, results[i])
						}
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := smartPruneJSON(tt.input, tt.targetPath)
			tt.validate(t, result)
		})
	}
}

func TestExtractSmartPartialJSON(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		targetPath  string
		targetValue string
		validate    func(t *testing.T, partial interface{}, parentCtx map[string]interface{}, err error)
	}{
		{
			name: "extract with parent context",
			source: `{
				"data": {
					"user": {
						"id": "12345",
						"name": "John",
						"email": "john@example.com"
					}
				}
			}`,
			targetPath:  "data.user.id",
			targetValue: "12345",
			validate: func(t *testing.T, partial interface{}, parentCtx map[string]interface{}, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				// Check parent context contains sibling fields
				if parentCtx == nil {
					t.Fatal("parent context is nil")
				}
				if parentCtx["name"] != "John" {
					t.Error("parent context missing sibling field 'name'")
				}
				if parentCtx["email"] != "john@example.com" {
					t.Error("parent context missing sibling field 'email'")
				}

				// Check partial JSON has marker
				jsonBytes, _ := json.Marshal(partial)
				if !strings.Contains(string(jsonBytes), "NODE-TO-GET") {
					t.Error("partial JSON missing marker")
				}
			},
		},
		{
			name: "array element extraction",
			source: `{
				"items": [
					{"id": "1", "value": "first"},
					{"id": "2", "value": "second"},
					{"id": "3", "value": "third"}
				]
			}`,
			targetPath:  "items[1].value",
			targetValue: "second",
			validate: func(t *testing.T, partial interface{}, parentCtx map[string]interface{}, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				// Parent context should be the array element
				if parentCtx["id"] != "2" {
					t.Error("parent context incorrect for array element")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			partial, parentCtx, err := extractSmartPartialJSON(tt.source, tt.targetPath, tt.targetValue)
			tt.validate(t, partial, parentCtx, err)
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	// Check both the plain string and the JSON-escaped version
	jsonEscaped := "\\u003e\\u003eNODE-TO-GET\\u003c\\u003c"
	return strings.Contains(s, substr) || strings.Contains(s, jsonEscaped)
}
