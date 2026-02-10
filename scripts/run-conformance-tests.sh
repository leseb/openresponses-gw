#!/bin/bash
# Run Open Responses conformance tests against local server
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SERVER_PORT="${SERVER_PORT:-8080}"
CONFORMANCE_REPO_URL="https://github.com/openresponses/openresponses.git"
CONFORMANCE_DIR="$PROJECT_ROOT/.conformance-tests"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}==== Open Responses Conformance Tests ====${NC}"

# Check for required tools
if ! command -v bun &> /dev/null && ! command -v npx &> /dev/null; then
    echo -e "${RED}Error: Neither bun nor npx found. Please install Node.js or Bun.${NC}"
    echo "  npm install -g bun"
    echo "  or use npx (comes with Node.js)"
    exit 1
fi

# Clone or update conformance test repository
if [ ! -d "$CONFORMANCE_DIR" ]; then
    echo -e "${YELLOW}Cloning conformance test repository...${NC}"
    git clone --depth 1 "$CONFORMANCE_REPO_URL" "$CONFORMANCE_DIR"
else
    echo -e "${YELLOW}Updating conformance test repository...${NC}"
    cd "$CONFORMANCE_DIR"
    git pull origin main
    cd "$PROJECT_ROOT"
fi

# Install dependencies in conformance test repo
echo -e "${YELLOW}Installing conformance test dependencies...${NC}"
cd "$CONFORMANCE_DIR"
if command -v bun &> /dev/null; then
    bun install
else
    npm install
fi
cd "$PROJECT_ROOT"

# Check if server binary exists
if [ ! -f "$PROJECT_ROOT/bin/openresponses-gw-server" ]; then
    echo -e "${RED}Error: Server binary not found. Run 'make build-server' first.${NC}"
    exit 1
fi

# Start server in background
echo -e "${YELLOW}Starting server on port $SERVER_PORT...${NC}"
SERVER_LOG="$PROJECT_ROOT/.server-test.log"
SERVER_PID_FILE="$PROJECT_ROOT/.server-test.pid"

# Clean up any existing server
if [ -f "$SERVER_PID_FILE" ]; then
    OLD_PID=$(cat "$SERVER_PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Stopping existing server (PID: $OLD_PID)..."
        kill "$OLD_PID" || true
        sleep 2
    fi
    rm -f "$SERVER_PID_FILE"
fi

# Start new server
"$PROJECT_ROOT/bin/openresponses-gw-server" \
    -config "$PROJECT_ROOT/config/config.yaml" \
    -port "$SERVER_PORT" \
    > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!
echo $SERVER_PID > "$SERVER_PID_FILE"

# Cleanup function
cleanup() {
    if [ -f "$SERVER_PID_FILE" ]; then
        PID=$(cat "$SERVER_PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo -e "${YELLOW}Stopping server (PID: $PID)...${NC}"
            kill "$PID" || true
            sleep 1
        fi
        rm -f "$SERVER_PID_FILE"
    fi
    rm -f "$SERVER_LOG"
}
trap cleanup EXIT

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server to be ready...${NC}"
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if curl -s "http://localhost:$SERVER_PORT/health" > /dev/null 2>&1; then
        echo -e "${GREEN}Server is ready!${NC}"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        echo -e "${RED}Server failed to start within timeout${NC}"
        echo "Server log:"
        cat "$SERVER_LOG"
        exit 1
    fi
    sleep 1
done

# Run conformance tests
echo -e "${YELLOW}Running conformance tests...${NC}"
cd "$CONFORMANCE_DIR"

# Use mock model since we're testing against local implementation
BASE_URL="http://localhost:$SERVER_PORT"
MODEL="${OPENRESPONSES_MODEL:-gpt-4}"
API_KEY="${OPENAI_API_KEY:-test-key}"

if command -v bun &> /dev/null; then
    TEST_CMD="bun run bin/compliance-test.ts"
else
    TEST_CMD="npx tsx bin/compliance-test.ts"
fi

# Run tests with verbose output
set +e
$TEST_CMD \
    --base-url "$BASE_URL" \
    --api-key "$API_KEY" \
    --model "$MODEL" \
    --verbose
TEST_EXIT_CODE=$?
set -e

cd "$PROJECT_ROOT"

# Report results
echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ All conformance tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Conformance tests failed${NC}"
    echo ""
    echo "Server log (last 50 lines):"
    tail -n 50 "$SERVER_LOG"
    exit $TEST_EXIT_CODE
fi
