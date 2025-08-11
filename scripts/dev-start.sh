#!/bin/bash

# 🚀 Development Environment Startup Script
# Quick start for local development with all services

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
    echo "                   🚀 DEVELOPMENT ENVIRONMENT STARTUP"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Starting all services for local development...${NC}"
    echo ""
}

# Load environment variables
load_environment() {
    cd "$PROJECT_ROOT"
    
    if [ -f ".env" ]; then
        log "Loading environment variables from .env"
        set -a
        source .env
        set +a
        success "Environment loaded"
    else
        warn ".env file not found, using defaults"
    fi
}

# Start infrastructure services
start_infrastructure() {
    log "Starting infrastructure services..."
    
    cd "$PROJECT_ROOT"
    
    # Start Docker Compose services
    log "Starting PostgreSQL, Redis, and monitoring..."
    docker-compose up -d postgres redis prometheus grafana
    
    # Wait for services to be ready
    log "Waiting for services to be ready..."
    sleep 5
    
    # Check PostgreSQL
    log "Checking PostgreSQL connection..."
    for i in {1..30}; do
        if docker-compose exec -T postgres pg_isready -U postgres > /dev/null 2>&1; then
            success "PostgreSQL is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "PostgreSQL failed to start"
            exit 1
        fi
        sleep 1
    done
    
    # Check Redis
    log "Checking Redis connection..."
    for i in {1..30}; do
        if docker-compose exec -T redis redis-cli ping > /dev/null 2>&1; then
            success "Redis is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "Redis failed to start"
            exit 1
        fi
        sleep 1
    done
}

# Run database migrations
run_migrations() {
    log "Running database migrations..."
    
    cd "$PROJECT_ROOT"
    
    # Build and run migration tool
    go build -o bin/migrate ./cmd/migrate
    
    export APP_DATABASE_HOST=localhost
    export APP_DATABASE_PORT=5433
    export APP_DATABASE_USER=postgres  
    export APP_DATABASE_PASSWORD=${DB_PASSWORD:-postgres}
    export APP_DATABASE_NAME=adprocessing
    export APP_DATABASE_SSLMODE=disable
    
    ./bin/migrate -up -v
    
    success "Database migrations completed"
}

# Build and start application services
start_application_services() {
    log "Building application services..."
    
    cd "$PROJECT_ROOT"
    
    # Build services
    log "Building ad-api..."
    go build -o bin/ad-api ./cmd/ad-api
    
    log "Building ad-processor..."
    go build -o bin/ad-processor ./cmd/ad-processor
    
    success "Services built successfully"
    
    # Start services in background
    log "Starting ad-processor..."
    nohup ./bin/ad-processor > logs/ad-processor.log 2>&1 &
    echo $! > .pids/ad-processor.pid
    
    sleep 2
    
    log "Starting ad-api..."
    nohup ./bin/ad-api > logs/ad-api.log 2>&1 &
    echo $! > .pids/ad-api.pid
    
    success "Application services started"
}

# Start AI agent
start_ai_agent() {
    log "Starting AI Agent service..."
    
    cd "$PROJECT_ROOT/ai-agent"
    
    # Install Python dependencies if not already installed
    if [ ! -d "venv" ]; then
        log "Setting up Python virtual environment..."
        python3 -m venv venv
        source venv/bin/activate
        pip install -r requirements.txt
    else
        source venv/bin/activate
    fi
    
    # Start AI agent
    log "Starting Python AI agent..."
    nohup python main.py > ../logs/ai-agent.log 2>&1 &
    echo $! > ../.pids/ai-agent.pid
    
    cd "$PROJECT_ROOT"
    success "AI Agent started"
}

# Create necessary directories
create_directories() {
    cd "$PROJECT_ROOT"
    
    mkdir -p logs
    mkdir -p .pids
    mkdir -p bin
}

# Wait for services to be ready
wait_for_services() {
    log "Waiting for all services to be ready..."
    
    # Wait for ad-api
    local api_ready=false
    for i in {1..60}; do
        if curl -s http://localhost:8080/health > /dev/null 2>&1; then
            api_ready=true
            break
        fi
        sleep 1
    done
    
    if [ "$api_ready" = true ]; then
        success "Ad API is ready"
    else
        warn "Ad API is not responding (may need more time)"
    fi
    
    # Wait for AI agent
    local ai_ready=false
    for i in {1..30}; do
        if curl -s http://localhost:8000/health > /dev/null 2>&1; then
            ai_ready=true
            break
        fi
        sleep 1
    done
    
    if [ "$ai_ready" = true ]; then
        success "AI Agent is ready"
    else
        warn "AI Agent is not responding (check logs if needed)"
    fi
}

# Run basic health checks
run_health_checks() {
    log "Running health checks..."
    
    # Check API health
    if curl -s http://localhost:8080/health | grep -q "healthy"; then
        success "✓ Ad API health check passed"
    else
        warn "⚠ Ad API health check failed"
    fi
    
    # Check database connectivity
    if curl -s http://localhost:8080/ready | grep -q "ready"; then
        success "✓ Database connectivity check passed"
    else
        warn "⚠ Database connectivity check failed"
    fi
    
    # Test ad creation
    local ad_response=$(curl -s -X POST http://localhost:8080/api/v1/ads \
        -H "Content-Type: application/json" \
        -d '{
            "title": "Dev Test Ad",
            "gameFamily": "Test",
            "targetAudience": ["developers"],
            "priority": 3,
            "maxWaitTime": 300
        }')
    
    if echo "$ad_response" | grep -q "adId"; then
        success "✓ Ad creation test passed"
    else
        warn "⚠ Ad creation test failed"
    fi
}

# Generate development summary
generate_summary() {
    echo ""
    echo -e "${PURPLE}============================================================================="
    echo "                   🎉 DEVELOPMENT ENVIRONMENT READY"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${GREEN}All services are running and ready for development!${NC}"
    echo ""
    echo -e "${CYAN}📋 Service URLs:${NC}"
    echo "  🌐 Ad Processing API:    http://localhost:8080"
    echo "  🤖 AI Agent Service:     http://localhost:8000"
    echo "  📊 Prometheus:           http://localhost:9090"
    echo "  📈 Grafana:             http://localhost:3000 (admin/admin123)"
    echo ""
    echo -e "${CYAN}📋 API Endpoints:${NC}"
    echo "  Health Check:            GET http://localhost:8080/health"
    echo "  Create Ad:              POST http://localhost:8080/api/v1/ads"
    echo "  Get Ad Status:           GET http://localhost:8080/api/v1/ads/{id}"
    echo "  AI Commands:            POST http://localhost:8080/api/v1/agent/command"
    echo "  Queue Stats:             GET http://localhost:8080/api/v1/ads/queue/stats"
    echo ""
    echo -e "${CYAN}📋 Useful Commands:${NC}"
    echo "  View logs:               ./scripts/logs.sh"
    echo "  Stop services:           ./scripts/dev-stop.sh"
    echo "  Restart services:        ./scripts/dev-restart.sh"
    echo "  Run tests:               ./scripts/test.sh"
    echo "  Load testing:            ./scripts/load-test.sh"
    echo ""
    echo -e "${CYAN}📋 Example API Usage:${NC}"
    cat << 'EOF'
  # Create a test ad
  curl -X POST http://localhost:8080/api/v1/ads \
    -H "Content-Type: application/json" \
    -d '{
      "title": "Epic RPG Adventure",
      "gameFamily": "RPG-Fantasy",
      "targetAudience": ["gamers", "rpg-fans"],
      "priority": 5,
      "maxWaitTime": 300
    }'
  
  # Get queue statistics
  curl http://localhost:8080/api/v1/ads/queue/stats
  
  # AI command example
  curl -X POST http://localhost:8080/api/v1/agent/command \
    -H "Content-Type: application/json" \
    -d '{"command": "Show the next 5 ads to be processed"}'
EOF
    echo ""
    echo -e "${YELLOW}⚠️  Note: Services are running in background. Use ./scripts/dev-stop.sh to stop them.${NC}"
    echo ""
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Start the complete development environment with all services."
    echo ""
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  --skip-infrastructure   Skip starting Docker infrastructure"
    echo "  --skip-migrations       Skip database migrations"
    echo "  --skip-ai-agent         Skip starting AI agent"
    echo "  --skip-health-checks    Skip health checks"
    echo ""
    echo "This script starts:"
    echo "  - PostgreSQL database"
    echo "  - Redis cache"
    echo "  - Prometheus monitoring"
    echo "  - Grafana dashboard"
    echo "  - Ad API service"
    echo "  - Ad Processor service"
    echo "  - AI Agent service"
    echo ""
}

# Cleanup function
cleanup() {
    if [ $? -ne 0 ]; then
        error "Startup failed. Check logs for details."
        echo ""
        echo "Logs:"
        echo "  Ad API:      tail -f logs/ad-api.log"
        echo "  Ad Processor: tail -f logs/ad-processor.log" 
        echo "  AI Agent:    tail -f logs/ai-agent.log"
        echo ""
        echo "To stop services: ./scripts/dev-stop.sh"
    fi
}

# Main execution
main() {
    # Parse command line arguments
    local skip_infrastructure=false
    local skip_migrations=false
    local skip_ai_agent=false
    local skip_health_checks=false
    
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
            --skip-migrations)
                skip_migrations=true
                shift
                ;;
            --skip-ai-agent)
                skip_ai_agent=true
                shift
                ;;
            --skip-health-checks)
                skip_health_checks=true
                shift
                ;;
            *)
                error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # Set up cleanup trap
    trap cleanup EXIT
    
    print_header
    create_directories
    load_environment
    
    if [ "$skip_infrastructure" = false ]; then
        start_infrastructure
    fi
    
    if [ "$skip_migrations" = false ]; then
        run_migrations
    fi
    
    start_application_services
    
    if [ "$skip_ai_agent" = false ]; then
        start_ai_agent
    fi
    
    wait_for_services
    
    if [ "$skip_health_checks" = false ]; then
        run_health_checks
    fi
    
    generate_summary
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi