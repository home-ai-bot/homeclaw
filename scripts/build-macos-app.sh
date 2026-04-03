#!/bin/bash
# PicoClaw macOS Build Script
# Builds all 2 executables: picoclaw, picoclaw-launcher
# Embeds workspace and config files into the binary

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
GRAY='\033[0;90m'
WHITE='\033[0;37m'
NC='\033[0m' # No Color

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="${PROJECT_ROOT}/build"

# Build configuration
GO_FLAGS="-v -tags stdjson"
LD_FLAGS="-s -w"

# Workspace paths
WORKSPACE_SOURCE="${PROJECT_ROOT}/homeclaw-workspace"
ONBOARD_DIR="${PROJECT_ROOT}/cmd/picoclaw/internal/onboard"
WORKSPACE_TARGET="${ONBOARD_DIR}/workspace"

# Ensure build directory exists
mkdir -p "$BUILD_DIR"

echo -e "${CYAN}"
echo "========================================"
echo "  PicoClaw macOS Build Script"
echo "========================================"
echo -e "${NC}"

# Change to project root
cd "$PROJECT_ROOT"

# Step 0: Copy workspace to embed directory (equivalent to go:generate)
echo -e "${MAGENTA}[0/2] Preparing workspace for embedding...${NC}"

# Remove existing workspace in onboard directory
if [ -d "$WORKSPACE_TARGET" ]; then
    echo -e "${GRAY}      Removing existing workspace copy...${NC}"
    rm -rf "$WORKSPACE_TARGET"
fi

# Copy workspace directory to onboard package for embedding
if [ -d "$WORKSPACE_SOURCE" ]; then
    echo -e "${GRAY}      Copying workspace to ${WORKSPACE_TARGET}...${NC}"
    cp -r "$WORKSPACE_SOURCE" "$WORKSPACE_TARGET"
    echo -e "${GREEN}      Workspace prepared for embedding!${NC}"
else
    echo -e "${RED}Error: Workspace source directory not found: $WORKSPACE_SOURCE${NC}"
    exit 1
fi

echo ""

# Build 1: picoclaw
echo -e "${YELLOW}[1/2] Building picoclaw...${NC}"
CGO_ENABLED=0 go build $GO_FLAGS -ldflags "$LD_FLAGS" -o "${BUILD_DIR}/picoclaw" ./cmd/picoclaw
echo -e "${GREEN}      picoclaw built successfully!${NC}"

# Build 2: picoclaw-launcher (web backend)
echo -e "${YELLOW}[2/2] Building picoclaw-launcher...${NC}"

# Always rebuild frontend to ensure latest changes are included
echo -e "${MAGENTA}      Building frontend...${NC}"
cd "${PROJECT_ROOT}/web/frontend"
npm install
npm run build:backend
cd "$PROJECT_ROOT"
echo -e "${GREEN}      Frontend built successfully!${NC}"

go build $GO_FLAGS -ldflags "$LD_FLAGS" -o "${BUILD_DIR}/picoclaw-launcher" ./web/backend
echo -e "${GREEN}      picoclaw-launcher built successfully!${NC}"

# Summary
echo ""
echo -e "${CYAN}========================================"
echo -e "${GREEN}  Build Complete!"
echo -e "${CYAN}========================================${NC}"
echo -e "${WHITE}"
echo "Output directory: $BUILD_DIR"
echo ""
echo "Built executables:"
echo -e "${NC}"

for f in "$BUILD_DIR"/picoclaw*; do
    if [ -f "$f" ]; then
        size=$(($(wc -c < "$f") / 1024 / 1024))
        name=$(basename "$f")
        echo -e "${GRAY}  - ${name} (${size} MB)${NC}"
    fi
done

echo ""
