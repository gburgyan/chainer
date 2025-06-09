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
			// Log deprecation warning
			log.Println("WARNING: Using legacy client without template support. This is deprecated and will be removed in a future version.")
			// For backward compatibility, return an error instead of using hardcoded prompt
			return fmt.Errorf("templated client is required for variable naming operations - legacy prompts have been removed")
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
