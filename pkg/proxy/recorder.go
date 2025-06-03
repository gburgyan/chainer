package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/gburgyan/chainer/pkg/har"
)

// Recorder handles recording HTTP transactions to HAR format
type Recorder struct {
	entries []har.Entry
	mu      sync.Mutex
	path    string
}

// NewRecorder creates a new HAR recorder
func NewRecorder(path string) *Recorder {
	return &Recorder{
		path:    path,
		entries: make([]har.Entry, 0),
	}
}

// RecordEntry adds a new entry to the HAR record
func (r *Recorder) RecordEntry(entry RecordedEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	harEntry := har.Entry{
		Request:  r.convertRequest(entry.Request),
		Response: r.convertResponse(entry.Response),
	}

	r.entries = append(r.entries, harEntry)
}

// Save writes the recorded entries to a HAR file
func (r *Recorder) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.path == "" {
		return fmt.Errorf("no record path specified")
	}

	harFile := har.HAR{
		Log: har.Log{
			Entries: r.entries,
		},
	}

	data, err := json.MarshalIndent(harFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal HAR: %w", err)
	}

	if err := os.WriteFile(r.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write HAR file: %w", err)
	}

	return nil
}

// GetEntryCount returns the number of recorded entries
func (r *Recorder) GetEntryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func (r *Recorder) convertRequest(req *RecordedRequest) har.Request {
	headers := make([]har.Header, 0)
	for name, values := range req.Headers {
		for _, value := range values {
			headers = append(headers, har.Header{
				Name:  name,
				Value: value,
			})
		}
	}

	harReq := har.Request{
		Method:  req.Method,
		URL:     req.URL,
		Headers: headers,
	}

	if len(req.Body) > 0 {
		harReq.PostData = &har.PostData{
			Text: string(req.Body),
		}
	}

	return harReq
}

func (r *Recorder) convertResponse(resp *RecordedResponse) har.Response {
	headers := make([]har.Header, 0)
	for name, values := range resp.Headers {
		for _, value := range values {
			headers = append(headers, har.Header{
				Name:  name,
				Value: value,
			})
		}
	}

	return har.Response{
		Status:     resp.Status,
		StatusText: resp.StatusText,
		Headers:    headers,
		Content: har.Content{
			Text: string(resp.Body),
		},
	}
}
