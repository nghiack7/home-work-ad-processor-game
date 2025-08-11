#!/bin/bash

# 🎮 Ad Generation Simulation Script
# Generates realistic gaming ads for testing the ad processing queue system
# Implements FR-SIM-001: Ad generation simulation with realistic gaming data

set -e

# Configuration
API_BASE_URL=${API_BASE_URL:-"http://localhost:8443/api"}
SIMULATION_TYPE=${1:-"balanced"}
AD_COUNT=${2:-50}
DELAY_MS=${3:-100}
OUTPUT_FILE=${4:-"simulation_results.json"}

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🎮 Gaming Ad Simulation Script${NC}"
echo -e "${BLUE}================================${NC}"
echo "📊 Simulation Type: $SIMULATION_TYPE"
echo "🎯 Ad Count: $AD_COUNT"
echo "⏱️  Delay: ${DELAY_MS}ms"
echo "💾 Output: $OUTPUT_FILE"
echo ""

# Game families and their characteristics
declare -A GAME_FAMILIES=(
    ["RPG-Fantasy"]="Epic fantasy adventures, dragons, magic"
    ["FPS-Military"]="Modern warfare, tactical combat, military themes"
    ["Strategy-RTS"]="Real-time strategy, base building, resource management"
    ["MOBA"]="Multiplayer online battle arena, team combat, competitive"
    ["Racing-Arcade"]="Fast-paced racing, arcade action, street racing"
    ["Puzzle-Match3"]="Match-3 puzzles, casual gaming, brain teasers"
    ["Sports-Football"]="American football, NFL, fantasy sports"
    ["Adventure-Action"]="Action adventures, exploration, treasure hunting"
    ["Horror-Survival"]="Survival horror, zombies, psychological thriller"
    ["Simulation-City"]="City building, management simulation, urban planning"
)

# Target audiences by game family
declare -A TARGET_AUDIENCES=(
    ["RPG-Fantasy"]="teens,young-adults,fantasy-fans"
    ["FPS-Military"]="young-adults,adults,competitive-gamers"
    ["Strategy-RTS"]="adults,strategy-players,hardcore-gamers"
    ["MOBA"]="teens,young-adults,esports-fans"
    ["Racing-Arcade"]="teens,young-adults,racing-fans"
    ["Puzzle-Match3"]="all-ages,casual-gamers,mobile-users"
    ["Sports-Football"]="young-adults,adults,sports-fans"
    ["Adventure-Action"]="teens,young-adults,adventure-seekers"
    ["Horror-Survival"]="young-adults,adults,horror-fans"
    ["Simulation-City"]="adults,simulation-fans,strategy-players"
)

# Ad titles by game family
declare -A AD_TITLES=(
    ["RPG-Fantasy"]="Dragon's Quest Legends,Mystic Realm Adventure,Fantasy Kingdom Wars,Enchanted Sword Legacy,Wizard's Tower Defense"
    ["FPS-Military"]="Modern Combat Strike,Tactical Warfare Elite,Military Ops Command,Special Forces Mission,Combat Zone Heroes"
    ["Strategy-RTS"]="Empire Builder Supreme,Command & Conquer Legends,Strategic Warfare,Base Defense Pro,Resource Wars"
    ["MOBA"]="Arena Champions,Battle Royale Legends,Team Combat Elite,MOBA Masters,Competitive Arena"
    ["Racing-Arcade"]="Street Racing Fury,Speed Demons,Turbo Rush,Racing Championship,Fast & Furious"
    ["Puzzle-Match3"]="Candy Crush Saga,Match Masters,Puzzle Quest,Gem Legends,Brain Teasers"
    ["Sports-Football"]="NFL Championship,Football Manager,Quarterback Challenge,Touchdown Heroes,Fantasy Football"
    ["Adventure-Action"]="Treasure Hunter,Action Adventure Pro,Explorer's Quest,Tomb Raider Legacy,Adventure Island"
    ["Horror-Survival"]="Zombie Apocalypse,Survival Horror,Dead Zone,Horror Nights,Survival Instinct"
    ["Simulation-City"]="City Builder Pro,Urban Planning Sim,Metropolis Manager,Smart City,Town Constructor"
)

# Priority distribution functions
get_priority_balanced() {
    echo $((1 + RANDOM % 5))
}

get_priority_high_load() {
    # 60% high priority (4-5), 40% others
    local rand=$((RANDOM % 10))
    if [ $rand -lt 6 ]; then
        echo $((4 + RANDOM % 2))
    else
        echo $((1 + RANDOM % 3))
    fi
}

get_priority_low_load() {
    # 60% low priority (1-2), 40% others
    local rand=$((RANDOM % 10))
    if [ $rand -lt 6 ]; then
        echo $((1 + RANDOM % 2))
    else
        echo $((3 + RANDOM % 3))
    fi
}

get_priority_realistic() {
    # Realistic distribution: 10% P5, 20% P4, 40% P3, 20% P2, 10% P1
    local rand=$((RANDOM % 100))
    if [ $rand -lt 10 ]; then
        echo 5
    elif [ $rand -lt 30 ]; then
        echo 4
    elif [ $rand -lt 70 ]; then
        echo 3
    elif [ $rand -lt 90 ]; then
        echo 2
    else
        echo 1
    fi
}

# Max wait time variations
get_max_wait_time() {
    local priority=$1
    case $priority in
        5) echo $((180 + RANDOM % 120));;  # 3-5 minutes for high priority
        4) echo $((300 + RANDOM % 180));;  # 5-8 minutes
        3) echo $((600 + RANDOM % 300));;  # 10-15 minutes
        2) echo $((900 + RANDOM % 600));;  # 15-25 minutes
        1) echo $((1800 + RANDOM % 600));; # 30-40 minutes for low priority
    esac
}

# Get random element from array
get_random_element() {
    local array_string="$1"
    IFS=',' read -ra elements <<< "$array_string"
    local count=${#elements[@]}
    local index=$((RANDOM % count))
    echo "${elements[$index]}"
}

# Generate ad data
generate_ad() {
    local game_families=($(printf '%s\n' "${!GAME_FAMILIES[@]}"))
    local family_count=${#game_families[@]}
    local game_family=${game_families[$((RANDOM % family_count))]}
    
    local title=$(get_random_element "${AD_TITLES[$game_family]}")
    local target_audience="${TARGET_AUDIENCES[$game_family]}"
    
    local priority
    case $SIMULATION_TYPE in
        "balanced") priority=$(get_priority_balanced);;
        "high-load") priority=$(get_priority_high_load);;
        "low-load") priority=$(get_priority_low_load);;
        "realistic") priority=$(get_priority_realistic);;
        *) priority=$(get_priority_balanced);;
    esac
    
    local max_wait_time=$(get_max_wait_time $priority)
    
    cat << EOF
{
    "title": "$title",
    "gameFamily": "$game_family",
    "targetAudience": $(echo "[$target_audience]" | sed 's/,/","/g' | sed 's/\[/["/' | sed 's/\]/"]/' | sed 's/""/"/' ),
    "priority": $priority,
    "maxWaitTime": $max_wait_time
}
EOF
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

# Create ad function
create_ad() {
    local ad_data="$1"
    local response=$(curl -s -X POST "$API_BASE_URL/ads" \
        -H "Content-Type: application/json" \
        -d "$ad_data")
    
    if echo "$response" | jq -e '.adId' > /dev/null 2>&1; then
        echo "$response"
        return 0
    else
        echo "ERROR: $response" >&2
        return 1
    fi
}

# Initialize results file
init_results() {
    cat > "$OUTPUT_FILE" << EOF
{
    "simulation": {
        "type": "$SIMULATION_TYPE",
        "adCount": $AD_COUNT,
        "delayMs": $DELAY_MS,
        "startTime": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    },
    "results": {
        "created": [],
        "failed": [],
        "statistics": {}
    }
}
EOF
}

# Update results function
update_results() {
    local ad_response="$1"
    local status="$2"
    local temp_file=$(mktemp)
    
    if [ "$status" = "success" ]; then
        jq --argjson ad "$ad_response" '.results.created += [$ad]' "$OUTPUT_FILE" > "$temp_file"
    else
        jq --arg error "$ad_response" '.results.failed += [$error]' "$OUTPUT_FILE" > "$temp_file"
    fi
    
    mv "$temp_file" "$OUTPUT_FILE"
}

# Calculate final statistics
finalize_results() {
    local end_time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local temp_file=$(mktemp)
    
    jq --arg endTime "$end_time" '
    .simulation.endTime = $endTime |
    .results.statistics = {
        "totalCreated": (.results.created | length),
        "totalFailed": (.results.failed | length),
        "successRate": ((.results.created | length) / (.results.created | length + .results.failed | length) * 100),
        "priorityDistribution": (.results.created | group_by(.priority) | map({priority: .[0].priority, count: length}) | sort_by(.priority)),
        "gameFamilyDistribution": (.results.created | group_by(.gameFamily) | map({gameFamily: .[0].gameFamily, count: length}) | sort_by(.count) | reverse)
    }' "$OUTPUT_FILE" > "$temp_file"
    
    mv "$temp_file" "$OUTPUT_FILE"
}

# Main simulation function
run_simulation() {
    echo -e "${YELLOW}🚀 Starting ad generation simulation...${NC}"
    echo ""
    
    local success_count=0
    local failure_count=0
    local start_time=$(date +%s)
    
    for i in $(seq 1 $AD_COUNT); do
        echo -n "📝 Creating ad $i/$AD_COUNT... "
        
        local ad_data=$(generate_ad)
        local response=$(create_ad "$ad_data")
        local create_result=$?
        
        if [ $create_result -eq 0 ]; then
            local ad_id=$(echo "$response" | jq -r '.adId')
            local priority=$(echo "$response" | jq -r '.priority')
            local position=$(echo "$response" | jq -r '.position // "N/A"')
            
            echo -e "${GREEN}✅ Created${NC} ID: ${ad_id:0:8}... Priority: $priority Position: $position"
            update_results "$response" "success"
            ((success_count++))
        else
            echo -e "${RED}❌ Failed${NC} - $response"
            update_results "$response" "failure"
            ((failure_count++))
        fi
        
        # Add delay between requests
        if [ $i -lt $AD_COUNT ]; then
            sleep $(echo "scale=3; $DELAY_MS / 1000" | bc -l)
        fi
    done
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo -e "${BLUE}📊 Simulation Complete${NC}"
    echo "⏱️  Duration: ${duration}s"
    echo -e "✅ Success: ${GREEN}$success_count${NC}"
    echo -e "❌ Failed: ${RED}$failure_count${NC}"
    
    if [ $success_count -gt 0 ]; then
        local success_rate=$((success_count * 100 / (success_count + failure_count)))
        echo "📈 Success Rate: ${success_rate}%"
    fi
    
    finalize_results
    echo "💾 Results saved to: $OUTPUT_FILE"
}

# Display usage
show_usage() {
    echo "Usage: $0 [SIMULATION_TYPE] [AD_COUNT] [DELAY_MS] [OUTPUT_FILE]"
    echo ""
    echo "Simulation Types:"
    echo "  balanced   - Equal distribution across all priorities (default)"
    echo "  high-load  - 60% high priority (4-5), 40% others"
    echo "  low-load   - 60% low priority (1-2), 40% others"  
    echo "  realistic  - Realistic distribution (10% P5, 20% P4, 40% P3, 20% P2, 10% P1)"
    echo ""
    echo "Examples:"
    echo "  $0 balanced 100 50                    # 100 ads, balanced priorities, 50ms delay"
    echo "  $0 high-load 500 10 load_test.json   # 500 ads, high priority load test"
    echo "  $0 realistic 1000 5                  # 1000 ads, realistic distribution, fast"
    echo ""
    echo "Environment Variables:"
    echo "  API_BASE_URL - Base URL for the API (default: http://localhost:8443/api)"
    echo ""
}

# Command line argument handling
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    show_usage
    exit 0
fi

# Validate dependencies
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

# Main execution
main() {
    # Check API health
    if ! check_api_health; then
        exit 1
    fi
    
    echo ""
    
    # Initialize results file
    init_results
    
    # Run the simulation
    run_simulation
    
    echo ""
    echo -e "${GREEN}🎉 Simulation completed successfully!${NC}"
    echo ""
    echo "Next steps:"
    echo "  • View results: cat $OUTPUT_FILE | jq '.results.statistics'"
    echo "  • Check queue stats: curl $API_BASE_URL/ads/queue/stats | jq '.'"
    echo "  • Monitor processing: watch -n 2 'curl -s $API_BASE_URL/ads/queue/stats | jq .'"
    echo ""
}

# Run main function
main "$@"