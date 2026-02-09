#!/bin/bash
# Start server and run conformance tests
# Usage: ./scripts/test-conformance-with-server.sh [model] [port] [api-key]
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Parse arguments with defaults
MODEL="${1:-ollama/gpt-oss:20b}"
SERVER_PORT="${2:-8080}"
API_KEY="${3:-none}"
BASE_URL="http://localhost:$SERVER_PORT"

echo -e "${BLUE}==== Conformance Test Runner ====${NC}"
echo -e "${YELLOW}Configuration:${NC}"
echo "  Server Port: $SERVER_PORT"
echo "  Base URL:    $BASE_URL"
echo "  Model:       $MODEL"
echo "  API Key:     ${API_KEY:0:10}..."
echo ""

# Check if server binary exists
if [ ! -f "$PROJECT_ROOT/bin/responses-gateway-server" ]; then
    echo -e "${RED}Error: Server binary not found${NC}"
    echo "Building server..."
    make -C "$PROJECT_ROOT" build-server
    echo ""
fi

# Server management
SERVER_LOG="$PROJECT_ROOT/.server-test.log"
SERVER_PID_FILE="$PROJECT_ROOT/.server-test.pid"

# Cleanup function
cleanup() {
    if [ -f "$SERVER_PID_FILE" ]; then
        PID=$(cat "$SERVER_PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo ""
            echo -e "${YELLOW}Stopping server (PID: $PID)...${NC}"
            kill "$PID" 2>/dev/null || true
            sleep 2
            # Force kill if still running
            if kill -0 "$PID" 2>/dev/null; then
                kill -9 "$PID" 2>/dev/null || true
            fi
        fi
        rm -f "$SERVER_PID_FILE"
    fi
    rm -f "$SERVER_LOG"
}
trap cleanup EXIT

# Stop any existing server on this port
if lsof -ti:$SERVER_PORT >/dev/null 2>&1; then
    echo -e "${YELLOW}Port $SERVER_PORT is in use, stopping existing process...${NC}"
    lsof -ti:$SERVER_PORT | xargs kill -9 2>/dev/null || true
    sleep 2
fi

# Start server
echo -e "${YELLOW}Starting server on port $SERVER_PORT...${NC}"
"$PROJECT_ROOT/bin/responses-gateway-server" \
    -port "$SERVER_PORT" \
    > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!
echo $SERVER_PID > "$SERVER_PID_FILE"
echo "  PID: $SERVER_PID"
echo ""

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server to be ready...${NC}"
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if curl -s "$BASE_URL/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Server is ready!${NC}"
        echo ""
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        echo -e "${RED}✗ Server failed to start within timeout${NC}"
        echo ""
        echo "Server log:"
        cat "$SERVER_LOG"
        exit 1
    fi
    sleep 1
    echo -n "."
done

# Run conformance tests
"$SCRIPT_DIR/test-conformance.sh" "$MODEL" "$BASE_URL" "$API_KEY"
TEST_EXIT_CODE=$?

# Cleanup happens automatically via trap
exit $TEST_EXIT_CODE
