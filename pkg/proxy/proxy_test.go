package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid configuration with recording",
			opts: Options{
				Port:       8080,
				RecordPath: "test.har",
			},
			wantErr: false,
		},
		{
			name: "valid configuration without recording",
			opts: Options{
				Port: 8080,
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			opts: Options{
				Port: 0,
			},
			wantErr: true,
			errMsg:  "invalid port: 0",
		},
		{
			name: "negative port",
			opts: Options{
				Port: -1,
			},
			wantErr: true,
			errMsg:  "invalid port: -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := New(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, server)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, server)
				assert.Equal(t, tt.opts.Port, server.Port)
				assert.Equal(t, tt.opts.RecordPath, server.RecordPath)
				assert.NotNil(t, server.client)

				if tt.opts.RecordPath != "" {
					assert.NotNil(t, server.recorder)
				} else {
					assert.Nil(t, server.recorder)
				}
			}
		})
	}
}

func TestServer_Stop(t *testing.T) {
	t.Run("stop without recorder", func(t *testing.T) {
		server, err := New(Options{
			Port: 8080,
		})
		require.NoError(t, err)

		err = server.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop with recorder", func(t *testing.T) {
		server, err := New(Options{
			Port:       8080,
			RecordPath: "test-stop.har",
		})
		require.NoError(t, err)

		// Add some test entries to the recorder
		server.recorder.RecordEntry(RecordedEntry{
			StartedDateTime: time.Now(),
			Time:            100,
			Request: &RecordedRequest{
				Method: "GET",
				URL:    "https://api.example.com/test",
			},
			Response: &RecordedResponse{
				Status:     200,
				StatusText: "OK",
			},
		})

		err = server.Stop()
		assert.NoError(t, err)

		// Cleanup
		_ = removeTestFile("test-stop.har")
	})
}

func TestServer_handleHTTPProxy(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		withRecording bool
		expectedError string
	}{
		{
			name:          "non-absolute URL",
			url:           "/relative/path",
			expectedError: "Invalid proxy request: URL must be absolute",
		},
		{
			name:          "absolute URL without recording",
			url:           "http://example.com/test",
			withRecording: false,
		},
		{
			name:          "absolute URL with recording",
			url:           "http://example.com/test",
			withRecording: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				Port: 8080,
			}
			if tt.withRecording {
				opts.RecordPath = "test-proxy.har"
			}

			server, err := New(opts)
			require.NoError(t, err)

			// Create test request
			req := httptest.NewRequest("GET", tt.url, nil)
			if tt.url != "/relative/path" {
				// Make URL absolute for valid tests
				req.URL, _ = url.Parse(tt.url)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Handle the request
			server.handleHTTPProxy(rr, req)

			if tt.expectedError != "" {
				assert.Equal(t, http.StatusBadRequest, rr.Code)
				assert.Contains(t, rr.Body.String(), tt.expectedError)
			}
		})
	}
}

func TestServer_forwardHTTPRequest(t *testing.T) {
	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back request info
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test-Header", "test-value")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"method":"%s","path":"%s"}`, r.Method, r.URL.Path)
	}))
	defer backend.Close()

	server, err := New(Options{Port: 8080})
	require.NoError(t, err)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GET request",
			method: "GET",
			path:   "/test",
		},
		{
			name:   "POST request",
			method: "POST",
			path:   "/api/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			targetURL := backend.URL + tt.path
			req := httptest.NewRequest(tt.method, targetURL, nil)
			req.URL, _ = url.Parse(targetURL)

			// Add proxy headers that should be removed
			req.Header.Set("Proxy-Connection", "keep-alive")
			req.Header.Set("Proxy-Authorization", "Basic test")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Forward the request
			server.forwardHTTPRequest(rr, req)

			// Check response
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "test-value", rr.Header().Get("X-Test-Header"))

			// Verify response body
			var resp map[string]string
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.method, resp["method"])
			assert.Equal(t, tt.path, resp["path"])
		})
	}
}

// Helper function to remove test files
func removeTestFile(path string) error {
	// Only remove files that start with "test-" for safety
	if len(path) > 5 && path[:5] == "test-" {
		return nil // Don't actually remove in tests
	}
	return nil
}
