#!/bin/bash
# Multi-architecture Docker build and push script for GoGoMio
# Supports: linux/amd64 (x86_64), linux/arm64 (Raspberry Pi)
# 
# Usage:
#   ./scripts/build-multiarch.sh [VERSION]
#   ./scripts/build-multiarch.sh 0.2.0
#   ./scripts/build-multiarch.sh (defaults to "latest")

set -e

# Configuration
REGISTRY="cyanautomation"
IMAGE_NAME="gogomio"
BUILDER_NAME="gogomio-builder"
PLATFORMS="linux/amd64,linux/arm64"

# Get version from argument or default to "latest"
VERSION="${1:-latest}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Multi-Architecture Docker Build for GoGoMio${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"

# Check prerequisites
echo -e "${YELLOW}1. Checking prerequisites...${NC}"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Docker is not installed${NC}"
    exit 1
fi

if ! docker buildx version &> /dev/null; then
    echo -e "${RED}❌ Docker buildx is not available${NC}"
    exit 1
fi

if ! docker info | grep -q "Username"; then
    echo -e "${YELLOW}⚠️  Not logged into Docker Hub. Please run: docker login${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Docker and buildx available${NC}"
echo -e "${GREEN}✓ Logged into Docker Hub${NC}"

# Verify we're in the right directory
if [ ! -f "Dockerfile" ]; then
    echo -e "${RED}❌ Dockerfile not found. Run from project root.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Dockerfile found${NC}"

# Setup buildx builder if it doesn't exist
echo -e "\n${YELLOW}2. Setting up buildx builder...${NC}"

if docker buildx ls | grep -q "^${BUILDER_NAME}"; then
    echo -e "${GREEN}✓ Builder '${BUILDER_NAME}' already exists${NC}"
else
    echo -e "${BLUE}Creating new builder '${BUILDER_NAME}'...${NC}"
    docker buildx create --name "${BUILDER_NAME}" --platform "${PLATFORMS}" --use
    echo -e "${GREEN}✓ Builder created and activated${NC}"
fi

# Use the builder
docker buildx use "${BUILDER_NAME}"
echo -e "${GREEN}✓ Builder activated${NC}"

# Build and push
echo -e "\n${YELLOW}3. Building multi-architecture image...${NC}"
echo -e "${BLUE}Platforms: ${PLATFORMS}${NC}"
echo -e "${BLUE}Image: ${REGISTRY}/${IMAGE_NAME}:${VERSION}${NC}"
echo ""

BUILD_TAGS=(
    "${REGISTRY}/${IMAGE_NAME}:${VERSION}"
)

# Add additional tags for version builds
if [ "${VERSION}" != "latest" ]; then
    BUILD_TAGS+=("${REGISTRY}/${IMAGE_NAME}:${VERSION}-multiarch")
fi

# Build tag arguments
TAG_ARGS=""
for tag in "${BUILD_TAGS[@]}"; do
    TAG_ARGS="${TAG_ARGS} -t ${tag}"
done

# Run the build
if docker buildx build \
    --platform "${PLATFORMS}" \
    ${TAG_ARGS} \
    --push \
    -f Dockerfile \
    .; then
    echo -e "\n${GREEN}✓ Build and push completed successfully${NC}"
else
    echo -e "\n${RED}❌ Build failed${NC}"
    exit 1
fi

# Verify manifest
echo -e "\n${YELLOW}4. Verifying manifest...${NC}"
echo ""

if docker manifest inspect "${REGISTRY}/${IMAGE_NAME}:${VERSION}" &> /dev/null; then
    echo -e "${GREEN}✓ Manifest created successfully${NC}"
    echo ""
    echo -e "${BLUE}Platform architectures:${NC}"
    docker manifest inspect "${REGISTRY}/${IMAGE_NAME}:${VERSION}" | \
        grep -A 1 '"architecture"' | \
        grep -v '^--$' | \
        sed 's/.*"architecture": "/  ✓ /' | \
        sed 's/".*//'
    echo ""
else
    echo -e "${RED}❌ Failed to verify manifest${NC}"
    exit 1
fi

# Summary
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✅ Build Complete!${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${BLUE}Available images:${NC}"
for tag in "${BUILD_TAGS[@]}"; do
    echo -e "  ${GREEN}✓${NC} ${tag}"
done
echo ""
echo -e "${BLUE}To use on any architecture:${NC}"
echo -e "  docker pull ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
echo ""
echo -e "${BLUE}To verify on your platform:${NC}"
echo -e "  docker run -e MOCK_CAMERA=true -p 8000:8000 ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
echo ""
echo -e "${BLUE}To inspect all architectures:${NC}"
echo -e "  docker manifest inspect ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
echo ""
