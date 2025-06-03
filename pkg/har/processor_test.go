package har

import (
	"testing"

	"github.com/gburgyan/chainer/pkg/util"
)

func TestFlattenJSON(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		valueLen int // Expected number of ValueReference instances
	}{
		{
			name:     "Simple JSON object",
			jsonStr:  `{"name": "test", "value": 123}`,
			wantErr:  false,
			valueLen: 2, // Should extract "name" and "value"
		},
		{
			name:     "Nested JSON object",
			jsonStr:  `{"user": {"name": "test", "id": 456}, "status": "active"}`,
			wantErr:  false,
			valueLen: 3, // Should extract "user.name", "user.id", and "status"
		},
		{
			name:     "JSON with array",
			jsonStr:  `{"items": ["one", "two", "three"]}`,
			wantErr:  false,
			valueLen: 3, // Should extract "items[0]", "items[1]", and "items[2]"
		},
		{
			name:     "Empty JSON",
			jsonStr:  `{}`,
			wantErr:  false,
			valueLen: 0, // Should extract nothing
		},
		{
			name:     "Invalid JSON",
			jsonStr:  `{"broken": "json"`,
			wantErr:  true,
			valueLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processor.FlattenJSON(tt.jsonStr)

			// Check error behavior
			if (err != nil) != tt.wantErr {
				t.Errorf("FlattenJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Skip further checks if we expected an error
			if tt.wantErr {
				return
			}

			// Check the number of extracted values
			if len(got) != tt.valueLen {
				t.Errorf("FlattenJSON() returned %d values, want %d", len(got), tt.valueLen)
			}
		})
	}
}

func TestExtractURLStrings(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name           string
		url            string
		wantErr        bool
		minValuesCount int      // We expect at least this many values to be extracted
		requiredPaths  []string // These paths must be included in the results
	}{
		{
			name:           "Simple URL",
			url:            "https://example.com/path",
			wantErr:        false,
			minValuesCount: 1,
			requiredPaths:  []string{"host"},
		},
		{
			name:           "URL with query parameters",
			url:            "https://example.com/api/search?q=test&page=1",
			wantErr:        false,
			minValuesCount: 3,
			requiredPaths:  []string{"host", "query.q[0]", "query.page[0]"},
		},
		{
			name:           "Invalid URL",
			url:            "://invalid-url",
			wantErr:        true,
			minValuesCount: 0,
			requiredPaths:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processor.extractURLStrings(tt.url)

			// Check error behavior
			if (err != nil) != tt.wantErr {
				t.Errorf("extractURLStrings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Skip further checks if we expected an error
			if tt.wantErr {
				return
			}

			// Check that we got at least the minimum number of values
			if len(got) < tt.minValuesCount {
				t.Errorf("extractURLStrings() returned %d values, want at least %d", len(got), tt.minValuesCount)
				return
			}

			// Extract all reference paths for easier comparison
			gotPaths := make([]string, len(got))
			for i, val := range got {
				gotPaths[i] = val.ReferencePath
			}

			// Check that all required paths are present
			for _, requiredPath := range tt.requiredPaths {
				found := false
				for _, gotPath := range gotPaths {
					if gotPath == requiredPath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extractURLStrings() missing required path %q", requiredPath)
				}
			}
		})
	}
}

func TestProcessBody(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		body        string
		contentType string
		wantErr     bool
		wantNil     bool // Should we expect nil result
		valueLen    int  // Expected number of ValueReference instances
	}{
		{
			name:        "JSON body",
			body:        `{"name": "test", "value": 123}`,
			contentType: "application/json",
			wantErr:     false,
			wantNil:     false,
			valueLen:    2, // Should extract "name" and "value"
		},
		{
			name:        "Form data",
			body:        "key1=value1&key2=value2",
			contentType: "application/x-www-form-urlencoded",
			wantErr:     false,
			wantNil:     false,
			valueLen:    2, // Should extract "key1[0]" and "key2[0]"
		},
		{
			name:        "Empty JSON body",
			body:        "",
			contentType: "application/json",
			wantErr:     false,
			wantNil:     true,
			valueLen:    0,
		},
		{
			name:        "Invalid JSON body",
			body:        `{"broken": "json"`,
			contentType: "application/json",
			wantErr:     true,
			wantNil:     false,
			valueLen:    0,
		},
		{
			name:        "Unsupported content type",
			body:        "some data",
			contentType: "text/plain",
			wantErr:     false,
			wantNil:     true,
			valueLen:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processor.processBody(tt.body, tt.contentType)

			// Check error behavior
			if (err != nil) != tt.wantErr {
				t.Errorf("processBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if nil result is expected
			if tt.wantNil && got != nil {
				t.Errorf("processBody() = %v, want nil", got)
				return
			}

			// Skip further checks if we expected an error or nil result
			if tt.wantErr || tt.wantNil {
				return
			}

			// Check the number of extracted values
			if len(got) != tt.valueLen {
				t.Errorf("processBody() returned %d values, want %d", len(got), tt.valueLen)
			}
		})
	}
}

func TestProcessHeaders(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name         string
		headers      []Header
		expectedLen  int
		headerValues map[string]string // Map of header name to expected value
	}{
		{
			name: "Standard headers",
			headers: []Header{
				{Name: "Content-Type", Value: "application/json"},
				{Name: "Authorization", Value: "Bearer token123"},
				{Name: "X-Custom-Header", Value: "custom-value"},
			},
			expectedLen: 3, // Adjusted expectation
			headerValues: map[string]string{
				"Content-Type":    "application/json",
				"Authorization":   "token123", // Bearer is stripped
				"X-Custom-Header": "custom-value",
			},
		},
		{
			name: "Blacklisted headers",
			headers: []Header{
				{Name: "content-length", Value: "100"},
				{Name: "host", Value: "example.com"},
				{Name: "connection", Value: "keep-alive"},
				{Name: "cache-control", Value: "no-cache"},
				{Name: "postman-token", Value: "abc123"},
			},
			expectedLen:  0,
			headerValues: map[string]string{},
		},
		{
			name:         "No headers",
			headers:      []Header{},
			expectedLen:  0,
			headerValues: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.processHeaders(tt.headers)

			// Check the number of processed headers
			if len(result) != tt.expectedLen {
				t.Errorf("processHeaders() returned %d headers, want %d", len(result), tt.expectedLen)
				return
			}

			// Check the header values
			for _, headerRef := range result {
				expectedValue, exists := tt.headerValues[headerRef.HeaderName]
				if !exists {
					t.Errorf("Unexpected header processed: %s", headerRef.HeaderName)
					continue
				}

				if headerRef.Value != expectedValue {
					t.Errorf("Header %s has value %v, want %v", headerRef.HeaderName, headerRef.Value, expectedValue)
				}
			}
		})
	}
}

func TestProcessEntry(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name     string
		entry    *Entry
		validate func(t *testing.T, callDetails *util.CallDetails)
	}{
		{
			name: "Basic GET request",
			entry: &Entry{
				Request: Request{
					Method: "GET",
					URL:    "https://api.example.com/test",
					Headers: []Header{
						{Name: "Accept", Value: "application/json"},
					},
				},
				Response: Response{
					Status: 200,
					Content: Content{
						MimeType: "application/json",
						Text:     `{"status": "ok"}`,
					},
					Headers: []Header{
						{Name: "Content-Type", Value: "application/json"},
					},
				},
			},
			validate: func(t *testing.T, callDetails *util.CallDetails) {
				// Should have request and response details
				if len(callDetails.RequestDetails) == 0 {
					t.Error("Expected request details to be populated")
				}
				if len(callDetails.ResponseDetails) == 0 {
					t.Error("Expected response details to be populated")
				}
				// Should have the entry set
				if callDetails.Entry == nil {
					t.Error("Expected entry to be set")
				}
			},
		},
		{
			name: "POST request with body",
			entry: &Entry{
				Request: Request{
					Method: "POST",
					URL:    "https://api.example.com/create",
					PostData: &PostData{
						MimeType: "application/json",
						Text:     `{"name": "test"}`,
					},
				},
				Response: Response{
					Status: 201,
					Content: Content{
						MimeType: "application/json",
						Text:     `{"id": 123, "name": "test"}`,
					},
				},
			},
			validate: func(t *testing.T, callDetails *util.CallDetails) {
				// Check that body values were extracted
				foundRequestName := false
				for _, ref := range callDetails.RequestDetails {
					if ref.Value == "test" && ref.ReferencePath == "name" {
						foundRequestName = true
						break
					}
				}
				if !foundRequestName {
					t.Error("Expected to find 'name' field from request body")
				}

				// Check response extraction
				foundResponseId := false
				for _, ref := range callDetails.ResponseDetails {
					if ref.Value == float64(123) && ref.ReferencePath == "id" {
						foundResponseId = true
						break
					}
				}
				if !foundResponseId {
					t.Error("Expected to find 'id' field from response body")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.processEntry(tt.entry)

			if result == nil {
				t.Fatal("processEntry returned nil")
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestExtractRequestBody(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name           string
		request        Request
		expectedLen    int
		expectedValues map[string]interface{}
	}{
		{
			name: "JSON body",
			request: Request{
				PostData: &PostData{
					MimeType: "application/json",
					Text:     `{"key": "value", "number": 42}`,
				},
			},
			expectedLen: 2,
			expectedValues: map[string]interface{}{
				"key":    "value",
				"number": float64(42),
			},
		},
		{
			name: "Form data body",
			request: Request{
				PostData: &PostData{
					MimeType: "application/x-www-form-urlencoded",
					Text:     "field1=value1&field2=value2",
				},
			},
			expectedLen: 2,
			expectedValues: map[string]interface{}{
				"field1[0]": "value1",
				"field2[0]": "value2",
			},
		},
		{
			name: "No post data",
			request: Request{
				PostData: nil,
			},
			expectedLen:    0,
			expectedValues: map[string]interface{}{},
		},
		{
			name: "Empty body",
			request: Request{
				PostData: &PostData{
					MimeType: "application/json",
					Text:     "",
				},
			},
			expectedLen:    0,
			expectedValues: map[string]interface{}{},
		},
		{
			name: "Invalid JSON",
			request: Request{
				PostData: &PostData{
					MimeType: "application/json",
					Text:     `{"invalid": "json"`,
				},
			},
			expectedLen:    0,
			expectedValues: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.extractRequestBody(tt.request)

			if len(result) != tt.expectedLen {
				t.Errorf("extractRequestBody() returned %d values, want %d", len(result), tt.expectedLen)
			}

			// Check expected values
			for _, ref := range result {
				if expectedVal, exists := tt.expectedValues[ref.ReferencePath]; exists {
					if ref.Value != expectedVal {
						t.Errorf("Value at path %s = %v, want %v", ref.ReferencePath, ref.Value, expectedVal)
					}
				}
			}
		})
	}
}

func TestExtractURLDetails(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name           string
		url            string
		minExpectedLen int
		checkValues    map[string]string // path -> expected value
	}{
		{
			name:           "Simple URL",
			url:            "https://api.example.com/v1/users",
			minExpectedLen: 3, // host + 2 path segments
			checkValues: map[string]string{
				"host":    "api.example.com",
				"path[1]": "v1",
				"path[2]": "users",
			},
		},
		{
			name:           "URL with query parameters",
			url:            "https://api.example.com/search?q=test&limit=10",
			minExpectedLen: 4, // host + path + 2 query params
			checkValues: map[string]string{
				"host":           "api.example.com",
				"query.q[0]":     "test",
				"query.limit[0]": "10",
			},
		},
		{
			name:           "Invalid URL",
			url:            "://invalid",
			minExpectedLen: 0,
			checkValues:    map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.extractURLDetails(tt.url)

			if len(result) < tt.minExpectedLen {
				t.Errorf("extractURLDetails() returned %d values, want at least %d", len(result), tt.minExpectedLen)
			}

			// Check that all values have URL source location
			for _, ref := range result {
				if ref.SourceLocation != util.SourceLocationUrl {
					t.Errorf("Expected SourceLocation to be URL, got %v", ref.SourceLocation)
				}
			}

			// Check specific expected values
			for _, ref := range result {
				if expectedVal, exists := tt.checkValues[ref.ReferencePath]; exists {
					if ref.Value != expectedVal {
						t.Errorf("Value at path %s = %v, want %v", ref.ReferencePath, ref.Value, expectedVal)
					}
				}
			}
		})
	}
}

func TestSetValueReferenceMetadata(t *testing.T) {
	processor := NewProcessor()

	// Create test data
	callDetails := &util.CallDetails{
		Name: "test-call",
	}

	refs := []*util.ValueReference{
		{Value: "value1", ReferencePath: "path1"},
		{Value: "value2", ReferencePath: "path2"},
		{Value: "value3", ReferencePath: "path3"},
	}

	// Test setting request metadata
	processor.setValueReferenceMetadata(refs, callDetails, util.SourceTypeRequest)

	for i, ref := range refs {
		if ref.Source != callDetails {
			t.Errorf("refs[%d].Source not set correctly", i)
		}
		if ref.SourceType != util.SourceTypeRequest {
			t.Errorf("refs[%d].SourceType = %v, want %v", i, ref.SourceType, util.SourceTypeRequest)
		}
	}

	// Test setting response metadata
	newRefs := []*util.ValueReference{
		{Value: "value4", ReferencePath: "path4"},
		{Value: "value5", ReferencePath: "path5"},
	}

	processor.setValueReferenceMetadata(newRefs, callDetails, util.SourceTypeResponse)

	for i, ref := range newRefs {
		if ref.Source != callDetails {
			t.Errorf("newRefs[%d].Source not set correctly", i)
		}
		if ref.SourceType != util.SourceTypeResponse {
			t.Errorf("newRefs[%d].SourceType = %v, want %v", i, ref.SourceType, util.SourceTypeResponse)
		}
	}
}

func TestProcessRequest(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		entry       *Entry
		callDetails *util.CallDetails
		validate    func(t *testing.T, refs []*util.ValueReference)
	}{
		{
			name: "Complete request with all components",
			entry: &Entry{
				Request: Request{
					Method: "POST",
					URL:    "https://api.example.com/users?filter=active",
					Headers: []Header{
						{Name: "Authorization", Value: "Bearer token123"},
						{Name: "X-Custom", Value: "custom-value"},
					},
					PostData: &PostData{
						MimeType: "application/json",
						Text:     `{"username": "john"}`,
					},
				},
			},
			callDetails: &util.CallDetails{},
			validate: func(t *testing.T, refs []*util.ValueReference) {
				// Should have URL, headers, and body values
				hasURL := false
				hasHeader := false
				hasBody := false

				for _, ref := range refs {
					if ref.SourceLocation == util.SourceLocationUrl {
						hasURL = true
					}
					if ref.SourceLocation == util.SourceLocationHeader {
						hasHeader = true
					}
					if ref.SourceLocation == util.SourceLocationBodyJson {
						hasBody = true
					}
					// All should be marked as request type
					if ref.SourceType != util.SourceTypeRequest {
						t.Errorf("Expected SourceType to be Request, got %v", ref.SourceType)
					}
				}

				if !hasURL {
					t.Error("Expected to find URL values")
				}
				if !hasHeader {
					t.Error("Expected to find header values")
				}
				if !hasBody {
					t.Error("Expected to find body values")
				}
			},
		},
		{
			name: "GET request without body",
			entry: &Entry{
				Request: Request{
					Method: "GET",
					URL:    "https://api.example.com/data/123",
					Headers: []Header{
						{Name: "Accept", Value: "application/json"},
					},
				},
			},
			callDetails: &util.CallDetails{},
			validate: func(t *testing.T, refs []*util.ValueReference) {
				// Should have URL and headers but no body
				for _, ref := range refs {
					if ref.SourceLocation == util.SourceLocationBodyJson {
						t.Error("Should not have body values for GET request without body")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.processRequest(tt.entry, tt.callDetails)

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestProcessResponse(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		entry       *Entry
		callDetails *util.CallDetails
		validate    func(t *testing.T, refs []*util.ValueReference)
	}{
		{
			name: "JSON response with headers",
			entry: &Entry{
				Response: Response{
					Status: 200,
					Content: Content{
						MimeType: "application/json",
						Text:     `{"result": "success", "data": {"id": 42}}`,
					},
					Headers: []Header{
						{Name: "X-Response-Id", Value: "resp-123"},
						{Name: "Cache-Control", Value: "no-cache"}, // This should be filtered
					},
				},
			},
			callDetails: &util.CallDetails{},
			validate: func(t *testing.T, refs []*util.ValueReference) {
				// Check for JSON values
				foundResult := false
				foundNestedId := false
				foundHeaderId := false

				for _, ref := range refs {
					if ref.Value == "success" && ref.ReferencePath == "result" {
						foundResult = true
					}
					if ref.Value == float64(42) && ref.ReferencePath == "data.id" {
						foundNestedId = true
					}
					if ref.Value == "resp-123" && ref.HeaderName == "X-Response-Id" {
						foundHeaderId = true
					}
					// All should be response type
					if ref.SourceType != util.SourceTypeResponse {
						t.Errorf("Expected SourceType to be Response, got %v", ref.SourceType)
					}
				}

				if !foundResult {
					t.Error("Expected to find 'result' field")
				}
				if !foundNestedId {
					t.Error("Expected to find nested 'data.id' field")
				}
				if !foundHeaderId {
					t.Error("Expected to find X-Response-Id header")
				}
			},
		},
		{
			name: "Empty response",
			entry: &Entry{
				Response: Response{
					Status: 204,
					Content: Content{
						MimeType: "application/json",
						Text:     "",
					},
					Headers: []Header{},
				},
			},
			callDetails: &util.CallDetails{},
			validate: func(t *testing.T, refs []*util.ValueReference) {
				// Should have no values
				if len(refs) != 0 {
					t.Errorf("Expected empty response to produce no values, got %d", len(refs))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.processResponse(tt.entry, tt.callDetails)

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestProcessHAR(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name             string
		har              HAR
		expectedCallsLen int
		validate         func(t *testing.T, calls []*util.CallDetails)
	}{
		{
			name: "Empty HAR",
			har: HAR{
				Log: Log{
					Entries: []Entry{},
				},
			},
			expectedCallsLen: 0,
		},
		{
			name: "Single entry integration",
			har: HAR{
				Log: Log{
					Entries: []Entry{
						{
							Request: Request{
								Method: "GET",
								URL:    "https://api.example.com/users/123",
								Headers: []Header{
									{Name: "Accept", Value: "application/json"},
								},
							},
							Response: Response{
								Status:     200,
								StatusText: "OK",
								Content: Content{
									MimeType: "application/json",
									Text:     `{"id": 123, "name": "John Doe"}`,
								},
								Headers: []Header{
									{Name: "Content-Type", Value: "application/json"},
								},
							},
						},
					},
				},
			},
			expectedCallsLen: 1,
			validate: func(t *testing.T, calls []*util.CallDetails) {
				if len(calls) != 1 {
					return
				}
				call := calls[0]

				// Basic integration check - ensure both request and response details exist
				if len(call.RequestDetails) == 0 {
					t.Error("Expected request details to be populated")
				}
				if len(call.ResponseDetails) == 0 {
					t.Error("Expected response details to be populated")
				}

				// Verify entry reference is maintained
				if call.Entry == nil {
					t.Error("Expected entry reference to be set")
				}
			},
		},
		{
			name: "Multiple entries",
			har: HAR{
				Log: Log{
					Entries: []Entry{
						{
							Request: Request{
								Method: "GET",
								URL:    "https://api.example.com/users",
							},
							Response: Response{
								Status: 200,
								Content: Content{
									MimeType: "application/json",
									Text:     `[{"id": 1}, {"id": 2}]`,
								},
							},
						},
						{
							Request: Request{
								Method: "POST",
								URL:    "https://api.example.com/users",
								PostData: &PostData{
									MimeType: "application/json",
									Text:     `{"name": "New User"}`,
								},
							},
							Response: Response{
								Status: 201,
								Content: Content{
									MimeType: "application/json",
									Text:     `{"id": 3, "name": "New User"}`,
								},
							},
						},
					},
				},
			},
			expectedCallsLen: 2,
		},
		{
			name: "Error handling - malformed JSON",
			har: HAR{
				Log: Log{
					Entries: []Entry{
						{
							Request: Request{
								Method: "GET",
								URL:    "https://api.example.com/broken",
							},
							Response: Response{
								Status: 200,
								Content: Content{
									MimeType: "application/json",
									Text:     `{"broken": "json"`,
								},
							},
						},
					},
				},
			},
			expectedCallsLen: 1,
			validate: func(t *testing.T, calls []*util.CallDetails) {
				if len(calls) != 1 {
					return
				}
				call := calls[0]

				// Should still process request details despite response error
				if len(call.RequestDetails) == 0 {
					t.Error("Expected request details even with malformed response")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := processor.ProcessHAR(tt.har)

			// Check the number of calls
			if len(calls) != tt.expectedCallsLen {
				t.Errorf("ProcessHAR() returned %d calls, want %d", len(calls), tt.expectedCallsLen)
				return
			}

			// Run custom validation if provided
			if tt.validate != nil {
				tt.validate(t, calls)
			}
		})
	}
}
