#!/bin/bash

# Test the verbose logging feature
echo "Running chainer with verbose flag..."

# Path to the HAR file (use the proxy HAR file as an example)
HAR_FILE="proxy20250219.har"

# Path to the output file
OUTPUT_FILE="collection_verbose_test.json"

# Run the chainer with verbose logging
./chainer -file="$HAR_FILE" -output="$OUTPUT_FILE" -verbose