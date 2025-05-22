#!/bin/bash

# This script is specifically designed to debug the variable naming issue
# It runs the chainer with the verbose flag and saves the output to a log file

echo "Running chainer with verbose flag to debug variable naming issues..."

# Path to the HAR file
HAR_FILE="proxy20250219.har"

# Path to the output file
OUTPUT_FILE="collection_debug.json"

# Path to the log file
LOG_FILE="debug_variable_naming.log"

# Run the chainer with verbose logging and save all output to the log file
./chainer -file="$HAR_FILE" -output="$OUTPUT_FILE" -verbose 2>&1 | tee "$LOG_FILE"

echo ""
echo "Debugging complete. Check $LOG_FILE for details."
echo "Look for 'ERROR: Insufficient results from variable name generation' in the log file."
echo "Also check the OpenAI Request and Response sections for the variable naming API call."