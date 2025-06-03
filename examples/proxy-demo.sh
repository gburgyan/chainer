#!/bin/bash

echo "=== Chainer Proxy Mode Demo ==="
echo ""
echo "This demo shows how to use chainer's proxy mode to capture API traffic."
echo ""

# Check if chainer is built
if [ ! -f "./chainer" ]; then
    echo "Building chainer..."
    go build .
fi

echo "1. Starting proxy server on port 8080..."
echo "   Mode: General forward proxy (works with any HTTP/HTTPS endpoint)"
echo "   Recording to: demo-capture.har"
echo ""
echo "Configure your application to use:"
echo "   HTTP_PROXY=http://localhost:8080"
echo "   HTTPS_PROXY=http://localhost:8080"
echo ""
echo "Press Ctrl+C to stop the proxy and save the HAR file."
echo ""

# Start the proxy
./chainer -proxy=8080 -record=demo-capture.har -output=demo-collection.json