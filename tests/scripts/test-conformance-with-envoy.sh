#!/bin/bash
# Start Gateway + Envoy and run conformance tests through Envoy
# Usage: ./tests/scripts/test-conformance-with-envoy.sh [model] [api-key]
#
# Requires:
#   - envoy binary on $PATH
#   - bin/openresponses-gw (built automatically if missing)
#
# NOTE: Streaming conformance tests will fail through Envoy because ExtProc
# uses ImmediateResponse which buffers the full response (no SSE streaming).
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Fixed ports matching tests/envoy/envoy.yaml
ENVOY_PORT=8081
ENVOY_ADMIN_PORT=9901

# Parse arguments with defaults
MODEL="${1:-ollama/gpt-oss:20b}"
API_KEY="${2:-none}"
BASE_URL="http://localhost:$ENVOY_PORT"

echo -e "${BLUE}==== Conformance Tests (Envoy ExtProc) ====${NC}"
echo -e "${YELLOW}Configuration:${NC}"
echo "  Envoy Port:   $ENVOY_PORT"
echo "  Base URL:     $BASE_URL"
echo "  Model:        $MODEL"
echo "  API Key:      ${API_KEY:0:10}..."
echo ""

# Check for envoy binary
if ! command -v envoy &> /dev/null; then
    echo -e "${RED}Error: envoy binary not found on PATH${NC}"
    echo "Install Envoy: https://www.envoyproxy.io/docs/envoy/latest/start/install"
    exit 1
fi

# Check if gateway binary exists, build if not
if [ ! -f "$PROJECT_ROOT/bin/openresponses-gw" ]; then
    echo -e "${YELLOW}Gateway binary not found, building...${NC}"
    make -C "$PROJECT_ROOT" build
    echo ""
fi

# Log and PID files
GATEWAY_LOG="$PROJECT_ROOT/.gateway-test.log"
GATEWAY_PID_FILE="$PROJECT_ROOT/.gateway-test.pid"
ENVOY_LOG="$PROJECT_ROOT/.envoy-test.log"
ENVOY_PID_FILE="$PROJECT_ROOT/.envoy-test.pid"

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"

    for name_pid_file in "Gateway:$GATEWAY_PID_FILE" "Envoy:$ENVOY_PID_FILE"; do
        name="${name_pid_file%%:*}"
        pid_file="${name_pid_file##*:}"
        if [ -f "$pid_file" ]; then
            PID=$(cat "$pid_file")
            if kill -0 "$PID" 2>/dev/null; then
                echo -e "  Stopping $name (PID: $PID)..."
                kill "$PID" 2>/dev/null || true
                sleep 1
                if kill -0 "$PID" 2>/dev/null; then
                    kill -9 "$PID" 2>/dev/null || true
                fi
            fi
            rm -f "$pid_file"
        fi
    done

    rm -f "$GATEWAY_LOG" "$ENVOY_LOG"
    echo -e "${GREEN}Cleanup done${NC}"
}
trap cleanup EXIT

# Stop any existing processes on the ports we need
for check_port in $ENVOY_PORT $ENVOY_ADMIN_PORT; do
    if lsof -ti:$check_port >/dev/null 2>&1; then
        echo -e "${YELLOW}Port $check_port is in use, stopping existing process...${NC}"
        lsof -ti:$check_port | xargs kill -9 2>/dev/null || true
        sleep 1
    fi
done

# Start gateway (HTTP + ExtProc in single process)
echo -e "${YELLOW}Starting gateway...${NC}"
"$PROJECT_ROOT/bin/openresponses-gw" \
    -config "$PROJECT_ROOT/tests/envoy/config.yaml" \
    > "$GATEWAY_LOG" 2>&1 &
GATEWAY_PID=$!
echo $GATEWAY_PID > "$GATEWAY_PID_FILE"
echo "  PID: $GATEWAY_PID"

# Wait for gateway to be ready (check gRPC port)
echo -n "  Waiting for gateway..."
MAX_ATTEMPTS=15
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if lsof -ti:10000 >/dev/null 2>&1; then
        echo ""
        echo -e "  ${GREEN}Gateway is ready${NC}"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        echo ""
        echo -e "${RED}Gateway failed to start within timeout${NC}"
        echo "Gateway log:"
        cat "$GATEWAY_LOG"
        exit 1
    fi
    echo -n "."
    sleep 1
done

# Start Envoy
echo -e "${YELLOW}Starting Envoy on port $ENVOY_PORT...${NC}"
envoy -c "$PROJECT_ROOT/tests/envoy/envoy.yaml" \
    --log-level warning \
    > "$ENVOY_LOG" 2>&1 &
ENVOY_PID=$!
echo $ENVOY_PID > "$ENVOY_PID_FILE"
echo "  PID: $ENVOY_PID"

# Wait for Envoy to be ready (check health endpoint through Envoy)
echo -n "  Waiting for Envoy..."
MAX_ATTEMPTS=15
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if curl -s "http://localhost:$ENVOY_ADMIN_PORT/ready" > /dev/null 2>&1; then
        echo ""
        echo -e "  ${GREEN}Envoy is ready${NC}"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        echo ""
        echo -e "${RED}Envoy failed to start within timeout${NC}"
        echo "Envoy log:"
        cat "$ENVOY_LOG"
        exit 1
    fi
    echo -n "."
    sleep 1
done
echo ""

# Run conformance tests through Envoy
"$SCRIPT_DIR/test-conformance.sh" "$MODEL" "$BASE_URL" "$API_KEY"
TEST_EXIT_CODE=$?

# Cleanup happens automatically via trap
exit $TEST_EXIT_CODE
