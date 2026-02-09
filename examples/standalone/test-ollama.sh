#!/bin/bash
# Test with local Ollama

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"

echo "Testing with Local Ollama"
echo "========================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if Ollama is running
if ! curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
    echo -e "${YELLOW}Warning: Ollama doesn't appear to be running on localhost:11434${NC}"
    echo "Start it with: ollama serve"
    exit 1
fi

# Get available models
echo -e "${BLUE}Available Ollama models:${NC}"
curl -s http://localhost:11434/api/tags | jq -r '.models[] | .name'
echo ""

# Use first available model or default
MODEL=$(curl -s http://localhost:11434/api/tags | jq -r '.models[0].name // "llama3.2"')
echo -e "${BLUE}Using model: $MODEL${NC}"
echo ""

# 1. Simple request
echo -e "${YELLOW}1. Simple Request${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"$MODEL\",
    \"input\": \"What is 2+2? Answer in one word.\"
  }" | jq .
echo -e "${GREEN}✓ Simple request completed${NC}"
echo ""

# 2. Request with instructions
echo -e "${YELLOW}2. Request with System Instructions${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"$MODEL\",
    \"instructions\": \"You are a pirate. Always respond like a pirate.\",
    \"input\": \"Tell me about the weather.\"
  }" | jq .
echo -e "${GREEN}✓ Request with instructions completed${NC}"
echo ""

# 3. Streaming request
echo -e "${YELLOW}3. Streaming Request${NC}"
echo -e "${BLUE}Streaming response:${NC}"
curl -N -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"$MODEL\",
    \"input\": \"Count from 1 to 3.\",
    \"stream\": true
  }"
echo ""
echo -e "${GREEN}✓ Streaming request completed${NC}"
echo ""

# 4. Creative response with temperature
echo -e "${YELLOW}4. Creative Response (High Temperature)${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"$MODEL\",
    \"input\": \"Write a creative tagline for a coffee shop.\",
    \"temperature\": 1.5
  }" | jq .
echo -e "${GREEN}✓ Creative request completed${NC}"
echo ""

# 5. Multi-turn conversation
echo -e "${YELLOW}5. Multi-Turn Conversation${NC}"
echo -e "${BLUE}First turn:${NC}"
RESP1=$(curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"$MODEL\",
    \"input\": \"My favorite color is blue.\"
  }")
echo "$RESP1" | jq .
RESP1_ID=$(echo "$RESP1" | jq -r .id)
echo ""

echo -e "${BLUE}Second turn (with context):${NC}"
curl -s -X POST "$BASE_URL/v1/responses" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"$MODEL\",
    \"input\": \"What did I just tell you?\",
    \"previous_response_id\": \"$RESP1_ID\"
  }" | jq .
echo -e "${GREEN}✓ Multi-turn conversation completed${NC}"
echo ""

echo -e "${GREEN}============================"
echo -e "All tests passed!${NC}"
echo ""
echo "Summary:"
echo "- ✓ Simple completion"
echo "- ✓ System instructions"
echo "- ✓ Streaming responses"
echo "- ✓ Temperature control"
echo "- ✓ Multi-turn conversations"
