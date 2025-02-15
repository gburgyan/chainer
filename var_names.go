package main

import (
	"log"
)

type VariableNameGenerator struct {
	OriginRequestUrl string `json:"origin_request_url"`
	ResponsePath     string `json:"response_path"`
}

// assignVariableNames assigns descriptive variable names to each chained value.
// It prepares input data based on the origin request URL and response path of each value.
// It calls the OpenAI API to generate meaningful names following best practices and updates each ChainedValueContext.
func assignVariableNames(chainedValues []*ChainedValueContext) {

	var variableNames []VariableNameGenerator
	for _, value := range chainedValues {
		variableNames = append(variableNames, VariableNameGenerator{
			OriginRequestUrl: value.ValueSource.Source.Entry.Request.URL,
			ResponsePath:     value.ValueSource.ReferencePath,
		})
	}

	res, err := CallOpenAIArray[string](`I want you to come up with good variable names for values retrieved from an API.
		Please ensure each variable name is descriptive and follows best practices and is unique, but don't be overly verbose.
	Don't include things like "identifier" or "value" in the name unless it's critical to the naming, just the most descriptive part as a human would name it.

		I'm giving you the URL that was called and the response JSON path that the value is retrieved from.

	The API is: Travelport JSON API.

		The response should be a JSON array of strings with each result corresponding to the name to
	give the variable to hold the value. Please only return the JSON and nothing else with no additional
	formatting or content.

		Here's what I need names for:`, variableNames)

	if err != nil {
		log.Fatalf("Error calling OpenAI: %v", err)
	}

	// Assign variable names
	for i, value := range chainedValues {
		value.VariableName = res[i]
	}
}
