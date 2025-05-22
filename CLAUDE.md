# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This repository contains a Go utility that converts HTTP Archive (HAR) files into Postman collections. It automatically identifies and creates variables for values passed between requests, making API flow automation easier.

## Key Features

- Analyzes HAR files to identify chained values (values that appear in a response and are then used in subsequent requests)
- Uses OpenAI to generate meaningful names for API endpoints and variables
- Creates Postman test scripts to extract response values into variables
- Supports pre-defined variable substitution
- Handles complex JSON paths for reliable value extraction

## Commands

### Building and Running

```bash
# Build the application
go build .

# Run with a HAR file
go run . -file=<path_to_har_file>

# Run with variables file
go run . -file=<path_to_har_file> -vars=<path_to_vars_file>

# Specify output file
go run . -file=<path_to_har_file> -output=my_collection.json
```

### Required Environment Variables

The application requires an OpenAI API key to be set as an environment variable:

```bash
export OPENAI_API_KEY=your_api_key_here
```

## Architecture Overview

The application is structured around the following key components:

1. **HAR File Processing**: `har.go` contains the structures and functions for parsing HAR files and extracting values from requests and responses.

2. **Value Chaining Analysis**: The main application logic in `main.go` identifies values that appear in responses and are later used in requests, marking them as "chained values" suitable for variable substitution.

3. **OpenAI Integration**: `openai.go`, `var_names.go`, and `var_paths.go` handle intelligent naming of API endpoints and variables using OpenAI's API. It includes retry logic for API calls.

4. **Postman Collection Generation**: `postman.go` generates the Postman collection structure with appropriate variable substitutions and test scripts for extracting values.

5. **Types and Models**: `types.go` defines the core data structures that represent the extraction and chaining process.

## Core Data Structures

- `ValueReference`: Represents a value extracted from a request or response, with metadata about its location and context.
- `ChainedValueContext`: Represents values shared across multiple HTTP calls, tracking their usage across requests and responses.
- `CallDetails`: Aggregates information for a single HTTP call, including request and response details and chained values.

## Code Conventions

- The code uses a structured approach with clear separation of concerns between HAR parsing, value extraction, and Postman collection generation.
- Error handling is implemented throughout with detailed error messages and retry logic for API calls.
- OpenAI API calls are designed to provide context and examples for generating meaningful names.