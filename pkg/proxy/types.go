package proxy

import (
	"net/http"
	"sync"
	"time"
)

// Server represents the proxy server
type Server struct {
	Port       int
	RecordPath string
	recorder   *Recorder
	client     *http.Client
	mu         sync.RWMutex
}

// Options for creating a new proxy server
type Options struct {
	Port       int
	RecordPath string
}

// RecordedEntry represents a single HTTP transaction to be recorded in HAR
type RecordedEntry struct {
	StartedDateTime time.Time
	Time            int64 // Duration in milliseconds
	Request         *RecordedRequest
	Response        *RecordedResponse
}

// RecordedRequest represents request data to be recorded
type RecordedRequest struct {
	Method      string
	URL         string
	HTTPVersion string
	Headers     map[string][]string
	BodySize    int64
	Body        []byte
}

// RecordedResponse represents response data to be recorded
type RecordedResponse struct {
	Status      int
	StatusText  string
	HTTPVersion string
	Headers     map[string][]string
	BodySize    int64
	Body        []byte
}
