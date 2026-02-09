#!/bin/bash
# Validate that openapi.yaml and openapi.go are in sync
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}==== Validating OpenAPI Spec Consistency ====${NC}"

# Check if files exist
if [ ! -f "$PROJECT_ROOT/openapi.yaml" ]; then
    echo -e "${RED}Error: openapi.yaml not found${NC}"
    exit 1
fi

if [ ! -f "$PROJECT_ROOT/pkg/adapters/http/openapi.go" ]; then
    echo -e "${RED}Error: pkg/adapters/http/openapi.go not found${NC}"
    exit 1
fi

# Check if required tools are available
if ! command -v yq &> /dev/null; then
    echo -e "${YELLOW}Warning: yq not found, skipping detailed validation${NC}"
    echo "  Install with: brew install yq (macOS) or snap install yq (Linux)"
    echo -e "${GREEN}✓ Files exist (detailed validation skipped)${NC}"
    exit 0
fi

# Extract key values from openapi.yaml
YAML_TITLE=$(yq eval '.info.title' "$PROJECT_ROOT/openapi.yaml")
YAML_VERSION=$(yq eval '.info.version' "$PROJECT_ROOT/openapi.yaml")
YAML_HAS_RESPONSES=$(yq eval '.paths."/v1/responses".post' "$PROJECT_ROOT/openapi.yaml" | grep -q "summary" && echo "true" || echo "false")
YAML_HAS_CHAT=$(yq eval '.paths."/v1/chat/completions"' "$PROJECT_ROOT/openapi.yaml" | grep -q "null" && echo "false" || echo "true")

# Check openapi.go for matching content
GO_HAS_RESPONSES=$(grep -q '"/v1/responses"' "$PROJECT_ROOT/pkg/adapters/http/openapi.go" && echo "true" || echo "false")
GO_HAS_CHAT=$(grep -q '"/v1/chat/completions"' "$PROJECT_ROOT/pkg/adapters/http/openapi.go" && echo "true" || echo "false")

echo "Validation checks:"
echo "  - YAML title: $YAML_TITLE"
echo "  - YAML version: $YAML_VERSION"
echo "  - YAML has /v1/responses: $YAML_HAS_RESPONSES"
echo "  - YAML has /v1/chat/completions: $YAML_HAS_CHAT"
echo "  - Go has /v1/responses: $GO_HAS_RESPONSES"
echo "  - Go has /v1/chat/completions: $GO_HAS_CHAT"

# Validate consistency
ERRORS=0

if [ "$YAML_HAS_RESPONSES" != "$GO_HAS_RESPONSES" ]; then
    echo -e "${RED}Error: /v1/responses endpoint mismatch between YAML and Go${NC}"
    ERRORS=$((ERRORS + 1))
fi

if [ "$YAML_HAS_CHAT" != "$GO_HAS_CHAT" ]; then
    echo -e "${RED}Error: /v1/chat/completions endpoint mismatch between YAML and Go${NC}"
    ERRORS=$((ERRORS + 1))
fi

# Check that both files were modified recently together (within 1 hour)
# macOS uses -f, Linux uses -c
if stat -f %m "$PROJECT_ROOT/openapi.yaml" >/dev/null 2>&1; then
    # macOS
    YAML_MTIME=$(stat -f %m "$PROJECT_ROOT/openapi.yaml")
    GO_MTIME=$(stat -f %m "$PROJECT_ROOT/pkg/adapters/http/openapi.go")
else
    # Linux
    YAML_MTIME=$(stat -c %Y "$PROJECT_ROOT/openapi.yaml")
    GO_MTIME=$(stat -c %Y "$PROJECT_ROOT/pkg/adapters/http/openapi.go")
fi

TIME_DIFF=$((YAML_MTIME - GO_MTIME))
TIME_DIFF=${TIME_DIFF#-}  # Absolute value

if [ "$TIME_DIFF" -gt 3600 ]; then
    echo -e "${YELLOW}Warning: openapi.yaml and openapi.go have different modification times${NC}"
    echo "  Consider updating both files together to keep them in sync"
    if date -r "$YAML_MTIME" >/dev/null 2>&1; then
        # macOS date
        echo "  YAML mtime: $(date -r $YAML_MTIME)"
        echo "  Go mtime:   $(date -r $GO_MTIME)"
    else
        # Linux date
        echo "  YAML mtime: $(date -d @$YAML_MTIME)"
        echo "  Go mtime:   $(date -d @$GO_MTIME)"
    fi
fi

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}✗ OpenAPI spec files are out of sync${NC}"
    exit 1
else
    echo -e "${GREEN}✓ OpenAPI spec files are consistent${NC}"
    exit 0
fi
