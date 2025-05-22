package postman

// PostmanCollection represents a Postman collection structure.
type PostmanCollection struct {
	Info      CollectionInfo    `json:"info"`
	Item      []PostmanItem     `json:"item"`
	Variables []PostmanVariable `json:"variable,omitempty"`
}

// CollectionInfo holds metadata about the Postman collection.
type CollectionInfo struct {
	Name    string `json:"name"`
	Schema  string `json:"schema"`
	Version string `json:"version"`
}

// PostmanVariable represents a variable in a Postman collection.
type PostmanVariable struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// PostmanItem represents a request item in a Postman collection.
type PostmanItem struct {
	Name     string            `json:"name"`
	Request  PostmanRequest    `json:"request"`
	Event    []PostmanEvent    `json:"event,omitempty"`
	Variable []PostmanVariable `json:"variable,omitempty"`
}

// PostmanRequest represents a request in a Postman collection.
type PostmanRequest struct {
	Method string              `json:"method"`
	Header []PostmanHeader     `json:"header,omitempty"`
	Body   *PostmanRequestBody `json:"body,omitempty"`
	URL    PostmanURL          `json:"url"`
}

// PostmanHeader represents an HTTP header in a Postman request.
type PostmanHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PostmanRequestBody represents the body of a Postman request.
type PostmanRequestBody struct {
	Mode       string            `json:"mode"`
	Raw        string            `json:"raw,omitempty"`
	Urlencoded []PostmanKeyValue `json:"urlencoded,omitempty"`
	FormData   []PostmanKeyValue `json:"formdata,omitempty"`
}

// PostmanKeyValue represents a key-value pair in a Postman request.
type PostmanKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

// PostmanURL represents a URL in a Postman request.
type PostmanURL struct {
	Raw      string              `json:"raw"`
	Protocol string              `json:"protocol"`
	Host     []string            `json:"host"`
	Path     []string            `json:"path"`
	Query    []PostmanQueryParam `json:"query,omitempty"`
}

// PostmanQueryParam represents a query parameter in a Postman URL.
type PostmanQueryParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PostmanEvent represents an event in a Postman request.
type PostmanEvent struct {
	Listen string             `json:"listen"`
	Script PostmanEventScript `json:"script"`
}

// PostmanEventScript represents a script in a Postman event.
type PostmanEventScript struct {
	Type string   `json:"type"`
	Exec []string `json:"exec"`
}
