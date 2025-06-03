package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// handleConnect handles HTTPS CONNECT requests for tunneling
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	// Extract destination host
	destHost := r.Host
	if destHost == "" {
		http.Error(w, "No destination host", http.StatusBadRequest)
		return
	}

	// Dial destination
	destConn, err := net.DialTimeout("tcp", destHost, 10*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to destination: %v", err), http.StatusBadGateway)
		return
	}
	defer destConn.Close()

	// Send 200 Connection Established
	w.WriteHeader(http.StatusOK)

	// Get the underlying connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Start bidirectional copy
	go transfer(destConn, clientConn)
	transfer(clientConn, destConn)
}

// transfer copies data between connections
func transfer(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}

// createTLSConfig creates a TLS configuration for the proxy
func createTLSConfig() *tls.Config {
	return &tls.Config{
		// Allow the proxy to handle various TLS versions
		MinVersion: tls.VersionTLS12,
		// You can add custom root CAs here if needed
		// RootCAs: customRootCAs,
	}
}
