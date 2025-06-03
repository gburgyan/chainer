package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRecorder(t *testing.T) {
	recorder := NewRecorder("test.har")
	assert.NotNil(t, recorder)
	assert.Equal(t, "test.har", recorder.path)
	assert.NotNil(t, recorder.entries)
	assert.Equal(t, 0, len(recorder.entries))
}

func TestRecorder_RecordEntry(t *testing.T) {
	recorder := NewRecorder("test.har")

	entry1 := RecordedEntry{
		StartedDateTime: time.Now(),
		Time:            150,
		Request: &RecordedRequest{
			Method:      "GET",
			URL:         "https://api.example.com/users",
			HTTPVersion: "HTTP/1.1",
			Headers: map[string][]string{
				"Accept":     {"application/json"},
				"User-Agent": {"test-client/1.0"},
			},
			BodySize: 0,
		},
		Response: &RecordedResponse{
			Status:      200,
			StatusText:  "OK",
			HTTPVersion: "HTTP/1.1",
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body:     []byte(`{"users": [{"id": 1, "name": "John"}]}`),
			BodySize: 38,
		},
	}

	entry2 := RecordedEntry{
		StartedDateTime: time.Now().Add(1 * time.Second),
		Time:            200,
		Request: &RecordedRequest{
			Method:      "POST",
			URL:         "https://api.example.com/users",
			HTTPVersion: "HTTP/1.1",
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body:     []byte(`{"name": "Jane", "email": "jane@example.com"}`),
			BodySize: 46,
		},
		Response: &RecordedResponse{
			Status:      201,
			StatusText:  "Created",
			HTTPVersion: "HTTP/1.1",
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
				"Location":     {"/users/2"},
			},
			Body:     []byte(`{"id": 2, "name": "Jane", "email": "jane@example.com"}`),
			BodySize: 55,
		},
	}

	// Record entries
	recorder.RecordEntry(entry1)
	recorder.RecordEntry(entry2)

	// Check entry count
	assert.Equal(t, 2, recorder.GetEntryCount())
	assert.Equal(t, 2, len(recorder.entries))
}

func TestRecorder_GetEntryCount(t *testing.T) {
	recorder := NewRecorder("test.har")

	assert.Equal(t, 0, recorder.GetEntryCount())

	// Add entries
	for i := 0; i < 5; i++ {
		recorder.RecordEntry(RecordedEntry{
			StartedDateTime: time.Now(),
			Time:            int64(i * 100),
			Request: &RecordedRequest{
				Method: "GET",
				URL:    "https://api.example.com/test",
			},
			Response: &RecordedResponse{
				Status:     200,
				StatusText: "OK",
			},
		})
	}

	assert.Equal(t, 5, recorder.GetEntryCount())
}

func TestRecorder_Save(t *testing.T) {
	// Create temp directory for test files
	tempDir := t.TempDir()
	harPath := filepath.Join(tempDir, "test-save.har")

	t.Run("save with valid path", func(t *testing.T) {
		recorder := NewRecorder(harPath)

		// Add test entries
		recorder.RecordEntry(RecordedEntry{
			StartedDateTime: time.Now(),
			Time:            100,
			Request: &RecordedRequest{
				Method: "GET",
				URL:    "https://api.example.com/test",
				Headers: map[string][]string{
					"Accept": {"application/json"},
				},
			},
			Response: &RecordedResponse{
				Status:     200,
				StatusText: "OK",
				Headers: map[string][]string{
					"Content-Type": {"application/json"},
				},
				Body: []byte(`{"result": "success"}`),
			},
		})

		// Save the file
		err := recorder.Save()
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(harPath)
		assert.NoError(t, err)

		// Read and verify content
		data, err := os.ReadFile(harPath)
		require.NoError(t, err)

		var harFile har.HAR
		err = json.Unmarshal(data, &harFile)
		require.NoError(t, err)

		assert.Equal(t, 1, len(harFile.Log.Entries))
		assert.Equal(t, "GET", harFile.Log.Entries[0].Request.Method)
		assert.Equal(t, "https://api.example.com/test", harFile.Log.Entries[0].Request.URL)
		assert.Equal(t, 200, harFile.Log.Entries[0].Response.Status)
	})

	t.Run("save with empty path", func(t *testing.T) {
		recorder := NewRecorder("")
		err := recorder.Save()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no record path specified")
	})

	t.Run("save to invalid path", func(t *testing.T) {
		recorder := NewRecorder("/invalid/path/that/does/not/exist/test.har")
		recorder.RecordEntry(RecordedEntry{
			StartedDateTime: time.Now(),
			Request:         &RecordedRequest{Method: "GET", URL: "test"},
			Response:        &RecordedResponse{Status: 200},
		})

		err := recorder.Save()
		assert.Error(t, err)
	})
}

func TestRecorder_convertRequest(t *testing.T) {
	recorder := NewRecorder("test.har")

	tests := []struct {
		name     string
		request  *RecordedRequest
		validate func(t *testing.T, harReq har.Request)
	}{
		{
			name: "simple GET request",
			request: &RecordedRequest{
				Method: "GET",
				URL:    "https://api.example.com/users",
				Headers: map[string][]string{
					"Accept":     {"application/json"},
					"User-Agent": {"test/1.0"},
				},
				BodySize: 0,
			},
			validate: func(t *testing.T, harReq har.Request) {
				assert.Equal(t, "GET", harReq.Method)
				assert.Equal(t, "https://api.example.com/users", harReq.URL)
				assert.Equal(t, 2, len(harReq.Headers))
				assert.Nil(t, harReq.PostData)
			},
		},
		{
			name: "POST request with body",
			request: &RecordedRequest{
				Method: "POST",
				URL:    "https://api.example.com/users",
				Headers: map[string][]string{
					"Content-Type": {"application/json"},
				},
				Body:     []byte(`{"name": "test"}`),
				BodySize: 16,
			},
			validate: func(t *testing.T, harReq har.Request) {
				assert.Equal(t, "POST", harReq.Method)
				assert.NotNil(t, harReq.PostData)
				assert.Equal(t, `{"name": "test"}`, harReq.PostData.Text)
			},
		},
		{
			name: "request with multiple header values",
			request: &RecordedRequest{
				Method: "GET",
				URL:    "https://api.example.com/test",
				Headers: map[string][]string{
					"Accept": {"application/json", "text/html"},
					"Cookie": {"session=abc123", "user=john"},
				},
			},
			validate: func(t *testing.T, harReq har.Request) {
				// Count total headers (should be 4)
				assert.Equal(t, 4, len(harReq.Headers))

				// Verify all header values are present
				acceptCount := 0
				cookieCount := 0
				for _, h := range harReq.Headers {
					if h.Name == "Accept" {
						acceptCount++
					}
					if h.Name == "Cookie" {
						cookieCount++
					}
				}
				assert.Equal(t, 2, acceptCount)
				assert.Equal(t, 2, cookieCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harReq := recorder.convertRequest(tt.request)
			tt.validate(t, harReq)
		})
	}
}

func TestRecorder_convertResponse(t *testing.T) {
	recorder := NewRecorder("test.har")

	tests := []struct {
		name     string
		response *RecordedResponse
		validate func(t *testing.T, harResp har.Response)
	}{
		{
			name: "simple successful response",
			response: &RecordedResponse{
				Status:      200,
				StatusText:  "OK",
				HTTPVersion: "HTTP/1.1",
				Headers: map[string][]string{
					"Content-Type": {"application/json"},
				},
				Body:     []byte(`{"result": "success"}`),
				BodySize: 21,
			},
			validate: func(t *testing.T, harResp har.Response) {
				assert.Equal(t, 200, harResp.Status)
				assert.Equal(t, "OK", harResp.StatusText)
				assert.Equal(t, 1, len(harResp.Headers))
				assert.Equal(t, `{"result": "success"}`, harResp.Content.Text)
			},
		},
		{
			name: "error response",
			response: &RecordedResponse{
				Status:      404,
				StatusText:  "Not Found",
				HTTPVersion: "HTTP/1.1",
				Headers: map[string][]string{
					"Content-Type": {"text/plain"},
				},
				Body:     []byte("Resource not found"),
				BodySize: 18,
			},
			validate: func(t *testing.T, harResp har.Response) {
				assert.Equal(t, 404, harResp.Status)
				assert.Equal(t, "Not Found", harResp.StatusText)
				assert.Equal(t, "Resource not found", harResp.Content.Text)
			},
		},
		{
			name: "response with multiple headers",
			response: &RecordedResponse{
				Status:     201,
				StatusText: "Created",
				Headers: map[string][]string{
					"Content-Type": {"application/json"},
					"Location":     {"/users/123"},
					"Set-Cookie":   {"session=xyz", "preferences=dark"},
				},
				Body: []byte(`{"id": 123}`),
			},
			validate: func(t *testing.T, harResp har.Response) {
				assert.Equal(t, 201, harResp.Status)
				assert.Equal(t, 4, len(harResp.Headers)) // Total header count

				// Verify Set-Cookie headers
				cookieCount := 0
				for _, h := range harResp.Headers {
					if h.Name == "Set-Cookie" {
						cookieCount++
					}
				}
				assert.Equal(t, 2, cookieCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harResp := recorder.convertResponse(tt.response)
			tt.validate(t, harResp)
		})
	}
}

func TestRecorder_ThreadSafety(t *testing.T) {
	recorder := NewRecorder("test.har")

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			recorder.RecordEntry(RecordedEntry{
				StartedDateTime: time.Now(),
				Time:            int64(idx),
				Request: &RecordedRequest{
					Method: "GET",
					URL:    "https://api.example.com/test",
				},
				Response: &RecordedResponse{
					Status:     200,
					StatusText: "OK",
				},
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all entries were recorded
	assert.Equal(t, 10, recorder.GetEntryCount())
}
