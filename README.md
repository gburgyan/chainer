# HAR to Postman Collection Converter

A Go utility that converts HTTP Archive (HAR) files into Postman collections, automatically identifying and creating variables for values passed between requests.

## Features

- Analyzes HAR files to identify chained values (values that appear in a response and are then used in subsequent requests)
- Intelligently names API endpoints and variables using AI
- Generates Postman test scripts to extract response values into variables
- Supports pre-defined variable substitution
- Handles complex JSON paths for reliable value extraction

## Installation

```bash
go get github.com/username/har-to-postman
```

## Usage

```bash
go run . -file=<path_to_har_file> [-vars=<path_to_vars_file>] [-output=collection.json]
```

### Command-Line Options

- `-file`: Path to the HAR file (required)
- `-vars`: Path to a JSON file with pre-defined variables (optional)
- `-output`: Output path for the generated Postman collection (default: `collection.json`)

### Variables JSON Format

The variables file should contain an array of objects with the following structure:

```json
[
  {
    "name": "variableName",
    "search_value": "valueToReplace",
    "initializer": "JavaScript code to initialize the variable (optional)"
  }
]
```

## How It Works

1. Parses the HAR file to extract HTTP requests and responses
2. Identifies values that appear in responses and are later used in requests
3. Uses OpenAI to generate meaningful names for requests and variables
4. Creates Postman test scripts to automatically extract values from responses
5. Replaces hardcoded values in requests with Postman variables
6. Generates a complete Postman collection with the converted requests

## Dependencies

- OpenAI API for intelligent naming (requires an `OPENAI_API_KEY` environment variable)

## Example

```bash
# Export HAR file from your browser's network inspector
# Process the HAR file
go run . -file=api_flow.har -output=my_collection.json

# Import generated collection.json into Postman
```

## Notes

- For information on the HAR file format, see the [HAR 1.2 Spec](http://www.softwareishard.com/blog/har-12-spec/)
- The generated Postman collection follows the [Postman Collection Format v2.1](https://schema.getpostman.com/json/collection/v2.1.0/collection.json)

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE).