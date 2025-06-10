package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/util"
)

// ComplexPathProcessor handles the refinement of JSON paths to make them more stable and reliable
type ComplexPathProcessor struct {
	Client OpenAIClientInterface
}

// NewComplexPathProcessor creates a new processor for refining complex JSON paths
func NewComplexPathProcessor(client OpenAIClientInterface) *ComplexPathProcessor {
	return &ComplexPathProcessor{
		Client: client,
	}
}

// ComplexPathRequest holds the data we'll feed to AI in JSON form.
type ComplexPathRequest struct {
	URL           string                 `json:"url"`
	CurrentPath   string                 `json:"current_path"`
	UsagePaths    []string               `json:"usage_paths"`
	UsageContexts []string               `json:"usage_contexts,omitempty"`
	PartialJSON   interface{}            `json:"partial_json"`
	Value         string                 `json:"value,omitempty"`
	ValueType     string                 `json:"value_type,omitempty"`
	ParentContext map[string]interface{} `json:"parent_context,omitempty"`
}

// UpdateComplexPaths refines JSON paths to make them more stable and reliable
func (p *ComplexPathProcessor) UpdateComplexPaths(values []*util.ChainedValueContext) {
	for _, chainedVal := range values {
		// Only operate if we have a valid source from the response
		if chainedVal.ValueSource == nil || chainedVal.ValueSource.SourceType != util.SourceTypeResponse {
			continue
		}

		// Skip if there's no entry
		if chainedVal.ValueSource.Source == nil || chainedVal.ValueSource.Source.Entry == nil {
			continue
		}

		// Get the HAR entry
		harEntry, ok := chainedVal.ValueSource.Source.Entry.(*har.Entry)
		if !ok {
			log.Printf("UpdateComplexPaths: entry is not a HAR entry")
			continue
		}

		rawJSON := harEntry.Response.Content.Text
		if rawJSON == "" {
			continue
		}

		// Extract enhanced context using smart pruning
		partialJSON, parentContext, err := extractSmartPartialJSON(
			rawJSON,
			chainedVal.ValueSource.ReferencePath,
			chainedVal.Value,
		)
		if err != nil {
			log.Printf("UpdateComplexPaths: error extracting partial JSON: %v", err)
			continue
		}

		// Build the enhanced request
		input := ComplexPathRequest{
			URL:           harEntry.Request.URL,
			CurrentPath:   chainedVal.ValueSource.ReferencePath,
			PartialJSON:   partialJSON,
			Value:         chainedVal.Value,
			ValueType:     detectValueType(chainedVal.Value),
			ParentContext: parentContext,
		}

		// Collect usage information with context
		for _, usage := range chainedVal.AllUsages {
			if usage.SourceType == util.SourceTypeRequest {
				input.UsagePaths = append(input.UsagePaths, usage.ReferencePath)
				// Add context about how it's used
				context := getUsageContext(usage)
				input.UsageContexts = append(input.UsageContexts, context)
			}
		}

		// Log enhanced context for debugging
		// Check if verbose logging is enabled
		verbose := false
		switch client := p.Client.(type) {
		case *OpenAIClient:
			verbose = client.Config.Verbose
		case *AnthropicClient:
			verbose = client.Config.Verbose
		case *TemplatedClient:
			// Check the underlying client
			if oc, ok := client.OpenAIClient.(*OpenAIClient); ok {
				verbose = oc.Config.Verbose
			} else if ac, ok := client.OpenAIClient.(*AnthropicClient); ok {
				verbose = ac.Config.Verbose
			}
		}

		if verbose {
			log.Printf("Enhanced context for path %q:\n- Value Type: %s\n- Usage Contexts: %v\n- Parent Keys: %v",
				chainedVal.ValueSource.ReferencePath,
				input.ValueType,
				input.UsageContexts,
				getMapKeys(parentContext))
		}

		// Call AI to get a refined/robust JSON path
		var newPath string

		// Check if we have a templated client
		if templatedClient, ok := p.Client.(TemplatedClientInterface); ok {
			var pathErr error
			newPath, pathErr = templatedClient.CallWithTemplate("complex_path_v2", nil, input)
			if pathErr != nil {
				log.Printf("UpdateComplexPaths: error calling AI for path: %v", pathErr)
				continue
			}
		} else {
			// Log deprecation warning
			log.Printf("WARNING: Using legacy client without template support. This is deprecated and will be removed in a future version.")
			log.Printf("UpdateComplexPaths: templated client is required - legacy prompts have been removed")
			continue
		}

		if newPath == "" {
			log.Printf("UpdateComplexPaths: no new path returned from AI")
			continue
		}

		// Validate the generated path
		if err := validateGeneratedPath(newPath); err != nil {
			log.Printf("UpdateComplexPaths: invalid path generated: %v", err)
			continue
		}

		// Update the reference path with the new stable/complex path
		chainedVal.ValueSource.ReferencePath = newPath
		log.Printf("Updated path from %q to %q", input.CurrentPath, newPath)
	}
}

// extractSmartPartialJSON extracts a structure-aware partial JSON
func extractSmartPartialJSON(source string, targetPath string, targetValue string) (interface{}, map[string]interface{}, error) {
	// Parse the source JSON
	var root interface{}
	if err := json.Unmarshal([]byte(source), &root); err != nil {
		return nil, nil, err
	}

	// Deep copy of the root JSON
	copiedBytes, err := json.Marshal(root)
	if err != nil {
		return nil, nil, err
	}
	var rootCopy interface{}
	if err := json.Unmarshal(copiedBytes, &rootCopy); err != nil {
		return nil, nil, err
	}

	// Replace the value at the target path with the semaphore
	tokens, err := splitPathTokens(targetPath)
	if err != nil {
		return nil, nil, err
	}

	// Navigate to parent to get context
	parentContext := navigateToParent(rootCopy, tokens)

	// Replace with marker
	if err := replaceAtPath(rootCopy, tokens, ">>NODE-TO-GET<<"); err != nil {
		return nil, nil, err
	}

	// Smart pruning: preserve structure while reducing size
	pruned := smartPruneJSON(rootCopy, tokens)

	return pruned, parentContext, nil
}

// navigateToParent navigates to the parent of the target and returns its context
func navigateToParent(data interface{}, tokens []string) map[string]interface{} {
	if len(tokens) <= 1 {
		return nil
	}

	current := data
	for i := 0; i < len(tokens)-1; i++ {
		token := tokens[i]
		key, idx, isArray, _ := parseArrayKey(token)

		if isArray {
			if key != "" {
				if m, ok := current.(map[string]interface{}); ok {
					if arr, ok := m[key].([]interface{}); ok && idx < len(arr) {
						current = arr[idx]
					}
				}
			} else {
				if arr, ok := current.([]interface{}); ok && idx < len(arr) {
					current = arr[idx]
				}
			}
		} else {
			if m, ok := current.(map[string]interface{}); ok {
				current = m[token]
			}
		}
	}

	// Return the parent object if it's a map
	if m, ok := current.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// smartPruneJSON intelligently prunes JSON while preserving structure
func smartPruneJSON(data interface{}, targetPath []string) interface{} {
	// Sophisticated pruning that:
	// 1. Keeps the full path to the target
	// 2. Preserves array structure (first, last, and elements around target)
	// 3. Keeps sibling fields at each level
	// 4. Truncates deep nested structures not on the path

	// Clone the data structure
	copiedBytes, err := json.Marshal(data)
	if err != nil {
		return data
	}
	var pruned interface{}
	if err := json.Unmarshal(copiedBytes, &pruned); err != nil {
		return data
	}

	// Prune the structure while preserving the path
	pruneRecursively(pruned, targetPath, 0, true)

	return pruned
}

// pruneRecursively performs the actual pruning recursively
func pruneRecursively(data interface{}, targetPath []string, currentDepth int, onPath bool) {
	if currentDepth >= len(targetPath) {
		// We've reached or passed the target depth
		return
	}

	token := targetPath[currentDepth]
	key, idx, isArray, _ := parseArrayKey(token)

	switch v := data.(type) {
	case map[string]interface{}:
		// For objects, keep siblings at each level on the path
		if onPath {
			// Process the target key
			if isArray && key != "" {
				if arr, ok := v[key].([]interface{}); ok {
					pruneArray(arr, targetPath, currentDepth, idx)
					v[key] = arr
				}
			} else if !isArray {
				if child, ok := v[token]; ok {
					pruneRecursively(child, targetPath, currentDepth+1, true)
				}
			}

			// Keep sibling fields but prune their contents if not on path
			for k, val := range v {
				if k != token && k != key {
					pruneDeepStructure(val, 2) // Keep 2 levels of structure for siblings
				}
			}
		} else {
			// Not on path - prune deeply
			pruneDeepStructure(v, 1)
		}

	case []interface{}:
		if onPath && currentDepth < len(targetPath) {
			_, targetIdx, targetIsArray, _ := parseArrayKey(targetPath[currentDepth])
			if targetIsArray && targetIdx >= 0 && targetIdx < len(v) {
				// Keep the target element and prune others
				pruneArray(v, targetPath, currentDepth, targetIdx)
			}
		} else {
			// Not on path - keep first and last elements only
			if len(v) > 3 {
				kept := []interface{}{v[0]}
				if len(v) > 1 {
					kept = append(kept, "... "+fmt.Sprintf("%d items omitted", len(v)-2)+" ...")
					kept = append(kept, v[len(v)-1])
				}
				// Clear the array and add kept elements
				for i := range v {
					if i < len(kept) {
						v[i] = kept[i]
					} else {
						v[i] = nil
					}
				}
				// Resize
				v = v[:len(kept)]
			}
		}
	}
}

// pruneArray intelligently prunes array elements
func pruneArray(arr []interface{}, targetPath []string, currentDepth int, targetIdx int) {
	if targetIdx < 0 || targetIdx >= len(arr) {
		return
	}

	// Always keep first and last elements
	keepIndices := map[int]bool{
		0:            true,
		len(arr) - 1: true,
		targetIdx:    true,
	}

	// Keep elements around the target (context window of 1)
	if targetIdx > 0 {
		keepIndices[targetIdx-1] = true
	}
	if targetIdx < len(arr)-1 {
		keepIndices[targetIdx+1] = true
	}

	// Process each element
	for i, elem := range arr {
		if keepIndices[i] {
			if i == targetIdx {
				// Recurse on the target element
				pruneRecursively(elem, targetPath, currentDepth+1, true)
			} else {
				// Prune non-target elements more aggressively
				pruneDeepStructure(elem, 1)
			}
		} else {
			// Replace with placeholder
			arr[i] = fmt.Sprintf("... item %d ...", i)
		}
	}
}

// pruneDeepStructure truncates deep nested structures
func pruneDeepStructure(data interface{}, maxDepth int) {
	if maxDepth <= 0 {
		return
	}

	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			switch child := val.(type) {
			case map[string]interface{}, []interface{}:
				if maxDepth > 1 {
					pruneDeepStructure(child, maxDepth-1)
				} else {
					// Replace deep structures with placeholders
					if _, ok := child.(map[string]interface{}); ok {
						v[key] = "{...}"
					} else {
						v[key] = "[...]"
					}
				}
			}
			// Keep primitive values as-is
		}

	case []interface{}:
		for i, elem := range v {
			switch child := elem.(type) {
			case map[string]interface{}, []interface{}:
				if maxDepth > 1 {
					pruneDeepStructure(child, maxDepth-1)
				} else {
					// Replace deep structures with placeholders
					if _, ok := child.(map[string]interface{}); ok {
						v[i] = "{...}"
					} else {
						v[i] = "[...]"
					}
				}
			}
			// Keep primitive values as-is
		}
	}
}

// detectValueType analyzes a value and returns its type
func detectValueType(value string) string {
	// Empty or very short values
	if value == "" {
		return "empty"
	}

	// Try to parse as various types
	var f float64
	if err := json.Unmarshal([]byte(value), &f); err == nil {
		return "number"
	}

	var b bool
	if err := json.Unmarshal([]byte(value), &b); err == nil {
		return "boolean"
	}

	// Pattern-based detection
	value = strings.TrimSpace(value)

	// UUID pattern
	if len(value) == 36 && strings.Count(value, "-") == 4 {
		return "uuid"
	}

	// ISO timestamp
	if strings.Contains(value, "T") && (strings.Contains(value, "Z") || strings.Contains(value, "+")) {
		return "timestamp"
	}

	// Email
	if strings.Contains(value, "@") && strings.Contains(value, ".") {
		return "email"
	}

	// URL
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return "url"
	}

	// Status-like values
	lowerValue := strings.ToLower(value)
	statusWords := []string{"active", "inactive", "pending", "completed", "failed", "success", "error"}
	for _, status := range statusWords {
		if lowerValue == status {
			return "status"
		}
	}

	// ID-like values
	if strings.HasSuffix(value, "Id") || strings.HasSuffix(value, "ID") ||
		strings.HasPrefix(value, "id_") || strings.HasPrefix(value, "ID_") {
		return "id"
	}

	// Token/Key
	if len(value) > 20 && !strings.Contains(value, " ") {
		return "token"
	}

	return "string"
}

// getUsageContext analyzes how a value is used in requests
func getUsageContext(usage *util.ValueReference) string {
	switch usage.SourceLocation {
	case util.SourceLocationHeader:
		return fmt.Sprintf("header:%s", usage.HeaderName)
	case util.SourceLocationUrl:
		return "url"
	case util.SourceLocationBodyJson:
		return fmt.Sprintf("body:%s", usage.ReferencePath)
	case util.SourceLocationBodyForm:
		return "form"
	default:
		return "unknown"
	}
}

// validateGeneratedPath performs basic validation on the generated path
func validateGeneratedPath(path string) error {
	if path == "" {
		return errors.New("empty path")
	}

	// Must start with responseJson or jsonpath.query
	if !strings.HasPrefix(path, "responseJson.") && !strings.HasPrefix(path, "jsonpath.query(") {
		return fmt.Errorf("path must start with 'responseJson.' or 'jsonpath.query('")
	}

	// Basic syntax validation for JSONPath
	if strings.HasPrefix(path, "jsonpath.query(") {
		if !strings.Contains(path, ")") {
			return errors.New("JSONPath query missing closing parenthesis")
		}
		if !strings.Contains(path, "'") && !strings.Contains(path, "\"") {
			return errors.New("JSONPath query missing path string")
		}
	}

	// Check for the marker in the path (should not be there)
	if strings.Contains(path, "NODE-TO-GET") {
		return errors.New("path contains the marker placeholder")
	}

	return nil
}

// getMapKeys returns the keys of a map as a slice
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Existing helper functions remain the same
func extractPartialJSON(source string, targetPath string, linesToKeep int) (string, error) {
	// ... existing implementation ...
	// This is kept for backward compatibility
	var root interface{}
	if err := json.Unmarshal([]byte(source), &root); err != nil {
		return "", err
	}

	copiedBytes, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	var rootCopy interface{}
	if err := json.Unmarshal(copiedBytes, &rootCopy); err != nil {
		return "", err
	}

	semaphore := ">>NODE-TO-GET<<"
	tokens, err := splitPathTokens(targetPath)
	if err != nil {
		return "", err
	}
	if err := replaceAtPath(rootCopy, tokens, semaphore); err != nil {
		return "", err
	}

	semaphoreJSON, err := json.Marshal(semaphore)
	if err != nil {
		return "", err
	}

	finalJSONBytes, err := json.MarshalIndent(rootCopy, "", "  ")
	if err != nil {
		return "", err
	}
	finalJSONStr := string(finalJSONBytes)

	lines := strings.Split(finalJSONStr, "\n")

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

func splitPathTokens(pathStr string) ([]string, error) {
	if pathStr == "" {
		return []string{}, nil
	}
	return strings.Split(pathStr, "."), nil
}

func parseArrayKey(token string) (string, int, bool, error) {
	start := strings.IndexRune(token, '[')
	if start == -1 {
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
		if key != "" {
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
