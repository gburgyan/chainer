package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

// flags holds the parsed command-line flag values.
type flags struct {
	harFilePath  string
	varsFilePath string
	outputPath   string
}

type varsInput struct {
	Name              string `json:"name"`
	SearchValue       string `json:"search_value"`
	InitializerPrompt string `json:"initializer,omitempty"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	// Parse command-line flags.
	f, err := parseFlags()
	if err != nil {
		return err
	}

	// Read and process the HAR file.
	har, err := readHar(f.harFilePath)
	if err != nil {
		return fmt.Errorf("error reading HAR file: %w", err)
	}
	callDetailsList := processHar(har)

	// Identify and process chained values.
	chainedValues := findChainedValues(callDetailsList)
	// Optionally substitute pre-defined variables from a YAML file.
	if f.varsFilePath != "" {
		predefinedVars := loadJSONVars(f.varsFilePath)
		chainedValues = extractPredefinedVars(callDetailsList, predefinedVars, chainedValues)
	}

	logInitialChainedValues(chainedValues)
	repopulateCallDetails(chainedValues)
	updateComplexPaths(chainedValues)
	assignVariableNames(chainedValues)

	// Build the Postman collection and write it to a file.
	collection := BuildPostmanCollection(callDetailsList, chainedValues)
	if err := WriteCollectionToFile(collection, f.outputPath); err != nil {
		return fmt.Errorf("error writing Postman collection: %w", err)
	}

	fmt.Println("Postman collection generated successfully.")
	return nil
}

// parseFlags extracts and validates command-line flags.
func parseFlags() (flags, error) {
	harFilePath := flag.String("file", "", "Path to the HAR file")
	varsFilePath := flag.String("vars", "", "Path to the YAML file with pre-defined variables")
	outputPath := flag.String("output", "collection.json", "Output path for the generated Postman collection")

	flag.Parse()

	if *harFilePath == "" {
		usage := "Usage: goharparser -file=<path_to_har_file> [-vars=<path_to_yaml_file>]"
		fmt.Println(usage)
		return flags{}, errors.New("missing HAR file path")
	}

	return flags{
		harFilePath:  *harFilePath,
		varsFilePath: *varsFilePath,
		outputPath:   *outputPath,
	}, nil
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

// extractPredefinedVars walks through all value references in the provided call details and,
// if the value exactly matches a pre-defined variable value from the YAML file, replaces it with a variable reference.
// For example, if the YAML file defines: username: admin, then a literal "admin" will be replaced with "{{username}}".
func extractPredefinedVars(callDetailsList []*CallDetails, vars []varsInput, values []*ChainedValueContext) []*ChainedValueContext {
	// Make a copy of the values to avoid modifying the original slice
	resultValues := make([]*ChainedValueContext, len(values))
	copy(resultValues, values)

	for _, cd := range callDetailsList {
		for _, vr := range cd.RequestDetails {
			cv := substituteValue(vr, vars)
			if cv != nil {
				resultValues = append(resultValues, cv)
			}
		}
	}

	return resultValues
}

// substituteValue checks if the value in the ValueReference is a string and,
// if it exactly matches one of the pre-defined values, replaces it with a variable placeholder.
func substituteValue(vr *ValueReference, vars []varsInput) *ChainedValueContext {
	str, ok := vr.Value.(string)
	if !ok {
		return nil
	}
	for _, vi := range vars {
		if str == vi.SearchValue {
			manualChainedValue := &ChainedValueContext{
				Value:          vi.SearchValue,
				AllUsages:      []*ValueReference{vr},
				ExternalSource: true,
				VariableName:   vi.Name,
				InitScript:     vi.InitializerPrompt,
			}
			return manualChainedValue
		}
	}
	return nil
}

func loadJSONVars(filePath string) []varsInput {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading JSON file: %v", err)
	}
	var vars []varsInput
	if err := json.Unmarshal(data, &vars); err != nil {
		log.Fatalf("Error parsing JSON file: %v", err)
	}
	return vars
}
