package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()
	assert.NotNil(t, builder)
}

func TestBuilder_BuildCollection(t *testing.T) {
	tests := []struct {
		name             string
		callDetails      []*util.CallDetails
		chainedValues    []*util.ChainedValueContext
		expectedItems    int
		expectedVars     int
		validateCallback func(t *testing.T, collection PostmanCollection)
	}{
		{
			name:          "empty input",
			callDetails:   []*util.CallDetails{},
			chainedValues: []*util.ChainedValueContext{},
			expectedItems: 0,
			expectedVars:  0,
		},
		{
			name: "single request without chained values",
			callDetails: []*util.CallDetails{
				{
					Name: "Get User",
					Entry: &har.Entry{
						Request: har.Request{
							Method: "GET",
							URL:    "https://api.example.com/users/123",
							Headers: []har.Header{
								{Name: "Authorization", Value: "Bearer token123"},
							},
						},
					},
					RequestChainedValues:  []*util.ValueReference{},
					ResponseChainedValues: []*util.ValueReference{},
				},
			},
			chainedValues: []*util.ChainedValueContext{},
			expectedItems: 1,
			expectedVars:  0,
			validateCallback: func(t *testing.T, collection PostmanCollection) {
				assert.Equal(t, "Get User", collection.Item[0].Name)
				assert.Equal(t, "GET", collection.Item[0].Request.Method)
			},
		},
		{
			name: "request with chained values",
			callDetails: []*util.CallDetails{
				{
					Name: "Create User",
					Entry: &har.Entry{
						Request: har.Request{
							Method: "POST",
							URL:    "https://api.example.com/users",
							Headers: []har.Header{
								{Name: "Content-Type", Value: "application/json"},
							},
							PostData: &har.PostData{
								Text: `{"name": "John Doe"}`,
							},
						},
					},
					RequestChainedValues: []*util.ValueReference{},
					ResponseChainedValues: []*util.ValueReference{
						{
							Value:         "user123",
							ReferencePath: "responseJson.userId",
							SourceType:    util.SourceTypeResponse,
							Context: &util.ChainedValueContext{
								VariableName: "userId",
							},
						},
					},
				},
				{
					Name: "Get User Details",
					Entry: &har.Entry{
						Request: har.Request{
							Method: "GET",
							URL:    "https://api.example.com/users/user123",
							Headers: []har.Header{
								{Name: "Authorization", Value: "Bearer token123"},
							},
						},
					},
					RequestChainedValues: []*util.ValueReference{
						{
							Value: "user123",
							Context: &util.ChainedValueContext{
								VariableName: "userId",
							},
						},
					},
					ResponseChainedValues: []*util.ValueReference{},
				},
			},
			chainedValues: []*util.ChainedValueContext{
				{
					VariableName: "userId",
					ValueSource: &util.ValueReference{
						ReferencePath: "responseJson.userId",
					},
				},
			},
			expectedItems: 2,
			expectedVars:  1,
			validateCallback: func(t *testing.T, collection PostmanCollection) {
				// Check first request has test script
				assert.Len(t, collection.Item[0].Event, 1)
				assert.Equal(t, "test", collection.Item[0].Event[0].Listen)

				// Check second request URL has variable replacement
				assert.Contains(t, collection.Item[1].Request.URL.Raw, "{{userId}}")
			},
		},
		{
			name: "request with init script",
			callDetails: []*util.CallDetails{
				{
					Name: "First Request",
					Entry: &har.Entry{
						Request: har.Request{
							Method: "GET",
							URL:    "https://api.example.com/test",
						},
					},
				},
			},
			chainedValues: []*util.ChainedValueContext{
				{
					VariableName: "timestamp",
					InitScript:   "result = Date.now();",
				},
			},
			expectedItems: 1,
			expectedVars:  1,
			validateCallback: func(t *testing.T, collection PostmanCollection) {
				// Check that first request has prerequest script
				assert.Len(t, collection.Item[0].Event, 2)
				var hasPreRequest bool
				for _, event := range collection.Item[0].Event {
					if event.Listen == "prerequest" {
						hasPreRequest = true
						assert.Contains(t, strings.Join(event.Script.Exec, "\n"), "result = Date.now();")
					}
				}
				assert.True(t, hasPreRequest, "Expected prerequest script to be present")
			},
		},
		{
			name: "nil call details in list",
			callDetails: []*util.CallDetails{
				nil,
				{
					Name: "Valid Request",
					Entry: &har.Entry{
						Request: har.Request{
							Method: "GET",
							URL:    "https://api.example.com/test",
						},
					},
				},
				nil,
			},
			chainedValues: []*util.ChainedValueContext{},
			expectedItems: 1,
			expectedVars:  0,
		},
		{
			name: "manually set variable without value source",
			callDetails: []*util.CallDetails{
				{
					Name: "Request with manual variable",
					Entry: &har.Entry{
						Request: har.Request{
							Method: "GET",
							URL:    "https://api.example.com/test",
						},
					},
				},
			},
			chainedValues: []*util.ChainedValueContext{
				{
					VariableName: "apiKey",
					ValueSource:  nil, // Manually set variable
				},
			},
			expectedItems: 1,
			expectedVars:  1,
			validateCallback: func(t *testing.T, collection PostmanCollection) {
				assert.Equal(t, "apiKey", collection.Variables[0].Key)
				assert.Equal(t, "Manually set variable", collection.Variables[0].Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			collection := builder.BuildCollection(tt.callDetails, tt.chainedValues)

			assert.Equal(t, "Generated Collection", collection.Info.Name)
			assert.Equal(t, "https://schema.getpostman.com/json/collection/v2.1.0/collection.json", collection.Info.Schema)
			assert.Equal(t, "2.1.0", collection.Info.Version)
			assert.Len(t, collection.Item, tt.expectedItems)
			assert.Len(t, collection.Variables, tt.expectedVars)

			if tt.validateCallback != nil {
				tt.validateCallback(t, collection)
			}
		})
	}
}

func TestBuilder_ReplaceChainedValuesInRequest(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name     string
		request  *util.CallDetails
		expected func(t *testing.T, result PostmanRequest)
	}{
		{
			name: "replace values in URL",
			request: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						Method:  "GET",
						URL:     "https://api.example.com/users/123/posts/456",
						Headers: []har.Header{},
					},
				},
				RequestChainedValues: []*util.ValueReference{
					{
						Value: "123",
						Context: &util.ChainedValueContext{
							VariableName: "userId",
						},
					},
					{
						Value: "456",
						Context: &util.ChainedValueContext{
							VariableName: "postId",
						},
					},
				},
			},
			expected: func(t *testing.T, result PostmanRequest) {
				assert.Equal(t, "GET", result.Method)
				assert.Contains(t, result.URL.Raw, "{{userId}}")
				assert.Contains(t, result.URL.Raw, "{{postId}}")
				assert.Contains(t, result.URL.Path, "{{userId}}")
				assert.Contains(t, result.URL.Path, "{{postId}}")
			},
		},
		{
			name: "replace values in headers",
			request: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						Method: "GET",
						URL:    "https://api.example.com/data",
						Headers: []har.Header{
							{Name: "Authorization", Value: "Bearer token123"},
							{Name: "X-Request-ID", Value: "req456"},
						},
					},
				},
				RequestChainedValues: []*util.ValueReference{
					{
						Value: "token123",
						Context: &util.ChainedValueContext{
							VariableName: "authToken",
						},
					},
					{
						Value: "req456",
						Context: &util.ChainedValueContext{
							VariableName: "requestId",
						},
					},
				},
			},
			expected: func(t *testing.T, result PostmanRequest) {
				assert.Len(t, result.Header, 2)
				for _, header := range result.Header {
					if header.Key == "Authorization" {
						assert.Equal(t, "Bearer {{authToken}}", header.Value)
					}
					if header.Key == "X-Request-ID" {
						assert.Equal(t, "{{requestId}}", header.Value)
					}
				}
			},
		},
		{
			name: "replace values in body",
			request: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						Method:  "POST",
						URL:     "https://api.example.com/create",
						Headers: []har.Header{},
						PostData: &har.PostData{
							Text: `{"userId": "123", "token": "abc456"}`,
						},
					},
				},
				RequestChainedValues: []*util.ValueReference{
					{
						Value: "123",
						Context: &util.ChainedValueContext{
							VariableName: "userId",
						},
					},
					{
						Value: "abc456",
						Context: &util.ChainedValueContext{
							VariableName: "apiToken",
						},
					},
				},
			},
			expected: func(t *testing.T, result PostmanRequest) {
				assert.NotNil(t, result.Body)
				assert.Equal(t, "raw", result.Body.Mode)
				assert.Contains(t, result.Body.Raw, "{{userId}}")
				assert.Contains(t, result.Body.Raw, "{{apiToken}}")
			},
		},
		{
			name: "skip certain headers",
			request: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						Method: "GET",
						URL:    "https://api.example.com/test",
						Headers: []har.Header{
							{Name: "Content-Type", Value: "application/json"},
							{Name: "Content-Length", Value: "123"},
							{Name: "Postman-Token", Value: "some-token"},
							{Name: "User-Agent", Value: "MyApp/1.0"},
						},
					},
				},
				RequestChainedValues: []*util.ValueReference{},
			},
			expected: func(t *testing.T, result PostmanRequest) {
				assert.Len(t, result.Header, 2)
				for _, header := range result.Header {
					assert.NotEqual(t, "Content-Length", header.Key)
					assert.NotEqual(t, "Postman-Token", header.Key)
				}
			},
		},
		{
			name: "empty body",
			request: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						Method:   "GET",
						URL:      "https://api.example.com/test",
						Headers:  []har.Header{},
						PostData: nil,
					},
				},
				RequestChainedValues: []*util.ValueReference{},
			},
			expected: func(t *testing.T, result PostmanRequest) {
				assert.NotNil(t, result.Body)
				assert.Empty(t, result.Body.Mode)
				assert.Empty(t, result.Body.Raw)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.ReplaceChainedValuesInRequest(tt.request)
			tt.expected(t, result)
		})
	}
}

func TestBuilder_ReplaceValuesInString(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name     string
		input    string
		values   []*util.ValueReference
		expected string
	}{
		{
			name:     "no replacements",
			input:    "Hello, World!",
			values:   []*util.ValueReference{},
			expected: "Hello, World!",
		},
		{
			name:  "single replacement",
			input: "User ID is 123",
			values: []*util.ValueReference{
				{
					Value: "123",
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
			},
			expected: "User ID is {{userId}}",
		},
		{
			name:  "multiple replacements",
			input: "User 123 has post 456",
			values: []*util.ValueReference{
				{
					Value: "123",
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
				{
					Value: "456",
					Context: &util.ChainedValueContext{
						VariableName: "postId",
					},
				},
			},
			expected: "User {{userId}} has post {{postId}}",
		},
		{
			name:  "value without context",
			input: "Test value 789",
			values: []*util.ValueReference{
				{
					Value:   "789",
					Context: nil,
				},
			},
			expected: "Test value 789",
		},
		{
			name:  "repeated value",
			input: "123 and 123 again",
			values: []*util.ValueReference{
				{
					Value: "123",
					Context: &util.ChainedValueContext{
						VariableName: "id",
					},
				},
			},
			expected: "{{id}} and {{id}} again",
		},
		{
			name:  "empty string",
			input: "",
			values: []*util.ValueReference{
				{
					Value: "test",
					Context: &util.ChainedValueContext{
						VariableName: "var",
					},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.ReplaceValuesInString(tt.input, tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuilder_BuildPostmanURL(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name     string
		details  *util.CallDetails
		expected func(t *testing.T, result PostmanURL)
	}{
		{
			name: "simple URL",
			details: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						URL: "https://api.example.com/users",
					},
				},
				RequestChainedValues: []*util.ValueReference{},
			},
			expected: func(t *testing.T, result PostmanURL) {
				assert.Equal(t, "https://api.example.com/users", result.Raw)
				assert.Equal(t, "https", result.Protocol)
				assert.Equal(t, []string{"api.example.com"}, result.Host)
				assert.Equal(t, []string{"users"}, result.Path)
				assert.Empty(t, result.Query)
			},
		},
		{
			name: "URL with path parameters",
			details: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						URL: "https://api.example.com/users/123/posts/456",
					},
				},
				RequestChainedValues: []*util.ValueReference{
					{
						Value: "123",
						Context: &util.ChainedValueContext{
							VariableName: "userId",
						},
					},
					{
						Value: "456",
						Context: &util.ChainedValueContext{
							VariableName: "postId",
						},
					},
				},
			},
			expected: func(t *testing.T, result PostmanURL) {
				assert.Contains(t, result.Raw, "{{userId}}")
				assert.Contains(t, result.Raw, "{{postId}}")
				assert.Equal(t, []string{"users", "{{userId}}", "posts", "{{postId}}"}, result.Path)
			},
		},
		{
			name: "URL with query parameters",
			details: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						URL: "https://api.example.com/search?q=test&limit=10",
					},
				},
				RequestChainedValues: []*util.ValueReference{
					{
						Value: "10",
						Context: &util.ChainedValueContext{
							VariableName: "pageLimit",
						},
					},
				},
			},
			expected: func(t *testing.T, result PostmanURL) {
				assert.Len(t, result.Query, 2)
				found := false
				for _, q := range result.Query {
					if q.Key == "limit" {
						assert.Equal(t, "{{pageLimit}}", q.Value)
						found = true
					}
				}
				assert.True(t, found, "Expected to find limit query parameter")
			},
		},
		{
			name: "URL with host replacement",
			details: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						URL: "https://tenant123.example.com/api/data",
					},
				},
				RequestChainedValues: []*util.ValueReference{
					{
						Value: "tenant123",
						Context: &util.ChainedValueContext{
							VariableName: "tenantId",
						},
					},
				},
			},
			expected: func(t *testing.T, result PostmanURL) {
				assert.Equal(t, []string{"{{tenantId}}.example.com"}, result.Host)
			},
		},
		{
			name: "URL with multiple query values for same key",
			details: &util.CallDetails{
				Entry: &har.Entry{
					Request: har.Request{
						URL: "https://api.example.com/search?tag=foo&tag=bar",
					},
				},
				RequestChainedValues: []*util.ValueReference{},
			},
			expected: func(t *testing.T, result PostmanURL) {
				assert.Len(t, result.Query, 2)
				tagCount := 0
				for _, q := range result.Query {
					if q.Key == "tag" {
						tagCount++
					}
				}
				assert.Equal(t, 2, tagCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildPostmanURL(tt.details)
			tt.expected(t, result)
		})
	}
}

func TestBuilder_CreateTestScript(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name          string
		chainedValues []*util.ValueReference
		expected      func(t *testing.T, event PostmanEvent)
	}{
		{
			name:          "no chained values",
			chainedValues: []*util.ValueReference{},
			expected: func(t *testing.T, event PostmanEvent) {
				assert.Equal(t, "test", event.Listen)
				assert.Equal(t, "text/javascript", event.Script.Type)
				assert.Len(t, event.Script.Exec, 1)
				assert.Equal(t, "var responseJson = pm.response.json();", event.Script.Exec[0])
			},
		},
		{
			name: "single response value",
			chainedValues: []*util.ValueReference{
				{
					Value:         "123",
					ReferencePath: "responseJson.data.userId",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
			},
			expected: func(t *testing.T, event PostmanEvent) {
				script := strings.Join(event.Script.Exec, "\n")
				assert.Contains(t, script, "var userId = responseJson.data.userId;")
				assert.Contains(t, script, `pm.collectionVariables.set("userId", userId);`)
				assert.Contains(t, script, "console.log('Variable: userId, Value:', userId);")
				assert.Contains(t, script, "} catch (e) {")
			},
		},
		{
			name: "multiple response values",
			chainedValues: []*util.ValueReference{
				{
					Value:         "123",
					ReferencePath: "responseJson.userId",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
				{
					Value:         "token456",
					ReferencePath: "responseJson.auth.token",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "authToken",
					},
				},
			},
			expected: func(t *testing.T, event PostmanEvent) {
				script := strings.Join(event.Script.Exec, "\n")
				assert.Contains(t, script, "var userId = responseJson.userId;")
				assert.Contains(t, script, "var authToken = responseJson.auth.token;")
			},
		},
		{
			name: "skip non-response values",
			chainedValues: []*util.ValueReference{
				{
					Value:         "123",
					ReferencePath: "requestJson.userId",
					SourceType:    util.SourceTypeRequest,
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
			},
			expected: func(t *testing.T, event PostmanEvent) {
				assert.Len(t, event.Script.Exec, 1)
				assert.Equal(t, "var responseJson = pm.response.json();", event.Script.Exec[0])
			},
		},
		{
			name: "skip duplicate variables",
			chainedValues: []*util.ValueReference{
				{
					Value:         "123",
					ReferencePath: "responseJson.userId",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
				{
					Value:         "123",
					ReferencePath: "responseJson.data.id",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
			},
			expected: func(t *testing.T, event PostmanEvent) {
				script := strings.Join(event.Script.Exec, "\n")
				// Should only have one occurrence of userId variable
				assert.Equal(t, 1, strings.Count(script, "var userId ="))
			},
		},
		{
			name: "skip values without context",
			chainedValues: []*util.ValueReference{
				{
					Value:         "123",
					ReferencePath: "responseJson.userId",
					SourceType:    util.SourceTypeResponse,
					Context:       nil,
				},
			},
			expected: func(t *testing.T, event PostmanEvent) {
				assert.Len(t, event.Script.Exec, 1)
				assert.Equal(t, "var responseJson = pm.response.json();", event.Script.Exec[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := builder.CreateTestScript(tt.chainedValues)
			tt.expected(t, event)
		})
	}
}

func TestBuilder_CreateInitScript(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name     string
		values   []*util.ChainedValueContext
		expected func(t *testing.T, event *PostmanEvent)
	}{
		{
			name:   "no init scripts",
			values: []*util.ChainedValueContext{},
			expected: func(t *testing.T, event *PostmanEvent) {
				assert.Nil(t, event)
			},
		},
		{
			name: "single init script",
			values: []*util.ChainedValueContext{
				{
					VariableName: "timestamp",
					InitScript:   "result = Date.now();",
				},
			},
			expected: func(t *testing.T, event *PostmanEvent) {
				require.NotNil(t, event)
				assert.Equal(t, "prerequest", event.Listen)
				script := strings.Join(event.Script.Exec, "\n")
				assert.Contains(t, script, "result = Date.now();")
				assert.Contains(t, script, `pm.collectionVariables.set("timestamp", result);`)
			},
		},
		{
			name: "multiple init scripts",
			values: []*util.ChainedValueContext{
				{
					VariableName: "timestamp",
					InitScript:   "result = Date.now();",
				},
				{
					VariableName: "uuid",
					InitScript:   "result = pm.variables.replaceIn('{{$guid}}');",
				},
			},
			expected: func(t *testing.T, event *PostmanEvent) {
				require.NotNil(t, event)
				script := strings.Join(event.Script.Exec, "\n")
				assert.Contains(t, script, "result = Date.now();")
				assert.Contains(t, script, "result = pm.variables.replaceIn('{{$guid}}');")
			},
		},
		{
			name: "skip empty init scripts",
			values: []*util.ChainedValueContext{
				{
					VariableName: "userId",
					InitScript:   "",
				},
			},
			expected: func(t *testing.T, event *PostmanEvent) {
				assert.Nil(t, event)
			},
		},
		{
			name: "mixed empty and non-empty init scripts",
			values: []*util.ChainedValueContext{
				{
					VariableName: "emptyVar",
					InitScript:   "",
				},
				{
					VariableName: "timestamp",
					InitScript:   "result = Date.now();",
				},
			},
			expected: func(t *testing.T, event *PostmanEvent) {
				require.NotNil(t, event)
				script := strings.Join(event.Script.Exec, "\n")
				assert.Contains(t, script, "result = Date.now();")
				assert.NotContains(t, script, "emptyVar")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := builder.CreateInitScript(tt.values)
			tt.expected(t, event)
		})
	}
}

func TestBuilder_WriteToFile(t *testing.T) {
	builder := NewBuilder()

	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "test_collection.json")

	collection := PostmanCollection{
		Info: CollectionInfo{
			Name:    "Test Collection",
			Schema:  "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
			Version: "2.1.0",
		},
		Item: []PostmanItem{
			{
				Name: "Test Request",
				Request: PostmanRequest{
					Method: "GET",
					URL: PostmanURL{
						Raw:      "https://api.example.com/test",
						Protocol: "https",
						Host:     []string{"api.example.com"},
						Path:     []string{"test"},
					},
				},
			},
		},
		Variables: []PostmanVariable{
			{
				Key:         "testVar",
				Value:       "testValue",
				Description: "Test variable",
			},
		},
	}

	err := builder.WriteToFile(collection, outputFile)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(outputFile)
	require.NoError(t, err)

	// Read and parse the file
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	var readCollection PostmanCollection
	err = json.Unmarshal(data, &readCollection)
	require.NoError(t, err)

	// Verify contents
	assert.Equal(t, collection.Info.Name, readCollection.Info.Name)
	assert.Len(t, readCollection.Item, 1)
	assert.Equal(t, "Test Request", readCollection.Item[0].Name)
	assert.Len(t, readCollection.Variables, 1)
	assert.Equal(t, "testVar", readCollection.Variables[0].Key)
}

func TestBuilder_WriteToFile_Error(t *testing.T) {
	builder := NewBuilder()

	// Try to write to an invalid path
	err := builder.WriteToFile(PostmanCollection{}, "/invalid/path/that/does/not/exist/file.json")
	assert.Error(t, err)
}

func TestBuilder_shouldSkipHeader(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name   string
		header har.Header
		skip   bool
	}{
		{
			name:   "normal header",
			header: har.Header{Name: "Authorization", Value: "Bearer token"},
			skip:   false,
		},
		{
			name:   "Content-Length header",
			header: har.Header{Name: "Content-Length", Value: "123"},
			skip:   true,
		},
		{
			name:   "Postman header",
			header: har.Header{Name: "Postman-Token", Value: "abc123"},
			skip:   true,
		},
		{
			name:   "Postman-Something header",
			header: har.Header{Name: "Postman-Something", Value: "value"},
			skip:   true,
		},
		{
			name:   "Content-Type header",
			header: har.Header{Name: "Content-Type", Value: "application/json"},
			skip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.shouldSkipHeader(tt.header)
			assert.Equal(t, tt.skip, result)
		})
	}
}

func TestBuilder_buildScriptForVariable(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name          string
		chainedValue  *util.ValueReference
		expectedLines []string
	}{
		{
			name: "simple variable extraction",
			chainedValue: &util.ValueReference{
				ReferencePath: "responseJson.userId",
				Context: &util.ChainedValueContext{
					VariableName: "userId",
				},
			},
			expectedLines: []string{
				"try {",
				"  var userId = responseJson.userId;",
				`  pm.collectionVariables.set("userId", userId);`,
				"  console.log('Variable: userId, Value:', userId);",
				"} catch (e) {",
				"  console.error('Error extracting variable userId:', e);",
				"}",
			},
		},
		{
			name: "nested variable extraction",
			chainedValue: &util.ValueReference{
				ReferencePath: "responseJson.data.user.id",
				Context: &util.ChainedValueContext{
					VariableName: "userId",
				},
			},
			expectedLines: []string{
				"try {",
				"  var userId = responseJson.data.user.id;",
				`  pm.collectionVariables.set("userId", userId);`,
				"  console.log('Variable: userId, Value:', userId);",
				"} catch (e) {",
				"  console.error('Error extracting variable userId:', e);",
				"}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := builder.buildScriptForVariable(tt.chainedValue)
			assert.Equal(t, tt.expectedLines, lines)
		})
	}
}

// Integration test
func TestBuilder_Integration(t *testing.T) {
	builder := NewBuilder()

	// Create a complete scenario
	callDetails := []*util.CallDetails{
		{
			Name: "Login",
			Entry: &har.Entry{
				Request: har.Request{
					Method: "POST",
					URL:    "https://api.example.com/auth/login",
					Headers: []har.Header{
						{Name: "Content-Type", Value: "application/json"},
					},
					PostData: &har.PostData{
						Text: `{"username": "user@example.com", "password": "password123"}`,
					},
				},
			},
			RequestChainedValues: []*util.ValueReference{},
			ResponseChainedValues: []*util.ValueReference{
				{
					Value:         "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
					ReferencePath: "responseJson.token",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "authToken",
					},
				},
				{
					Value:         "user123",
					ReferencePath: "responseJson.userId",
					SourceType:    util.SourceTypeResponse,
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
			},
		},
		{
			Name: "Get User Profile",
			Entry: &har.Entry{
				Request: har.Request{
					Method: "GET",
					URL:    "https://api.example.com/users/user123",
					Headers: []har.Header{
						{Name: "Authorization", Value: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
					},
				},
			},
			RequestChainedValues: []*util.ValueReference{
				{
					Value: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
					Context: &util.ChainedValueContext{
						VariableName: "authToken",
					},
				},
				{
					Value: "user123",
					Context: &util.ChainedValueContext{
						VariableName: "userId",
					},
				},
			},
			ResponseChainedValues: []*util.ValueReference{},
		},
	}

	chainedValues := []*util.ChainedValueContext{
		{
			VariableName: "authToken",
			ValueSource: &util.ValueReference{
				ReferencePath: "responseJson.token",
			},
		},
		{
			VariableName: "userId",
			ValueSource: &util.ValueReference{
				ReferencePath: "responseJson.userId",
			},
		},
		{
			VariableName: "timestamp",
			InitScript:   "result = Date.now();",
		},
	}

	collection := builder.BuildCollection(callDetails, chainedValues)

	// Verify collection structure
	assert.Equal(t, "Generated Collection", collection.Info.Name)
	assert.Len(t, collection.Item, 2)
	assert.Len(t, collection.Variables, 3)

	// Verify first request (Login)
	loginRequest := collection.Item[0]
	assert.Equal(t, "Login", loginRequest.Name)
	assert.Equal(t, "POST", loginRequest.Request.Method)
	assert.Contains(t, loginRequest.Request.URL.Raw, "/auth/login")

	// Should have both test and prerequest scripts
	assert.Len(t, loginRequest.Event, 2)

	// Verify second request (Get User Profile)
	profileRequest := collection.Item[1]
	assert.Equal(t, "Get User Profile", profileRequest.Name)
	assert.Contains(t, profileRequest.Request.URL.Raw, "/users/{{userId}}")

	// Check header replacement
	found := false
	for _, header := range profileRequest.Request.Header {
		if header.Key == "Authorization" {
			assert.Equal(t, "Bearer {{authToken}}", header.Value)
			found = true
		}
	}
	assert.True(t, found, "Authorization header not found")

	// Write to file and verify
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "integration_test.json")
	err := builder.WriteToFile(collection, outputFile)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(outputFile)
	require.NoError(t, err)
}
