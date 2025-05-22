#!/bin/bash
set -e

# Run example script for chainer

# Ensure script is run from the root directory
cd "$(dirname "$0")/.."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build the application
go build -o bin/chainer .

# Check if OPENAI_API_KEY is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "Error: OPENAI_API_KEY environment variable is not set."
    echo "Please set it with: export OPENAI_API_KEY=your_api_key"
    exit 1
fi

# Check if a HAR file was provided
if [ -z "$1" ]; then
    echo "Error: No HAR file provided."
    echo "Usage: $0 <path_to_har_file> [<path_to_vars_file>] [<output_file>]"
    exit 1
fi

HAR_FILE="$1"
VARS_FILE="$2"
OUTPUT_FILE="${3:-collection.json}"

# Run the application
if [ -n "$VARS_FILE" ]; then
    echo "Running with HAR file $HAR_FILE and variables file $VARS_FILE..."
    ./bin/chainer -file="$HAR_FILE" -vars="$VARS_FILE" -output="$OUTPUT_FILE"
else
    echo "Running with HAR file $HAR_FILE..."
    ./bin/chainer -file="$HAR_FILE" -output="$OUTPUT_FILE"
fi

echo "Postman collection generated at: $OUTPUT_FILE"