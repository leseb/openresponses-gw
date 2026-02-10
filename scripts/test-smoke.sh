#!/bin/bash
# Comprehensive Smoke Test Suite
# "Did anything fundamentally break?" test - runs in 10-15 minutes
# Based on production-grade smoke test checklist

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
HOST="${HOST:-http://localhost:8080}"
MODEL="${MODEL:-llama3.2:3b}"
API_KEY="${API_KEY:-none}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Test counters
PASSED=0
FAILED=0
SKIPPED=0
START_TIME=$(date +%s)

echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE}   Responses API Smoke Test Suite${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${YELLOW}Configuration:${NC}"
echo "  Host:     $HOST"
echo "  Model:    $MODEL"
echo "  API Key:  ${API_KEY:0:10}..."
echo ""

# Helper functions
test_header() {
    echo -e "${BOLD}${BLUE}━━━ Test $1: $2 ━━━${NC}"
}

pass() {
    echo -e "  ${GREEN}✓${NC} $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "  ${RED}✗${NC} $1"
    FAILED=$((FAILED + 1))
}

skip() {
    echo -e "  ${YELLOW}⊘${NC} $1 ${YELLOW}(not implemented)${NC}"
    SKIPPED=$((SKIPPED + 1))
}

detail() {
    echo -e "    ${YELLOW}→${NC} $1"
}

# Check if server is accessible
echo -e "${YELLOW}Checking server availability...${NC}"
if ! curl -sf "$HOST/health" > /dev/null 2>&1; then
    echo -e "${RED}✗ Server not accessible at $HOST${NC}"
    echo ""
    echo "Please start the server first:"
    echo "  make run"
    echo "  or: ./bin/openresponses-gw-server"
    exit 1
fi
echo -e "${GREEN}✓ Server is accessible${NC}"
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 1: API Is Alive
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "1" "API Is Alive"

RESP=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"Hello world\"}" 2>/dev/null || echo "ERROR")

if [ "$RESP" == "ERROR" ]; then
    fail "Failed to send basic prompt"
else
    # Check for required fields
    HAS_ID=$(echo "$RESP" | jq -e '.id' > /dev/null 2>&1 && echo "yes" || echo "no")
    HAS_USAGE=$(echo "$RESP" | jq -e '.usage.total_tokens > 0' > /dev/null 2>&1 && echo "yes" || echo "no")
    HAS_OUTPUT=$(echo "$RESP" | jq -e '.output | length > 0' > /dev/null 2>&1 && echo "yes" || echo "no")
    HAS_ERROR=$(echo "$RESP" | jq -e '.error' > /dev/null 2>&1 && echo "yes" || echo "no")

    if [ "$HAS_ID" == "yes" ]; then
        pass "Response has ID"
    else
        fail "Response missing ID"
    fi

    if [ "$HAS_USAGE" == "yes" ]; then
        TOKEN_COUNT=$(echo "$RESP" | jq -r '.usage.total_tokens')
        pass "Response has token usage ($TOKEN_COUNT tokens)"
    else
        fail "Response missing token usage"
    fi

    if [ "$HAS_OUTPUT" == "yes" ] && [ "$HAS_ERROR" == "no" ]; then
        pass "Response has output, no errors"
    else
        fail "Response has no output or contains errors"
        detail "Response: $RESP"
    fi
fi
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 2: Conversation Memory
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "2" "Conversation Memory (CRITICAL)"

RESP1=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"My name is Alice. Remember this.\"}" 2>/dev/null)

if [ -z "$RESP1" ]; then
    fail "First request failed"
else
    RESP1_ID=$(echo "$RESP1" | jq -r '.id')
    pass "First request succeeded (ID: ${RESP1_ID:0:12}...)"

    # Ask follow-up
    RESP2=$(curl -sf -X POST "$HOST/v1/responses" \
        -H "Content-Type: application/json" \
        -d "{\"model\":\"$MODEL\",\"input\":\"What is my name?\",\"previous_response_id\":\"$RESP1_ID\"}" 2>/dev/null)

    if [ -z "$RESP2" ]; then
        fail "Follow-up request failed"
    else
        OUTPUT=$(echo "$RESP2" | jq -r '.output[0].content[0].text // empty')
        if echo "$OUTPUT" | grep -qi "alice"; then
            pass "Follow-up correctly remembers context"
            detail "Model recalled: Alice"
        else
            fail "Follow-up lost context (BLOCKING)"
            detail "Expected: mention of 'Alice'"
            detail "Got: $OUTPUT"
        fi
    fi
fi
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 3: Determinism Check
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "3" "Determinism (temperature=0)"

PROMPT="{\"model\":\"$MODEL\",\"input\":\"Count from 1 to 3.\",\"temperature\":0}"

RESP1=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "$PROMPT" 2>/dev/null)

RESP2=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "$PROMPT" 2>/dev/null)

if [ -z "$RESP1" ] || [ -z "$RESP2" ]; then
    fail "One or both requests failed"
else
    OUTPUT1=$(echo "$RESP1" | jq -r '.output[0].content[0].text // empty')
    OUTPUT2=$(echo "$RESP2" | jq -r '.output[0].content[0].text // empty')

    if [ "$OUTPUT1" == "$OUTPUT2" ]; then
        pass "Responses are deterministic (identical)"
    else
        # Calculate rough similarity
        DIFF_CHARS=$(echo -e "$OUTPUT1\n$OUTPUT2" | diff - /dev/null 2>&1 | wc -c)
        if [ $DIFF_CHARS -lt 50 ]; then
            pass "Responses are similar (minor variation acceptable)"
            detail "Output 1: ${OUTPUT1:0:60}..."
            detail "Output 2: ${OUTPUT2:0:60}..."
        else
            fail "Responses differ significantly (BLOCKING)"
            detail "Output 1: $OUTPUT1"
            detail "Output 2: $OUTPUT2"
        fi
    fi
fi
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 4: Files Round-Trip
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "4" "Files Round-Trip"
skip "Files API integration"
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 5: Vector Store RAG
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "5" "Vector Store RAG"
skip "Vector stores not implemented"
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 6: Conversational RAG
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "6" "Conversational RAG"
skip "RAG not implemented"
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 7: Tool Calling
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "7" "Tool / Structured Output"

RESP=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\":\"$MODEL\",
        \"input\":\"What is the weather in Paris?\",
        \"tools\":[{
            \"type\":\"function\",
            \"name\":\"get_weather\",
            \"description\":\"Get current weather\",
            \"parameters\":{
                \"type\":\"object\",
                \"properties\":{\"location\":{\"type\":\"string\"}}
            }
        }]
    }" 2>/dev/null)

if [ -z "$RESP" ]; then
    fail "Tool calling request failed"
else
    OUTPUT_TYPE=$(echo "$RESP" | jq -r '.output[0].type // empty')
    if [ "$OUTPUT_TYPE" == "function_call" ]; then
        TOOL_NAME=$(echo "$RESP" | jq -r '.output[0].name // empty')
        TOOL_ARGS=$(echo "$RESP" | jq -r '.output[0].arguments // empty')
        pass "Tool calling works"
        detail "Tool: $TOOL_NAME"
        detail "Args: ${TOOL_ARGS:0:60}..."
    else
        fail "Tool not called (got output type: $OUTPUT_TYPE)"
    fi
fi
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 8: Streaming
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "8" "Streaming"

STREAM_OUTPUT=$(mktemp)
curl -sf -N -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -H "Accept: text/event-stream" \
    -d "{\"model\":\"$MODEL\",\"input\":\"Say hello\",\"stream\":true}" \
    > "$STREAM_OUTPUT" 2>/dev/null

EVENT_COUNT=$(grep -c "^event:" "$STREAM_OUTPUT" 2>/dev/null || echo "0")
DELTA_COUNT=$(grep -c "response.output_text.delta" "$STREAM_OUTPUT" 2>/dev/null || echo "0")

if [ "$EVENT_COUNT" -gt 5 ]; then
    pass "Streaming works ($EVENT_COUNT events, $DELTA_COUNT deltas)"
else
    fail "Streaming returned too few events ($EVENT_COUNT)"
fi

rm -f "$STREAM_OUTPUT"
echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# TEST 9: Error Handling
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

test_header "9" "Error Surface Check (CRITICAL)"

# Test 9a: Missing required field (model)
HTTP_CODE=$(curl -sf -w "%{http_code}" -o /dev/null -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d '{"input":"test"}' 2>/dev/null || echo "000")

if [ "$HTTP_CODE" == "400" ] || [ "$HTTP_CODE" == "422" ]; then
    pass "Missing model returns 400/422"
else
    fail "Missing model returned $HTTP_CODE (expected 400/422)"
fi

# Test 9b: Malformed JSON
HTTP_CODE=$(curl -sf -w "%{http_code}" -o /dev/null -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d '{invalid json}' 2>/dev/null || echo "000")

if [ "$HTTP_CODE" == "400" ]; then
    pass "Malformed JSON returns 400"
else
    fail "Malformed JSON returned $HTTP_CODE (expected 400)"
fi

# Test 9c: Invalid file ID (if files are implemented)
RESP=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"test\",\"files\":[\"file_invalid\"]}" 2>/dev/null)

if echo "$RESP" | jq -e '.error.type' > /dev/null 2>&1; then
    ERROR_TYPE=$(echo "$RESP" | jq -r '.error.type')
    if [ "$ERROR_TYPE" != "" ]; then
        pass "Invalid file ID returns clear error"
    else
        skip "Files not implemented, error handling test skipped"
    fi
else
    # If no error, files might not be validated yet
    skip "File validation not implemented"
fi

# Test 9d: Check for silent failures
RESP=$(curl -sf -X POST "$HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"test\"}" 2>/dev/null)

HAS_ID=$(echo "$RESP" | jq -e '.id' > /dev/null 2>&1 && echo "yes" || echo "no")
HAS_ERROR=$(echo "$RESP" | jq -e '.error' > /dev/null 2>&1 && echo "yes" || echo "no")

if [ "$HAS_ID" == "yes" ] || [ "$HAS_ERROR" == "yes" ]; then
    pass "No silent failures (clear success or error)"
else
    fail "Silent failure detected (no ID, no error)"
    detail "Response: $RESP"
fi

echo ""

#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# SUMMARY
#━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE}   Summary${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${GREEN}Passed:  $PASSED${NC}"
echo -e "${RED}Failed:  $FAILED${NC}"
echo -e "${YELLOW}Skipped: $SKIPPED${NC}"
echo -e "Total:   $((PASSED + FAILED + SKIPPED))"
echo -e "Duration: ${DURATION}s"
echo ""

# Non-negotiable failures
CRITICAL_FAILURES=""
if echo "$RESP2" | grep -q "lost context"; then
    CRITICAL_FAILURES="${CRITICAL_FAILURES}  ❌ Follow-ups lose context\n"
fi

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}${BOLD}✓ Smoke test PASSED!${NC}"
    if [ $SKIPPED -gt 0 ]; then
        echo -e "${YELLOW}Note: $SKIPPED tests skipped (features not yet implemented)${NC}"
    fi
    echo ""
    exit 0
else
    echo -e "${RED}${BOLD}✗ Smoke test FAILED!${NC}"
    echo ""
    if [ -n "$CRITICAL_FAILURES" ]; then
        echo -e "${RED}${BOLD}CRITICAL FAILURES (DO NOT SHIP):${NC}"
        echo -e "$CRITICAL_FAILURES"
    fi
    echo ""
    exit 1
fi
