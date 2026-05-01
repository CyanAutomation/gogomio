#!/bin/bash
# Create GitHub labels for GoGoMio issue/PR categorization
# Requires: gh CLI (https://cli.github.com)
# Usage: ./scripts/setup-labels.sh

set -e

REPO="CyanAutomation/gogomio"

# Colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}GoGoMio Label Setup${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"

# Check for gh CLI
if ! command -v gh &> /dev/null; then
    echo -e "${YELLOW}✗ GitHub CLI (gh) not found.${NC}"
    echo -e "Install from: https://cli.github.com"
    exit 1
fi

echo -e "${GREEN}✓ GitHub CLI found${NC}"

# Check authentication
if ! gh auth status &> /dev/null; then
    echo -e "${YELLOW}✗ Not authenticated with GitHub${NC}"
    echo "Run: gh auth login"
    exit 1
fi

echo -e "${GREEN}✓ Authenticated with GitHub${NC}"
echo ""

# Define labels: name, description, color
declare -a LABELS=(
    "bug|Report a defect or issue|d73a49"
    "feature|Request or implement a new feature|a2eeef"
    "documentation|Improvements or additions to documentation|0075ca"
    "testing|Test improvements, coverage, E2E tests|fbca04"
    "performance|Performance optimization or benchmarking|d4c5f9"
    "good-first-issue|Suitable for newcomers/contributors|7057ff"
    "dependencies|Dependency updates or upgrades|0366d6"
    "ci|CI/CD pipeline improvements|5319e7"
)

echo -e "${BLUE}Creating labels...${NC}"
echo ""

for label in "${LABELS[@]}"; do
    IFS='|' read -r NAME DESCRIPTION COLOR <<< "$label"
    
    echo -ne "  Creating label '${YELLOW}${NAME}${NC}'... "
    
    # Check if label already exists
    if gh label list --repo "$REPO" --limit 100 | grep -q "^$NAME"; then
        echo -e "${YELLOW}(already exists)${NC}"
        continue
    fi
    
    # Create label
    if gh label create "$NAME" \
        --repo "$REPO" \
        --description "$DESCRIPTION" \
        --color "$COLOR" 2>/dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${YELLOW}(skipped)${NC}"
    fi
done

echo ""
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ Label setup complete!${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════${NC}"
echo ""
echo "Labels are now available for use in issues and pull requests."
echo "View them at: https://github.com/$REPO/labels"
