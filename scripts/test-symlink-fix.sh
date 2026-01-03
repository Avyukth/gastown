#!/usr/bin/env bash
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PROJECT="$(cd "$(dirname "$0")/.." && pwd)"
TEST_DIR="/tmp/gt-symlink-test-$$"
GT_BASE="$TEST_DIR/gt-base"
GT_FIX="$TEST_DIR/gt-fix"
BASE_COMMIT="386dbf85fba50ff5f6e09cb9aee7886f4a8d282e"
FIX_BRANCH="fix/workspace-symlink-detection"

echo -e "${GREEN}=== Symlink Fix Integration Test ===${NC}"
echo "Project:     $PROJECT"
echo "Test dir:    $TEST_DIR"
echo "Base commit: $BASE_COMMIT"
echo "Fix branch:  $FIX_BRANCH"
echo ""

cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    rm -rf "$TEST_DIR" 2>/dev/null || true
    cd "$PROJECT" && git checkout "$FIX_BRANCH" 2>/dev/null || true
}
trap cleanup EXIT

if [[ ! -L "/tmp" ]]; then
    echo -e "${RED}/tmp is not a symlink - test won't demonstrate the bug${NC}"
    echo "Continuing anyway..."
fi
echo "/tmp -> $(readlink /tmp 2>/dev/null || echo 'not a symlink')"

mkdir -p "$TEST_DIR"
ORIG_BRANCH=$(git rev-parse --abbrev-ref HEAD)

echo ""
echo -e "${GREEN}=== Phase 1: Build BASE binary (commit $BASE_COMMIT) ===${NC}"
cd "$PROJECT"
git checkout -q "$BASE_COMMIT"
echo "Checked out: $(git log --oneline -1)"
go build -o "$GT_BASE" ./cmd/gt
echo "Built: $GT_BASE"
"$GT_BASE" version 2>/dev/null || echo "(version: $(git rev-parse --short HEAD))"

echo ""
echo -e "${GREEN}=== Phase 2: Build FIX binary (branch $FIX_BRANCH) ===${NC}"
cd "$PROJECT"
git checkout -q "$FIX_BRANCH"
echo "Checked out: $(git log --oneline -1)"
go build -o "$GT_FIX" ./cmd/gt
echo "Built: $GT_FIX"
"$GT_FIX" version

echo ""
echo -e "${GREEN}=== Phase 3: Create test workspace in /tmp ===${NC}"
WORKSPACE="$TEST_DIR/workspace"
mkdir -p "$WORKSPACE/mayor"
echo '{"type":"town","name":"symlink-test"}' > "$WORKSPACE/mayor/town.json"
mkdir -p "$WORKSPACE/testrig/polecats/worker"
mkdir -p "$WORKSPACE/testrig/.git"
echo "ref: refs/heads/main" > "$WORKSPACE/testrig/.git/HEAD"

echo "Workspace: $WORKSPACE"
echo ""
echo "Path check from worker directory:"
cd "$WORKSPACE/testrig/polecats/worker"
echo "  pwd:    $(pwd)"
echo "  pwd -P: $(pwd -P)"
if [[ "$(pwd)" != "$(pwd -P)" ]]; then
    echo -e "  ${GREEN}✓ Symlink path discrepancy confirmed${NC}"
else
    echo -e "  ${YELLOW}⚠ No symlink discrepancy${NC}"
fi

echo ""
echo -e "${GREEN}=== Phase 4: Test gt-base (buggy binary) ===${NC}"
cd "$WORKSPACE/testrig/polecats/worker"

echo "Running: gt-base status"
"$GT_BASE" status 2>&1 | head -15 || true

echo ""
echo "Running: gt-base rig list"
"$GT_BASE" rig list 2>&1 | head -10 || true

echo ""
echo -e "${GREEN}=== Phase 5: Test gt-fix (fixed binary) ===${NC}"
cd "$WORKSPACE/testrig/polecats/worker"

echo "Running: gt-fix status"
"$GT_FIX" status 2>&1 | head -15 || true

echo ""
echo "Running: gt-fix rig list"
"$GT_FIX" rig list 2>&1 | head -10 || true

echo ""
echo -e "${GREEN}=== Phase 6: Direct Find() behavior comparison ===${NC}"
cd "$WORKSPACE/testrig/polecats/worker"

cat > "$TEST_DIR/compare.go" << 'GOCODE'
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func findBuggy(start string) string {
	abs, _ := filepath.Abs(start)
	resolved, _ := filepath.EvalSymlinks(abs)
	current := resolved
	for {
		if _, err := os.Stat(filepath.Join(current, "mayor", "town.json")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current { return "" }
		current = parent
	}
}

func findFixed(start string) string {
	abs, _ := filepath.Abs(start)
	current := abs
	for {
		if _, err := os.Stat(filepath.Join(current, "mayor", "town.json")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current { return "" }
		current = parent
	}
}

func main() {
	cwd, _ := os.Getwd()
	
	fmt.Println("BASE (with EvalSymlinks):")
	root1 := findBuggy(cwd)
	rel1, _ := filepath.Rel(root1, cwd)
	fmt.Printf("  townRoot: %s\n", root1)
	fmt.Printf("  relative: %s\n", rel1)
	if len(rel1) > 2 && rel1[:2] == ".." {
		fmt.Println("  STATUS: BUG (path starts with ../)")
	} else {
		fmt.Println("  STATUS: OK")
	}
	
	fmt.Println("")
	fmt.Println("FIX (without EvalSymlinks):")
	root2 := findFixed(cwd)
	rel2, _ := filepath.Rel(root2, cwd)
	fmt.Printf("  townRoot: %s\n", root2)
	fmt.Printf("  relative: %s\n", rel2)
	if len(rel2) > 2 && rel2[:2] == ".." {
		fmt.Println("  STATUS: BUG")
		os.Exit(1)
	}
	fmt.Println("  STATUS: OK")
}
GOCODE

go run "$TEST_DIR/compare.go"

echo ""
echo -e "${GREEN}=== Test Complete ===${NC}"
