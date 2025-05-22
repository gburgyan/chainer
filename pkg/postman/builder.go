package postman

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/util"
)

// Builder handles the creation of Postman collections.
type Builder struct {
	// Configuration options could be added here
}

// NewBuilder creates a new Postman collection builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// BuildCollection creates a Postman collection from the given call details and chained values.
func (b *Builder) BuildCollection(callDetailsList []*util.CallDetails, chainedValues []*util.ChainedValueContext) PostmanCollection {
	var items []PostmanItem

	initScript := b.CreateInitScript(chainedValues)

	for i, callDetails := range callDetailsList {
		if callDetails == nil {
			continue
		}
		postmanRequest := b.ReplaceChainedValuesInRequest(callDetails)

		// Check if this request's response has values to extract
		var events []PostmanEvent
		script := b.CreateTestScript(callDetails.ResponseChainedValues)
		events = append(events, script)

		if i == 0 && initScript != nil {
			events = append(events, *initScript)
		}

		item := PostmanItem{
			Name:    callDetails.Name,
			Request: postmanRequest,
			Event:   events,
		}
		items = append(items, item)
	}

	variables := make([]PostmanVariable, len(chainedValues))
	for i, chainedValue := range chainedValues {
		var description string
		if chainedValue.ValueSource != nil {
			description = chainedValue.ValueSource.ReferencePath
		} else {
			description = "Manually set variable"
		}

		variables[i] = PostmanVariable{
			Key:         chainedValue.VariableName,
			Description: description,
		}
	}

	collection := PostmanCollection{
		Info: CollectionInfo{
			Name:    "Generated Collection",
			Schema:  "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
			Version: "2.1.0",
		},
		Item:      items,
		Variables: variables,
	}

	return collection
}

// ReplaceChainedValuesInRequest constructs a PostmanRequest for inclusion in the Postman collection.
// It replaces occurrences of chained values in the request URL, headers, and body with Postman variable placeholders.
func (b *Builder) ReplaceChainedValuesInRequest(request *util.CallDetails) PostmanRequest {
	// Replace chained values in the request URL
	entry := request.Entry.(*har.Entry)
	requestUrl := b.BuildPostmanURL(request)

	// Replace chained values in the request headers
	var headers []PostmanHeader
	for _, header := range entry.Request.Headers {
		if b.shouldSkipHeader(header) {
			continue
		}
		headers = append(headers, PostmanHeader{
			Key:   header.Name,
			Value: b.ReplaceValuesInString(header.Value, request.RequestChainedValues),
		})
	}

	// Replace chained values in the request body
	var body PostmanRequestBody
	if entry.Request.PostData != nil {
		body = PostmanRequestBody{
			Mode: "raw",
			Raw:  b.ReplaceValuesInString(entry.Request.PostData.Text, request.RequestChainedValues),
		}
	}

	postmanRequest := PostmanRequest{
		Method: entry.Request.Method,
		Header: headers,
		Body:   &body,
		URL:    requestUrl,
	}

	return postmanRequest
}

// shouldSkipHeader determines if a header should be excluded from the Postman request.
func (b *Builder) shouldSkipHeader(header har.Header) bool {
	// Skip headers that are automatically set by Postman
	if strings.HasPrefix(header.Name, "Postman-") {
		return true
	}

	// Skip Content-Length header
	if header.Name == "Content-Length" {
		return true
	}

	return false
}

// ReplaceValuesInString replaces all occurrences of specified values in an input string with corresponding Postman variable placeholders.
// It iterates over the provided ValueReference instances to perform the substitutions.
func (b *Builder) ReplaceValuesInString(input string, valueToVariableName []*util.ValueReference) string {
	for _, v := range valueToVariableName {
		valueString := fmt.Sprintf("%v", v.Value)
		if v.Context == nil {
			// log and continue
			log.Printf("Value %v has no context", valueString)
			continue
		}
		input = strings.ReplaceAll(input, valueString, "{{"+v.Context.VariableName+"}}")
	}
	return input
}

// BuildPostmanURL builds a PostmanURL struct for a Postman request.
// It parses the original request URL and replaces path segments and query parameters that match chained values with variable placeholders.
func (b *Builder) BuildPostmanURL(callDetails *util.CallDetails) PostmanURL {
	entry := callDetails.Entry.(*har.Entry)
	rawUrl := entry.Request.URL

	parsedURL, _ := url.Parse(rawUrl)
	// Build the PostmanURL struct with parsed URL components

	postmanURL := PostmanURL{
		Raw:      b.ReplaceValuesInString(rawUrl, callDetails.RequestChainedValues),
		Protocol: parsedURL.Scheme,
		Host:     []string{b.ReplaceValuesInString(parsedURL.Host, callDetails.RequestChainedValues)},
	}

	// Split the path into individual components
	pathComponents := strings.Split(parsedURL.Path, "/")
	var path []string
	for _, component := range pathComponents {
		if component != "" {
			// Replace path parameters with Postman variables
			path = append(path, b.ReplaceValuesInString(component, callDetails.RequestChainedValues))
		}
	}
	postmanURL.Path = path

	// Split the query string into individual components
	queryComponents := parsedURL.Query()
	var query []PostmanQueryParam
	for key, values := range queryComponents {
		for _, value := range values {
			query = append(query, PostmanQueryParam{
				Key:   key,
				Value: b.ReplaceValuesInString(value, callDetails.RequestChainedValues),
			})
		}
	}
	postmanURL.Query = query

	return postmanURL
}

// CreateTestScript generates a Postman test script event to extract values from responses.
// It creates JavaScript code that retrieves values from the response JSON and sets them as collection variables.
func (b *Builder) CreateTestScript(chainedValues []*util.ValueReference) PostmanEvent {
	var scriptLines []string
	scriptLines = append(scriptLines, "var responseJson = pm.response.json();")

	usedVariables := make(map[string]bool)

	for _, chainedValue := range chainedValues {
		if chainedValue.SourceType != util.SourceTypeResponse {
			continue
		}
		if chainedValue.Context == nil {
			continue
		}
		variableName := chainedValue.Context.VariableName
		if usedVariables[variableName] {
			continue
		}
		usedVariables[variableName] = true

		scriptLines = append(scriptLines, b.buildScriptForVariable(chainedValue)...)
	}

	return PostmanEvent{
		Listen: "test",
		Script: PostmanEventScript{
			Type: "text/javascript",
			Exec: scriptLines,
		},
	}
}

// buildScriptForVariable constructs JavaScript code snippets for extracting a single variable from the response.
// It includes error handling and sets the extracted value as a Postman collection variable.
func (b *Builder) buildScriptForVariable(chainedValue *util.ValueReference) []string {
	var scriptLines []string
	collectionVarName := chainedValue.Context.VariableName

	// Build JavaScript code to extract the value with error handling
	jsPath := chainedValue.ReferencePath
	scriptLines = append(scriptLines, "try {")

	valueExtraction := fmt.Sprintf("  var %s = %s;", collectionVarName, jsPath)
	scriptLines = append(scriptLines, valueExtraction)

	setVariable := fmt.Sprintf("  pm.collectionVariables.set(\"%s\", %s);", collectionVarName, collectionVarName)
	scriptLines = append(scriptLines, setVariable)
	printToConsole := fmt.Sprintf("  console.log('Variable: %s, Value:', %s);", collectionVarName, collectionVarName)
	scriptLines = append(scriptLines, printToConsole)
	scriptLines = append(scriptLines, "} catch (e) {")
	logError := fmt.Sprintf("  console.error('Error extracting variable %s:', e);", collectionVarName)
	scriptLines = append(scriptLines, logError)
	scriptLines = append(scriptLines, "}")

	return scriptLines
}

// CreateInitScript creates a pre-request script for initializing variables.
func (b *Builder) CreateInitScript(values []*util.ChainedValueContext) *PostmanEvent {
	var scriptLines []string
	scriptLines = append(scriptLines, "var result = {};")

	for _, chainedValue := range values {
		collectionVarName := chainedValue.VariableName

		if chainedValue.InitScript != "" {
			scriptLines = append(scriptLines, "try {")
			scriptLines = append(scriptLines, chainedValue.InitScript)

			setVariable := fmt.Sprintf("  pm.collectionVariables.set(\"%s\", result);", collectionVarName)
			scriptLines = append(scriptLines, setVariable)
			printToConsole := fmt.Sprintf("  console.log('Variable: %s, Value:' + result);", collectionVarName)
			scriptLines = append(scriptLines, printToConsole)
			scriptLines = append(scriptLines, "} catch (e) {")
			logError := fmt.Sprintf("  console.error('Error extracting variable %s:', e);", collectionVarName)
			scriptLines = append(scriptLines, logError)
			scriptLines = append(scriptLines, "}")
		}
	}

	if len(scriptLines) <= 1 {
		return nil
	}

	return &PostmanEvent{
		Listen: "prerequest",
		Script: PostmanEventScript{
			Type: "text/javascript",
			Exec: scriptLines,
		},
	}
}

// WriteToFile serializes the Postman collection into JSON format with proper indentation.
// It writes the JSON data to the specified filename and returns any errors encountered.
func (b *Builder) WriteToFile(collection PostmanCollection, filename string) error {
	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling collection to JSON: %w", err)
	}
	return os.WriteFile(filename, data, 0644)
}
