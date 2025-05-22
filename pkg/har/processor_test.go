package har

import (
	"testing"
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
