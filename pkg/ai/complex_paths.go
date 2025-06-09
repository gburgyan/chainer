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
			// Try enhanced template first, fall back to basic
			var pathErr error
			newPath, pathErr = templatedClient.CallWithTemplate("complex_path_v2", nil, input)
			if pathErr != nil {
				// Fall back to basic template
				newPath, pathErr = templatedClient.CallWithTemplate("complex_path", nil, input)
				if pathErr != nil {
					log.Printf("UpdateComplexPaths: error calling AI for path: %v", pathErr)
					continue
				}
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
	// For now, we'll use the line-based approach but with better structure preservation
	// In a production version, this would implement sophisticated pruning that:
	// 1. Keeps the full path to the target
	// 2. Preserves array structure (first, last, and elements around target)
	// 3. Keeps sibling fields at each level
	// 4. Truncates deep nested structures not on the path

	// Convert to pretty JSON and extract context
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return data
	}

	jsonStr := string(jsonBytes)
	lines := strings.Split(jsonStr, "\n")

	// Find the marker
	markerLine := -1
	for i, line := range lines {
		if strings.Contains(line, ">>NODE-TO-GET<<") || strings.Contains(line, "\\u003e\\u003eNODE-TO-GET\\u003c\\u003c") {
			markerLine = i
			break
		}
	}

	if markerLine == -1 {
		return data
	}

	// Extract with better context - more lines and structure-aware
	contextLines := 30 // More context
	startLine := markerLine - contextLines
	if startLine < 0 {
		startLine = 0
	}
	endLine := markerLine + contextLines + 1
	if endLine > len(lines) {
		endLine = len(lines)
	}

	// Ensure we have complete JSON structure
	// Find the nearest opening brace/bracket before start
	for startLine > 0 && !strings.Contains(lines[startLine], "{") && !strings.Contains(lines[startLine], "[") {
		startLine--
	}

	// Find the nearest closing brace/bracket after end
	for endLine < len(lines)-1 && !strings.Contains(lines[endLine-1], "}") && !strings.Contains(lines[endLine-1], "]") {
		endLine++
	}

	partialLines := lines[startLine:endLine]
	partialJSON := strings.Join(partialLines, "\n")

	// Try to parse it back to ensure it's valid JSON
	var result interface{}
	if err := json.Unmarshal([]byte(partialJSON), &result); err != nil {
		// If it's not valid, return the original pruned version
		return data
	}

	return result
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
