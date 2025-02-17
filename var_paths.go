package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

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
or JSONPath.

If a JSONPath is not required or applicable, you can return the original path as is.

Do not include any additional explanation or markup, simply the javascript expression.`
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
