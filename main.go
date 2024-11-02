package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"strings"
)

type SourceType int

const (
	SourceTypeRequest SourceType = iota
	SourceTypeResponse
)

// ValueReference represents a value along with its JavaScript-like path and header location.
type ValueReference struct {
	Value               any    `json:"value"`
	JavascriptReference string `json:"javascript_reference"`
	UrlLocation         int    `json:"url_location"`
	HeaderName          string `json:"header_name"`

	Source     *CallDetails `json:"source"`
	SourceType SourceType   `json:"source_type"`
	Context    *ChainedValueContext
}

func (r *ValueReference) IsInteresting() bool {
	if r.Value == nil {
		return false
	}

	if strings.Contains(r.JavascriptReference, "@type") {
		return false
	}

	if r.HeaderName == "Content-Type" {
		return false
	}

	// If it's a string, make sure it's at least 5 characters long
	// If it's an int, make sure it's greater than 1000
	// If it's a float, make sure it's greater than 1000.0

	switch v := r.Value.(type) {
	case string:
		return len(v) >= 4
	case int:
		return v > 1000
	case float64:
		return v > 1000.0
	}
	return false
}

// CallDetails holds the processed details for a single HAR entry, separating requests and responses.
type CallDetails struct {
	RequestDetails  []*ValueReference `json:"request_details"`
	ResponseDetails []*ValueReference `json:"response_details"`

	Entry *Entry `json:"entry"`

	RequestChainedValues  []*ValueReference `json:"request_chained_values"`
	ResponseChainedValues []*ValueReference `json:"response_chained_values"`
}

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

// printCallDetails is a helper function to print CallDetails with indentation.
func printCallDetails(call CallDetails, index int) {
	fmt.Printf("Entry %d:\n", index+1)

	// Print Request Details
	if len(call.RequestDetails) > 0 {
		fmt.Println("  Request Details:")
		for _, rd := range call.RequestDetails {
			if rd.IsInteresting() {
				fmt.Printf("    - Value: %s\n", rd.Value)
				fmt.Printf("      JS Path: %s\n", rd.JavascriptReference)
				fmt.Printf("      Header Location: %d\n", rd.UrlLocation)
			}
		}
	}

	// Print Response Details
	if len(call.ResponseDetails) > 0 {
		fmt.Println("  Response Details:")
		for _, rd := range call.ResponseDetails {
			if rd.IsInteresting() {
				fmt.Printf("    - Value: %s\n", rd.Value)
				fmt.Printf("      JS Path: %s\n", rd.JavascriptReference)
				fmt.Printf("      Header Location: %d\n", rd.UrlLocation)
			}
		}
	}

	fmt.Println()
}

func main() {
	// Define and parse command-line flags
	harFilePath := flag.String("file", "", "Path to the HAR file")
	flag.Parse()

	if *harFilePath == "" {
		fmt.Println("Usage: goharparser -file=<path_to_har_file>")
		os.Exit(1)
	}

	// Read the HAR file
	harData, err := os.ReadFile(*harFilePath)
	if err != nil {
		log.Fatalf("Error reading HAR file: %v", err)
	}

	// Unmarshal the HAR data
	var har HAR
	if err := json.Unmarshal(harData, &har); err != nil {
		log.Fatalf("Error parsing HAR file: %v", err)
	}

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

	chainedValues := FindChainedValues(callDetailsList)
	for i, chainedValue := range chainedValues {
		fmt.Printf("Chained Value %d:\n", i+1)
		fmt.Printf("  Value: %s\n", chainedValue.Value)
		fmt.Println("  Context:")
		for _, ref := range chainedValue.Context {
			var requestOrResponse string
			if ref.SourceType == SourceTypeRequest {
				requestOrResponse = "Request"
			} else {
				requestOrResponse = "Response"
			}
			fmt.Printf("    - %s - %s\n", requestOrResponse, ref.JavascriptReference)
		}
		fmt.Println()
	}

	repopulateCallDetails(chainedValues)

	// Build the Postman collection
	collection := BuildPostmanCollection(callDetailsList, chainedValues)

	// Output to a file
	err = WriteCollectionToFile(collection, "collection.json")
	if err != nil {
		log.Fatalf("Error writing Postman collection: %v", err)
	}

	fmt.Println("Postman collection generated successfully.")

}

type ChainedValueContext struct {
	Value        string
	Context      []*ValueReference
	VariableName string
}

// FindChainedValues filters CallDetails to find interesting values that appear in multiple requests and responses.
func FindChainedValues(callDetailsList []*CallDetails) []*ChainedValueContext {
	// Map to keep track of values and their occurrences
	valueOccurrences := make(map[string][]*ValueReference)

	// Iterate over each CallDetails
	for _, callDetails := range callDetailsList {
		// Process RequestDetails
		for _, reqDetail := range callDetails.RequestDetails {
			if reqDetail.IsInteresting() {
				valueStr := fmt.Sprintf("%v", reqDetail.Value)
				valueOccurrences[valueStr] = append(valueOccurrences[valueStr], reqDetail)
			}
		}

		// Process ResponseDetails
		for _, respDetail := range callDetails.ResponseDetails {
			if respDetail.IsInteresting() {
				valueStr := fmt.Sprintf("%v", respDetail.Value)
				valueOccurrences[valueStr] = append(valueOccurrences[valueStr], respDetail)
			}
		}
	}

	// Filter to keep only values that appear in multiple requests and responses
	var chainedValues []*ChainedValueContext
	for value, refs := range valueOccurrences {
		if len(refs) > 1 {
			chainedValues = append(chainedValues, &ChainedValueContext{
				Value:   value,
				Context: refs,
			})
		}
	}

	// Now exclude values that appear in requests before responses. Also exclude values that appear in the same request.
	// Additionally, exclude values that appear in the same response. For responses, only include the first appearance.
	var filteredChainedValues []*ChainedValueContext

NextChainedValue:
	for _, chainedValue := range chainedValues {
		//var seenRequest bool
		var seenResponse bool
		includeVal := false
		for _, contextItem := range chainedValue.Context {
			if contextItem.SourceType == SourceTypeRequest {
				if !seenResponse {
					continue NextChainedValue
				}
				//seenRequest = true
				includeVal = true
			} else {
				seenResponse = true
			}
		}
		if includeVal {
			filteredChainedValues = append(filteredChainedValues, chainedValue)
		}
	}

	return filteredChainedValues
}

func repopulateCallDetails(chainedValues []*ChainedValueContext) {
	for _, chainedValue := range chainedValues {
		for _, contextItem := range chainedValue.Context {
			if contextItem.Source != nil {
				if contextItem.SourceType == SourceTypeRequest {
					contextItem.Context = chainedValue
					contextItem.Source.RequestChainedValues = append(contextItem.Source.RequestChainedValues, contextItem)
				} else {
					contextItem.Context = chainedValue
					contextItem.Source.ResponseChainedValues = append(contextItem.Source.ResponseChainedValues, contextItem)
				}
			} else {
				log.Printf("Source is nil for value: %s, type %v", chainedValue.Value, contextItem.SourceType)
			}
		}
	}
}
