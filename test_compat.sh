#!/usr/bin/env bash
# FastBrew vs Homebrew Compatibility Test Harness
# Tests command parity, flags, and exit codes

set -e

FASTBREW="./fastbrew"
BREW="brew"
FAILED=0
PASSED=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED++))
}

# Test no-arg behavior matches brew (shows help)
test_no_arg() {
    log_test "Testing no-arg behavior (should show help)"
    
    # Test brew behavior
    brew_help=$($BREW --help 2>&1 | head -5)
    
    # Test fastbrew behavior  
    if $FASTBREW --help >/dev/null 2>&1; then
        log_pass "fastbrew --help works"
    else
        log_fail "fastbrew --help failed"
    fi
}

# Test update command matches brew update behavior
test_update() {
    log_test "Testing update command"
    
    # FastBrew should update both brew and its own index
    if $FASTBREW update --help >/dev/null 2>&1; then
        log_pass "fastbrew update --help works"
    else
        log_fail "fastbrew update --help failed"
    fi
}

# Test command list parity
test_commands() {
    log_test "Testing command parity"
    
    # Get brew commands
    brew_cmds=$($BREW commands 2>/dev/null | grep -v "^==>" | sort)
    
    # Get fastbrew commands  
    fastbrew_cmds=$($FASTBREW --help 2>/dev/null | grep "^  [a-z]" | awk '{print $1}' | sort)
    
    # Critical commands that must exist
    critical="install uninstall update upgrade search list info deps outdated cleanup doctor tap services link unlink pin unpin"
    
    for cmd in $critical; do
        if echo "$fastbrew_cmds" | grep -q "^${cmd}$"; then
            log_pass "Critical command '$cmd' exists"
        else
            log_fail "Critical command '$cmd' missing"
        fi
    done
}

# Test flag parity for common commands
test_flags() {
    log_test "Testing flag parity"
    
    # Test install flags
    if $FASTBREW install --help 2>&1 | grep -q "\-\-verbose"; then
        log_pass "install --verbose flag exists"
    else
        log_fail "install --verbose flag missing"
    fi
    
    # Test list flags
    if $FASTBREW list --help 2>&1 | grep -q "\-\-versions"; then
        log_pass "list --versions flag exists"
    else
        log_fail "list --versions flag missing (brew compatible)"
    fi
}

# Run all tests
main() {
    echo "===================================="
    echo "FastBrew Compatibility Test Harness"
    echo "===================================="
    echo ""
    
    test_no_arg
    test_update
    test_commands
    test_flags
    
    echo ""
    echo "===================================="
    echo "Results: $PASSED passed, $FAILED failed"
    echo "===================================="
    
    if [ $FAILED -gt 0 ]; then
        exit 1
    fi
}

main "$@"
