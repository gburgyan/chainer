package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// New creates a new proxy server
func New(opts Options) (*Server, error) {
	if opts.Port <= 0 {
		return nil, fmt.Errorf("invalid port: %d", opts.Port)
	}

	s := &Server{
		Port:       opts.Port,
		RecordPath: opts.RecordPath,
		client:     CreateHTTPClient(),
	}

	if opts.RecordPath != "" {
		s.recorder = NewRecorder(opts.RecordPath)
	}

	return s, nil
}

// Start starts the proxy server
func (s *Server) Start() error {
	// Create main handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			// Handle HTTPS tunneling
			s.handleConnect(w, r)
		} else {
			// Handle regular HTTP proxy requests
			s.handleHTTPProxy(w, r)
		}
	})

	addr := fmt.Sprintf(":%d", s.Port)
	log.Printf("Starting forward proxy server on %s", addr)
	if s.RecordPath != "" {
		log.Printf("Recording HAR to: %s", s.RecordPath)
	}

	return http.ListenAndServe(addr, handler)
}

// handleHTTPProxy handles standard HTTP proxy requests
func (s *Server) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	// Ensure the URL is absolute
	if !r.URL.IsAbs() {
		http.Error(w, "Invalid proxy request: URL must be absolute", http.StatusBadRequest)
		return
	}

	// Record the request if recording is enabled
	if s.recorder != nil {
		s.recordHTTPTransaction(w, r)
		return
	}

	// Forward the request without recording
	s.forwardHTTPRequest(w, r)
}

// forwardHTTPRequest forwards an HTTP request to the target server
func (s *Server) forwardHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// Remove proxy-specific headers
	r.Header.Del("Proxy-Connection")
	r.Header.Del("Proxy-Authenticate")
	r.Header.Del("Proxy-Authorization")

	// Create new request
	targetURL := r.URL.String()
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating request: %v", err), http.StatusBadGateway)
		return
	}

	// Copy headers
	proxyReq.Header = r.Header.Clone()

	// Perform the request
	resp, err := s.client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error forwarding request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	io.Copy(w, resp.Body)
}

// recordHTTPTransaction records an HTTP transaction and forwards it
func (s *Server) recordHTTPTransaction(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Capture request body
	var requestBody []byte
	if r.Body != nil {
		requestBody, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(requestBody))
	}

	// Create recorded request
	recordedReq := &RecordedRequest{
		Method:      r.Method,
		URL:         r.URL.String(),
		HTTPVersion: r.Proto,
		Headers:     r.Header,
		BodySize:    int64(len(requestBody)),
		Body:        requestBody,
	}

	// Use custom response writer to capture response
	rw := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		headers:        make(http.Header),
		body:           &bytes.Buffer{},
	}

	// Forward the request
	s.forwardHTTPRequest(rw, r)

	// Calculate duration
	duration := time.Since(startTime).Milliseconds()

	// Create recorded response
	recordedResp := &RecordedResponse{
		Status:      rw.statusCode,
		StatusText:  http.StatusText(rw.statusCode),
		HTTPVersion: r.Proto,
		Headers:     rw.headers,
		BodySize:    int64(rw.body.Len()),
		Body:        rw.body.Bytes(),
	}

	// Record the entry
	s.recorder.RecordEntry(RecordedEntry{
		StartedDateTime: startTime,
		Time:            duration,
		Request:         recordedReq,
		Response:        recordedResp,
	})
}

// Stop gracefully stops the proxy server and saves any recorded data
func (s *Server) Stop() error {
	if s.recorder != nil {
		log.Printf("Saving HAR file with %d entries", s.recorder.GetEntryCount())
		return s.recorder.Save()
	}
	return nil
}

// responseRecorder captures response data
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	headers    http.Header
	body       *bytes.Buffer
	written    bool
}

func (rw *responseRecorder) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		// Copy headers
		for k, v := range rw.ResponseWriter.Header() {
			rw.headers[k] = v
		}
		rw.ResponseWriter.WriteHeader(code)
		rw.written = true
	}
}

func (rw *responseRecorder) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}
