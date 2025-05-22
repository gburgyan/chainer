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
# Role and Objective
You are a variable naming expert for API data extraction in Postman collections. Your task is to generate descriptive, best-practice variable names for values extracted from API responses.

# Instructions
Create meaningful variable names for values retrieved from API responses. These names will be used in a Postman collection to store values extracted from responses and used in subsequent requests.

## Naming Conventions
- Make each variable name descriptive and follow JavaScript naming best practices
- Ensure all names are unique within the collection
- Be concise but clear - avoid being overly verbose
- Don't include generic terms like "identifier" or "value" unless critical to understanding
- Focus on the most descriptive part as a human would naturally name it
- If a proposed name is provided, use that as the variable name

# Reasoning Steps
1. Analyze the origin request URL to understand the API endpoint context
2. Consider the JSON path to understand where in the response the value comes from
3. Review the example value to understand the data type and content
4. Check if a proposed name is already provided and use it if available
5. Ensure the name is unique among all variables in the collection

# Output Format
Return an array of objects with a 1:1 correspondence to the input array. Each object should contain a "name" property with the variable name.
The response must be a completely undecorated JSON array.

# Examples
## Example 1
Input:
[
    {
        "origin_request_url": "https://api.travelport.com/v1/air/flight",
        "response_path": "$.data.flights[0].segments[0].departure_time",
        "example_value": "2025-01-02",
        "proposed_name": "departureTime"
    },
    {
        "origin_request_url": "https://{{baseURL}}/11/air/book/airoffer/reservationworkbench/{{reservationValue}}/offers/buildfromcatalogproductoffering",
        "response_path": "OfferListResponse.OfferID[0].Identifier.value",
        "example_value": "o21"
    },
    {
        "origin_request_url": "https://{{baseURL}}/11/air/book/reservation/reservations/{{confirmationLocator}}",
        "response_path": "ReservationResponse.Reservation.Offer[0].Price.CurrencyCode.value",
        "example_value": "USD"
    }
]

Output:
[
    {
        "name": "departureTime",
    },
	{
		"name": "offerIdentifier",
	},
	{
		"name": "reservationOfferCurrency",
	}
]

# Context
This naming task is part of a HAR to Postman Collection converter. The variable names you create will be used to replace hardcoded values in the
collection with Postman environment variables, making the collection more dynamic and reusable.

# Final instructions
There must be a 1:1 correspondence between input and output arrays - every input must have a corresponding output. Return only raw JSON
without any explanations or decorations.
`, variableNames)

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
