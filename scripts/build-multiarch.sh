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

# Check if ARM64 emulation is available
echo -e "\n${YELLOW}Checking QEMU ARM64 support...${NC}"
if ! docker run --privileged --rm tonistiigi/binfmt:latest | grep -q '"linux/arm64"'; then
    echo -e "${RED}❌ ARM64 emulation not available in this environment${NC}"
    echo ""
    echo -e "${YELLOW}This dev container only supports amd64. Three solutions:${NC}"
    echo ""
    echo -e "${BLUE}1. Use GitHub Actions (RECOMMENDED)${NC}"
    echo -e "   Push to 'main' or create a git tag to trigger automatic build"
    echo -e "   ${BLUE}git tag -a v0.2.0 -m 'Release 0.2.0'${NC}"
    echo -e "   ${BLUE}git push origin v0.2.0${NC}"
    echo -e "   View progress at: https://github.com/CyanAutomation/gogomio/actions"
    echo ""
    echo -e "${BLUE}2. Build only amd64 (dev/testing only)${NC}"
    echo -e "   ${BLUE}docker buildx build --platform linux/amd64 -t cyanautomation/gogomio:latest --push .${NC}"
    echo -e "   ⚠️  WARNING: This breaks Raspberry Pi compatibility"
    echo ""
    echo -e "${BLUE}3. Run from a system with ARM64 support${NC}"
    echo -e "   Use native Linux on Raspberry Pi or enable nested virtualization"
    echo ""
    exit 1
fi
echo -e "${GREEN}✓ QEMU ARM64 support available${NC}"

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
