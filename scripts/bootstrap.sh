#!/bin/bash
# Bootstrap script for GoGoMio development setup
# Initializes .env and starts Docker Compose mock environment

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}GoGoMio Bootstrap - Development Setup${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"

# Step 1: Check for .env file
echo -e "\n${BLUE}1. Checking environment configuration...${NC}"
if [ -f .env ]; then
    echo -e "${GREEN}✓ .env file already exists${NC}"
else
    if [ ! -f .env.example ]; then
        echo -e "${YELLOW}✗ .env.example not found${NC}"
        exit 1
    fi
    echo -e "${YELLOW}→ Creating .env from .env.example${NC}"
    cp .env.example .env
    echo -e "${GREEN}✓ .env created successfully${NC}"
fi

# Step 2: Check Docker availability
echo -e "\n${BLUE}2. Checking prerequisites...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}✗ Docker not found. Please install Docker to continue.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker is available${NC}"

if ! command -v docker-compose &> /dev/null; then
    echo -e "${YELLOW}✗ docker-compose not found. Please install Docker Compose.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ docker-compose is available${NC}"

# Step 3: Confirm ready to start
echo -e "\n${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Setup complete!${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "To start the development environment, run:"
echo -e "${YELLOW}  docker-compose -f docker-compose.mock.yml up${NC}"
echo ""
echo -e "The server will be available at ${YELLOW}http://localhost:8000${NC}"
echo ""
echo -e "Quick test commands:"
echo -e "  ${YELLOW}curl http://localhost:8000/health${NC}"
echo -e "  ${YELLOW}curl http://localhost:8000/snapshot.jpg -o frame.jpg${NC}"
echo ""
