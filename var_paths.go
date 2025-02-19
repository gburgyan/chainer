package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
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

		// Prune the JSON to get only the partial structure
		prunedJSON, err := extractPartialJSON(rawJSON, chainedVal.ValueSource.ReferencePath, 30)
		if err != nil {
			log.Printf("updateComplexPaths: error extracting partial JSON: %v", err)
			continue
		}

		// Print the pruned JSON for debugging
		log.Printf("Pruned JSON for path %q: %s", chainedVal.ValueSource.ReferencePath, prunedJSON)

		// Build the prompt input (the user message) for refining the JSON path
		input := []ComplexPathRequest{
			{
				URL:         respEntry.Request.URL,
				CurrentPath: chainedVal.ValueSource.ReferencePath,
				PartialJSON: prunedJSON,
				Value:       chainedVal.Value,
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
	Value       string      `json:"value"`
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

The exact path of the value is provided in the "current_path" field, and the partial JSON snippet is in the "partial_json" field.

The partial JSON snippet is a small piece of the JSON response that contains the value. This is to help you understand the context
of the value and the structure of the JSON. You can use this to determine the best way to access the value.

The value that we're interested in is in the "value" field. This is the actual value that we want to access in the JSON.

Use context clues from both the current path and the partial JSON to determine the best way to access the value.

If a JSONPath is not required or applicable, you can return the original path as is.

Do not include any additional explanation or markup, simply the javascript expression.`
}

// extractPartialJSON extracts a snippet of the JSON source surrounding the target path.
// It replaces the value at targetPath with a unique semaphore (GUID), then returns
// the specified number of context lines (linesToKeep) before and after the line containing it.
func extractPartialJSON(source string, targetPath string, linesToKeep int) (string, error) {
	// 0) Parse the source JSON.
	var root interface{}
	if err := json.Unmarshal([]byte(source), &root); err != nil {
		return "", err
	}

	// 1) Deep copy of the root JSON (via marshal/unmarshal).
	copiedBytes, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	var rootCopy interface{}
	if err := json.Unmarshal(copiedBytes, &rootCopy); err != nil {
		return "", err
	}

	// 2) Make semaphore value: a new GUID.
	semaphore := uuid.New().String()

	// 3) Replace the value at the target path with the semaphore value.
	tokens, err := splitPathTokens(targetPath)
	if err != nil {
		return "", err
	}
	if err := replaceAtPath(rootCopy, tokens, semaphore); err != nil {
		return "", err
	}

	// 4) Serialize the replaced value (the semaphore) to a JSON string.
	semaphoreJSON, err := json.Marshal(semaphore)
	if err != nil {
		return "", err
	}

	// 6) Serialize the root JSON to a string (with pretty printing).
	finalJSONBytes, err := json.MarshalIndent(rootCopy, "", "  ")
	if err != nil {
		return "", err
	}
	finalJSONStr := string(finalJSONBytes)

	// 7) Split the root JSON string by newline.
	lines := strings.Split(finalJSONStr, "\n")

	// 5) Find the line number of the semaphore value.
	targetLine := -1
	semaphoreStr := string(semaphoreJSON)
	for i, line := range lines {
		if strings.Contains(line, semaphoreStr) {
			targetLine = i
			break
		}
	}
	if targetLine == -1 {
		return "", errors.New("semaphore not found in output JSON")
	}

	// 8) Return the linesToKeep lines before and after the line that contains the semaphore.
	startLine := targetLine - linesToKeep
	if startLine < 0 {
		startLine = 0
	}
	endLine := targetLine + linesToKeep + 1
	if endLine > len(lines) {
		endLine = len(lines)
	}
	partialLines := lines[startLine:endLine]
	partialJSON := strings.Join(partialLines, "\n")
	return partialJSON, nil
}

// splitPathTokens is a helper that splits a path like "foo[2].bar" into tokens.
// Adjust the delimiter if your path format differs.
func splitPathTokens(pathStr string) ([]string, error) {
	if pathStr == "" {
		return []string{}, nil
	}
	return strings.Split(pathStr, "."), nil
}

// parseArrayKey checks if a token is something like "foo[2]", "foo", or "[2]".
// If array syntax is detected, it returns (key, index, true, nil). Otherwise, it returns (token, -1, false, nil).
func parseArrayKey(token string) (string, int, bool, error) {
	start := strings.IndexRune(token, '[')
	if start == -1 {
		// No bracket → treat it as a plain key.
		return token, -1, false, nil
	}
	end := strings.IndexRune(token, ']')
	if end == -1 || end < start {
		return "", -1, false, fmt.Errorf("mismatched brackets in token: %s", token)
	}

	keyPart := token[:start]
	idxPart := token[start+1 : end]
	if idxPart == "" {
		return keyPart, -1, false, fmt.Errorf("empty array index in token: %s", token)
	}

	var idx int
	_, err := fmt.Sscanf(idxPart, "%d", &idx)
	if err != nil {
		return keyPart, -1, false, fmt.Errorf("unable to parse index %q in token: %s", idxPart, token)
	}
	return keyPart, idx, true, nil
}

// replaceAtPath recursively traverses the JSON object to replace the value at the specified path with the semaphore.
func replaceAtPath(data interface{}, tokens []string, semaphore string) error {
	if len(tokens) == 0 {
		return errors.New("empty path tokens")
	}
	token := tokens[0]
	key, idx, isArray, err := parseArrayKey(token)
	if err != nil {
		return err
	}
	if isArray {
		// Handle tokens with array syntax.
		if key != "" {
			// Case like "foo[2]": data must be a JSON object with key "foo" holding an array.
			m, ok := data.(map[string]interface{})
			if !ok {
				return fmt.Errorf("expected JSON object for key %s", key)
			}
			arr, ok := m[key].([]interface{})
			if !ok {
				return fmt.Errorf("expected array at key %s", key)
			}
			if idx < 0 || idx >= len(arr) {
				return fmt.Errorf("index %d out of range for array at key %s", idx, key)
			}
			if len(tokens) == 1 {
				arr[idx] = semaphore
				return nil
			}
			return replaceAtPath(arr[idx], tokens[1:], semaphore)
		} else {
			// Case like "[2]": data itself should be an array.
			arr, ok := data.([]interface{})
			if !ok {
				return fmt.Errorf("expected JSON array")
			}
			if idx < 0 || idx >= len(arr) {
				return fmt.Errorf("index %d out of range in array", idx)
			}
			if len(tokens) == 1 {
				arr[idx] = semaphore
				return nil
			}
			return replaceAtPath(arr[idx], tokens[1:], semaphore)
		}
	} else {
		// Token is a simple key.
		m, ok := data.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected JSON object to have key %s", token)
		}
		if len(tokens) == 1 {
			m[token] = semaphore
			return nil
		}
		child, exists := m[token]
		if !exists {
			return fmt.Errorf("key %s not found", token)
		}
		return replaceAtPath(child, tokens[1:], semaphore)
	}
}
