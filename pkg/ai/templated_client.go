package ai

import (
	"fmt"

	"github.com/gburgyan/chainer/pkg/template"
)

// TemplatedClient extends the OpenAI client with template support
type TemplatedClient struct {
	OpenAIClient OpenAIClientInterface
	Templates    *template.TemplateManager
}

// Make TemplatedClient implement the OpenAIClientInterface

// CallBase forwards the call to the underlying OpenAIClient
func (c *TemplatedClient) CallBase(prompt string, input interface{}) (string, error) {
	return c.OpenAIClient.CallBase(prompt, input)
}

// CallString forwards the call to the underlying OpenAIClient
func (c *TemplatedClient) CallString(prompt string, input interface{}) (string, error) {
	return c.OpenAIClient.CallString(prompt, input)
}

// CallArray forwards the call to the underlying OpenAIClient
func (c *TemplatedClient) CallArray(prompt string, input interface{}, result interface{}) error {
	return c.OpenAIClient.CallArray(prompt, input, result)
}

// CallObject forwards the call to the underlying OpenAIClient
func (c *TemplatedClient) CallObject(prompt string, input interface{}, result interface{}) error {
	return c.OpenAIClient.CallObject(prompt, input, result)
}

// NewTemplatedClient creates a new client with template support
func NewTemplatedClient(config *Config) (*TemplatedClient, error) {
	client, err := NewOpenAIClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating OpenAI client: %w", err)
	}

	templates, err := template.DefaultManager()
	if err != nil {
		return nil, fmt.Errorf("error loading templates: %w", err)
	}

	return &TemplatedClient{
		OpenAIClient: client,
		Templates:    templates,
	}, nil
}

// CallWithTemplate renders a template and uses it as the prompt for the API call
func (c *TemplatedClient) CallWithTemplate(templateName string, templateData interface{}, input interface{}) (string, error) {
	prompt, err := c.Templates.Render(templateName, templateData)
	if err != nil {
		return "", fmt.Errorf("error rendering template %s: %w", templateName, err)
	}

	return c.OpenAIClient.CallBase(prompt, input)
}

// CallArrayWithTemplate renders a template and uses it as the prompt for an array-returning API call
func (c *TemplatedClient) CallArrayWithTemplate(templateName string, templateData interface{}, input interface{}, result interface{}) error {
	prompt, err := c.Templates.Render(templateName, templateData)
	if err != nil {
		return fmt.Errorf("error rendering template %s: %w", templateName, err)
	}

	return c.OpenAIClient.CallArray(prompt, input, result)
}

// CallObjectWithTemplate renders a template and uses it as the prompt for an object-returning API call
func (c *TemplatedClient) CallObjectWithTemplate(templateName string, templateData interface{}, input interface{}, result interface{}) error {
	prompt, err := c.Templates.Render(templateName, templateData)
	if err != nil {
		return fmt.Errorf("error rendering template %s: %w", templateName, err)
	}

	return c.OpenAIClient.CallObject(prompt, input, result)
}
