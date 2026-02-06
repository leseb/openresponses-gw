#!/bin/bash
# Test requests for the Responses API Gateway

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"

echo "Testing OpenAI Responses Gateway"
echo "================================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 1. Health check
echo -e "${YELLOW}1. Health Check${NC}"
curl -s "$BASE_URL/health" | jq .
echo -e "${GREEN}✓ Health check passed${NC}"
echo ""

# 2. Simple non-streaming request
echo -e "${YELLOW}2. Simple Non-Streaming Request${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "What is 2+2?"
  }' | jq .
echo -e "${GREEN}✓ Non-streaming request completed${NC}"
echo ""

# 3. Streaming request
echo -e "${YELLOW}3. Streaming Request${NC}"
curl -N -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Write a short poem about programming",
    "stream": true
  }'
echo ""
echo -e "${GREEN}✓ Streaming request completed${NC}"
echo ""

# 4. Multi-turn conversation
echo -e "${YELLOW}4. Multi-Turn Conversation${NC}"
# First request
echo "First request:"
RESP1=$(curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "What is the capital of France?"
  }')
echo "$RESP1" | jq .
RESP1_ID=$(echo "$RESP1" | jq -r .id)
echo ""

# Follow-up request
echo "Follow-up request (previous_response_id: $RESP1_ID):"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"gpt-4\",
    \"input\": \"What about Germany?\",
    \"previous_response_id\": \"$RESP1_ID\"
  }" | jq .
echo -e "${GREEN}✓ Multi-turn conversation completed${NC}"
echo ""

# 5. Request with metadata
echo -e "${YELLOW}5. Request with Metadata${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Hello",
    "metadata": {
      "user_id": "user123",
      "session_id": "sess456"
    }
  }' | jq .
echo -e "${GREEN}✓ Request with metadata completed${NC}"
echo ""

echo -e "${GREEN}================================="
echo -e "All tests passed!${NC}"
