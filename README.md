# Chainer - HAR to Postman Collection Converter with Proxy Mode

A Go utility that converts HTTP Archive (HAR) files into Postman collections, automatically identifying and creating variables for values passed between requests. Now includes a built-in HTTP proxy to capture and record API traffic.

## Features

- **HAR Processing**: Analyzes HAR files to identify chained values (values that appear in a response and are then used in subsequent requests)
- **Proxy Mode**: Built-in HTTP/HTTPS proxy server to capture and record API traffic to HAR files
- **Intelligent Naming**: Uses AI (OpenAI or Anthropic) to generate meaningful names for API endpoints and variables
- **Variable Extraction**: Generates Postman test scripts to extract response values into variables
- **Pre-defined Variables**: Supports pre-defined variable substitution
- **Complex JSON Paths**: Handles complex JSON paths for reliable value extraction
- **Path Refinement**: Optional AI-powered refinement of JSON paths for better stability across varying API responses

## Installation

```bash
go get github.com/gburgyan/chainer
```

## AI Provider Configuration

Chainer supports multiple AI providers for intelligent naming:

### OpenAI
- Set environment variable: `export OPENAI_API_KEY=sk-...`
- Or specify in config: `provider: "openai"`
- Models: `gpt-4`, `gpt-3.5-turbo`, etc.

### Anthropic
- Set environment variable: `export ANTHROPIC_API_KEY=sk-ant-...`
- Or specify in config: `provider: "anthropic"`
- Models: `claude-3-opus-20240229`, `claude-3-sonnet-20240229`, `claude-3-haiku-20240307`

### Auto-detection
If no provider is specified, Chainer will:
1. Check model name (e.g., "gpt-4" → OpenAI, "claude-3" → Anthropic)
2. Check available API keys in environment
3. Default to OpenAI for backward compatibility

## Usage

### HAR Processing Mode

Process an existing HAR file to create a Postman collection:

```bash
# Basic processing
chainer -file=<path_to_har_file> [-vars=<path_to_vars_file>] [-output=collection.json]

# With complex path refinement for better stability
chainer -file=<path_to_har_file> -refine-paths [-output=collection.json]
```

### Proxy Mode

Start a general HTTP/HTTPS forward proxy to capture traffic from multiple endpoints:

```bash
# Start proxy and record all traffic to HAR file
chainer -proxy=8080 -record=capture.har

# Start proxy, record HAR, and immediately generate Postman collection when stopped
chainer -proxy=8080 -record=capture.har -output=collection.json
```

The proxy acts as a general forward proxy, allowing you to:
- Capture traffic from multiple domains and endpoints
- Record complete API workflows (e.g., authenticate to one service, then use tokens for another)
- Handle both HTTP and HTTPS traffic transparently

### Using Configuration File

```bash
chainer -config=config.yaml
```

### Command-Line Options

#### HAR Processing Options
- `-file`: Path to the HAR file (required for HAR mode)
- `-vars`: Path to a JSON file with pre-defined variables (optional)
- `-output`: Output path for the generated Postman collection (default: `collection.json`)

#### Proxy Options
- `-proxy`: Port to run the proxy server on (e.g., 8080)
- `-record`: Path to save the recorded HAR file

#### General Options
- `-config`: Path to YAML configuration file
- `-verbose`: Enable verbose logging
- `-provider`: AI provider to use: 'openai' or 'anthropic' (auto-detects if not specified)
- `-model`: Specific AI model to use (e.g., 'gpt-4', 'claude-3-haiku-20240307')
- `-refine-paths`: Enable complex JSON path refinement for more stable value extraction

### Configuration File Format

```yaml
# HAR processing configuration
har_file: "path/to/your.har"
vars_file: "path/to/variables.json"  # optional
output: "collection.json"

# AI configuration
ai:
  provider: "anthropic"       # or "openai" (auto-detects if not specified)
  api_key: "sk-your-api-key"  # Or set OPENAI_API_KEY/ANTHROPIC_API_KEY env var
  model: "claude-3-haiku-20240307"  # or "gpt-4", etc.
  max_tokens: 4096            # optional
  verbose: false              # optional
  refine_complex_paths: false # optional - enable complex path refinement

# Proxy configuration
proxy:
  port: 8080
  record: "capture.har"
```

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

### Proxy Mode Usage

1. **Configure your application** to use the proxy:
   ```bash
   export HTTP_PROXY=http://localhost:8080
   export HTTPS_PROXY=http://localhost:8080
   ```

2. **Start the proxy**:
   ```bash
   chainer -proxy=8080 -record=api-traffic.har
   ```

3. **Make your API calls** through the proxy - any HTTP/HTTPS endpoints

4. **Stop the proxy** with Ctrl+C to save the HAR file

5. **Generate Postman collection** (optional):
   ```bash
   chainer -file=api-traffic.har -output=collection.json
   ```

The proxy now works as a general forward proxy, so you can:
- Call multiple different APIs and services
- Capture complete workflows (e.g., auth flow followed by API calls)
- Record traffic from any HTTP/HTTPS endpoint

## Project Structure

The project is organized into several packages:

- `cmd` - Command-line application code
- `pkg/har` - HAR file processing and parsing
- `pkg/util` - Core domain types and chain finding logic
- `pkg/postman` - Postman collection generation
- `pkg/ai` - AI integration for naming (supports OpenAI and Anthropic)
- `pkg/proxy` - HTTP/HTTPS proxy server and HAR recording

## How It Works

1. Parses the HAR file to extract HTTP requests and responses
2. Identifies values that appear in responses and are later used in requests
3. Uses AI (OpenAI or Anthropic) to generate meaningful names for requests and variables
4. (Optional) Refines complex JSON paths to make them more stable across varying API responses
5. Creates Postman test scripts to automatically extract values from responses
6. Replaces hardcoded values in requests with Postman variables
7. Generates a complete Postman collection with the converted requests

### Complex Path Refinement

When enabled with `-refine-paths`, the tool uses AI to analyze JSON response structures and create more stable extraction paths. This is particularly useful for:

- APIs that return arrays where order might change
- Complex nested structures where simple paths might break
- Cases where JSONPath queries would be more reliable than simple dot notation

Example: Instead of `responseJson.Documents[1].DocumentID`, the tool might generate:
`jsonpath.query(responseJson, "Documents[?(@.DocumentType == 'TicketNumber')].DocumentID")`

## Dependencies

- AI API for intelligent naming - supports:
  - OpenAI (requires `OPENAI_API_KEY` environment variable)
  - Anthropic (requires `ANTHROPIC_API_KEY` environment variable)

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o chainer .
```

## Examples

### Processing an Existing HAR File

```bash
# Export HAR file from your browser's network inspector
# Process the HAR file
chainer -file=api_flow.har -output=my_collection.json

# Import generated collection.json into Postman
```

### Recording API Traffic with Proxy

```bash
# Terminal 1: Start the proxy
chainer -proxy=8080 -record=session.har

# Terminal 2: Configure your app to use the proxy
export HTTP_PROXY=http://localhost:8080
export HTTPS_PROXY=http://localhost:8080

# Make your API calls to any endpoints
curl https://api.example.com/login -d '{"user":"test"}'
curl https://api.example.com/users -H "Authorization: Bearer $TOKEN"
curl https://another-api.com/data
curl https://third-service.io/webhook

# Stop proxy with Ctrl+C in Terminal 1
# Process the recorded HAR file
chainer -file=session.har -output=api_collection.json
```

### One-Step Recording and Processing

```bash
# Record traffic and automatically generate Postman collection when stopped
chainer -proxy=8080 -record=session.har -output=collection.json
```

### Multi-Service Workflow Example

```bash
# Start the proxy
chainer -proxy=8080 -record=workflow.har

# In another terminal, run your workflow
export HTTP_PROXY=http://localhost:8080
export HTTPS_PROXY=http://localhost:8080

# 1. Authenticate with auth service
TOKEN=$(curl -X POST https://auth.company.com/oauth/token \
  -d "grant_type=client_credentials" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" | jq -r '.access_token')

# 2. Use token with different API
curl https://api.company.com/v1/users \
  -H "Authorization: Bearer $TOKEN"

# 3. Call a third-party service
curl https://external-api.com/webhook \
  -H "X-API-Key: $API_KEY" \
  -d '{"event": "user.created"}'

# Stop proxy and generate collection
# The Postman collection will automatically detect the token flow!
```

## Notes

- For information on the HAR file format, see the [HAR 1.2 Spec](http://www.softwareishard.com/blog/har-12-spec/)
- The generated Postman collection follows the [Postman Collection Format v2.1](https://schema.getpostman.com/json/collection/v2.1.0/collection.json)

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE).