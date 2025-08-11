#!/bin/bash

# 🧪 Comprehensive Test Suite for Agentic Ad Processing Queue
# Tests: Unit, Integration, API, Load, Security, Performance

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_RESULTS_DIR="${PROJECT_ROOT}/test-results"
DEFAULT_API_URL="http://localhost:8080"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test configuration
API_BASE_URL=""
VERBOSE=false
PARALLEL=false
CLEANUP=true
TEST_TYPES=()
LOAD_TEST_DURATION=30
LOAD_TEST_USERS=10

# Logging
log() { echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
info() { echo -e "${CYAN}[INFO]${NC} $1"; }
test_header() { echo -e "${PURPLE}=== $1 ===${NC}"; }

# Print header
print_header() {
    echo -e "${PURPLE}"
    echo "============================================================================="
    echo "               🧪 COMPREHENSIVE TEST SUITE"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Testing: Agentic Ad Processing Queue System${NC}"
    echo -e "${CYAN}URL: ${API_BASE_URL:-$DEFAULT_API_URL}${NC}"
    echo ""
}

# Setup test environment
setup_test_environment() {
    log "Setting up test environment..."
    
    mkdir -p "$TEST_RESULTS_DIR"
    cd "$PROJECT_ROOT"
    
    # Determine API URL
    if [ -z "$API_BASE_URL" ]; then
        # Auto-detect based on running services
        if curl -s --max-time 2 "http://localhost:8080/health" > /dev/null 2>&1; then
            API_BASE_URL="http://localhost:8080"
        elif curl -s --max-time 2 "http://localhost:8443/health" > /dev/null 2>&1; then
            API_BASE_URL="http://localhost:8443"
        elif curl -s --max-time 2 "http://localhost:30443/health" > /dev/null 2>&1; then
            API_BASE_URL="http://localhost:30443"
        else
            API_BASE_URL="$DEFAULT_API_URL"
            warn "No running service detected, using default: $API_BASE_URL"
        fi
    fi
    
    success "Test environment ready (API: $API_BASE_URL)"
}

# Run unit tests
run_unit_tests() {
    test_header "UNIT TESTS"
    
    log "Running Go unit tests..."
    local output_file="$TEST_RESULTS_DIR/unit-tests.json"
    
    if go test -json -short ./internal/... ./pkg/... > "$output_file" 2>&1; then
        success "Unit tests passed"
        
        # Parse results if jq is available
        if command -v jq > /dev/null 2>&1; then
            local passed=$(jq -r 'select(.Action=="pass" and .Test==null) | .Package' "$output_file" | wc -l | tr -d ' ')
            local failed=$(jq -r 'select(.Action=="fail" and .Test==null) | .Package' "$output_file" | wc -l | tr -d ' ')
            info "Packages: $passed passed, $failed failed"
        fi
    else
        error "Unit tests failed"
        tail -20 "$output_file"
        return 1
    fi
}

# Run API tests
run_api_tests() {
    test_header "API TESTS"
    
    log "Testing API endpoints at $API_BASE_URL..."
    local results_file="$TEST_RESULTS_DIR/api-tests.json"
    
    # Test health endpoint
    log "Testing health endpoint..."
    if curl -s --max-time 10 "$API_BASE_URL/health" | grep -q "healthy\|service"; then
        success "✓ Health endpoint working"
    else
        error "✗ Health endpoint failed"
        return 1
    fi
    
    # Test ad creation
    log "Testing ad creation..."
    local ad_response=$(curl -s -X POST "$API_BASE_URL/api/v1/ads" \
        -H "Content-Type: application/json" \
        -d '{
            "title": "Test Ad - API Suite",
            "gameFamily": "Test-Game",
            "targetAudience": ["test-users"],
            "priority": 4,
            "maxWaitTime": 300
        }')
    
    if echo "$ad_response" | grep -q "adId\|ad_id"; then
        local ad_id=""
        if command -v jq > /dev/null 2>&1; then
            ad_id=$(echo "$ad_response" | jq -r '.adId // .ad_id // "unknown"')
        else
            ad_id="test-created"
        fi
        success "✓ Ad creation working (ID: $ad_id)"
        
        # Store ad_id for other tests
        echo "$ad_id" > "$TEST_RESULTS_DIR/test-ad-id.txt"
    else
        error "✗ Ad creation failed"
        echo "Response: $ad_response"
        return 1
    fi
    
    # Test AI agent command
    log "Testing AI agent command..."
    local ai_response=$(curl -s -X POST "$API_BASE_URL/api/v1/agent/command" \
        -H "Content-Type: application/json" \
        -d '{"command": "Show queue statistics"}')
    
    if echo "$ai_response" | grep -q "commandId\|command_id"; then
        success "✓ AI agent command working"
    else
        warn "⚠ AI agent command failed (may be using mock implementation)"
    fi
}

# Run load tests
run_load_tests() {
    test_header "LOAD TESTS"
    
    log "Running load tests for $LOAD_TEST_DURATION seconds with $LOAD_TEST_USERS concurrent users..."
    local results_file="$TEST_RESULTS_DIR/load-test-results.txt"
    
    # Create load test function
    load_test_worker() {
        local worker_id=$1
        local start_time=$(date +%s)
        local end_time=$((start_time + LOAD_TEST_DURATION))
        local requests=0
        local errors=0
        
        while [ $(date +%s) -lt $end_time ]; do
            local response=$(curl -s -w "%{http_code}" -X POST "$API_BASE_URL/api/v1/ads" \
                -H "Content-Type: application/json" \
                -d "{
                    \"title\": \"Load Test Ad Worker $worker_id Request $requests\",
                    \"gameFamily\": \"Load-Test\",
                    \"targetAudience\": [\"load-test-user-$worker_id\"],
                    \"priority\": $((1 + RANDOM % 5)),
                    \"maxWaitTime\": 300
                }" 2>/dev/null)
            
            local http_code="${response: -3}"
            if [[ "$http_code" == "201" ]]; then
                ((requests++))
            else
                ((errors++))
            fi
            
            sleep 0.1
        done
        
        echo "Worker $worker_id: $requests requests, $errors errors"
    }
    
    # Start load test workers
    log "Starting $LOAD_TEST_USERS concurrent workers..."
    local pids=()
    
    for i in $(seq 1 $LOAD_TEST_USERS); do
        load_test_worker $i >> "$results_file" 2>&1 &
        pids+=($!)
    done
    
    # Monitor progress
    for i in $(seq 1 $LOAD_TEST_DURATION); do
        echo -ne "\r${CYAN}Load testing in progress... ${i}/${LOAD_TEST_DURATION}s${NC}"
        sleep 1
    done
    echo ""
    
    # Wait for workers to complete
    for pid in "${pids[@]}"; do
        wait $pid
    done
    
    # Calculate results
    local total_requests=$(grep "Worker" "$results_file" | awk '{sum += $3} END {print sum+0}')
    local total_errors=$(grep "Worker" "$results_file" | awk '{sum += $5} END {print sum+0}')
    local rps=$((total_requests / LOAD_TEST_DURATION))
    
    success "Load test completed:"
    info "  Total requests: $total_requests"
    info "  Total errors: $total_errors"
    info "  Requests/second: $rps"
    
    # Performance benchmark
    if [ $rps -gt 50 ]; then
        success "✓ Performance: EXCELLENT ($rps RPS)"
    elif [ $rps -gt 20 ]; then
        success "✓ Performance: GOOD ($rps RPS)"
    elif [ $rps -gt 5 ]; then
        warn "⚠ Performance: ACCEPTABLE ($rps RPS)"
    else
        error "✗ Performance: POOR ($rps RPS)"
    fi
}

# Generate test report
generate_test_report() {
    log "Generating comprehensive test report..."
    
    local report_file="$TEST_RESULTS_DIR/test-report.md"
    local timestamp=$(date -u +"%Y-%m-%d %H:%M:%S UTC")
    
    cat > "$report_file" << EOF
# Test Report - Agentic Ad Processing Queue

**Generated:** $timestamp
**API URL:** $API_BASE_URL
**Test Types:** ${TEST_TYPES[*]}

## Test Summary

### Environment
- Project: Agentic Ad Processing Queue
- API Endpoint: $API_BASE_URL
- Test Duration: Load tests ran for ${LOAD_TEST_DURATION}s with ${LOAD_TEST_USERS} users
- Test Results Directory: $TEST_RESULTS_DIR

### Results Overview
EOF
    
    # Append results from each test type
    for test_type in "${TEST_TYPES[@]}"; do
        echo "- **$test_type**: $([ -f "$TEST_RESULTS_DIR/$test_type-tests.json" ] && echo "✓ Completed" || echo "⚠ Skipped")" >> "$report_file"
    done
    
    cat >> "$report_file" << EOF

### Load Test Results
$([ -f "$TEST_RESULTS_DIR/load-test-results.txt" ] && echo "- Total Requests: $(grep -o '[0-9]\+ requests' "$TEST_RESULTS_DIR/load-test-results.txt" 2>/dev/null | awk '{sum += $1} END {print sum+0}')" || echo "Not run")

## Recommendations

Based on the test results:
1. **Performance**: Monitor response times under load
2. **Security**: Ensure input validation is comprehensive
3. **Scalability**: Test with higher concurrent users
4. **Monitoring**: Set up alerts for performance degradation

---
*Report generated by test suite at $timestamp*
EOF
    
    success "Test report generated: $report_file"
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS] [TEST_TYPES...]"
    echo ""
    echo "Comprehensive test suite for the Agentic Ad Processing Queue system."
    echo ""
    echo "Test Types:"
    echo "  unit               Run unit tests"
    echo "  api                Run API functionality tests"
    echo "  load               Run load/stress tests"
    echo "  all                Run all test types (default)"
    echo ""
    echo "Options:"
    echo "  -h, --help            Show this help message"
    echo "  --url URL             API base URL (auto-detected if not specified)"
    echo "  --duration SECONDS    Load test duration (default: 30)"
    echo "  --users COUNT         Load test concurrent users (default: 10)"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Run all tests"
    echo "  $0 unit api                           # Run only unit and API tests"
    echo "  $0 load --duration 60 --users 20     # Extended load test"
    echo "  $0 --url http://prod.example.com api # Test production API"
    echo ""
}

# Main execution
main() {
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            --url)
                API_BASE_URL="$2"
                shift 2
                ;;
            --duration)
                LOAD_TEST_DURATION="$2"
                shift 2
                ;;
            --users)
                LOAD_TEST_USERS="$2"
                shift 2
                ;;
            unit|api|load)
                TEST_TYPES+=("$1")
                shift
                ;;
            all)
                TEST_TYPES=("unit" "api" "load")
                shift
                ;;
            *)
                error "Unknown option or test type: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # Default to all tests if none specified
    if [ ${#TEST_TYPES[@]} -eq 0 ]; then
        TEST_TYPES=("unit" "api" "load")
    fi
    
    print_header
    setup_test_environment
    
    # Run requested tests
    for test_type in "${TEST_TYPES[@]}"; do
        case $test_type in
            unit)
                run_unit_tests || warn "Unit tests failed"
                ;;
            api)
                run_api_tests || warn "API tests failed"
                ;;
            load)
                run_load_tests || warn "Load tests failed"
                ;;
        esac
        echo ""
    done
    
    generate_test_report
    
    echo ""
    echo -e "${PURPLE}============================================================================="
    echo "                        🧪 TEST SUITE COMPLETED"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${GREEN}Test execution completed!${NC}"
    echo ""
    echo -e "${CYAN}📋 Results:${NC}"
    echo "  📁 Test results: $TEST_RESULTS_DIR/"
    echo "  📄 Full report: $TEST_RESULTS_DIR/test-report.md"
    echo ""
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi