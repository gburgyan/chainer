package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
)

type PostmanCollection struct {
	Info      CollectionInfo    `json:"info"`
	Item      []PostmanItem     `json:"item"`
	Variables []PostmanVariable `json:"variable,omitempty"`
}

type CollectionInfo struct {
	Name    string `json:"name"`
	Schema  string `json:"schema"`
	Version string `json:"version"`
}

type PostmanVariable struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type PostmanItem struct {
	Name     string            `json:"name"`
	Request  PostmanRequest    `json:"request"`
	Event    []PostmanEvent    `json:"event,omitempty"`
	Variable []PostmanVariable `json:"variable,omitempty"`
}

type PostmanRequest struct {
	Method string              `json:"method"`
	Header []PostmanHeader     `json:"header,omitempty"`
	Body   *PostmanRequestBody `json:"body,omitempty"`
	URL    PostmanURL          `json:"url"`
}

type PostmanHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PostmanRequestBody struct {
	Mode       string            `json:"mode"`
	Raw        string            `json:"raw,omitempty"`
	Urlencoded []PostmanKeyValue `json:"urlencoded,omitempty"`
	FormData   []PostmanKeyValue `json:"formdata,omitempty"`
}

type PostmanKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

type PostmanURL struct {
	Raw      string              `json:"raw"`
	Protocol string              `json:"protocol"`
	Host     []string            `json:"host"`
	Path     []string            `json:"path"`
	Query    []PostmanQueryParam `json:"query,omitempty"`
}

type PostmanQueryParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PostmanEvent struct {
	Listen string             `json:"listen"`
	Script PostmanEventScript `json:"script"`
}

type PostmanEventScript struct {
	Type string   `json:"type"`
	Exec []string `json:"exec"`
}

func AssignVariableNames(chainedValues []*ChainedValueContext) {
	var variableNameCounter int
	for _, value := range chainedValues {
		variableName := fmt.Sprintf("var_%d", variableNameCounter)
		value.VariableName = variableName
		variableNameCounter++
	}
}

func ReplaceChainedValuesInRequest(request *CallDetails) PostmanRequest {
	// Replace chained values in the request URL
	requestUrl := BuildPostmanURL(request)
	// Replace chained values in the request headers
	var headers []PostmanHeader
	for _, header := range request.Entry.Request.Headers {
		headers = append(headers, PostmanHeader{
			Key:   header.Name,
			Value: ReplaceValuesInString(header.Value, request.RequestChainedValues),
		})
	}
	// Replace chained values in the request body
	var body PostmanRequestBody
	if request.Entry.Request.PostData != nil {
		body = PostmanRequestBody{
			Mode: "raw",
			Raw:  ReplaceValuesInString(request.Entry.Request.PostData.Text, request.RequestChainedValues),
		}
	}

	postmanRequest := PostmanRequest{
		Method: request.Entry.Request.Method,
		Header: headers,
		Body:   &body,
		URL:    requestUrl,
	}

	return postmanRequest
}

func ReplaceValuesInString(input string, valueToVariableName []*ValueReference) string {
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

func BuildPostmanURL(callDetails *CallDetails) PostmanURL {
	rawUrl := callDetails.Entry.Request.URL

	parsedURL, _ := url.Parse(rawUrl)
	// Build the PostmanURL struct with parsed URL components

	postmanURL := PostmanURL{
		Raw:      ReplaceValuesInString(rawUrl, callDetails.RequestChainedValues),
		Protocol: parsedURL.Scheme,
		Host:     []string{parsedURL.Host},
	}

	// Split the path into individual components
	pathComponents := strings.Split(parsedURL.Path, "/")
	var path []string
	for _, component := range pathComponents {
		if component != "" {
			// Replace path parameters with Postman variables
			for _, ref := range callDetails.RequestChainedValues {
				valueString := fmt.Sprintf("%v", ref.Value)
				if component == valueString {
					component = "{{" + ref.Context.VariableName + "}}"
					break
				}
			}
			path = append(path, component)
		}
	}
	postmanURL.Path = path

	// Split the query string into individual components
	queryComponents := parsedURL.Query()
	var query []PostmanQueryParam
	for key, values := range queryComponents {
		for _, value := range values {
			// Replace query parameters with Postman variables
			for _, ref := range callDetails.RequestChainedValues {
				valueString := fmt.Sprintf("%v", ref.Value)
				if value == valueString {
					value = "{{" + ref.Context.VariableName + "}}"
					break
				}
			}
			query = append(query, PostmanQueryParam{
				Key:   key,
				Value: value,
			})
		}
	}
	postmanURL.Query = query

	return postmanURL
}

func CreateTestScript(chainedValues []*ValueReference) PostmanEvent {
	var scriptLines []string
	scriptLines = append(scriptLines, "var responseJson = pm.response.json();")

	usedVariables := make(map[string]bool)

	for _, chainedValue := range chainedValues {
		if chainedValue.SourceType != SourceTypeResponse {
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

		scriptLines = append(scriptLines, buildScriptForVariable(chainedValue)...)
	}

	return PostmanEvent{
		Listen: "test",
		Script: PostmanEventScript{
			Type: "text/javascript",
			Exec: scriptLines,
		},
	}
}

func buildScriptForVariable(chainedValue *ValueReference) []string {
	var scriptLines []string
	collectionVarName := chainedValue.Context.VariableName

	// Build JavaScript code to extract the value with error handling
	jsPath := chainedValue.JavascriptReference
	scriptLines = append(scriptLines, "try {")
	valueExtraction := fmt.Sprintf("  var %s = responseJson.%s;", collectionVarName, jsPath)
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

func BuildPostmanCollection(callDetailsList []*CallDetails, chainedValues []*ChainedValueContext) PostmanCollection {
	var items []PostmanItem

	AssignVariableNames(chainedValues)

	for _, callDetails := range callDetailsList {
		if callDetails == nil {
			continue
		}
		postmanRequest := ReplaceChainedValuesInRequest(callDetails)

		// Check if this request's response has values to extract
		var events []PostmanEvent
		script := CreateTestScript(callDetails.ResponseChainedValues)
		//for _, chainedValue := range chainedValues {
		//	for _, ref := range chainedValue.Context {
		//		if ref.Source == &callDetails && ref.SourceType == SourceTypeResponse {
		//
		//		}
		//	}
		//}
		events = append(events, script)

		parsedUrl, err := url.Parse(callDetails.Entry.Request.URL)
		if err != nil {
			log.Printf("Error parsing URL %s: %v", callDetails.Entry.Request.URL, err)
			continue
		}
		item := PostmanItem{
			Name:    fmt.Sprintf("Req %s", parsedUrl.Path),
			Request: postmanRequest,
			Event:   events,
		}
		items = append(items, item)
	}

	variables := make([]PostmanVariable, len(chainedValues))
	for i, chainedValue := range chainedValues {
		variables[i] = PostmanVariable{
			Key: chainedValue.VariableName,
			//Value: fmt.Sprintf("%v", chainedValue.Value),
			Description: chainedValue.ValueSource.JavascriptReference,
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

func WriteCollectionToFile(collection PostmanCollection, filename string) error {
	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}
