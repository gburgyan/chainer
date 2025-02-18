package main

import (
	"log"
)

type VariableGenerator struct {
	OriginRequestUrl  string `json:"origin_request_url"`
	ResponsePath      string `json:"response_path"`
	ExampleValue      string `json:"example_value,omitempty"`
	InitializerPrompt string `json:"initializer_prompt,omitempty"`
}

type VariableGeneratorResponse struct {
	VariableName string `json:"variable_name"`
	InitScript   string `json:"init_script"`
}

// assignVariableNames assigns descriptive variable names to each chained value.
// It prepares input data based on the origin request URL and response path of each value.
// It calls the OpenAI API to generate meaningful names following best practices and updates each ChainedValueContext.
func assignVariableNames(chainedValues []*ChainedValueContext) {

	var variableNames []VariableGenerator
	for _, cv := range chainedValues {
		vg := VariableGenerator{
			ExampleValue:      cv.Value,
			InitializerPrompt: cv.InitScript,
		}
		if cv.ValueSource == nil {
			vg.OriginRequestUrl = cv.AllUsages[0].Source.Entry.Request.URL
			vg.ResponsePath = cv.AllUsages[0].ReferencePath
		} else {
			vg.OriginRequestUrl = cv.ValueSource.Source.Entry.Request.URL
			vg.ResponsePath = cv.ValueSource.ReferencePath
		}

		variableNames = append(variableNames, vg)
	}

	res, err := CallOpenAIArray[VariableGeneratorResponse](`
I want you to come up with good variable names and optional initializers for values retrieved from an API.
Please ensure each variable name is descriptive and follows best practices and is unique, but don't be overly verbose.
Don't include things like "identifier" or "value" in the name unless it's critical to the naming, just the most descriptive part as a human would name it.

The API is the Travelport JSON API. Use the knowledge of the API to come up with good names.

The response should be an array of objects with each object containing the variable name and an optional initialization script if it is applicable. There
needs to be a variable called "result" that holds the final result of the script. This will be executed in a JavaScript environment in the Postman collection.
It will be run inside of a braced block, but do not include the braces in the response.

An example of the input data is an array of objects like this:

[
	{
		  "origin_request_url": "https://api.travelport.com/v1/air/flight",
		  "response_path": "$.data.flights[0].segments[0].departure_time",
		  "example_value": "2025-01-02",
		  "initializer_prompt": "A day that is 3 weeks from the time the script is run"
	}
]

The format of the response should be an array of objects like this:

[
	{
		"variable_name": "departureTime",
		"init_script": "var result = new Date(); result.setDate(result.getDate() + 21); result.setHours(0, 0, 0, 0); result = result.toISOString().split('T')[0];"
	}
]

If no initialization script is requested, please omit the "init_script" field.

There should be a 1:1 correspondence between the input and output arrays.

Here's what I need to get processed:`, variableNames)

	if err != nil {
		log.Fatalf("Error calling OpenAI: %v", err)
	}

	// Assign variable names
	for i, value := range chainedValues {
		value.VariableName = res[i].VariableName
		value.InitScript = res[i].InitScript
	}
}
