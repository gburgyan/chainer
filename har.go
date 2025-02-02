package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"strings"
)

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

// FlattenJSON takes a JSON string and flattens it into a slice of ValueReference pointers.
// It first unmarshals the JSON into an interface{} and then recursively extracts all leaf nodes,
// tracking the full "path" to each value.
func FlattenJSON(data string) ([]*ValueReference, error) {
	var jsonData interface{}
	if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
		return nil, err
	}
	// Start with an empty slice for ancestors.
	return flatten("", nil, jsonData), nil
}

// flatten recursively walks through a JSON structure, extracting leaf nodes as ValueReference instances.
// It keeps track of the current path (prefix) and ancestors to provide full context for each value.
func flatten(prefix string, ancestors []interface{}, data interface{}) []*ValueReference {
	var valueRefs []*ValueReference

	switch v := data.(type) {
	case map[string]interface{}:
		// Append a copy of the current map to the ancestors.
		newAncestors := append(append([]interface{}{}, ancestors...), v)
		for key, value := range v {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			// Recurse with the updated context.
			valueRefs = append(valueRefs, flatten(fullKey, newAncestors, value)...)
		}
	case []interface{}:
		// Append a copy of the current slice to the ancestors.
		newAncestors := append(append([]interface{}{}, ancestors...), v)
		for i, value := range v {
			fullKey := fmt.Sprintf("%s[%d]", prefix, i)
			// Recurse with the updated context.
			valueRefs = append(valueRefs, flatten(fullKey, newAncestors, value)...)
		}
	default:
		// Base case: a leaf node. Create a ValueReference that includes the context.
		valueRefs = append(valueRefs, &ValueReference{
			Value:          v,
			ReferencePath:  prefix,
			UrlLocation:    0, // Placeholder: set as needed.
			Ancestors:      ancestors,
			SourceLocation: SourceLocationBodyJson,
		})
	}

	return valueRefs
}

// ExtractURLStrings parses a raw URL string to extract components such as host, path segments,
// and query parameter values. Each component is converted into a ValueReference with an appropriate reference path.
func ExtractURLStrings(rawURL string) ([]*ValueReference, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	var valueRefs []*ValueReference
	valueRef := ValueReference{
		Value:          parsedURL.Host,
		ReferencePath:  fmt.Sprintf("host"),
		UrlLocation:    0,
		SourceLocation: SourceLocationUrl,
	}
	valueRefs = append(valueRefs, &valueRef)

	// Extract path segments
	cleanPath := path.Clean(parsedURL.Path)
	segments := strings.Split(cleanPath, "/")
	for i, segment := range segments {
		if segment != "" {
			valueRef := ValueReference{
				Value:          segment,
				ReferencePath:  fmt.Sprintf("path[%d]", i),
				UrlLocation:    i,
				SourceLocation: SourceLocationUrl,
			}
			valueRefs = append(valueRefs, &valueRef)
		}
	}

	// Extract query parameter values
	queryIndex := len(segments) // Offset for query parameters
	for key, values := range parsedURL.Query() {
		for j, value := range values {
			valueRef := ValueReference{
				Value:         value,
				ReferencePath: fmt.Sprintf("query.%s[%d]", key, j),
				UrlLocation:   queryIndex,
			}
			valueRefs = append(valueRefs, &valueRef)
			queryIndex++
		}
	}

	return valueRefs, nil
}

// processBody processes the body of an HTTP request or response.
// It assumes the body is in JSON format (or form data) and flattens it into ValueReference instances.
func processBody(body string, contentType string) ([]*ValueReference, error) {
	// Check if the content type is JSON
	if contentType == "application/json" {
		// Check if body is empty
		if strings.TrimSpace(body) == "" {
			return nil, nil
		}

		// Flatten the JSON body
		flatRefs, err := FlattenJSON(body)
		if err != nil {
			return nil, err
		}

		return flatRefs, nil
	} else if contentType == "application/x-www-form-urlencoded" {
		// Handle form data
		formValues, err := url.ParseQuery(body)
		if err != nil {
			return nil, err
		}

		var valueRefs []*ValueReference
		for key, values := range formValues {
			for i, value := range values {
				valueRef := ValueReference{
					Value:          value,
					ReferencePath:  fmt.Sprintf("%s[%d]", key, i),
					SourceLocation: SourceLocationBodyForm,
				}
				valueRefs = append(valueRefs, &valueRef)
			}
		}
		return valueRefs, nil
	}
	return nil, nil
}

// processHeaders processes HTTP headers and converts them into ValueReference instances.
// It filters out blacklisted headers and handles special cases such as stripping tokens from authorization headers.
func processHeaders(headers []Header) []*ValueReference {
	// Define a blacklist of headers to ignore
	blacklist := map[string]struct{}{
		"content-length": {},
		"host":           {},
		"connection":     {},
		"cache-control":  {},
		"postman-token":  {},
	}

	var headerRefs []*ValueReference
	for _, header := range headers {
		// Check if the header is in the blacklist
		if _, found := blacklist[strings.ToLower(header.Name)]; found {
			continue
		}

		headerRef := ValueReference{
			Value:          header.Value,
			HeaderName:     header.Name,
			SourceLocation: SourceLocationHeader,
			ReferencePath:  header.Name,
		}
		// Remove the "Bearer" token from the header value if it's an authorization header
		if strings.ToLower(header.Name) == "authorization" {
			headerRef.Value = strings.TrimPrefix(header.Value, "Bearer ")
		}
		headerRefs = append(headerRefs, &headerRef)
	}
	return headerRefs
}

// processHar iterates over each entry in the HAR log.
// It extracts and processes request and response details, including URLs, headers, and bodies.
// It collects ValueReference instances for both requests and responses and assembles a list of CallDetails.
func processHar(har HAR) []*CallDetails {
	// Slice to keep track of all CallDetails
	var callDetailsList []*CallDetails

	// Process each entry
	for i := range har.Log.Entries {
		entry := &har.Log.Entries[i]
		log.Printf("Processing entry: %s", entry.Request.URL)

		callDetails := CallDetails{
			Entry: entry,
		}

		// Process Request Body
		reqBody := ""
		if entry.Request.PostData != nil && entry.Request.PostData.Text != "" {
			reqBody = entry.Request.PostData.Text
		}
		requestMimeType := ""
		if entry.Request.PostData != nil {
			requestMimeType = entry.Request.PostData.MimeType
		}
		reqDetails, err := processBody(reqBody, requestMimeType)
		if err != nil {
			log.Printf("Error processing request body: %v", err)
			// Continue processing even if there's an error in the request body
		}
		reqHeaderDetails := processHeaders(entry.Request.Headers)
		reqDetails = append(reqDetails, reqHeaderDetails...)
		for j := range reqDetails {
			reqDetails[j].Source = &callDetails
			reqDetails[j].SourceType = SourceTypeRequest
		}
		callDetails.RequestDetails = reqDetails

		// Process Response Body
		respBody := entry.Response.Content.Text
		respDetails, err := processBody(respBody, entry.Response.Content.MimeType)
		if err != nil {
			log.Printf("Error processing response body: %v", err)
			// Continue processing even if there's an error in the response body
		}
		respHeaderDetails := processHeaders(entry.Response.Headers)
		respDetails = append(respDetails, respHeaderDetails...)
		for j := range respDetails {
			respDetails[j].Source = &callDetails
			respDetails[j].SourceType = SourceTypeResponse
		}
		callDetails.ResponseDetails = respDetails

		// Extract URL strings
		urlValues, err := ExtractURLStrings(entry.Request.URL)
		if err != nil {
			log.Printf("Error extracting URL strings: %v", err)
			// Continue processing even if there's an error in URL parsing
		}

		for j := range urlValues {
			urlValues[j].Source = &callDetails
			urlValues[j].SourceType = SourceTypeRequest
			urlValues[j].SourceLocation = SourceLocationUrl
		}
		callDetails.RequestDetails = append(callDetails.RequestDetails, urlValues...)

		// Append the CallDetails to the list
		callDetailsList = append(callDetailsList, &callDetails)
	}
	return callDetailsList
}

// readHar reads the HAR file from the specified path.
// It unmarshals the JSON content into a HAR struct and returns any errors encountered.
func readHar(harFilePath string) (HAR, error) {
	// Read the HAR file
	harData, err := os.ReadFile(harFilePath)
	if err != nil {
		log.Fatalf("Error reading HAR file: %v", err)
	}

	// Unmarshal the HAR data
	var har HAR
	if err := json.Unmarshal(harData, &har); err != nil {
		log.Fatalf("Error parsing HAR file: %v", err)
	}
	return har, err
}
