#!/bin/bash

echo "=== Testing Chainer Proxy ==="
echo ""
echo "This script sends test requests through the chainer proxy."
echo "Make sure the proxy is running on port 8080 first!"
echo ""

# Configure proxy
export HTTP_PROXY=http://localhost:8080
export HTTPS_PROXY=http://localhost:8080

echo "1. Testing GET request..."
curl -s https://httpbin.org/get | jq '.'

echo ""
echo "2. Testing POST request with JSON..."
curl -s -X POST https://httpbin.org/post \
  -H "Content-Type: application/json" \
  -d '{"name": "test", "value": 123}' | jq '.'

echo ""
echo "3. Testing request with headers..."
curl -s https://httpbin.org/headers \
  -H "X-Custom-Header: test-value" \
  -H "Authorization: Bearer test-token" | jq '.'

echo ""
echo "Done! Check the proxy output to see the captured requests."