package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gburgyan/chainer/cmd"
	"github.com/gburgyan/chainer/pkg/ai"
	"github.com/gburgyan/chainer/pkg/postman"
)

// TestEndToEndFlow tests the entire flow from HAR to Postman collection
// This test requires the OPENAI_API_KEY environment variable to be set
func TestEndToEndFlow(t *testing.T) {
	// Skip if OPENAI_API_KEY is not set to avoid failing in CI environments
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	// Create a temp directory for the test
	tempDir, err := os.MkdirTemp("", "chainer-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test HAR file
	harFilePath := filepath.Join(tempDir, "test.har")
	if err := os.WriteFile(harFilePath, []byte(testHAR), 0644); err != nil {
		t.Fatalf("Failed to write test HAR file: %v", err)
	}

	// Create test variables file
	varsFilePath := filepath.Join(tempDir, "vars.json")
	if err := os.WriteFile(varsFilePath, []byte(testVars), 0644); err != nil {
		t.Fatalf("Failed to write test variables file: %v", err)
	}

	// Output path for the Postman collection
	outputPath := filepath.Join(tempDir, "collection.json")

	// Create application config
	config := &cmd.Config{
		HarFilePath:  harFilePath,
		VarsFilePath: varsFilePath,
		OutputPath:   outputPath,
		AIConfig:     ai.DefaultConfig(),
	}

	// Run the application
	app := cmd.NewApp(config)
	if err := app.Run(); err != nil {
		t.Fatalf("Application run failed: %v", err)
	}

	// Verify that the output file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("Output file was not created: %v", err)
	}

	// Read and parse the output file
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	var collection postman.PostmanCollection
	if err := json.Unmarshal(outputData, &collection); err != nil {
		t.Fatalf("Failed to parse output as Postman collection: %v", err)
	}

	// Verify that the collection has the expected items
	if len(collection.Item) != 2 {
		t.Errorf("Expected 2 items in collection, got %d", len(collection.Item))
	}

	// Verify that variables were created
	if len(collection.Variables) == 0 {
		t.Errorf("Expected variables in collection, got none")
	}

	// Check that the predefined variable was included
	foundPredefined := false
	for _, v := range collection.Variables {
		if v.Key == "apiKey" {
			foundPredefined = true
			break
		}
	}
	if !foundPredefined {
		t.Errorf("Predefined variable 'apiKey' not found in collection")
	}

	// Verify that test scripts were created
	for _, item := range collection.Item {
		if len(item.Event) == 0 {
			t.Errorf("Expected item %s to have events, got none", item.Name)
		}
	}
}

// Test HAR file with two requests where the second request uses a value from the first response
const testHAR = `{
  "log": {
    "entries": [
      {
        "request": {
          "method": "GET",
          "url": "https://api.example.com/users",
          "headers": [
            {
              "name": "Accept",
              "value": "application/json"
            }
          ]
        },
        "response": {
          "status": 200,
          "statusText": "OK",
          "content": {
            "mimeType": "application/json",
            "text": "{\"users\":[{\"id\":12345,\"name\":\"Test User\"}]}"
          },
          "headers": [
            {
              "name": "Content-Type",
              "value": "application/json"
            }
          ]
        }
      },
      {
        "request": {
          "method": "GET",
          "url": "https://api.example.com/users/12345/details",
          "headers": [
            {
              "name": "Accept",
              "value": "application/json"
            },
            {
              "name": "Authorization",
              "value": "Bearer api-key-12345"
            }
          ]
        },
        "response": {
          "status": 200,
          "statusText": "OK",
          "content": {
            "mimeType": "application/json",
            "text": "{\"user\":{\"id\":12345,\"name\":\"Test User\",\"email\":\"test@example.com\"}}"
          },
          "headers": [
            {
              "name": "Content-Type",
              "value": "application/json"
            }
          ]
        }
      }
    ]
  }
}`

// Test variables file with a predefined variable
const testVars = `[
  {
    "name": "apiKey",
    "search_value": "api-key-12345",
    "initializer": "result = 'test-api-key';"
  }
]`
