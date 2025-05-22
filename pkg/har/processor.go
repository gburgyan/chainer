package har

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/gburgyan/chainer/pkg/util"
)

// Processor handles HAR file processing and value extraction.
type Processor struct {
	// Configuration options could be added here
}

// NewProcessor creates a new HAR processor.
func NewProcessor() *Processor {
	return &Processor{}
}

// ReadHAR reads a HAR file from the specified path.
// It unmarshals the JSON content into a HAR struct and returns any errors encountered.
func (p *Processor) ReadHAR(harFilePath string) (*HAR, error) {
	// Read the HAR file
	harData, err := os.ReadFile(harFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading HAR file: %w", err)
	}

	// Unmarshal the HAR data
	var har HAR
	if err := json.Unmarshal(harData, &har); err != nil {
		return nil, fmt.Errorf("error parsing HAR file: %w", err)
	}
	return &har, nil
}

// Process extracts call details from a HAR file.
func (p *Processor) Process(harFilePath string) ([]*util.CallDetails, error) {
	har, err := p.ReadHAR(harFilePath)
	if err != nil {
		return nil, err
	}
	return p.ProcessHAR(*har), nil
}

// ProcessHAR iterates over each entry in the HAR log.
// It extracts and processes request and response details, including URLs, headers, and bodies.
// It collects ValueReference instances for both requests and responses and assembles a list of CallDetails.
func (p *Processor) ProcessHAR(har HAR) []*util.CallDetails {
	// Slice to keep track of all CallDetails
	var callDetailsList []*util.CallDetails

	// Process each entry
	for i := range har.Log.Entries {
		entry := &har.Log.Entries[i]
		log.Printf("Processing entry: %s", entry.Request.URL)

		callDetails := &util.CallDetails{
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
		reqDetails, err := p.processBody(reqBody, requestMimeType)
		if err != nil {
			log.Printf("Error processing request body: %v", err)
			// Continue processing even if there's an error in the request body
		}
		reqHeaderDetails := p.processHeaders(entry.Request.Headers)
		reqDetails = append(reqDetails, reqHeaderDetails...)
		for j := range reqDetails {
			reqDetails[j].Source = callDetails
			reqDetails[j].SourceType = util.SourceTypeRequest
		}
		callDetails.RequestDetails = reqDetails

		// Process Response Body
		respBody := entry.Response.Content.Text
		respDetails, err := p.processBody(respBody, entry.Response.Content.MimeType)
		if err != nil {
			log.Printf("Error processing response body: %v", err)
			// Continue processing even if there's an error in the response body
		}
		respHeaderDetails := p.processHeaders(entry.Response.Headers)
		respDetails = append(respDetails, respHeaderDetails...)
		for j := range respDetails {
			respDetails[j].Source = callDetails
			respDetails[j].SourceType = util.SourceTypeResponse
		}
		callDetails.ResponseDetails = respDetails

		// Extract URL strings
		urlValues, err := p.extractURLStrings(entry.Request.URL)
		if err != nil {
			log.Printf("Error extracting URL strings: %v", err)
			// Continue processing even if there's an error in URL parsing
		}

		for j := range urlValues {
			urlValues[j].Source = callDetails
			urlValues[j].SourceType = util.SourceTypeRequest
			urlValues[j].SourceLocation = util.SourceLocationUrl
		}
		callDetails.RequestDetails = append(callDetails.RequestDetails, urlValues...)

		// Append the CallDetails to the list
		callDetailsList = append(callDetailsList, callDetails)
	}
	return callDetailsList
}

// FlattenJSON takes a JSON string and flattens it into a slice of ValueReference pointers.
// It first unmarshals the JSON into an interface{} and then recursively extracts all leaf nodes,
// tracking the full "path" to each value.
func (p *Processor) FlattenJSON(data string) ([]*util.ValueReference, error) {
	var jsonData interface{}
	if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
		return nil, err
	}
	// Start with an empty slice for ancestors.
	return p.flatten("", nil, jsonData), nil
}

// flatten recursively walks through a JSON structure, extracting leaf nodes as ValueReference instances.
// It keeps track of the current path (prefix) and ancestors to provide full context for each value.
func (p *Processor) flatten(prefix string, ancestors []interface{}, data interface{}) []*util.ValueReference {
	var valueRefs []*util.ValueReference

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
			valueRefs = append(valueRefs, p.flatten(fullKey, newAncestors, value)...)
		}
	case []interface{}:
		// Append a copy of the current slice to the ancestors.
		newAncestors := append(append([]interface{}{}, ancestors...), v)
		for i, value := range v {
			fullKey := fmt.Sprintf("%s[%d]", prefix, i)
			// Recurse with the updated context.
			valueRefs = append(valueRefs, p.flatten(fullKey, newAncestors, value)...)
		}
	default:
		// Base case: a leaf node. Create a ValueReference that includes the context.
		valueRefs = append(valueRefs, &util.ValueReference{
			Value:          v,
			ReferencePath:  prefix,
			UrlLocation:    0, // Placeholder: set as needed.
			Ancestors:      ancestors,
			SourceLocation: util.SourceLocationBodyJson,
		})
	}

	return valueRefs
}

// ExtractURLStrings parses a raw URL string to extract components such as host, path segments,
// and query parameter values. Each component is converted into a ValueReference with an appropriate reference path.
func (p *Processor) extractURLStrings(rawURL string) ([]*util.ValueReference, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	var valueRefs []*util.ValueReference
	valueRef := util.ValueReference{
		Value:          parsedURL.Host,
		ReferencePath:  fmt.Sprintf("host"),
		UrlLocation:    0,
		SourceLocation: util.SourceLocationUrl,
	}
	valueRefs = append(valueRefs, &valueRef)

	// Extract path segments
	cleanPath := path.Clean(parsedURL.Path)
	segments := strings.Split(cleanPath, "/")
	for i, segment := range segments {
		if segment != "" {
			valueRef := util.ValueReference{
				Value:          segment,
				ReferencePath:  fmt.Sprintf("path[%d]", i),
				UrlLocation:    i,
				SourceLocation: util.SourceLocationUrl,
			}
			valueRefs = append(valueRefs, &valueRef)
		}
	}

	// Extract query parameter values
	queryIndex := len(segments) // Offset for query parameters
	for key, values := range parsedURL.Query() {
		for j, value := range values {
			valueRef := util.ValueReference{
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
func (p *Processor) processBody(body string, contentType string) ([]*util.ValueReference, error) {
	// Check if the content type is JSON
	if contentType == "application/json" {
		// Check if body is empty
		if strings.TrimSpace(body) == "" {
			return nil, nil
		}

		// Flatten the JSON body
		flatRefs, err := p.FlattenJSON(body)
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

		var valueRefs []*util.ValueReference
		for key, values := range formValues {
			for i, value := range values {
				valueRef := util.ValueReference{
					Value:          value,
					ReferencePath:  fmt.Sprintf("%s[%d]", key, i),
					SourceLocation: util.SourceLocationBodyForm,
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
func (p *Processor) processHeaders(headers []Header) []*util.ValueReference {
	// Define a blacklist of headers to ignore
	blacklist := map[string]struct{}{
		"content-length": {},
		"host":           {},
		"connection":     {},
		"cache-control":  {},
		"postman-token":  {},
	}

	var headerRefs []*util.ValueReference
	for _, header := range headers {
		// Check if the header is in the blacklist
		if _, found := blacklist[strings.ToLower(header.Name)]; found {
			continue
		}

		headerRef := util.ValueReference{
			Value:          header.Value,
			HeaderName:     header.Name,
			SourceLocation: util.SourceLocationHeader,
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
