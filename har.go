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

// HAR represents the root of the HAR file.
type HAR struct {
	Log Log `json:"log"`
}

// Log contains the entries.
type Log struct {
	Entries []Entry `json:"entries"`
}

// Entry represents a single HTTP transaction.
type Entry struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// Request represents the HTTP request.
type Request struct {
	Method   string    `json:"method"`
	URL      string    `json:"url"`
	PostData *PostData `json:"postData,omitempty"`
	Headers  []Header  `json:"headers,omitempty"` // Optional: To handle headers if needed
	// Other fields can be added as needed
}

// PostData represents the post data of the request.
type PostData struct {
	MimeType string      `json:"mimeType"`
	Text     string      `json:"text,omitempty"`
	Params   []PostParam `json:"params,omitempty"`
	// Other fields can be added as needed
}

// PostParam represents parameters in post data.
type PostParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Other fields can be added as needed
}

// Header represents a single HTTP header.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Response represents the HTTP response.
type Response struct {
	Status      int      `json:"status"`
	StatusText  string   `json:"statusText"`
	Content     Content  `json:"content"`
	RedirectURL string   `json:"redirectURL,omitempty"`
	Headers     []Header `json:"headers,omitempty"` // Optional: To handle headers if needed
	// Other fields can be added as needed
}

// Content represents the content of the response.
type Content struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	// Other fields can be added as needed
}

// FlattenJSON takes a JSON string and returns a slice of ValueReference with JavaScript-like paths.
func FlattenJSON(data string) ([]*ValueReference, error) {
	var jsonData interface{}
	if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
		return nil, err
	}
	var valueRefs []*ValueReference
	flatten("", jsonData, &valueRefs)
	return valueRefs, nil
}

// flatten is a helper function that recursively flattens JSON data into ValueReference slices.
func flatten(prefix string, data interface{}, valueRefs *[]*ValueReference) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			flatten(fullKey, value, valueRefs)
		}
	case []interface{}:
		for i, value := range v {
			fullKey := fmt.Sprintf("%s[%d]", prefix, i)
			flatten(fullKey, value, valueRefs)
		}
	default:
		// Create a ValueReference
		valueRef := ValueReference{
			Value:               v,
			JavascriptReference: prefix,
			UrlLocation:         0, // Placeholder: Set as needed
		}
		*valueRefs = append(*valueRefs, &valueRef)
	}
}

// ExtractURLStrings parses the URL and extracts path segments and query parameter values.
func ExtractURLStrings(rawURL string) ([]*ValueReference, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	var valueRefs []*ValueReference

	// Extract path segments
	cleanPath := path.Clean(parsedURL.Path)
	segments := strings.Split(cleanPath, "/")
	for i, segment := range segments {
		if segment != "" {
			valueRef := ValueReference{
				Value:               segment,
				JavascriptReference: fmt.Sprintf("path[%d]", i),
				UrlLocation:         i,
			}
			valueRefs = append(valueRefs, &valueRef)
		}
	}

	// Extract query parameter values
	queryIndex := len(segments) // Offset for query parameters
	for key, values := range parsedURL.Query() {
		for j, value := range values {
			valueRef := ValueReference{
				Value:               value,
				JavascriptReference: fmt.Sprintf("query.%s[%d]", key, j),
				UrlLocation:         queryIndex,
			}
			valueRefs = append(valueRefs, &valueRef)
			queryIndex++
		}
	}

	return valueRefs, nil
}

// processBody processes the body string, assuming it's JSON, and returns a slice of ValueReference.
func processBody(body string) ([]*ValueReference, error) {
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
}

// processHeaders processes headers and returns a slice of ValueReference.
// This function is optional and can be used if you want to include headers in CallDetails.
func processHeaders(headers []Header) []*ValueReference {
	var headerRefs []*ValueReference
	for _, header := range headers {
		headerRef := ValueReference{
			Value:      header.Value,
			HeaderName: header.Name,
		}
		// Remove the "bearer" token from the header value if it's an authorization header
		if strings.ToLower(header.Name) == "authorization" {
			headerRef.Value = strings.TrimPrefix(header.Value, "Bearer ")
		}
		headerRefs = append(headerRefs, &headerRef)
	}
	return headerRefs
}

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
		reqDetails, err := processBody(reqBody)
		if err != nil {
			log.Printf("Error processing request body: %v", err)
			// Continue processing even if there's an error in the request body
		}
		reqHeaderDetails := processHeaders(entry.Request.Headers)
		reqDetails = append(reqDetails, reqHeaderDetails...)
		for i := range reqDetails {
			reqDetails[i].Source = &callDetails
			reqDetails[i].SourceType = SourceTypeRequest
		}
		callDetails.RequestDetails = reqDetails

		// Optionally, process request headers
		// reqHeaderRefs := processHeaders(entry.Request.Headers)
		// callDetails.RequestDetails = append(callDetails.RequestDetails, reqHeaderRefs...)

		// Process Response Body
		respBody := entry.Response.Content.Text
		respDetails, err := processBody(respBody)
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
		}
		callDetails.RequestDetails = append(callDetails.RequestDetails, urlValues...)

		// Append the CallDetails to the list
		callDetailsList = append(callDetailsList, &callDetails)
	}
	return callDetailsList
}

func readHar(harFilePath string) (error, HAR) {
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
	return err, har
}
