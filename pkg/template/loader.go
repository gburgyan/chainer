package template

import (
	"embed"
)

//go:embed templates/*.tmpl
var TemplateFS embed.FS

// DefaultManager returns a template manager with the embedded templates
func DefaultManager() (*TemplateManager, error) {
	tm := NewManager(TemplateFS)
	if err := tm.Load(); err != nil {
		return nil, err
	}
	return tm, nil
}
