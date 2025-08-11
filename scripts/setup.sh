#!/bin/bash

# 🚀 Agentic Ad Processing Queue - Setup Script
# Complete environment setup and validation

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REQUIRED_TOOLS=("go" "docker" "kubectl" "kind")
OPTIONAL_TOOLS=("jq" "curl" "redis-cli" "psql")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log() { echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
info() { echo -e "${CYAN}[INFO]${NC} $1"; }

# Print header
print_header() {
    echo -e "${PURPLE}"
    echo "============================================================================="
    echo "               🚀 AGENTIC AD PROCESSING QUEUE SETUP"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Project: Intelligent Ad Processing Queue with AI Agent${NC}"
    echo -e "${CYAN}Author:  Solution Architecture Team${NC}"
    echo -e "${CYAN}Version: Production-Ready v1.0${NC}"
    echo ""
}

# Check tool installation
check_tool() {
    local tool="$1"
    local required="$2"
    
    if command -v "$tool" &> /dev/null; then
        local version=$(${tool} --version 2>/dev/null | head -1 || echo "unknown")
        success "$tool is installed ($version)"
        return 0
    else
        if [ "$required" = "true" ]; then
            error "$tool is required but not installed"
            return 1
        else
            warn "$tool is not installed (optional)"
            return 0
        fi
    fi
}

# Check all prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    local missing=0
    
    # Check required tools
    for tool in "${REQUIRED_TOOLS[@]}"; do
        if ! check_tool "$tool" "true"; then
            ((missing++))
        fi
    done
    
    # Check optional tools
    info "Checking optional tools..."
    for tool in "${OPTIONAL_TOOLS[@]}"; do
        check_tool "$tool" "false"
    done
    
    if [ $missing -gt 0 ]; then
        error "$missing required tools are missing"
        echo ""
        echo "Installation instructions:"
        echo "  Go:      https://golang.org/doc/install"
        echo "  Docker:  https://docs.docker.com/get-docker/"
        echo "  kubectl: https://kubernetes.io/docs/tasks/tools/"
        echo "  Kind:    https://kind.sigs.k8s.io/docs/user/quick-start/"
        exit 1
    fi
    
    success "All required tools are installed"
}

# Check Go version
check_go_version() {
    log "Validating Go version..."
    local go_version=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' || echo "0.0")
    local required_version="1.21"
    
    if [ "$(printf '%s\n' "$required_version" "$go_version" | sort -V | head -n1)" = "$required_version" ]; then
        success "Go version $go_version meets requirement (>= $required_version)"
    else
        error "Go version $go_version is too old. Required: >= $required_version"
        exit 1
    fi
}

# Validate project structure
validate_project_structure() {
    log "Validating project structure..."
    
    local required_files=(
        "go.mod"
        "go.sum"
        "cmd/ad-api/main.go"
        "cmd/ad-processor/main.go"
        "internal/domain/ad/ad.go"
        "docker-compose.yml"
        "k8s/ad-api.yaml"
        "migrations/001_initial_schema.sql"
    )
    
    cd "$PROJECT_ROOT"
    
    for file in "${required_files[@]}"; do
        if [ -f "$file" ]; then
            info "✓ $file"
        else
            error "✗ Missing required file: $file"
            exit 1
        fi
    done
    
    success "Project structure is valid"
}

# Setup Go workspace
setup_go_workspace() {
    log "Setting up Go workspace..."
    
    cd "$PROJECT_ROOT"
    
    # Download dependencies
    log "Downloading Go dependencies..."
    go mod tidy
    go mod download
    
    # Verify dependencies
    go mod verify
    
    success "Go workspace is ready"
}

# Setup environment files
setup_environment() {
    log "Setting up environment configuration..."
    
    cd "$PROJECT_ROOT"
    
    # Create .env file if it doesn't exist
    if [ ! -f ".env" ]; then
        cat > .env << 'EOF'
# Database Configuration
DB_PASSWORD=secure_postgres_password_2024
DATABASE_URL=postgresql://postgres:secure_postgres_password_2024@localhost:5433/adprocessing?sslmode=disable

# Redis Configuration  
REDIS_PASSWORD=secure_redis_password_2024

# AI Integration (Optional - will use mock if not set)
GOOGLE_AI_API_KEY=

# Application Environment
APP_ENV=development
APP_LOG_LEVEL=info
APP_PORT=8080

# Queue Configuration
WORKER_COUNT=3
MAX_WAIT_TIME_SECONDS=300
ANTI_STARVATION_ENABLED=true

# Monitoring
PROMETHEUS_PORT=9090
GRAFANA_PORT=3000
EOF
        success "Created .env file with default configuration"
        warn "Please update .env file with your specific configuration"
    else
        info ".env file already exists"
    fi
    
    # Make sure all scripts are executable
    log "Making scripts executable..."
    find scripts -name "*.sh" -exec chmod +x {} \;
    
    success "Environment setup completed"
}

# Setup Docker environment
setup_docker() {
    log "Setting up Docker environment..."
    
    # Check if Docker is running
    if ! docker info &> /dev/null; then
        error "Docker is not running. Please start Docker Desktop."
        exit 1
    fi
    
    success "Docker is running"
    
    # Create necessary Docker networks
    log "Creating Docker networks..."
    docker network create ad-processing-network 2>/dev/null || info "Network ad-processing-network already exists"
    
    success "Docker environment is ready"
}

# Setup Kind cluster
setup_kind_cluster() {
    log "Setting up Kind Kubernetes cluster..."
    
    # Check if cluster already exists
    if kind get clusters | grep -q "ad-processing"; then
        info "Kind cluster 'ad-processing' already exists"
        return 0
    fi
    
    # Create Kind cluster with custom configuration
    log "Creating Kind cluster..."
    kind create cluster --name ad-processing --config k8s/kind-config.yaml 2>/dev/null || {
        # Fallback to default Kind cluster
        kind create cluster --name ad-processing
    }
    
    # Set kubectl context
    kubectl config use-context kind-ad-processing
    
    success "Kind cluster created and configured"
}

# Build Docker images
build_images() {
    log "Building Docker images..."
    
    cd "$PROJECT_ROOT"
    
    # Build all services
    local services=("migrate" "ad-api" "ad-processor")
    
    for service in "${services[@]}"; do
        log "Building $service image..."
        if [ -f "Dockerfile.$service" ]; then
            docker build -f "Dockerfile.$service" -t "$service:latest" . --quiet
            success "Built $service:latest"
        else
            warn "Dockerfile.$service not found, skipping"
        fi
    done
    
    # Build AI agent
    if [ -d "ai-agent" ]; then
        log "Building AI agent image..."
        docker build -f ai-agent/Dockerfile -t ai-agent:latest ./ai-agent --quiet
        success "Built ai-agent:latest"
    fi
}

# Run basic tests
run_basic_tests() {
    log "Running basic validation tests..."
    
    cd "$PROJECT_ROOT"
    
    # Test Go compilation
    log "Testing Go compilation..."
    go build -o /tmp/test-ad-api ./cmd/ad-api
    go build -o /tmp/test-ad-processor ./cmd/ad-processor
    rm -f /tmp/test-ad-api /tmp/test-ad-processor
    
    # Run unit tests
    log "Running unit tests..."
    go test -short ./internal/... -count=1
    
    success "Basic tests passed"
}

# Generate summary
generate_summary() {
    echo ""
    echo -e "${PURPLE}============================================================================="
    echo "                        🎉 SETUP COMPLETED SUCCESSFULLY"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${GREEN}Your Agentic Ad Processing Queue is ready for development and deployment!${NC}"
    echo ""
    echo -e "${CYAN}📋 Next Steps:${NC}"
    echo "  1. Review and update .env file with your configuration"
    echo "  2. Choose your deployment method:"
    echo ""
    echo -e "${YELLOW}🚀 Quick Development Start:${NC}"
    echo "     ./scripts/dev-start.sh"
    echo ""
    echo -e "${YELLOW}🏗️ Production Kubernetes Deployment:${NC}"
    echo "     ./scripts/k8s-deploy.sh"
    echo ""
    echo -e "${YELLOW}🐳 Docker Compose Deployment:${NC}"
    echo "     ./scripts/docker-start.sh"
    echo ""
    echo -e "${CYAN}📚 Documentation:${NC}"
    echo "  - README.md - Complete project documentation"
    echo "  - k8s/README.md - Kubernetes deployment guide"
    echo "  - docs/ - Additional documentation"
    echo ""
    echo -e "${CYAN}🧪 Testing:${NC}"
    echo "  - ./scripts/test.sh - Run all tests"
    echo "  - ./scripts/load-test.sh - Performance testing"
    echo "  - ./scripts/acceptance-test.sh - Acceptance testing"
    echo ""
    echo -e "${CYAN}📊 Monitoring:${NC}"
    echo "  - Prometheus: http://localhost:9090"
    echo "  - Grafana: http://localhost:3000 (admin/admin123)"
    echo ""
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -h, --help           Show this help message"
    echo "  --skip-docker        Skip Docker setup"
    echo "  --skip-kind          Skip Kind cluster creation"
    echo "  --skip-build         Skip Docker image builds"
    echo "  --skip-tests         Skip basic tests"
    echo ""
    echo "This script sets up the complete development environment for"
    echo "the Agentic Ad Processing Queue system."
}

# Main execution
main() {
    # Parse command line arguments
    local skip_docker=false
    local skip_kind=false
    local skip_build=false
    local skip_tests=false
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            --skip-docker)
                skip_docker=true
                shift
                ;;
            --skip-kind)
                skip_kind=true
                shift
                ;;
            --skip-build)
                skip_build=true
                shift
                ;;
            --skip-tests)
                skip_tests=true
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
    
    # Run setup steps
    check_prerequisites
    check_go_version
    validate_project_structure
    setup_go_workspace
    setup_environment
    
    if [ "$skip_docker" = false ]; then
        setup_docker
    fi
    
    if [ "$skip_kind" = false ]; then
        setup_kind_cluster
    fi
    
    if [ "$skip_build" = false ]; then
        build_images
    fi
    
    if [ "$skip_tests" = false ]; then
        run_basic_tests
    fi
    
    generate_summary
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi