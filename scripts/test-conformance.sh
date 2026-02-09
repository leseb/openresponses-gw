#!/bin/bash
# Simple wrapper to run Open Responses conformance tests
# Usage: ./scripts/test-conformance.sh [model] [base-url] [api-key]
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFORMANCE_DIR="$PROJECT_ROOT/.conformance-tests"
CONFORMANCE_REPO_URL="https://github.com/openresponses/openresponses.git"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments with defaults
MODEL="${1:-ollama/gpt-oss:20b}"
BASE_URL="${2:-http://localhost:8080}"
API_KEY="${3:-none}"

echo -e "${BLUE}==== Open Responses Conformance Tests ====${NC}"
echo -e "${YELLOW}Configuration:${NC}"
echo "  Base URL: $BASE_URL"
echo "  Model:    $MODEL"
echo "  API Key:  ${API_KEY:0:10}..."
echo ""

# Check for required tools
if ! command -v bun &> /dev/null && ! command -v npx &> /dev/null; then
    echo -e "${RED}Error: Neither bun nor npx found.${NC}"
    echo "Please install Node.js or Bun:"
    echo "  npm install -g bun"
    echo "  or use npx (comes with Node.js)"
    exit 1
fi

# Clone or update conformance test repository
if [ ! -d "$CONFORMANCE_DIR" ]; then
    echo -e "${YELLOW}Cloning conformance test repository...${NC}"
    git clone --depth 1 "$CONFORMANCE_REPO_URL" "$CONFORMANCE_DIR"
    echo ""
else
    echo -e "${YELLOW}Updating conformance test repository...${NC}"
    cd "$CONFORMANCE_DIR"
    git pull origin main --quiet
    cd "$PROJECT_ROOT"
    echo ""
fi

# Install dependencies
echo -e "${YELLOW}Installing conformance test dependencies...${NC}"
cd "$CONFORMANCE_DIR"
if command -v bun &> /dev/null; then
    bun install --quiet 2>/dev/null || bun install
else
    npm install --quiet 2>/dev/null || npm install
fi
cd "$PROJECT_ROOT"
echo ""

# Check if server is accessible
echo -e "${YELLOW}Checking server availability...${NC}"
if ! curl -s "$BASE_URL/health" > /dev/null 2>&1; then
    echo -e "${RED}Error: Server not accessible at $BASE_URL${NC}"
    echo ""
    echo "Please start the server first:"
    echo "  make run"
    echo "  or in another terminal:"
    echo "  ./bin/responses-gateway-server -config config/config.yaml"
    echo ""
    exit 1
fi
echo -e "${GREEN}✓ Server is accessible${NC}"
echo ""

# Run conformance tests
echo -e "${BLUE}==== Running Conformance Tests ====${NC}"
echo ""
cd "$CONFORMANCE_DIR"

if command -v bun &> /dev/null; then
    bun run test:compliance \
        --base-url "$BASE_URL" \
        --api-key "$API_KEY" \
        --model "$MODEL" \
        --verbose
    TEST_EXIT_CODE=$?
else
    npx tsx bin/compliance-test.ts \
        --base-url "$BASE_URL" \
        --api-key "$API_KEY" \
        --model "$MODEL" \
        --verbose
    TEST_EXIT_CODE=$?
fi

cd "$PROJECT_ROOT"
echo ""

# Report results
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ All conformance tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Conformance tests failed (exit code: $TEST_EXIT_CODE)${NC}"
    exit $TEST_EXIT_CODE
fi
