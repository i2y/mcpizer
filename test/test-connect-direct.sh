#!/bin/bash

# Test Connect-RPC service directly
echo "Testing Connect-RPC service directly..."

# Call the Eliza service
curl -X POST \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -d '{"sentence": "Hello from MCPizer test"}' \
  https://demo.connectrpc.com/connectrpc.eliza.v1.ElizaService/Say

echo ""
echo ""

# Test with empty sentence
echo "Testing with empty sentence..."
curl -X POST \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -d '{"sentence": ""}' \
  https://demo.connectrpc.com/connectrpc.eliza.v1.ElizaService/Say

echo ""
