package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// flags holds the parsed command-line flag values.
type flags struct {
	harFilePath  string
	varsFilePath string
	outputPath   string
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
	logInitialChainedValues(chainedValues)
	repopulateCallDetails(chainedValues)
	updateComplexPaths(chainedValues)
	assignVariableNames(chainedValues)

	// Optionally substitute pre-defined variables from a YAML file.
	if f.varsFilePath != "" {
		predefinedVars := loadYAMLVars(f.varsFilePath)
		substitutePredefinedVariables(callDetailsList, predefinedVars)
	}

	// Build the Postman collection and write it to a file.
	collection := BuildPostmanCollection(callDetailsList, chainedValues)
	if err := WriteCollectionToFile(collection, f.outputPath); err != nil {
		return fmt.Errorf("error writing Postman collection: %w", err)
	}

	fmt.Println("Postman collection generated successfully.")
	return nil
}

func updateComplexPaths(values []*ChainedValueContext) {
	// For each ChainedValueContext, we will:
	// 1. Parse the JSON from the original response.
	// 2. Prune the JSON so that it keeps only the relevant branch for this value
	//    (all parent nodes + up to 3 levels under the node).
	// 3. Call OpenAI with the URL, the original JSON path, and the pruned JSON.
	// 4. Store the refined/updated path in ValueSource.ReferencePath.

	for _, chainedVal := range values {
		// Only operate if we have a valid source from the response
		if chainedVal.ValueSource == nil || chainedVal.ValueSource.SourceType != SourceTypeResponse {
			continue
		}

		respEntry := chainedVal.ValueSource.Source.Entry
		if respEntry == nil {
			continue
		}

		rawJSON := respEntry.Response.Content.Text
		if rawJSON == "" {
			continue
		}

		// Parse the response JSON
		var fullResponse interface{}
		if err := json.Unmarshal([]byte(rawJSON), &fullResponse); err != nil {
			log.Printf("updateComplexPaths: error unmarshalling response JSON: %v", err)
			continue
		}

		// Prune the JSON to get only the partial structure
		prunedJSON, err := extractPartialJSON(fullResponse, chainedVal.ValueSource.ReferencePath, 4)
		if err != nil {
			log.Printf("updateComplexPaths: error extracting partial JSON: %v", err)
			continue
		}

		// Build the prompt input (the user message) for refining the JSON path
		input := []ComplexPathRequest{
			{
				URL:         respEntry.Request.URL,
				CurrentPath: chainedVal.ValueSource.ReferencePath,
				PartialJSON: prunedJSON,
			},
		}

		// Craft the final prompt for OpenAI
		userPrompt := buildComplexPathPrompt()

		// Call OpenAI to get a refined/robust JSON path
		// We expect a single string in return representing the updated path
		newPathList, err := CallOpenAIString(userPrompt, input)
		if err != nil {
			log.Printf("updateComplexPaths: error calling OpenAI for path: %v", err)
			continue
		}
		if len(newPathList) == 0 {
			log.Printf("updateComplexPaths: no new path returned from OpenAI")
			continue
		}

		// Update the reference path with the new stable/complex path
		chainedVal.ValueSource.ReferencePath = newPathList
		log.Printf("Updated path from %q to %q", input[0].CurrentPath, chainedVal.ValueSource.ReferencePath)
	}
}

// -- Additional helper code below --

// ComplexPathRequest holds the data we'll feed to OpenAI in JSON form.
type ComplexPathRequest struct {
	URL         string      `json:"url"`
	CurrentPath string      `json:"current_path"`
	PartialJSON interface{} `json:"partial_json"`
}

// buildComplexPathPrompt builds the user message for OpenAI asking it to produce
// either a simple JSON expression or a JSONPath for the value if the structure is complicated.
func buildComplexPathPrompt() string {
	return `
I have a JSON response from the Travelport JSON API. I want you to look at the JSON snippet and the
current JSON path I used to find a particular value. If the path is stable enough with simple array indexing
and object names, give me that path in return. If the JSON structure is more complex or an array index is not guaranteed,
please provide a JSONPath expression that is robust enough to find this value. As this will be used to replay the API
calls, please ensure the path is stable and reliable across different responses -- if there are accesses to arrays, IF
IT MAKES SENSE, you can rewrite the array index to 0 or some other smaller number. I.e. if the path is
foo[2].bar.baz.qux, you can simplify it to foo[0].bar.baz.qux -- again, if and only if it makes sense in the context.


The response is trimmed so you only see
the last few levels and immediate ancestors. Output should be a single string (JSON-encoded) representing the refined path
or JSONPath. Do not include any additional explanation.`
}

// extractPartialJSON prunes the given JSON so that only the node at `targetPath` is fully expanded
// with up to `levels` levels beneath it, and preserves the chain of parent objects up to the root.
// Everything else (unrelated branches or too-deep sibling branches) is removed/truncated.
//
// For simplicity, we use the existing ValueReference.ReferencePath. If your real code uses a different
// path notation than "dot notation + [N] for arrays", you will need to adjust the parsing logic.
func extractPartialJSON(root interface{}, targetPath string, levels int) (interface{}, error) {
	// 1. Split the path into tokens. E.g. "foo[2].bar" -> ["foo[2]", "bar"]
	pathTokens, err := splitPathTokens(targetPath)
	if err != nil {
		return nil, err
	}

	// 2. Recursively walk the JSON, building a pruned structure
	pruned, err := pruneJSON(root, pathTokens, 0, levels)
	if err != nil {
		return nil, err
	}

	return pruned, nil
}

// pruneJSON is a recursive function that descends the JSON tree according to the
// pathTokens. Once we reach the leaf (the final path token), we keep up to `levels`
// levels below it. For ancestor nodes, we remove unrelated siblings.
func pruneJSON(node interface{}, pathTokens []string, depth, levels int) (interface{}, error) {
	if len(pathTokens) == 0 {
		// We've reached the leaf. Keep up to `levels` levels below this node
		return keepLevels(node, levels)
	}

	switch val := node.(type) {
	case map[string]interface{}:
		// The next token should be a key in this map or a key with array indexing
		key, index, hasIndex, err := parseArrayKey(pathTokens[0])
		if err != nil {
			return nil, err
		}

		// We'll build a new map with only the relevant branch for `key`
		newMap := make(map[string]interface{})
		child, exists := val[key]
		if !exists {
			// Key doesn't exist -> can't proceed
			return newMap, nil
		}

		// If there's an array index, we attempt to navigate an array
		if hasIndex {
			arr, ok := child.([]interface{})
			if !ok {
				// It's not actually an array -> can't proceed further
				newMap[key] = child // or empty
			} else if index < 0 || index >= len(arr) {
				// Index out of bounds -> keep partial
				newMap[key] = nil
			} else {
				prunedChild, err := pruneJSON(arr[index], pathTokens[1:], depth+1, levels)
				if err != nil {
					return nil, err
				}
				// Reconstruct an array with just the one relevant index (if you prefer)
				newArr := []interface{}{prunedChild}
				newMap[key] = newArr
			}
		} else {
			// Normal object path
			prunedChild, err := pruneJSON(child, pathTokens[1:], depth+1, levels)
			if err != nil {
				return nil, err
			}
			newMap[key] = prunedChild
		}
		return newMap, nil

	case []interface{}:
		// If the pathToken is something like [N], parse that
		_, index, hasIndex, err := parseArrayKey(pathTokens[0])
		if err != nil {
			return nil, err
		}
		if !hasIndex {
			// Means we had something like "arrayName" but got an array -> mismatch
			return val, nil
		}
		if index < 0 || index >= len(val) {
			// Index out of range -> keep partial
			return nil, nil
		}
		prunedChild, err := pruneJSON(val[index], pathTokens[1:], depth+1, levels)
		if err != nil {
			return nil, err
		}
		newArr := make([]interface{}, len(val))
		for i := range val {
			if i == index {
				newArr[i] = prunedChild
			} else {
				newArr[i] = nil
			}
		}
		return newArr, nil

	default:
		// If we are still supposed to parse path tokens but the node is primitive, there's nothing to do
		return node, nil
	}
}

// keepLevels keeps up to `levels` levels below the current node. If `levels` is 0, it returns
// just a placeholder or nil. If `levels` is positive, recursively descends. This prevents
// massive or irrelevant subtrees from showing up in the pruned JSON.
func keepLevels(node interface{}, levels int) (interface{}, error) {
	if levels <= 0 {
		// return a placeholder instead of the real data
		return "...", nil
	}

	switch val := node.(type) {
	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for k, v := range val {
			pruned, _ := keepLevels(v, levels-1)
			newMap[k] = pruned
		}
		return newMap, nil
	case []interface{}:
		newArr := make([]interface{}, len(val))
		for i, v := range val {
			pruned, _ := keepLevels(v, levels-1)
			newArr[i] = pruned
		}
		return newArr, nil
	default:
		// primitive
		return val, nil
	}
}

// splitPathTokens is a simple helper that splits a path like "foo[2].bar" into
// tokens ["foo[2]", "bar"]. Adjust if your path format differs.
func splitPathTokens(pathStr string) ([]string, error) {
	// naive approach: split on '.'
	if pathStr == "" {
		return []string{}, nil
	}
	return strings.Split(pathStr, "."), nil
}

// parseArrayKey checks if a token is something like "foo[2]" or just "foo" or "[2]" (array at root).
// If it's array syntax, returns (key, index, true, nil). If it's an object key, returns (token, -1, false, nil).
func parseArrayKey(token string) (string, int, bool, error) {
	// If the token has an opening bracket, attempt to parse out "foo" and [2].
	// This is a quick heuristic; real code may need to handle multiple brackets, etc.
	start := strings.IndexRune(token, '[')
	if start == -1 {
		// no bracket -> just a key
		return token, -1, false, nil
	}
	end := strings.IndexRune(token, ']')
	if end == -1 || end < start {
		return "", -1, false, fmt.Errorf("mismatched brackets in token: %s", token)
	}

	keyPart := token[:start]
	idxPart := token[start+1 : end]
	if idxPart == "" {
		// means "foo[]" is invalid for indexing
		return keyPart, -1, false, fmt.Errorf("empty array index in token: %s", token)
	}

	var idx int
	_, err := fmt.Sscanf(idxPart, "%d", &idx)
	if err != nil {
		return keyPart, -1, false, fmt.Errorf("unable to parse index %q in token: %s", idxPart, token)
	}
	return keyPart, idx, true, nil
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
