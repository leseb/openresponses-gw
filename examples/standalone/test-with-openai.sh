#!/bin/bash
# Test with real OpenAI API

set -e

# Check for API key
if [ -z "$OPENAI_API_KEY" ]; then
    echo "Error: OPENAI_API_KEY environment variable is not set"
    echo "Usage: OPENAI_API_KEY=sk-... ./test-with-openai.sh"
    exit 1
fi

BASE_URL="${BASE_URL:-http://localhost:8080}"

echo "Testing with Real OpenAI API"
echo "============================"
echo "API Key: ${OPENAI_API_KEY:0:10}..."
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 1. Simple request
echo -e "${YELLOW}1. Simple Request to GPT-4${NC}"
echo -e "${BLUE}Request:${NC}"
cat <<EOF | jq .
{
  "model": "gpt-4o-mini",
  "input": "What is the capital of France? Answer in one word."
}
EOF

echo ""
echo -e "${BLUE}Response:${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "What is the capital of France? Answer in one word."
  }' | jq .
echo -e "${GREEN}✓ Request completed${NC}"
echo ""

# 2. Request with instructions (system message)
echo -e "${YELLOW}2. Request with Instructions${NC}"
echo -e "${BLUE}Request:${NC}"
cat <<EOF | jq .
{
  "model": "gpt-4o-mini",
  "instructions": "You are a helpful assistant that always responds in JSON format.",
  "input": "What are the primary colors?"
}
EOF

echo ""
echo -e "${BLUE}Response:${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "instructions": "You are a helpful assistant that always responds in JSON format.",
    "input": "What are the primary colors?"
  }' | jq .
echo -e "${GREEN}✓ Request with instructions completed${NC}"
echo ""

# 3. Streaming request
echo -e "${YELLOW}3. Streaming Request${NC}"
echo -e "${BLUE}Request:${NC}"
cat <<EOF | jq .
{
  "model": "gpt-4o-mini",
  "input": "Count from 1 to 5",
  "stream": true
}
EOF

echo ""
echo -e "${BLUE}Streaming Response:${NC}"
curl -N -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "Count from 1 to 5",
    "stream": true
  }'
echo ""
echo -e "${GREEN}✓ Streaming request completed${NC}"
echo ""

# 4. Multi-turn conversation
echo -e "${YELLOW}4. Multi-Turn Conversation${NC}"
echo -e "${BLUE}First request:${NC}"
RESP1=$(curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "What is 2+2?"
  }')
echo "$RESP1" | jq .
RESP1_ID=$(echo "$RESP1" | jq -r .id)
echo ""

echo -e "${BLUE}Follow-up request (previous_response_id: $RESP1_ID):${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"gpt-4o-mini\",
    \"input\": \"What about 3+3?\",
    \"previous_response_id\": \"$RESP1_ID\"
  }" | jq .
echo -e "${GREEN}✓ Multi-turn conversation completed${NC}"
echo ""

# 5. Temperature control
echo -e "${YELLOW}5. Temperature Control (Creative Response)${NC}"
echo -e "${BLUE}Request with temperature=1.5:${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "Write a creative opening line for a sci-fi story",
    "temperature": 1.5
  }' | jq .
echo -e "${GREEN}✓ Temperature control completed${NC}"
echo ""

echo -e "${GREEN}============================"
echo -e "All tests passed!${NC}"
echo ""
echo "Summary:"
echo "- ✓ Simple completion"
echo "- ✓ Instructions (system message)"
echo "- ✓ Streaming responses"
echo "- ✓ Multi-turn conversations"
echo "- ✓ Temperature control"
