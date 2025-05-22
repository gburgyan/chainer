package template

import (
	"testing"
	"testing/fstest"
)

func TestTemplateManager(t *testing.T) {
	// Create a mock filesystem for testing
	mockFS := fstest.MapFS{
		"test.tmpl": &fstest.MapFile{
			Data: []byte("Hello, {{.Name}}!"),
		},
		"nested/another.tmpl": &fstest.MapFile{
			Data: []byte("This is a {{.Type}} template."),
		},
		"notatemplate.txt": &fstest.MapFile{
			Data: []byte("This is not a template."),
		},
	}

	// Create a template manager with the mock filesystem
	tm := NewManager(mockFS)

	// Load the templates
	err := tm.Load()
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	// Check that the right number of templates were loaded
	if len(tm.templates) != 2 {
		t.Errorf("Expected 2 templates, got %d", len(tm.templates))
	}

	// Check that the templates were loaded correctly
	if !tm.HasTemplate("test") {
		t.Errorf("Expected template 'test' to be loaded")
	}
	if !tm.HasTemplate("another") {
		t.Errorf("Expected template 'another' to be loaded")
	}
	if tm.HasTemplate("notatemplate") {
		t.Errorf("Did not expect non-template file to be loaded")
	}

	// Test rendering a template
	result, err := tm.Render("test", struct{ Name string }{"World"})
	if err != nil {
		t.Fatalf("Failed to render template: %v", err)
	}
	if result != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", result)
	}

	// Test rendering a nested template
	result, err = tm.Render("another", struct{ Type string }{"nested"})
	if err != nil {
		t.Fatalf("Failed to render template: %v", err)
	}
	if result != "This is a nested template." {
		t.Errorf("Expected 'This is a nested template.', got '%s'", result)
	}

	// Test error handling for non-existent template
	_, err = tm.Render("nonexistent", nil)
	if err == nil {
		t.Errorf("Expected error when rendering non-existent template")
	}
}

func TestDefaultManager(t *testing.T) {
	// Test loading the default embedded templates
	tm, err := DefaultManager()
	if err != nil {
		t.Fatalf("Failed to create default template manager: %v", err)
	}

	// Check that expected templates were loaded
	if !tm.HasTemplate("call_naming") {
		t.Errorf("Expected call_naming template to be loaded")
	}
	if !tm.HasTemplate("variable_naming") {
		t.Errorf("Expected variable_naming template to be loaded")
	}
}
