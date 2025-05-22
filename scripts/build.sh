#!/bin/bash
set -e

# Build script for chainer

# Ensure script is run from the root directory
cd "$(dirname "$0")/.."

echo "Building chainer..."
go build -o bin/chainer .

echo "Running tests..."
go test ./pkg/... ./cmd/... ./test/...

echo "Build and tests completed successfully!"