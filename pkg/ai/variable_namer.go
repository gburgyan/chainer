package ai

import (
	"fmt"
	"log"

	"github.com/gburgyan/chainer/pkg/har"
	"github.com/gburgyan/chainer/pkg/util"
)

// VariableGenerator holds information needed to generate a variable name.
type VariableGenerator struct {
	OriginRequestUrl  string `json:"origin_request_url"`
	ResponsePath      string `json:"response_path"`
	ExampleValue      string `json:"example_value,omitempty"`
	InitializerPrompt string `json:"initializer_prompt,omitempty"`
	ProposedName      string `json:"proposed_name,omitempty"`
}

// VariableGeneratorResponse represents the AI-generated variable name.
type VariableGeneratorResponse struct {
	VariableName string `json:"name"`
}

// OpenAIClientInterface defines the interface for OpenAI clients
type OpenAIClientInterface interface {
	CallBase(prompt string, input interface{}) (string, error)
	CallString(prompt string, input interface{}) (string, error)
	CallArray(prompt string, input interface{}, result interface{}) error
	CallObject(prompt string, input interface{}, result interface{}) error
}

// TemplatedClientInterface extends the base client with template support
type TemplatedClientInterface interface {
	OpenAIClientInterface
	CallWithTemplate(templateName string, templateData interface{}, input interface{}) (string, error)
	CallArrayWithTemplate(templateName string, templateData interface{}, input interface{}, result interface{}) error
	CallObjectWithTemplate(templateName string, templateData interface{}, input interface{}, result interface{}) error
}

// VariableNamer handles the assignment of descriptive names to variables.
type VariableNamer struct {
	Client OpenAIClientInterface
}

// NewVariableNamer creates a new VariableNamer with the specified OpenAI client.
func NewVariableNamer(client OpenAIClientInterface) *VariableNamer {
	return &VariableNamer{
		Client: client,
	}
}

// AssignVariableNames assigns descriptive variable names to each chained value.
// It prepares input data based on the origin request URL and response path of each value.
// It calls the OpenAI API to generate meaningful names following best practices and updates each ChainedValueContext.
func (vn *VariableNamer) AssignVariableNames(chainedValues []*util.ChainedValueContext) error {
	// Check for verbose logging
	verbose := false
	if openAIClient, ok := vn.Client.(*OpenAIClient); ok {
		verbose = openAIClient.Config.Verbose
	} else if templatedClient, ok := vn.Client.(*TemplatedClient); ok {
		if openAIClient, ok := templatedClient.OpenAIClient.(*OpenAIClient); ok {
			verbose = openAIClient.Config.Verbose
		}
	}

	if verbose {
		fmt.Printf("\n=== Variable Name Generation ===\n")
		fmt.Printf("Number of chained values to name: %d\n", len(chainedValues))
	}

	var variableNames []VariableGenerator
	for i, cv := range chainedValues {
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
			if verbose {
				fmt.Printf("Warning: Chained value %d has no ValueSource, using AllUsages[0]\n", i)
			}
			if len(cv.AllUsages) > 0 {
				if entry, ok := cv.AllUsages[0].Source.Entry.(*har.Entry); ok {
					vg.OriginRequestUrl = entry.Request.URL
					vg.ResponsePath = cv.AllUsages[0].ReferencePath
				} else if verbose {
					fmt.Printf("Warning: Entry type assertion failed for chained value %d\n", i)
				}
			} else if verbose {
				fmt.Printf("Warning: Chained value %d has no AllUsages\n", i)
			}
		} else {
			if entry, ok := cv.ValueSource.Source.Entry.(*har.Entry); ok {
				vg.OriginRequestUrl = entry.Request.URL
				vg.ResponsePath = cv.ValueSource.ReferencePath
			} else if verbose {
				fmt.Printf("Warning: Entry type assertion failed for chained value %d ValueSource\n", i)
			}
		}

		if verbose {
			fmt.Printf("Variable %d: Value='%s', Path='%s', URL='%s'\n",
				i, vg.ExampleValue, vg.ResponsePath, vg.OriginRequestUrl)
		}

		variableNames = append(variableNames, vg)
	}

	if verbose {
		fmt.Printf("Prepared %d variable generators\n", len(variableNames))
	}

	// Wrap the entire operation in retry logic to handle response count mismatches
	retryableOperation := util.WithRetries(func(variableNames []VariableGenerator, chainedValues []*util.ChainedValueContext) error {
		var results []VariableGeneratorResponse
		var err error

		// Check if the client supports templates
		if templatedClient, ok := vn.Client.(TemplatedClientInterface); ok {
			if verbose {
				fmt.Println("Using templated client for variable naming")
			}
			// Use the template system
			err = templatedClient.CallArrayWithTemplate("variable_naming", nil, variableNames, &results)
		} else {
			if verbose {
				fmt.Println("Using legacy client for variable naming")
			}
			// Fall back to the legacy approach with embedded prompt
			err = vn.Client.CallArray(legacyVariableNamingPrompt, variableNames, &results)
		}

		if err != nil {
			log.Printf("Error calling OpenAI: %v", err)
			return fmt.Errorf("error calling OpenAI for variable names: %w", err)
		}

		if verbose {
			fmt.Printf("Received %d results from OpenAI\n", len(results))
			for i, result := range results {
				fmt.Printf("Result %d: Name='%s'\n", i, result.VariableName)
			}
		}

		// Check if we got the right number of results
		if len(results) != len(chainedValues) {
			if verbose {
				fmt.Printf("ERROR: Response count mismatch (got %d, need %d), will retry\n",
					len(results), len(chainedValues))
			}
			return fmt.Errorf("response count mismatch: got %d results, expected %d",
				len(results), len(chainedValues))
		}

		// Assign variable names
		for i, value := range chainedValues {
			value.VariableName = results[i].VariableName
			if verbose {
				fmt.Printf("Assigned name '%s' to chained value %d\n", value.VariableName, i)
			}
		}

		return nil
	}, 3)

	// Execute the retryable operation
	if err := retryableOperation(variableNames, chainedValues); err != nil {
		return err
	}

	if verbose {
		fmt.Println("=== Variable naming complete ===")
	}

	return nil
}

// Legacy prompt for backward compatibility
const legacyVariableNamingPrompt = `
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
`
