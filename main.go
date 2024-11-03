package main

import (
	"flag"
	"fmt"
	"log"
	"os"
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

type ChainedValueContext struct {
	Value        string
	AllUsages    []*ValueReference
	ValueSource  *ValueReference
	VariableName string
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

	// If it's a string, make sure it's at least 4 characters long
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

func main() {
	// Define and parse command-line flags
	harFilePath := flag.String("file", "", "Path to the HAR file")
	flag.Parse()

	if *harFilePath == "" {
		fmt.Println("Usage: goharparser -file=<path_to_har_file>")
		os.Exit(1)
	}

	err, har := readHar(*harFilePath)

	callDetailsList := processHar(har)

	chainedValues := findChainedValues(callDetailsList)
	logInitialChainedValues(chainedValues)

	repopulateCallDetails(chainedValues)

	assignVariableNames(chainedValues)

	// Build the Postman collection
	collection := BuildPostmanCollection(callDetailsList, chainedValues)

	// Output to a file
	err = WriteCollectionToFile(collection, "collection.json")
	if err != nil {
		log.Fatalf("Error writing Postman collection: %v", err)
	}

	fmt.Println("Postman collection generated successfully.")
}

func logInitialChainedValues(chainedValues []*ChainedValueContext) {
	for i, chainedValue := range chainedValues {
		fmt.Printf("Chained Value %d:\n", i+1)
		fmt.Printf("  Value: %s\n", chainedValue.Value)
		fmt.Println("  Context:")
		for _, ref := range chainedValue.AllUsages {
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
}

// findChainedValues filters CallDetails to find interesting values that appear in multiple requests and responses.
func findChainedValues(callDetailsList []*CallDetails) []*ChainedValueContext {
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
				Value:     value,
				AllUsages: refs,
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
		for _, contextItem := range chainedValue.AllUsages {
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
		for _, contextItem := range chainedValue.AllUsages {
			if contextItem.Source != nil {
				if contextItem.SourceType == SourceTypeRequest {
					contextItem.Context = chainedValue
					contextItem.Source.RequestChainedValues = append(contextItem.Source.RequestChainedValues, contextItem)
				} else {
					contextItem.Context = chainedValue
					contextItem.Source.ResponseChainedValues = append(contextItem.Source.ResponseChainedValues, contextItem)
					if chainedValue.ValueSource == nil {
						chainedValue.ValueSource = contextItem
					}
				}
			} else {
				log.Printf("Source is nil for value: %s, type %v", chainedValue.Value, contextItem.SourceType)
			}
		}
	}
}
