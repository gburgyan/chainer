package main

import (
	"log"
)

type VariableGenerator struct {
	OriginRequestUrl  string `json:"origin_request_url"`
	ResponsePath      string `json:"response_path"`
	ExampleValue      string `json:"example_value,omitempty"`
	InitializerPrompt string `json:"initializer_prompt,omitempty"`
	ProposedName      string `json:"proposed_name,omitempty"`
}

type VariableGeneratorResponse struct {
	VariableName string `json:"name"`
}

// assignVariableNames assigns descriptive variable names to each chained value.
// It prepares input data based on the origin request URL and response path of each value.
// It calls the OpenAI API to generate meaningful names following best practices and updates each ChainedValueContext.
func assignVariableNames(chainedValues []*ChainedValueContext) error {

	var variableNames []VariableGenerator
	for _, cv := range chainedValues {
		// Ensure value isn't longer than 50 chars
		val := cv.Value
		if len(val) > 50 {
			val = val[:50]
		}
		vg := VariableGenerator{
			ExampleValue:      val,
			InitializerPrompt: cv.InitScript,
			ProposedName:      cv.VariableName,
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
Don't include things like "identifier" or "value" in the name unless it's critical to the naming, just the most descriptive
part as a human would name it.

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
	      "proposed_name": "departureTime"
	}
]

The format of the response should be an array of objects like this:

[
	{
		"name": "departureTime",
	}, ...
]

If a proposed name is provided, please use that as the variable name (and also return it)
Ensure that there are no conflicts with other variable names.
There should be a 1:1 correspondence between the input and output arrays. Every input *must* have a corresponding output.

Please return a completely undecorated JSON response with just the array of objects.`, variableNames)

	if err != nil {
		log.Fatalf("Error calling OpenAI: %v", err)
		return err
	}

	// Assign variable names
	for i, value := range chainedValues {
		value.VariableName = res[i].VariableName
	}

	return nil
}
