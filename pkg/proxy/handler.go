package proxy

import (
	"crypto/tls"
	"net/http"
	"time"
)

// CreateHTTPClient creates an HTTP client with appropriate settings for proxying
func CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false, // Set to true if you need to proxy self-signed certificates
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// ModifyRequest allows for request modification before forwarding
func ModifyRequest(req *http.Request, targetHost string) {
	// Remove hop-by-hop headers
	req.Header.Del("Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("TE")
	req.Header.Del("Trailers")
	req.Header.Del("Transfer-Encoding")
	req.Header.Del("Upgrade")

	// Update host header
	req.Header.Set("X-Forwarded-Host", req.Host)
	req.Header.Set("X-Forwarded-For", req.RemoteAddr)
	req.Header.Set("X-Real-IP", req.RemoteAddr)
}

// ModifyResponse allows for response modification before returning to client
func ModifyResponse(resp *http.Response) error {
	// Remove hop-by-hop headers from response
	resp.Header.Del("Connection")
	resp.Header.Del("Keep-Alive")
	resp.Header.Del("Proxy-Authenticate")
	resp.Header.Del("Proxy-Authorization")
	resp.Header.Del("TE")
	resp.Header.Del("Trailers")
	resp.Header.Del("Transfer-Encoding")
	resp.Header.Del("Upgrade")

	return nil
}
