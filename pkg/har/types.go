package har

// HAR represents the root structure of a HAR (HTTP Archive) file.
// It contains a log of HTTP transactions.
type HAR struct {
	// Log holds the collection of HTTP entries.
	Log Log `json:"log"`
}

// Log encapsulates the log section of a HAR file,
// containing a slice of HTTP transaction entries.
type Log struct {
	// Entries is a list of HTTP transactions recorded in the HAR file.
	Entries []Entry `json:"entries"`
}

// Entry represents a single HTTP transaction as recorded in a HAR file.
// It includes both the HTTP request and response details.
type Entry struct {
	// Request contains the details of the HTTP request.
	Request Request `json:"request"`
	// Response contains the details of the HTTP response.
	Response Response `json:"response"`
}

// Request represents an HTTP request.
// It includes the method, URL, optional POST data, and headers.
type Request struct {
	// Method is the HTTP method (e.g. GET, POST).
	Method string `json:"method"`
	// URL is the target URL for the request.
	URL string `json:"url"`
	// PostData contains the payload of the request (if any).
	PostData *PostData `json:"postData,omitempty"`
	// Headers is a list of HTTP headers sent with the request.
	Headers []Header `json:"headers,omitempty"` // Optional: To handle headers if needed
	// Additional fields can be added as needed.
}

// PostData represents the payload data of an HTTP request.
// It includes the MIME type and either a text payload or parameters.
type PostData struct {
	// MimeType indicates the MIME type of the post data (e.g., application/json).
	MimeType string `json:"mimeType"`
	// Text contains the raw textual payload (if available).
	Text string `json:"text,omitempty"`
	// Params is a list of parameters included in the post data.
	Params []PostParam `json:"params,omitempty"`
	// Additional fields can be added as needed.
}

// PostParam represents an individual parameter within the POST data of a request.
type PostParam struct {
	// Name is the name of the parameter.
	Name string `json:"name"`
	// Value is the value of the parameter.
	Value string `json:"value"`
	// Additional fields can be added as needed.
}

// Header represents a single HTTP header.
// It consists of a header name and its corresponding value.
type Header struct {
	// Name is the name of the HTTP header.
	Name string `json:"name"`
	// Value is the value associated with the header.
	Value string `json:"value"`
}

// Response represents an HTTP response.
// It contains status information, the content payload, headers, and other metadata.
type Response struct {
	// Status is the HTTP status code (e.g., 200, 404).
	Status int `json:"status"`
	// StatusText provides a textual description of the status.
	StatusText string `json:"statusText"`
	// Content holds the body content of the response.
	Content Content `json:"content"`
	// RedirectURL is the URL to which the response is redirecting (if applicable).
	RedirectURL string `json:"redirectURL,omitempty"`
	// Headers is a list of HTTP headers included in the response.
	Headers []Header `json:"headers,omitempty"` // Optional: To handle headers if needed
	// Additional fields can be added as needed.
}

// Content represents the payload of an HTTP response.
// It includes details such as the MIME type and the textual content.
type Content struct {
	// MimeType indicates the MIME type of the response content.
	MimeType string `json:"mimeType"`
	// Text contains the actual textual content of the response (if available).
	Text string `json:"text,omitempty"`
	// Additional fields can be added as needed.
}
