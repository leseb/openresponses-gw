#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Starting Envoy ExtProc Stack ==="

# Start the stack
echo "Starting Docker Compose stack..."
docker-compose up -d

# Wait for services to be healthy
echo "Waiting for services to be healthy..."
sleep 5

# Check Envoy admin interface
echo "Checking Envoy admin interface..."
if ! curl -sf http://localhost:9901/stats > /dev/null; then
    echo "❌ Envoy admin interface not responding"
    docker-compose logs
    exit 1
fi
echo "✓ Envoy admin interface is up"

# Check ExtProc stats
echo "Checking ExtProc connection..."
if ! curl -s http://localhost:9901/stats | grep -q ext_proc; then
    echo "⚠️  ExtProc stats not found yet, waiting..."
    sleep 5
fi
echo "✓ ExtProc filter is configured"

# Pull Ollama model if needed
echo "Ensuring Ollama model is available..."
docker-compose exec -T ollama ollama pull llama3.2:3b || echo "Model may already exist"
sleep 2

# Test non-streaming request
echo ""
echo "=== Testing Non-Streaming Request ==="
RESPONSE=$(curl -s -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:3b",
    "input": "What is 2+2? Answer with just the number."
  }')

echo "Response received:"
echo "$RESPONSE" | jq .

# Validate response structure
if ! echo "$RESPONSE" | jq -e '.id' > /dev/null; then
    echo "❌ Response missing 'id' field"
    exit 1
fi
echo "✓ Response has 'id' field"

if ! echo "$RESPONSE" | jq -e '.status' > /dev/null; then
    echo "❌ Response missing 'status' field"
    exit 1
fi
echo "✓ Response has 'status' field"

if ! echo "$RESPONSE" | jq -e '.output' > /dev/null; then
    echo "❌ Response missing 'output' field"
    exit 1
fi
echo "✓ Response has 'output' field"

if ! echo "$RESPONSE" | jq -e '.usage' > /dev/null; then
    echo "❌ Response missing 'usage' field"
    exit 1
fi
echo "✓ Response has 'usage' field"

# Check status
STATUS=$(echo "$RESPONSE" | jq -r '.status')
if [ "$STATUS" != "completed" ]; then
    echo "❌ Expected status 'completed', got '$STATUS'"
    exit 1
fi
echo "✓ Response status is 'completed'"

# Test error handling - missing model field
echo ""
echo "=== Testing Error Handling (Missing Model) ==="
ERROR_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"input": "test"}')

echo "Error response received:"
echo "$ERROR_RESPONSE" | jq .

if ! echo "$ERROR_RESPONSE" | jq -e '.error.type' > /dev/null; then
    echo "❌ Error response missing 'error.type' field"
    exit 1
fi
echo "✓ Error response has proper format"

ERROR_TYPE=$(echo "$ERROR_RESPONSE" | jq -r '.error.type')
if [ "$ERROR_TYPE" != "invalid_request_error" ]; then
    echo "❌ Expected error type 'invalid_request_error', got '$ERROR_TYPE'"
    exit 1
fi
echo "✓ Error type is correct"

# Test error handling - malformed JSON
echo ""
echo "=== Testing Error Handling (Malformed JSON) ==="
MALFORMED_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{invalid json}')

echo "Malformed JSON response:"
echo "$MALFORMED_RESPONSE" | jq .

if ! echo "$MALFORMED_RESPONSE" | jq -e '.error.type' > /dev/null; then
    echo "❌ Malformed JSON response missing 'error.type' field"
    exit 1
fi
echo "✓ Malformed JSON error response has proper format"

MALFORMED_ERROR_TYPE=$(echo "$MALFORMED_RESPONSE" | jq -r '.error.type')
if [ "$MALFORMED_ERROR_TYPE" != "bad_request" ]; then
    echo "❌ Expected error type 'bad_request', got '$MALFORMED_ERROR_TYPE'"
    exit 1
fi
echo "✓ Malformed JSON error type is correct"

echo ""
echo "=== All Tests Passed! ==="
echo ""
echo "Stack is running. To view logs:"
echo "  docker-compose logs -f"
echo ""
echo "To stop the stack:"
echo "  docker-compose down"
echo ""
echo "To test manually:"
echo "  curl -X POST http://localhost:8080/v1/responses \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"model\":\"llama3.2:3b\",\"input\":\"Hello!\"}'"
