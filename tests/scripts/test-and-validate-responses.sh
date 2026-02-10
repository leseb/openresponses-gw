#!/bin/bash
# Complete test: rebuild, restart, and validate Responses API

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Responses API - Complete Validation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Step 1: Rebuild
echo "1. Rebuilding server..."
make build-server > /dev/null 2>&1
echo "   ✓ Build complete"

# Step 2: Stop old server
echo ""
echo "2. Stopping old server..."
pkill -f "openresponses-gw-server" 2>/dev/null || true
sleep 2
echo "   ✓ Old server stopped"

# Step 3: Start new server
echo ""
echo "3. Starting server..."
nohup ./bin/openresponses-gw-server > /tmp/gateway.log 2>&1 &
SERVER_PID=$!
sleep 3

# Wait for health check
for i in {1..10}; do
    if curl -sf http://localhost:8080/health > /dev/null 2>&1; then
        echo "   ✓ Server ready (PID: $SERVER_PID)"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "   ✗ Server failed to start"
        cat /tmp/gateway.log
        exit 1
    fi
    sleep 1
done

# Step 4: Run tests
echo ""
echo "4. Running validation tests..."
echo ""

./tests/scripts/test-responses-minimal.sh

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Validation complete!"
echo ""
echo "Server logs: tail -f /tmp/gateway.log"
echo "Stop server: kill $SERVER_PID"
