package util

import (
	"fmt"
	"log"
)

// ChainFinder identifies chained values across HTTP calls.
type ChainFinder struct {
	// Configuration options could be added here
}

// NewChainFinder creates a new ChainFinder.
func NewChainFinder() *ChainFinder {
	return &ChainFinder{}
}

// FindChainedValues analyzes the call details to identify values that appear in multiple requests and responses.
// It filters out values that are not considered "interesting" and returns a slice of ChainedValueContext.
func (cf *ChainFinder) FindChainedValues(callDetailsList []*CallDetails) []*ChainedValueContext {
	// Map to keep track of values and their occurrences
	valueOccurrences := make(map[string][]*ValueReference)

	// Iterate over each CallDetails
	for _, callDetails := range callDetailsList {
		// Process RequestDetails
		for _, reqDetail := range callDetails.RequestDetails {
			if reqDetail.IsInteresting() {
				valueStr := fmt.Sprintf("%v", reqDetail.Value)
				valueOccurrences[valueStr] = append(valueOccurrences[valueStr], reqDetail)
			}
		}

		// Process ResponseDetails
		for _, respDetail := range callDetails.ResponseDetails {
			if respDetail.IsInteresting() {
				valueStr := fmt.Sprintf("%v", respDetail.Value)
				valueOccurrences[valueStr] = append(valueOccurrences[valueStr], respDetail)
			}
		}
	}

	// Filter to keep only values that appear in multiple requests and responses
	var chainedValues []*ChainedValueContext
	for value, refs := range valueOccurrences {
		if len(refs) > 1 {
			chainedValues = append(chainedValues, &ChainedValueContext{
				Value:     value,
				AllUsages: refs,
			})
		}
	}

	// Now exclude values that appear in requests before responses. Also exclude values that appear in the same request.
	// Additionally, exclude values that appear in the same response. For responses, only include the first appearance.
	var filteredChainedValues []*ChainedValueContext

NextChainedValue:
	for _, chainedValue := range chainedValues {
		var seenResponse bool
		includeVal := false
		for _, contextItem := range chainedValue.AllUsages {
			if contextItem.SourceType == SourceTypeRequest {
				if !seenResponse {
					continue NextChainedValue
				}
				includeVal = true
			} else {
				seenResponse = true
			}
		}
		if includeVal {
			filteredChainedValues = append(filteredChainedValues, chainedValue)
		}
	}

	return filteredChainedValues
}

// RepopulateCallDetails updates each CallDetails instance by linking it to the associated chained values.
// It assigns the Context field of ValueReference to point to the corresponding ChainedValueContext.
func (cf *ChainFinder) RepopulateCallDetails(chainedValues []*ChainedValueContext) {
	for _, chainedValue := range chainedValues {
		for _, contextItem := range chainedValue.AllUsages {
			if contextItem.Source != nil {
				if contextItem.SourceType == SourceTypeRequest {
					contextItem.Context = chainedValue
					contextItem.Source.RequestChainedValues = append(contextItem.Source.RequestChainedValues, contextItem)
				} else {
					contextItem.Context = chainedValue
					contextItem.Source.ResponseChainedValues = append(contextItem.Source.ResponseChainedValues, contextItem)
					if chainedValue.ValueSource == nil {
						chainedValue.ValueSource = contextItem
					}
				}
			} else {
				log.Printf("Source is nil for value: %s, type %v", chainedValue.Value, contextItem.SourceType)
			}
		}
	}
}

// ExtractPredefinedVars walks through all value references in the provided call details and,
// if the value exactly matches a pre-defined variable value, replaces it with a variable reference.
func (cf *ChainFinder) ExtractPredefinedVars(callDetailsList []*CallDetails, vars []PredefinedVariable, values []*ChainedValueContext) []*ChainedValueContext {
	// Make a copy of the values to avoid modifying the original slice
	resultValues := make([]*ChainedValueContext, len(values))
	copy(resultValues, values)

	for _, v := range vars {
		manualChainedValue := &ChainedValueContext{
			Value:          v.SearchValue,
			ExternalSource: true,
			VariableName:   v.Name,
			InitScript:     v.InitializerPrompt,
		}
		found := false
		for _, cd := range callDetailsList {
			for _, vr := range cd.RequestDetails {
				found = cf.addPredefinedUseIfFound(manualChainedValue, vr) || found
			}
		}
		if found {
			resultValues = append(resultValues, manualChainedValue)
		}
	}

	return resultValues
}

// addPredefinedUseIfFound checks if the value in the ValueReference is a string and,
// if it exactly matches one of the pre-defined values, replaces it with a variable placeholder.
func (cf *ChainFinder) addPredefinedUseIfFound(cv *ChainedValueContext, vr *ValueReference) bool {
	str, ok := vr.Value.(string)
	if !ok {
		return false
	}
	if str == cv.Value {
		cv.AllUsages = append(cv.AllUsages, vr)
		return true
	}
	return false
}

// PredefinedVariable represents a variable defined by the user.
type PredefinedVariable struct {
	Name              string `json:"name"`
	SearchValue       string `json:"search_value"`
	InitializerPrompt string `json:"initializer,omitempty"`
}

// LogChainedValues logs the chained values for debugging purposes.
func (cf *ChainFinder) LogChainedValues(chainedValues []*ChainedValueContext) {
	for i, chainedValue := range chainedValues {
		fmt.Printf("Chained Value %d:\n", i+1)
		fmt.Printf("  Value: %s\n", chainedValue.Value)
		fmt.Println("  Context:")
		for _, ref := range chainedValue.AllUsages {
			var requestOrResponse string
			if ref.SourceType == SourceTypeRequest {
				requestOrResponse = "Request"
			} else {
				requestOrResponse = "Response"
			}
			fmt.Printf("    - %s - %s\n", requestOrResponse, ref.ReferencePath)
		}
		fmt.Println()
	}
}
