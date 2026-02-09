#!/bin/bash
# Minimal smoke test for Responses API - validates schema + implementation

set -e

HOST="${HOST:-http://localhost:8080}"
MODEL="${MODEL:-llama3.2:3b}"

echo "━━━ Minimal Responses API Test ━━━"
echo "Host: $HOST"
echo "Model: $MODEL"
echo ""

# Test 1: Create response
echo "1. Creating response..."
RESP=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"Say: Hello World\"}")

RESP_ID=$(echo "$RESP" | jq -r '.id')
echo "   ✓ Created: $RESP_ID"

# Validate response structure
if ! echo "$RESP" | jq -e '.id and .status and .output' > /dev/null; then
    echo "   ✗ Invalid response structure"
    echo "$RESP" | jq .
    exit 1
fi
echo "   ✓ Response structure valid"

# Test 2: Retrieve response (NEW endpoint)
echo ""
echo "2. Retrieving response..."
GET_RESP=$(curl -sf -X GET "$HOST/v1/responses/$RESP_ID")

GET_ID=$(echo "$GET_RESP" | jq -r '.id')
if [ "$GET_ID" != "$RESP_ID" ]; then
    echo "   ✗ ID mismatch: expected $RESP_ID, got $GET_ID"
    exit 1
fi
echo "   ✓ Retrieved: $GET_ID"

# Test 3: Verify chat completion was called
echo ""
echo "3. Validating LLM response..."
OUTPUT=$(echo "$RESP" | jq -r '.output[0].content[0].text // empty')
if [ -z "$OUTPUT" ]; then
    echo "   ✗ No output text received"
    exit 1
fi

if echo "$OUTPUT" | grep -qi "hello"; then
    echo "   ✓ LLM responded: ${OUTPUT:0:50}..."
else
    echo "   ⚠ LLM response unexpected: $OUTPUT"
fi

# Test 4: Verify usage stats (proves actual LLM call)
echo ""
echo "4. Validating token usage..."
TOTAL_TOKENS=$(echo "$RESP" | jq -r '.usage.total_tokens // 0')
if [ "$TOTAL_TOKENS" -gt 0 ]; then
    echo "   ✓ Tokens used: $TOTAL_TOKENS (LLM was called)"
else
    echo "   ✗ No token usage - LLM may not have been called"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ All tests passed!"
echo "   • POST /v1/responses works"
echo "   • GET /v1/responses/{id} works"
echo "   • Chat completions translation works"
echo "   • LLM backend responds correctly"
