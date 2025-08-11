#!/bin/bash

# 🛑 Development Environment Stop Script
# Gracefully stop all development services

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging
log() { echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
info() { echo -e "${CYAN}[INFO]${NC} $1"; }

# Print header
print_header() {
    echo -e "${PURPLE}"
    echo "============================================================================="
    echo "                   🛑 STOPPING DEVELOPMENT ENVIRONMENT"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Gracefully stopping all services...${NC}"
    echo ""
}

# Stop application services
stop_application_services() {
    log "Stopping application services..."
    
    cd "$PROJECT_ROOT"
    
    # Stop services using PID files
    local services=("ad-api" "ad-processor" "ai-agent")
    
    for service in "${services[@]}"; do
        if [ -f ".pids/${service}.pid" ]; then
            local pid=$(cat ".pids/${service}.pid")
            if kill -0 "$pid" 2>/dev/null; then
                log "Stopping $service (PID: $pid)..."
                kill -TERM "$pid" 2>/dev/null || true
                
                # Wait for graceful shutdown
                for i in {1..10}; do
                    if ! kill -0 "$pid" 2>/dev/null; then
                        success "$service stopped gracefully"
                        break
                    fi
                    sleep 1
                done
                
                # Force kill if still running
                if kill -0 "$pid" 2>/dev/null; then
                    warn "Force killing $service..."
                    kill -KILL "$pid" 2>/dev/null || true
                fi
            fi
            rm -f ".pids/${service}.pid"
        else
            info "$service PID file not found (may not be running)"
        fi
    done
}

# Stop infrastructure services
stop_infrastructure() {
    log "Stopping infrastructure services..."
    
    cd "$PROJECT_ROOT"
    
    # Stop Docker Compose services
    log "Stopping Docker services..."
    docker-compose down
    
    success "Infrastructure services stopped"
}

# Clean up resources
cleanup_resources() {
    log "Cleaning up resources..."
    
    cd "$PROJECT_ROOT"
    
    # Remove PID files directory
    rm -rf .pids
    
    # Clean up logs (optional - keep them for debugging)
    if [ "$CLEAN_LOGS" = "true" ]; then
        log "Cleaning up log files..."
        rm -f logs/*.log
    fi
    
    # Clean up build artifacts (optional)
    if [ "$CLEAN_BUILD" = "true" ]; then
        log "Cleaning up build artifacts..."
        rm -rf bin/
    fi
    
    success "Cleanup completed"
}

# Kill any remaining processes
kill_remaining_processes() {
    log "Checking for remaining processes..."
    
    # Find processes by common ports
    local ports=(8080 8000 5433 6379 9090 3000)
    local killed=false
    
    for port in "${ports[@]}"; do
        local pids=$(lsof -ti:$port 2>/dev/null || true)
        if [ -n "$pids" ]; then
            warn "Found processes on port $port: $pids"
            echo "$pids" | xargs kill -TERM 2>/dev/null || true
            killed=true
        fi
    done
    
    if [ "$killed" = true ]; then
        sleep 2
        warn "Some processes were killed"
    else
        info "No remaining processes found"
    fi
}

# Check if services are really stopped
verify_shutdown() {
    log "Verifying services are stopped..."
    
    local ports=(8080 8000)
    local still_running=false
    
    for port in "${ports[@]}"; do
        if curl -s --max-time 2 "http://localhost:$port/health" > /dev/null 2>&1; then
            warn "Service on port $port is still responding"
            still_running=true
        fi
    done
    
    if [ "$still_running" = false ]; then
        success "All services have stopped"
    else
        warn "Some services may still be running"
    fi
}

# Generate stop summary
generate_summary() {
    echo ""
    echo -e "${PURPLE}============================================================================="
    echo "                     🛑 SERVICES STOPPED SUCCESSFULLY"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${GREEN}Development environment has been shut down.${NC}"
    echo ""
    echo -e "${CYAN}📋 Stopped Services:${NC}"
    echo "  ✓ Ad API Service"
    echo "  ✓ Ad Processor Service"  
    echo "  ✓ AI Agent Service"
    echo "  ✓ PostgreSQL Database"
    echo "  ✓ Redis Cache"
    echo "  ✓ Prometheus Monitoring"
    echo "  ✓ Grafana Dashboard"
    echo ""
    echo -e "${CYAN}📋 Resources:${NC}"
    if [ "$CLEAN_LOGS" != "true" ]; then
        echo "  📋 Log files preserved in logs/ directory"
    else
        echo "  🗑️  Log files cleaned up"
    fi
    
    if [ "$CLEAN_BUILD" != "true" ]; then
        echo "  📦 Build artifacts preserved in bin/ directory"
    else
        echo "  🗑️  Build artifacts cleaned up"
    fi
    echo ""
    echo -e "${CYAN}📋 Next Steps:${NC}"
    echo "  To start again:          ./scripts/dev-start.sh"
    echo "  To view preserved logs:  tail -f logs/*.log"
    echo "  For production setup:    ./scripts/k8s-deploy.sh"
    echo ""
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Stop all development services gracefully."
    echo ""
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  --skip-infrastructure   Skip stopping Docker infrastructure"
    echo "  --clean-logs           Remove log files during cleanup"
    echo "  --clean-build          Remove build artifacts during cleanup"
    echo "  --force                Force kill all processes without grace period"
    echo ""
    echo "This script stops:"
    echo "  - All application services (API, Processor, AI Agent)"
    echo "  - Infrastructure services (PostgreSQL, Redis, Prometheus, Grafana)"
    echo "  - Cleans up PID files and optionally logs/builds"
    echo ""
}

# Force kill all processes
force_kill_all() {
    warn "Force killing all processes..."
    
    # Kill by process name patterns
    pkill -f "ad-api" 2>/dev/null || true
    pkill -f "ad-processor" 2>/dev/null || true
    pkill -f "ai-agent.*main.py" 2>/dev/null || true
    
    # Kill by ports
    kill_remaining_processes
    
    # Stop Docker forcefully
    docker-compose down --remove-orphans -v 2>/dev/null || true
    
    warn "Force kill completed"
}

# Main execution
main() {
    # Parse command line arguments
    local skip_infrastructure=false
    local force_kill=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            --skip-infrastructure)
                skip_infrastructure=true
                shift
                ;;
            --clean-logs)
                export CLEAN_LOGS=true
                shift
                ;;
            --clean-build)
                export CLEAN_BUILD=true
                shift
                ;;
            --force)
                force_kill=true
                shift
                ;;
            *)
                error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    print_header
    
    if [ "$force_kill" = true ]; then
        force_kill_all
    else
        stop_application_services
        
        if [ "$skip_infrastructure" = false ]; then
            stop_infrastructure
        fi
        
        kill_remaining_processes
    fi
    
    cleanup_resources
    verify_shutdown
    generate_summary
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi