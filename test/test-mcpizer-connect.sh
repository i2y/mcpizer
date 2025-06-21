#!/bin/bash

# Start MCPizer in background
echo "Starting MCPizer..."
MCPIZER_LISTEN_ADDR=:9090 ./mcpizer -config=test/test-connect-config.yaml > /tmp/mcpizer.log 2>&1 &
MCPIZER_PID=$!

# Wait for server to start
echo "Waiting for MCPizer to start..."
sleep 3

# Check if MCPizer is running
if ! ps -p $MCPIZER_PID > /dev/null; then
    echo "MCPizer failed to start. Log:"
    cat /tmp/mcpizer.log
    exit 1
fi

echo "MCPizer started with PID: $MCPIZER_PID"

# Test 1: List tools
echo ""
echo "Test 1: Listing tools..."
curl -X POST http://localhost:9090/sse \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "tools/list", "id": 1}'

echo ""
echo ""

# Test 2: Call ElizaService_Say
echo "Test 2: Calling ElizaService_Say..."
curl -X POST http://localhost:9090/sse \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0", 
    "method": "tools/call", 
    "id": 2,
    "params": {
      "name": "ElizaService_Say",
      "arguments": {
        "sentence": "Hello from MCPizer!"
      }
    }
  }'

echo ""
echo ""

# Stop MCPizer
echo "Stopping MCPizer..."
kill $MCPIZER_PID
wait $MCPIZER_PID 2>/dev/null

echo "Test completed."
