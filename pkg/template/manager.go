package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"text/template"
)

// TemplateManager handles loading and rendering of template files
type TemplateManager struct {
	// FS is the filesystem containing the templates
	FS fs.FS

	// Cache of loaded templates
	templates map[string]*template.Template
}

// NewManager creates a new template manager with the given filesystem
func NewManager(fsys fs.FS) *TemplateManager {
	return &TemplateManager{
		FS:        fsys,
		templates: make(map[string]*template.Template),
	}
}

// Load loads all templates from the filesystem
func (tm *TemplateManager) Load() error {
	return fs.WalkDir(tm.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking directory: %w", err)
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip non-template files
		if !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		// Read the template file
		content, err := fs.ReadFile(tm.FS, path)
		if err != nil {
			return fmt.Errorf("error reading template file %s: %w", path, err)
		}

		// Parse the template
		tmpl, err := template.New(filepath.Base(path)).Parse(string(content))
		if err != nil {
			return fmt.Errorf("error parsing template %s: %w", path, err)
		}

		// Store the parsed template
		name := strings.TrimSuffix(filepath.Base(path), ".tmpl")
		tm.templates[name] = tmpl
		return nil
	})
}

// Render renders a template with the given data
func (tm *TemplateManager) Render(name string, data interface{}) (string, error) {
	tmpl, ok := tm.templates[name]
	if !ok {
		return "", fmt.Errorf("template %s not found", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template %s: %w", name, err)
	}

	return buf.String(), nil
}

// GetTemplate returns the named template
func (tm *TemplateManager) GetTemplate(name string) (*template.Template, error) {
	tmpl, ok := tm.templates[name]
	if !ok {
		return nil, fmt.Errorf("template %s not found", name)
	}
	return tmpl, nil
}

// HasTemplate checks if a template exists
func (tm *TemplateManager) HasTemplate(name string) bool {
	_, ok := tm.templates[name]
	return ok
}
