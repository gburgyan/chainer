package main

// SourceType represents the origin of a value extracted from an HTTP transaction.
// It indicates whether the value was found in the request or in the response.
type SourceType int

const (
	// SourceTypeRequest indicates that the value comes from the HTTP request.
	SourceTypeRequest SourceType = iota

	// SourceTypeResponse indicates that the value comes from the HTTP response.
	SourceTypeResponse
)

// SourceLocation represents the specific location within an HTTP transaction
// where a value was discovered. This may include headers, the JSON body,
// URL segments, or form data.
type SourceLocation int

const (
	// SourceLocationHeader indicates that the value was found in an HTTP header.
	SourceLocationHeader SourceLocation = iota

	// SourceLocationBodyJson indicates that the value was extracted from a JSON body.
	SourceLocationBodyJson

	// SourceLocationBodyForm indicates that the value was extracted from form data in the body.
	SourceLocationBodyForm

	// SourceLocationUrl indicates that the value was extracted from the URL (host, path, or query).
	SourceLocationUrl
)

// CallDetails aggregates information for a single HTTP call.
// It holds details extracted from both the request and response, along with any
// chained values that have been identified for potential variable substitution.
// Additionally, it maintains a reference to the original HAR entry.
type CallDetails struct {
	// RequestDetails contains all the value references extracted from the request.
	RequestDetails []*ValueReference `json:"request_details"`

	// ResponseDetails contains all the value references extracted from the response.
	ResponseDetails []*ValueReference `json:"response_details"`

	// Entry holds the raw HAR entry for this call.
	// TODO: An Entry should be more generic and not tied to HAR.
	Entry *Entry `json:"entry"`

	// RequestChainedValues contains value references in the request that have been
	// identified as being part of a variable chaining scenario.
	RequestChainedValues []*ValueReference `json:"request_chained_values"`

	// ResponseChainedValues contains value references in the response that have been
	// identified as being part of a variable chaining scenario.
	ResponseChainedValues []*ValueReference `json:"response_chained_values"`
}

// ValueReference encapsulates an extracted value along with metadata describing
// its location and context within an HTTP transaction. This includes its reference
// path (similar to a JavaScript property path), its source (e.g. request or response),
// and the precise location (e.g. header, body, URL). It also optionally stores contextual
// and ancestral information for chained value substitution.
type ValueReference struct {
	// Value is the actual extracted value. It can be of any type.
	Value any `json:"value"`

	// ReferencePath provides a JavaScript-like path to locate the value within the source.
	ReferencePath string `json:"javascript_reference"`

	// UrlLocation indicates the location within the URL where the value was found.
	UrlLocation int `json:"url_location"`

	// HeaderName is the name of the header where the value was extracted.
	// TODO: Remove this field
	HeaderName string `json:"header_name"`

	// Source points to the CallDetails from which this value was extracted.
	Source *CallDetails `json:"source"`

	// SourceType indicates whether the value came from the request or response.
	SourceType SourceType `json:"source_type"`

	// SourceLocation specifies the exact location within the HTTP transaction (e.g., header, JSON body).
	SourceLocation SourceLocation `json:"source_location"`

	// Context points to the ChainedValueContext if this value is part of a chained substitution.
	Context *ChainedValueContext

	// Ancestors contains a list of parent objects leading to this value (used when flattening JSON).
	Ancestors []interface{}
}

// ChainedValueContext holds information about a value that is shared across
// multiple HTTP calls (i.e. chained). It stores the actual value as well as all
// the usage contexts, the original source of the value, and an optional variable name
// that can be used for substitution in a Postman collection.
type ChainedValueContext struct {
	// Value is the string representation of the chained value.
	Value string

	// AllUsages contains all ValueReference instances where this value is used.
	AllUsages []*ValueReference

	// ValueSource points to the original ValueReference that is considered the source of this chained value.
	ValueSource *ValueReference

	// VariableName is an optional name assigned to the value for substitution purposes.
	VariableName string

	// True if this was a manually added variable
	ExternalSource bool

	// Initialization script for the variable (if applicable)
	InitScript string
}
