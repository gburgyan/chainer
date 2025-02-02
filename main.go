package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// SourceType represents the origin of a value extracted from an HTTP transaction.
// It indicates whether the value was found in the request or in the response.
type SourceType int

const (
	// SourceTypeRequest indicates that the value comes from the HTTP request.
	SourceTypeRequest SourceType = iota

	// SourceTypeResponse indicates that the value comes from the HTTP response.
	SourceTypeResponse
)

// SourceLocation represents the specific location within an HTTP transaction
// where a value was discovered. This may include headers, the JSON body,
// URL segments, or form data.
type SourceLocation int

const (
	// SourceLocationHeader indicates that the value was found in an HTTP header.
	SourceLocationHeader SourceLocation = iota

	// SourceLocationBodyJson indicates that the value was extracted from a JSON body.
	SourceLocationBodyJson

	// SourceLocationBodyForm indicates that the value was extracted from form data in the body.
	SourceLocationBodyForm

	// SourceLocationUrl indicates that the value was extracted from the URL (host, path, or query).
	SourceLocationUrl
)

// CallDetails aggregates information for a single HTTP call.
// It holds details extracted from both the request and response, along with any
// chained values that have been identified for potential variable substitution.
// Additionally, it maintains a reference to the original HAR entry.
type CallDetails struct {
	// RequestDetails contains all the value references extracted from the request.
	RequestDetails []*ValueReference `json:"request_details"`

	// ResponseDetails contains all the value references extracted from the response.
	ResponseDetails []*ValueReference `json:"response_details"`

	// Entry holds the raw HAR entry for this call.
	// TODO: An Entry should be more generic and not tied to HAR.
	Entry *Entry `json:"entry"`

	// RequestChainedValues contains value references in the request that have been
	// identified as being part of a variable chaining scenario.
	RequestChainedValues []*ValueReference `json:"request_chained_values"`

	// ResponseChainedValues contains value references in the response that have been
	// identified as being part of a variable chaining scenario.
	ResponseChainedValues []*ValueReference `json:"response_chained_values"`
}

// ValueReference encapsulates an extracted value along with metadata describing
// its location and context within an HTTP transaction. This includes its reference
// path (similar to a JavaScript property path), its source (e.g. request or response),
// and the precise location (e.g. header, body, URL). It also optionally stores contextual
// and ancestral information for chained value substitution.
type ValueReference struct {
	// Value is the actual extracted value. It can be of any type.
	Value any `json:"value"`

	// ReferencePath provides a JavaScript-like path to locate the value within the source.
	ReferencePath string `json:"javascript_reference"`

	// UrlLocation indicates the location within the URL where the value was found.
	UrlLocation int `json:"url_location"`

	// HeaderName is the name of the header where the value was extracted.
	// TODO: Remove this field
	HeaderName string `json:"header_name"`

	// Source points to the CallDetails from which this value was extracted.
	Source *CallDetails `json:"source"`

	// SourceType indicates whether the value came from the request or response.
	SourceType SourceType `json:"source_type"`

	// SourceLocation specifies the exact location within the HTTP transaction (e.g., header, JSON body).
	SourceLocation SourceLocation `json:"source_location"`

	// Context points to the ChainedValueContext if this value is part of a chained substitution.
	Context *ChainedValueContext

	// Ancestors contains a list of parent objects leading to this value (used when flattening JSON).
	Ancestors []interface{}
}

// ChainedValueContext holds information about a value that is shared across
// multiple HTTP calls (i.e. chained). It stores the actual value as well as all
// the usage contexts, the original source of the value, and an optional variable name
// that can be used for substitution in a Postman collection.
type ChainedValueContext struct {
	// Value is the string representation of the chained value.
	Value string

	// AllUsages contains all ValueReference instances where this value is used.
	AllUsages []*ValueReference

	// ValueSource points to the original ValueReference that is considered the source of this chained value.
	ValueSource *ValueReference

	// VariableName is an optional name assigned to the value for substitution purposes.
	VariableName string
}

// IsInteresting determines whether a ValueReference is significant for chaining.
// It filters out values that are nil, too short (for strings), or below a threshold (for numbers).
// It also excludes specific headers and JSON properties that are not useful for variable substitution.
func (r *ValueReference) IsInteresting() bool {
	if r.Value == nil {
		return false
	}

	if strings.Contains(r.ReferencePath, "@type") {
		return false
	}

	if r.HeaderName == "Content-Type" {
		return false
	}

	// If it's a string, make sure it's at least 2 characters long
	// If it's an int, make sure it's greater than 1000
	// If it's a float, make sure it's greater than 1000.0

	switch v := r.Value.(type) {
	case string:
		return len(v) >= 2
	case int:
		return v > 1000
	case float64:
		return v > 1000.0
	}
	return false
}

// main is the entry point of the program.
// It parses command-line flags, reads and processes the HAR file, identifies chained values,
// assigns variable names, substitutes pre-defined variables (if provided), builds the Postman collection,
// and writes it to a file.
func main() {
	// Define and parse command-line flags
	harFilePath := flag.String("file", "", "Path to the HAR file")
	varsFilePath := flag.String("vars", "", "Path to the YAML file with pre-defined variables")
	flag.Parse()

	if *harFilePath == "" {
		fmt.Println("Usage: goharparser -file=<path_to_har_file> [-vars=<path_to_yaml_file>]")
		os.Exit(1)
	}

	har, err := readHar(*harFilePath)
	if err != nil {
		log.Fatalf("Error reading HAR file: %v", err)
	}

	callDetailsList := processHar(har)

	chainedValues := findChainedValues(callDetailsList)
	logInitialChainedValues(chainedValues)

	repopulateCallDetails(chainedValues)
	assignVariableNames(chainedValues)

	// If a YAML file with pre-defined variables is provided, load it and substitute values.
	if *varsFilePath != "" {
		predefinedVars := loadYAMLVars(*varsFilePath)
		substitutePredefinedVariables(callDetailsList, predefinedVars)
	}

	// Build the Postman collection
	collection := BuildPostmanCollection(callDetailsList, chainedValues)

	// Output to a file
	err = WriteCollectionToFile(collection, "collection.json")
	if err != nil {
		log.Fatalf("Error writing Postman collection: %v", err)
	}

	fmt.Println("Postman collection generated successfully.")
}

// logInitialChainedValues logs the initial set of chained values for debugging purposes.
// It prints each value along with its usage context in requests and responses.
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
			fmt.Printf("    - %s - %s\n", requestOrResponse, ref.ReferencePath)
		}
		fmt.Println()
	}
}

// findChainedValues analyzes the call details to identify values that appear in multiple requests and responses.
// It filters out values that are not considered "interesting" and returns a slice of ChainedValueContext.
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
		var seenResponse bool
		includeVal := false
		for _, contextItem := range chainedValue.AllUsages {
			if contextItem.SourceType == SourceTypeRequest {
				if !seenResponse {
					continue NextChainedValue
				}
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

// repopulateCallDetails updates each CallDetails instance by linking it to the associated chained values.
// It assigns the Context field of ValueReference to point to the corresponding ChainedValueContext.
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

// substitutePredefinedVariables walks through all value references in the provided call details and,
// if the value exactly matches a pre-defined variable value from the YAML file, replaces it with a variable reference.
// For example, if the YAML file defines: username: admin, then a literal "admin" will be replaced with "{{username}}".
func substitutePredefinedVariables(callDetailsList []*CallDetails, vars map[string]string) {
	for _, cd := range callDetailsList {
		for _, vr := range cd.RequestDetails {
			substituteValue(vr, vars)
		}
		for _, vr := range cd.ResponseDetails {
			substituteValue(vr, vars)
		}
	}
}

// substituteValue checks if the value in the ValueReference is a string and,
// if it exactly matches one of the pre-defined values, replaces it with a variable placeholder.
func substituteValue(vr *ValueReference, vars map[string]string) {
	str, ok := vr.Value.(string)
	if !ok {
		return
	}
	for varName, varVal := range vars {
		if str == varVal {
			vr.Value = fmt.Sprintf("{{%s}}", varName)
			// Once a match is found, no need to check the other variables.
			break
		}
	}
}

// loadYAMLVars reads the YAML file at the given path and unmarshals it into a map[string]string.
// The YAML file should contain key-value pairs like:
//
//	username: admin
//	password: secret
func loadYAMLVars(filePath string) map[string]string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}
	var vars map[string]string
	if err := yaml.Unmarshal(data, &vars); err != nil {
		log.Fatalf("Error parsing YAML file: %v", err)
	}
	return vars
}
