#!/bin/bash
# Start ExtProc + Envoy and run conformance tests through Envoy
# Usage: ./tests/scripts/test-conformance-with-envoy.sh [model] [api-key]
#
# Requires:
#   - envoy binary on $PATH
#   - bin/openresponses-gw-extproc (built automatically if missing)
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
EXTPROC_PORT=10000
ENVOY_PORT=8081
ENVOY_ADMIN_PORT=9901

# Parse arguments with defaults
MODEL="${1:-ollama/gpt-oss:20b}"
API_KEY="${2:-none}"
BASE_URL="http://localhost:$ENVOY_PORT"

echo -e "${BLUE}==== Conformance Tests (Envoy ExtProc) ====${NC}"
echo -e "${YELLOW}Configuration:${NC}"
echo "  Envoy Port:   $ENVOY_PORT"
echo "  ExtProc Port: $EXTPROC_PORT"
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

# Check if ExtProc binary exists, build if not
if [ ! -f "$PROJECT_ROOT/bin/openresponses-gw-extproc" ]; then
    echo -e "${YELLOW}ExtProc binary not found, building...${NC}"
    make -C "$PROJECT_ROOT" build-extproc
    echo ""
fi

# Log and PID files
EXTPROC_LOG="$PROJECT_ROOT/.extproc-test.log"
EXTPROC_PID_FILE="$PROJECT_ROOT/.extproc-test.pid"
ENVOY_LOG="$PROJECT_ROOT/.envoy-test.log"
ENVOY_PID_FILE="$PROJECT_ROOT/.envoy-test.pid"

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"

    for name_pid_file in "ExtProc:$EXTPROC_PID_FILE" "Envoy:$ENVOY_PID_FILE"; do
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

    rm -f "$EXTPROC_LOG" "$ENVOY_LOG"
    echo -e "${GREEN}Cleanup done${NC}"
}
trap cleanup EXIT

# Stop any existing processes on the ports we need
for check_port in $EXTPROC_PORT $ENVOY_PORT $ENVOY_ADMIN_PORT; do
    if lsof -ti:$check_port >/dev/null 2>&1; then
        echo -e "${YELLOW}Port $check_port is in use, stopping existing process...${NC}"
        lsof -ti:$check_port | xargs kill -9 2>/dev/null || true
        sleep 1
    fi
done

# Start ExtProc server
echo -e "${YELLOW}Starting ExtProc server on port $EXTPROC_PORT...${NC}"
"$PROJECT_ROOT/bin/openresponses-gw-extproc" \
    -config "$PROJECT_ROOT/tests/envoy/extproc-config.yaml" \
    -port "$EXTPROC_PORT" \
    > "$EXTPROC_LOG" 2>&1 &
EXTPROC_PID=$!
echo $EXTPROC_PID > "$EXTPROC_PID_FILE"
echo "  PID: $EXTPROC_PID"

# Wait for ExtProc to be ready (check if port is listening)
echo -n "  Waiting for ExtProc..."
MAX_ATTEMPTS=15
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if lsof -ti:$EXTPROC_PORT >/dev/null 2>&1; then
        echo ""
        echo -e "  ${GREEN}ExtProc is ready${NC}"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        echo ""
        echo -e "${RED}ExtProc failed to start within timeout${NC}"
        echo "ExtProc log:"
        cat "$EXTPROC_LOG"
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
