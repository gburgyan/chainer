# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This repository contains a Go utility called "chainer" that converts HTTP Archive (HAR) files into Postman collections. It automatically identifies and creates variables for values passed between requests, making API flow automation easier. The tool also includes a built-in HTTP/HTTPS proxy server that can capture and record API traffic to HAR files.

## Key Features

- Analyzes HAR files to identify chained values (values that appear in a response and are then used in subsequent requests)
- Uses OpenAI to generate meaningful names for API endpoints and variables
- Creates Postman test scripts to extract response values into variables
- Supports pre-defined variable substitution
- Handles complex JSON paths for reliable value extraction
- Built-in HTTP/HTTPS proxy server for capturing API traffic
- Records proxy traffic to HAR files for later processing
- Supports automatic Postman collection generation from proxy recordings

## Commands

### Building and Running

```bash
# Build the application
go build .

# HAR Processing Mode
go run . -file=<path_to_har_file>
go run . -file=<path_to_har_file> -vars=<path_to_vars_file>
go run . -file=<path_to_har_file> -output=my_collection.json

# Proxy Mode (general forward proxy)
go run . -proxy=8080 -record=capture.har
go run . -proxy=8080 -record=capture.har -output=collection.json

# Using config file
go run . -config=config.yaml
```

### Required Environment Variables

The application requires an OpenAI API key to be set as an environment variable:

```bash
export OPENAI_API_KEY=your_api_key_here
```

## Architecture Overview

The application is structured around the following key components:

1. **Application Flow**: `cmd/app.go` contains the main application logic that handles both HAR processing and proxy modes. It parses command-line arguments, manages configuration, and coordinates the different components.

2. **HAR File Processing**: `pkg/har/` contains the structures and functions for parsing HAR files and extracting values from requests and responses.

3. **Value Chaining Analysis**: `pkg/util/` identifies values that appear in responses and are later used in requests, marking them as "chained values" suitable for variable substitution.

4. **OpenAI Integration**: `pkg/ai/` handles intelligent naming of API endpoints and variables using OpenAI's API. It includes retry logic for API calls and supports templated prompts.

5. **Postman Collection Generation**: `pkg/postman/` generates the Postman collection structure with appropriate variable substitutions and test scripts for extracting values.

6. **Proxy Server**: `pkg/proxy/` implements a general HTTP/HTTPS forward proxy server that can capture and record API traffic to HAR files. It supports CONNECT tunneling for HTTPS traffic and can proxy requests to any endpoint, making it ideal for capturing complete API workflows across multiple services.

7. **Types and Models**: Various `types.go` files define the core data structures that represent the extraction and chaining process.

## Core Data Structures

- `ValueReference`: Represents a value extracted from a request or response, with metadata about its location and context.
- `ChainedValueContext`: Represents values shared across multiple HTTP calls, tracking their usage across requests and responses.
- `CallDetails`: Aggregates information for a single HTTP call, including request and response details and chained values.

## Code Conventions

- The code uses a structured approach with clear separation of concerns between HAR parsing, value extraction, and Postman collection generation.
- Error handling is implemented throughout with detailed error messages and retry logic for API calls.
- OpenAI API calls are designed to provide context and examples for generating meaningful names.