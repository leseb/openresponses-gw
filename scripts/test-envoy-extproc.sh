#!/bin/bash
# Integration test for Envoy ExtProc adapter
# Tests the full flow: Client → Envoy → ExtProc → Engine → Response
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXAMPLES_DIR="$PROJECT_ROOT/examples/envoy"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
ENVOY_HOST="${ENVOY_HOST:-http://localhost:8080}"
MODEL="${MODEL:-llama3.2:3b}"
TIMEOUT=60

echo -e "${BLUE}==== Envoy ExtProc Integration Tests ====${NC}"
echo -e "${YELLOW}Configuration:${NC}"
echo "  Envoy URL:  $ENVOY_HOST"
echo "  Model:      $MODEL"
echo ""

# Change to examples directory
cd "$EXAMPLES_DIR"

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    docker-compose down -v 2>/dev/null || true
}

# Register cleanup on exit
trap cleanup EXIT

# Start services
echo -e "${YELLOW}Starting Docker Compose stack...${NC}"
docker-compose up -d

echo -e "${YELLOW}Waiting for services to be ready...${NC}"

# Wait for Envoy admin interface
echo -n "Waiting for Envoy admin... "
for i in {1..30}; do
    if curl -sf http://localhost:9901/ready > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Timeout${NC}"
        echo "Envoy logs:"
        docker-compose logs envoy | tail -50
        exit 1
    fi
    sleep 1
done

# Wait for ExtProc
echo -n "Waiting for ExtProc service... "
for i in {1..30}; do
    if docker-compose ps envoy-extproc | grep -q "Up"; then
        echo -e "${GREEN}✓${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Timeout${NC}"
        docker-compose logs envoy-extproc | tail -50
        exit 1
    fi
    sleep 1
done

# Wait for Envoy to be serving
echo -n "Waiting for Envoy proxy... "
for i in {1..30}; do
    if curl -sf "$ENVOY_HOST/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Timeout${NC}"
        exit 1
    fi
    sleep 1
done

echo ""
echo -e "${BLUE}==== Running Tests ====${NC}"
echo ""

# Test counter
PASSED=0
FAILED=0

# Test 1: Basic non-streaming request
echo -n "Test 1: Basic non-streaming request... "
RESPONSE=$(curl -sf -X POST "$ENVOY_HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"Hello\"}" || echo "ERROR")

if echo "$RESPONSE" | jq -e '.id and .object == "response" and .status' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ PASS${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}✗ FAIL${NC}"
    echo "Response: $RESPONSE"
    FAILED=$((FAILED + 1))
fi

# Test 2: Request with system instructions
echo -n "Test 2: Request with instructions... "
RESPONSE=$(curl -sf -X POST "$ENVOY_HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"Say hi\",\"instructions\":\"You are a pirate\"}" || echo "ERROR")

if echo "$RESPONSE" | jq -e '.instructions == "You are a pirate"' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ PASS${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}✗ FAIL${NC}"
    echo "Response: $RESPONSE"
    FAILED=$((FAILED + 1))
fi

# Test 3: Request with tools
echo -n "Test 3: Request with tools... "
RESPONSE=$(curl -sf -X POST "$ENVOY_HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d '{
        "model":"'$MODEL'",
        "input":"What is the weather?",
        "tools":[{
            "type":"function",
            "name":"get_weather",
            "description":"Get weather",
            "parameters":{"type":"object","properties":{"location":{"type":"string"}}}
        }]
    }' || echo "ERROR")

if echo "$RESPONSE" | jq -e '.tools[0].name == "get_weather"' > /dev/null 2>&1; then
    echo -e "${GREEN}✓ PASS${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}✗ FAIL${NC}"
    echo "Response: $RESPONSE"
    FAILED=$((FAILED + 1))
fi

# Test 4: Invalid request (missing model)
echo -n "Test 4: Invalid request handling... "
HTTP_CODE=$(curl -sf -w "%{http_code}" -o /dev/null -X POST "$ENVOY_HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d '{"input":"test"}' || echo "000")

if [ "$HTTP_CODE" == "400" ] || [ "$HTTP_CODE" == "422" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}✗ FAIL (got HTTP $HTTP_CODE)${NC}"
    FAILED=$((FAILED + 1))
fi

# Test 5: Verify response structure matches Open Responses spec
echo -n "Test 5: Response structure validation... "
RESPONSE=$(curl -sf -X POST "$ENVOY_HOST/v1/responses" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"input\":\"Test\"}" || echo "ERROR")

# Check required fields
VALIDATION_PASSED=true
if ! echo "$RESPONSE" | jq -e '.id' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.object == "response"' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.created_at' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.model' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.status' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.output' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.usage' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.tools' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi
if ! echo "$RESPONSE" | jq -e '.temperature' > /dev/null 2>&1; then VALIDATION_PASSED=false; fi

if [ "$VALIDATION_PASSED" == "true" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}✗ FAIL${NC}"
    echo "Response: $RESPONSE"
    FAILED=$((FAILED + 1))
fi

# Test 6: Check Envoy ExtProc stats
echo -n "Test 6: ExtProc filter stats... "
STATS=$(curl -sf http://localhost:9901/stats | grep ext_proc || echo "")
if echo "$STATS" | grep -q "ext_proc"; then
    echo -e "${GREEN}✓ PASS${NC}"
    PASSED=$((PASSED + 1))
    # Show some interesting stats
    echo "  ExtProc stats:"
    echo "$STATS" | grep "ext_proc.response" | head -3 | sed 's/^/    /'
else
    echo -e "${RED}✗ FAIL${NC}"
    FAILED=$((FAILED + 1))
fi

echo ""
echo -e "${BLUE}==== Test Summary ====${NC}"
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"
echo -e "Total:  $((PASSED + FAILED))"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All ExtProc integration tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    echo ""
    echo "Logs for debugging:"
    echo -e "${YELLOW}=== ExtProc Logs ===${NC}"
    docker-compose logs envoy-extproc | tail -30
    echo ""
    echo -e "${YELLOW}=== Envoy Logs ===${NC}"
    docker-compose logs envoy | tail -30
    exit 1
fi
