#!/bin/bash

# 🐳 Docker Compose Production Startup Script
# Complete production-ready deployment using Docker Compose

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"
ENV_FILE="${PROJECT_ROOT}/.env"

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
    echo "               🐳 DOCKER COMPOSE PRODUCTION DEPLOYMENT"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Deploying production-ready services with Docker Compose...${NC}"
    echo ""
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed"
        exit 1
    fi
    
    # Check Docker Compose
    if ! docker compose version &> /dev/null && ! docker-compose --version &> /dev/null; then
        error "Docker Compose is not installed"
        exit 1
    fi
    
    # Check if Docker is running
    if ! docker info &> /dev/null; then
        error "Docker is not running. Please start Docker Desktop."
        exit 1
    fi
    
    success "Prerequisites check passed"
}

# Setup environment
setup_environment() {
    log "Setting up environment..."
    
    cd "$PROJECT_ROOT"
    
    # Load .env file if it exists
    if [ -f "$ENV_FILE" ]; then
        log "Loading environment from .env file"
        set -a
        source "$ENV_FILE"
        set +a
    else
        warn ".env file not found, creating with defaults"
        cat > "$ENV_FILE" << 'EOF'
# Production Environment Configuration
APP_ENV=production
APP_LOG_LEVEL=info

# Database Configuration
DB_PASSWORD=secure_postgres_password_2024
POSTGRES_DB=adprocessing
POSTGRES_USER=postgres

# Redis Configuration
REDIS_PASSWORD=secure_redis_password_2024

# AI Integration
GOOGLE_AI_API_KEY=

# Application Ports
API_PORT=8443
AI_AGENT_PORT=8080
POSTGRES_PORT=5433
REDIS_PORT=6380
PROMETHEUS_PORT=9090
GRAFANA_PORT=3000

# Performance Configuration
WORKER_COUNT=3
MAX_WAIT_TIME_SECONDS=300
ANTI_STARVATION_ENABLED=true
EOF
        warn "Please update .env file with your configuration before proceeding"
    fi
    
    # Validate required environment variables
    local required_vars=("DB_PASSWORD" "REDIS_PASSWORD")
    local missing_vars=()
    
    for var in "${required_vars[@]}"; do
        if [ -z "${!var}" ]; then
            missing_vars+=("$var")
        fi
    done
    
    if [ ${#missing_vars[@]} -gt 0 ]; then
        error "Missing required environment variables: ${missing_vars[*]}"
        error "Please update .env file with the required values"
        exit 1
    fi
    
    success "Environment configured"
}

# Build images
build_images() {
    log "Building Docker images..."
    
    cd "$PROJECT_ROOT"
    
    # Build services
    local services=("ad-api" "ad-processor" "migrate")
    
    for service in "${services[@]}"; do
        if [ -f "Dockerfile.${service}" ]; then
            log "Building ${service} image..."
            docker build -f "Dockerfile.${service}" -t "${service}:latest" . --quiet
            success "Built ${service}:latest"
        else
            warn "Dockerfile.${service} not found, skipping"
        fi
    done
    
    # Build AI agent
    if [ -d "ai-agent" ] && [ -f "ai-agent/Dockerfile" ]; then
        log "Building AI agent image..."
        docker build -f ai-agent/Dockerfile -t ai-agent:latest ./ai-agent --quiet
        success "Built ai-agent:latest"
    else
        warn "AI agent Dockerfile not found, skipping"
    fi
    
    success "All images built successfully"
}

# Start infrastructure services
start_infrastructure() {
    log "Starting infrastructure services..."
    
    cd "$PROJECT_ROOT"
    
    # Start infrastructure services first
    log "Starting PostgreSQL and Redis..."
    docker compose up -d postgres redis
    
    # Wait for services to be ready
    log "Waiting for infrastructure services to be ready..."
    
    # Wait for PostgreSQL
    for i in {1..60}; do
        if docker compose exec postgres pg_isready -U postgres > /dev/null 2>&1; then
            success "PostgreSQL is ready"
            break
        fi
        if [ $i -eq 60 ]; then
            error "PostgreSQL failed to start within timeout"
            exit 1
        fi
        sleep 1
    done
    
    # Wait for Redis
    for i in {1..30}; do
        if docker compose exec redis redis-cli ping > /dev/null 2>&1; then
            success "Redis is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            error "Redis failed to start within timeout"
            exit 1
        fi
        sleep 1
    done
}

# Run database migrations
run_migrations() {
    log "Running database migrations..."
    
    cd "$PROJECT_ROOT"
    
    # Create migration service configuration
    cat > docker-compose.migration.yml << EOF
version: '3.8'
services:
  migrate:
    image: migrate:latest
    depends_on:
      - postgres
    environment:
      - APP_DATABASE_HOST=postgres
      - APP_DATABASE_PORT=5432
      - APP_DATABASE_USER=postgres
      - APP_DATABASE_PASSWORD=\${DB_PASSWORD}
      - APP_DATABASE_NAME=adprocessing
      - APP_DATABASE_SSLMODE=disable
    networks:
      - app-network
    command: ["-up", "-v"]

networks:
  app-network:
    external: true
EOF
    
    # Run migrations
    docker compose -f docker-compose.migration.yml up migrate --remove-orphans
    docker compose -f docker-compose.migration.yml down
    rm -f docker-compose.migration.yml
    
    success "Database migrations completed"
}

# Start application services
start_application_services() {
    log "Starting application services..."
    
    cd "$PROJECT_ROOT"
    
    # Start all services
    log "Starting all application services..."
    docker compose up -d
    
    success "Application services started"
}

# Start monitoring services
start_monitoring() {
    log "Starting monitoring services..."
    
    cd "$PROJECT_ROOT"
    
    # Start Prometheus and Grafana
    log "Starting Prometheus and Grafana..."
    docker compose up -d prometheus grafana
    
    success "Monitoring services started"
}

# Wait for all services to be ready
wait_for_services() {
    log "Waiting for all services to be ready..."
    
    # Wait for ad-api
    local api_ready=false
    for i in {1..60}; do
        if curl -s --max-time 5 "http://localhost:${API_PORT:-8443}/health" > /dev/null 2>&1; then
            api_ready=true
            break
        fi
        sleep 2
    done
    
    if [ "$api_ready" = true ]; then
        success "Ad API is ready"
    else
        warn "Ad API is not responding (check logs: docker compose logs ad-api)"
    fi
    
    # Wait for AI agent
    local ai_ready=false
    for i in {1..30}; do
        if curl -s --max-time 5 "http://localhost:${AI_AGENT_PORT:-8080}/health" > /dev/null 2>&1; then
            ai_ready=true
            break
        fi
        sleep 2
    done
    
    if [ "$ai_ready" = true ]; then
        success "AI Agent is ready"
    else
        warn "AI Agent is not responding (check logs: docker compose logs ai-agent)"
    fi
    
    # Wait for monitoring
    if curl -s --max-time 5 "http://localhost:${PROMETHEUS_PORT:-9090}/-/ready" > /dev/null 2>&1; then
        success "Prometheus is ready"
    else
        warn "Prometheus is not ready"
    fi
    
    if curl -s --max-time 5 "http://localhost:${GRAFANA_PORT:-3000}/api/health" > /dev/null 2>&1; then
        success "Grafana is ready"
    else
        warn "Grafana is not ready"
    fi
}

# Run health checks
run_health_checks() {
    log "Running comprehensive health checks..."
    
    # API Health Check
    if curl -s "http://localhost:${API_PORT:-8443}/health" | grep -q "healthy"; then
        success "✓ Ad API health check passed"
    else
        warn "⚠ Ad API health check failed"
    fi
    
    # Database connectivity
    if curl -s "http://localhost:${API_PORT:-8443}/ready" | grep -q "ready"; then
        success "✓ Database connectivity check passed"
    else
        warn "⚠ Database connectivity check failed"
    fi
    
    # Test ad creation
    local test_ad_response=$(curl -s -X POST "http://localhost:${API_PORT:-8443}/api/v1/ads" \
        -H "Content-Type: application/json" \
        -d '{
            "title": "Production Test Ad",
            "gameFamily": "Production-Test",
            "targetAudience": ["test-users"],
            "priority": 3,
            "maxWaitTime": 300
        }')
    
    if echo "$test_ad_response" | grep -q "adId"; then
        success "✓ Ad creation test passed"
        
        # Test AI agent command
        local ai_command_response=$(curl -s -X POST "http://localhost:${API_PORT:-8443}/api/v1/agent/command" \
            -H "Content-Type: application/json" \
            -d '{"command": "Show queue statistics"}')
        
        if echo "$ai_command_response" | grep -q "commandId"; then
            success "✓ AI agent command test passed"
        else
            warn "⚠ AI agent command test failed"
        fi
    else
        warn "⚠ Ad creation test failed"
    fi
}

# Generate deployment summary
generate_summary() {
    echo ""
    echo -e "${PURPLE}============================================================================="
    echo "                   🐳 DOCKER DEPLOYMENT COMPLETED"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${GREEN}Production environment is ready!${NC}"
    echo ""
    echo -e "${CYAN}📋 Service URLs:${NC}"
    echo "  🌐 Ad Processing API:    http://localhost:${API_PORT:-8443}"
    echo "  🤖 AI Agent Service:     http://localhost:${AI_AGENT_PORT:-8080}"  
    echo "  📊 Prometheus:           http://localhost:${PROMETHEUS_PORT:-9090}"
    echo "  📈 Grafana:             http://localhost:${GRAFANA_PORT:-3000} (admin/admin123)"
    echo ""
    echo -e "${CYAN}📋 Database Services:${NC}"
    echo "  🐘 PostgreSQL:          localhost:${POSTGRES_PORT:-5433}"
    echo "  🔴 Redis:               localhost:${REDIS_PORT:-6380}"
    echo ""
    echo -e "${CYAN}📋 API Endpoints:${NC}"
    echo "  Health Check:           GET http://localhost:${API_PORT:-8443}/health"
    echo "  API Documentation:      GET http://localhost:${API_PORT:-8443}/swagger"
    echo "  Create Ad:             POST http://localhost:${API_PORT:-8443}/api/v1/ads"
    echo "  AI Commands:           POST http://localhost:${API_PORT:-8443}/api/v1/agent/command"
    echo "  Queue Statistics:       GET http://localhost:${API_PORT:-8443}/api/v1/ads/queue/stats"
    echo "  Metrics:               GET http://localhost:${API_PORT:-8443}/metrics"
    echo ""
    echo -e "${CYAN}📋 Management Commands:${NC}"
    echo "  View logs:             docker compose logs -f [service]"
    echo "  Scale services:        docker compose up -d --scale ad-processor=3"
    echo "  Stop all:              docker compose down"
    echo "  Stop and cleanup:      docker compose down -v --remove-orphans"
    echo "  Restart service:       docker compose restart [service]"
    echo ""
    echo -e "${CYAN}📋 Testing Commands:${NC}"
    echo "  API Tests:             ./scripts/test.sh --url http://localhost:${API_PORT:-8443}"
    echo "  Load Test:             ./scripts/load-test.sh --url http://localhost:${API_PORT:-8443}"
    echo "  Acceptance Tests:      ./scripts/acceptance-test.sh"
    echo ""
    echo -e "${CYAN}📋 Example Usage:${NC}"
    cat << EOF
  # Create an ad
  curl -X POST http://localhost:${API_PORT:-8443}/api/v1/ads \\
    -H "Content-Type: application/json" \\
    -d '{
      "title": "Epic Adventure Game",
      "gameFamily": "RPG-Fantasy",
      "targetAudience": ["gamers", "fantasy-fans"],
      "priority": 5,
      "maxWaitTime": 600
    }'
  
  # AI command
  curl -X POST http://localhost:${API_PORT:-8443}/api/v1/agent/command \\
    -H "Content-Type: application/json" \\
    -d '{"command": "Show the next 10 ads to be processed"}'
EOF
    echo ""
    echo -e "${YELLOW}⚠️  Note: Services are running with production configuration.${NC}"
    echo -e "${YELLOW}⚠️  Use 'docker compose down -v' to stop and remove all data.${NC}"
    echo ""
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Deploy the complete production environment using Docker Compose."
    echo ""
    echo "Options:"
    echo "  -h, --help             Show this help message"
    echo "  --skip-build           Skip building Docker images"
    echo "  --skip-migrations      Skip database migrations"
    echo "  --skip-health-checks   Skip health checks"
    echo "  --scale-processors N   Scale ad-processor to N instances"
    echo ""
    echo "Environment Variables (set in .env file):"
    echo "  DB_PASSWORD           Database password (required)"
    echo "  REDIS_PASSWORD        Redis password (required)"
    echo "  GOOGLE_AI_API_KEY     Google AI API key (optional)"
    echo "  API_PORT             API service port (default: 8443)"
    echo "  WORKER_COUNT         Number of workers (default: 3)"
    echo ""
}

# Main execution
main() {
    # Parse command line arguments
    local skip_build=false
    local skip_migrations=false
    local skip_health_checks=false
    local scale_processors=""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            --skip-build)
                skip_build=true
                shift
                ;;
            --skip-migrations)
                skip_migrations=true
                shift
                ;;
            --skip-health-checks)
                skip_health_checks=true
                shift
                ;;
            --scale-processors)
                scale_processors="$2"
                shift 2
                ;;
            *)
                error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    print_header
    check_prerequisites
    setup_environment
    
    if [ "$skip_build" = false ]; then
        build_images
    fi
    
    start_infrastructure
    
    if [ "$skip_migrations" = false ]; then
        run_migrations
    fi
    
    start_application_services
    start_monitoring
    
    # Scale processors if requested
    if [ -n "$scale_processors" ]; then
        log "Scaling ad-processor to $scale_processors instances..."
        docker compose up -d --scale ad-processor="$scale_processors"
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