#!/bin/bash

# 🤖 AI Agent Command Demonstration Script
# Showcases the AI agent command processing capabilities
# Implements FR-SIM-003: Command demonstration with comprehensive examples

set -e

# Configuration
API_BASE_URL=${API_BASE_URL:-"http://localhost:8443/api"}
DELAY_SECONDS=${1:-2}
DEMO_MODE=${2:-"interactive"}

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${BLUE}🤖 AI Agent Command Demonstration${NC}"
echo -e "${BLUE}====================================${NC}"
echo "🌐 API URL: $API_BASE_URL"
echo "⏱️  Delay: ${DELAY_SECONDS}s between commands"
echo "🎭 Mode: $DEMO_MODE"
echo ""

# Command categories and examples
declare -a QUEUE_MODIFICATION_COMMANDS=(
    "Change priority to 5 for all ads in the RPG-Fantasy family"
    "Set priority to 4 for ads older than 10 minutes"
    "Change priority to 3 for all ads in the Strategy-RTS family"
    "Set priority to 2 for ads older than 15 minutes"
)

declare -a SYSTEM_CONFIGURATION_COMMANDS=(
    "Enable starvation mode"
    "Set maximum wait time to 300 seconds"
    "Set worker count to 5"
    "Disable starvation mode"
)

declare -a STATUS_QUERY_COMMANDS=(
    "Show the next 5 ads to be processed"
    "List all ads waiting longer than 5 minutes"
    "Show the next 10 ads to be processed"
    "List all ads waiting longer than 2 minutes"
)

declare -a ANALYTICS_COMMANDS=(
    "What's the current queue distribution by priority?"
    "Show queue performance summary"
    "What's the current queue distribution by priority?"
    "Show queue performance summary"
)

# Command execution function
execute_command() {
    local command="$1"
    local category="$2"
    
    echo -e "${CYAN}🎯 Category: ${category}${NC}"
    echo -e "${YELLOW}💬 Command: \"${command}\"${NC}"
    
    local payload=$(cat << EOF
{
    "command": "$command"
}
EOF
    )
    
    echo -n "🔄 Processing... "
    
    local response=$(curl -s -X POST "$API_BASE_URL/agent/command" \
        -H "Content-Type: application/json" \
        -d "$payload")
    
    local curl_exit_code=$?
    
    if [ $curl_exit_code -eq 0 ]; then
        # Check if response contains expected fields
        if echo "$response" | jq -e '.commandId' > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Success${NC}"
            
            local command_id=$(echo "$response" | jq -r '.commandId')
            local status=$(echo "$response" | jq -r '.status')
            local execution_time=$(echo "$response" | jq -r '.executionTime // "N/A"')
            
            echo -e "   📋 Command ID: ${command_id:0:8}..."
            echo -e "   📊 Status: $status"
            echo -e "   ⏰ Execution Time: $execution_time"
            
            # Display command result if available
            if echo "$response" | jq -e '.result' > /dev/null 2>&1; then
                echo -e "   ${PURPLE}📈 Result:${NC}"
                echo "$response" | jq -r '.result' | sed 's/^/      /'
            fi
            
            # Display specific result fields based on command type
            case "$category" in
                "Queue Modification")
                    if echo "$response" | jq -e '.result.adsModified' > /dev/null 2>&1; then
                        local ads_modified=$(echo "$response" | jq -r '.result.adsModified // 0')
                        echo -e "   🔄 Ads Modified: $ads_modified"
                    fi
                    ;;
                "Analytics")
                    if echo "$response" | jq -e '.result.distribution' > /dev/null 2>&1; then
                        echo -e "   ${PURPLE}📊 Distribution:${NC}"
                        echo "$response" | jq -r '.result.distribution[]? | "      Priority \(.priority): \(.count) ads"' 2>/dev/null || true
                    fi
                    ;;
                "Status Query")
                    if echo "$response" | jq -e '.result.ads' > /dev/null 2>&1; then
                        local ad_count=$(echo "$response" | jq -r '.result.ads | length')
                        echo -e "   📋 Ads Retrieved: $ad_count"
                    fi
                    ;;
            esac
        else
            echo -e "${RED}❌ Error${NC}"
            echo "   Response: $response"
        fi
    else
        echo -e "${RED}❌ Request Failed${NC}"
        echo "   Error: Unable to connect to API"
    fi
    
    echo ""
}

# Interactive pause function
pause_if_interactive() {
    if [ "$DEMO_MODE" = "interactive" ]; then
        echo -e "${BLUE}Press Enter to continue or Ctrl+C to exit...${NC}"
        read -r
        echo ""
    else
        sleep "$DELAY_SECONDS"
    fi
}

# Health check function
check_api_health() {
    echo -n "🔍 Checking API health... "
    if curl -s -f "$API_BASE_URL/../health" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ API is healthy${NC}"
        return 0
    else
        echo -e "${RED}❌ API is not responding${NC}"
        echo "Please ensure the API server is running at $API_BASE_URL"
        return 1
    fi
}

# Check command endpoint availability
check_command_endpoint() {
    echo -n "🤖 Checking AI agent endpoint... "
    local test_response=$(curl -s -X POST "$API_BASE_URL/agent/command" \
        -H "Content-Type: application/json" \
        -d '{"command": "test"}' 2>/dev/null)
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Endpoint is available${NC}"
        return 0
    else
        echo -e "${RED}❌ Command endpoint is not available${NC}"
        echo "Please ensure the AI agent service is running"
        return 1
    fi
}

# Setup demonstration data
setup_demo_data() {
    echo -e "${YELLOW}🎮 Setting up demonstration data...${NC}"
    
    # Create some test ads for the demonstration
    local test_ads=(
        '{"title":"Dragon Quest Adventure","gameFamily":"RPG-Fantasy","targetAudience":["teens","young-adults"],"priority":3,"maxWaitTime":600}'
        '{"title":"Modern Combat Strike","gameFamily":"FPS-Military","targetAudience":["young-adults","adults"],"priority":4,"maxWaitTime":300}'
        '{"title":"Strategy Empire","gameFamily":"Strategy-RTS","targetAudience":["adults","strategy-players"],"priority":2,"maxWaitTime":900}'
        '{"title":"Racing Championship","gameFamily":"Racing-Arcade","targetAudience":["teens","young-adults"],"priority":1,"maxWaitTime":1200}'
        '{"title":"Puzzle Quest Pro","gameFamily":"Puzzle-Match3","targetAudience":["all-ages","casual-gamers"],"priority":5,"maxWaitTime":180}'
    )
    
    local created_count=0
    for ad_data in "${test_ads[@]}"; do
        local response=$(curl -s -X POST "$API_BASE_URL/ads" \
            -H "Content-Type: application/json" \
            -d "$ad_data")
        
        if echo "$response" | jq -e '.adId' > /dev/null 2>&1; then
            ((created_count++))
            echo -n "✅ "
        else
            echo -n "❌ "
        fi
    done
    
    echo ""
    echo "📊 Created $created_count test ads for demonstration"
    echo ""
}

# Display queue status
show_initial_status() {
    echo -e "${YELLOW}📊 Initial Queue Status${NC}"
    echo "========================"
    
    local stats_response=$(curl -s "$API_BASE_URL/ads/queue/stats")
    if echo "$stats_response" | jq -e '.totalSize' > /dev/null 2>&1; then
        echo "Queue Statistics:"
        echo "$stats_response" | jq -r '
        "  📋 Total Ads: \(.totalSize)",
        "  🎯 By Priority:",
        (.priorityDistribution[]? | "    Priority \(.priority): \(.count) ads"),
        "  ⚡ Processing Rate: \(.processingRate // "N/A") ads/min"
        '
    else
        echo "Unable to fetch queue statistics"
    fi
    
    echo ""
}

# Main demonstration sections
demo_queue_modification() {
    echo -e "${GREEN}🔄 QUEUE MODIFICATION COMMANDS${NC}"
    echo "==============================="
    echo "These commands modify ad priorities and queue ordering:"
    echo ""
    
    for command in "${QUEUE_MODIFICATION_COMMANDS[@]}"; do
        execute_command "$command" "Queue Modification"
        pause_if_interactive
    done
}

demo_system_configuration() {
    echo -e "${GREEN}⚙️ SYSTEM CONFIGURATION COMMANDS${NC}"
    echo "================================="
    echo "These commands configure system behavior and parameters:"
    echo ""
    
    for command in "${SYSTEM_CONFIGURATION_COMMANDS[@]}"; do
        execute_command "$command" "System Configuration"
        pause_if_interactive
    done
}

demo_status_queries() {
    echo -e "${GREEN}📋 STATUS QUERY COMMANDS${NC}"
    echo "========================"
    echo "These commands retrieve information about queue state:"
    echo ""
    
    for command in "${STATUS_QUERY_COMMANDS[@]}"; do
        execute_command "$command" "Status Query"
        pause_if_interactive
    done
}

demo_analytics() {
    echo -e "${GREEN}📊 ANALYTICS COMMANDS${NC}"
    echo "====================="
    echo "These commands provide insights and performance metrics:"
    echo ""
    
    for command in "${ANALYTICS_COMMANDS[@]}"; do
        execute_command "$command" "Analytics"
        pause_if_interactive
    done
}

# Error handling demonstration
demo_error_handling() {
    echo -e "${GREEN}❌ ERROR HANDLING DEMONSTRATION${NC}"
    echo "==============================="
    echo "Testing how the system handles invalid commands:"
    echo ""
    
    local error_commands=(
        "This is not a valid command"
        "Change priority to 10 for all ads"  # Invalid priority
        "Show the next -5 ads"               # Invalid count
        ""                                   # Empty command
    )
    
    for command in "${error_commands[@]}"; do
        if [ -z "$command" ]; then
            command="[Empty Command]"
        fi
        execute_command "$command" "Error Test"
        pause_if_interactive
    done
}

# Natural language variations demonstration  
demo_natural_language() {
    echo -e "${GREEN}🗣️ NATURAL LANGUAGE VARIATIONS${NC}"
    echo "==============================="
    echo "Testing different ways to express the same commands:"
    echo ""
    
    local variations=(
        "What is the current queue distribution by priority?"
        "Show me the queue distribution by priority"
        "Display queue distribution by priority level"
        "Can you show the next 3 ads to be processed?"
        "I want to see the next 3 ads in the queue"
        "List the next 3 ads that will be processed"
    )
    
    for command in "${variations[@]}"; do
        execute_command "$command" "Natural Language"
        pause_if_interactive
    done
}

# Performance demonstration
demo_performance() {
    echo -e "${GREEN}⚡ PERFORMANCE DEMONSTRATION${NC}"
    echo "============================"
    echo "Testing rapid command execution:"
    echo ""
    
    local start_time=$(date +%s)
    local command_count=0
    
    local rapid_commands=(
        "What's the current queue distribution by priority?"
        "Show the next 5 ads to be processed"
        "What's the current queue distribution by priority?"
        "Show the next 3 ads to be processed"
        "What's the current queue distribution by priority?"
    )
    
    for command in "${rapid_commands[@]}"; do
        execute_command "$command" "Performance Test"
        ((command_count++))
        sleep 0.5  # Short delay for rapid execution
    done
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo -e "${PURPLE}📈 Performance Results:${NC}"
    echo "  🔢 Commands Executed: $command_count"
    echo "  ⏱️  Total Duration: ${duration}s"
    echo "  📊 Average Rate: $(echo "scale=2; $command_count / $duration" | bc -l) commands/second"
    echo ""
}

# Final status check
show_final_status() {
    echo -e "${YELLOW}📊 Final Queue Status${NC}"
    echo "====================="
    
    local stats_response=$(curl -s "$API_BASE_URL/ads/queue/stats")
    if echo "$stats_response" | jq -e '.totalSize' > /dev/null 2>&1; then
        echo "Updated Queue Statistics:"
        echo "$stats_response" | jq -r '
        "  📋 Total Ads: \(.totalSize)",
        "  🎯 By Priority:",
        (.priorityDistribution[]? | "    Priority \(.priority): \(.count) ads"),
        "  ⚡ Processing Rate: \(.processingRate // "N/A") ads/min"
        '
    else
        echo "Unable to fetch final queue statistics"
    fi
    
    echo ""
}

# Usage information
show_usage() {
    echo "Usage: $0 [DELAY_SECONDS] [DEMO_MODE]"
    echo ""
    echo "Parameters:"
    echo "  DELAY_SECONDS  - Delay between commands in automatic mode (default: 2)"
    echo "  DEMO_MODE     - 'interactive' or 'automatic' (default: interactive)"
    echo ""
    echo "Demo Modes:"
    echo "  interactive   - Pause between each command for user input"
    echo "  automatic     - Run continuously with specified delay"
    echo ""
    echo "Examples:"
    echo "  $0                    # Interactive mode with 2s delay"
    echo "  $0 1 automatic        # Automatic mode with 1s delay"
    echo "  $0 3 interactive      # Interactive mode with 3s delay"
    echo ""
    echo "Environment Variables:"
    echo "  API_BASE_URL - Base URL for the API (default: http://localhost:8443/api)"
    echo ""
}

# Main execution function
main() {
    # Handle help requests
    if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
        show_usage
        exit 0
    fi
    
    # Check dependencies
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}❌ Error: curl is required${NC}"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        echo -e "${RED}❌ Error: jq is required${NC}"
        exit 1
    fi
    
    if ! command -v bc &> /dev/null; then
        echo -e "${RED}❌ Error: bc is required${NC}"
        exit 1
    fi
    
    # Check API health
    if ! check_api_health; then
        exit 1
    fi
    
    # Check command endpoint
    if ! check_command_endpoint; then
        exit 1
    fi
    
    echo ""
    
    # Setup demonstration data
    setup_demo_data
    
    # Show initial status
    show_initial_status
    
    if [ "$DEMO_MODE" = "interactive" ]; then
        echo -e "${BLUE}🎭 Starting Interactive AI Command Demonstration${NC}"
        echo "This demonstration will showcase various AI agent capabilities."
        echo ""
        pause_if_interactive
    fi
    
    # Run demonstration sections
    demo_queue_modification
    demo_system_configuration
    demo_status_queries
    demo_analytics
    demo_natural_language
    demo_error_handling
    demo_performance
    
    # Show final status
    show_final_status
    
    echo -e "${GREEN}🎉 AI Agent Command Demonstration Complete!${NC}"
    echo ""
    echo "Summary of demonstrated capabilities:"
    echo "  ✅ Queue Modification Commands"
    echo "  ✅ System Configuration Commands"
    echo "  ✅ Status Query Commands"
    echo "  ✅ Analytics Commands"
    echo "  ✅ Natural Language Processing"
    echo "  ✅ Error Handling"
    echo "  ✅ Performance Testing"
    echo ""
    echo "🔗 For more information, see the API documentation or run:"
    echo "   curl $API_BASE_URL/../docs"
    echo ""
}

# Execute main function
main "$@"