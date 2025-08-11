#!/bin/bash

# 🏗️ Production-Ready Build Script
# Build all services and Docker images with proper versioning and optimization

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION=${1:-$(git rev-parse --short HEAD 2>/dev/null || echo "dev")}
REGISTRY=${REGISTRY:-"localhost"}
BUILD_MODE=${BUILD_MODE:-"production"}

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
    echo "               🏗️ AGENTIC AD PROCESSING QUEUE - BUILD SYSTEM"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Version: $VERSION${NC}"
    echo -e "${CYAN}Registry: $REGISTRY${NC}"
    echo -e "${CYAN}Build Mode: $BUILD_MODE${NC}"
    echo ""
}

# Check prerequisites
check_prerequisites() {
    log "Checking build prerequisites..."
    
    # Check required tools
    local required_tools=("go" "docker")
    local missing_tools=()
    
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        error "Missing required tools: ${missing_tools[*]}"
        exit 1
    fi
    
    # Check Docker is running
    if ! docker info &> /dev/null; then
        error "Docker is not running. Please start Docker Desktop."
        exit 1
    fi
    
    # Check Go version
    local go_version=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' || echo "0.0")
    local required_version="1.21"
    
    if [ "$(printf '%s\n' "$required_version" "$go_version" | sort -V | head -n1)" != "$required_version" ]; then
        error "Go version $go_version is too old. Required: >= $required_version"
        exit 1
    fi
    
    success "Prerequisites check passed"
}

# Clean build artifacts
clean_build() {
    log "Cleaning previous build artifacts..."
    
    cd "$PROJECT_ROOT"
    
    # Clean Go build cache
    go clean -cache -modcache || warn "Failed to clean Go cache"
    
    # Clean bin directory
    rm -rf bin/
    mkdir -p bin/
    
    # Clean test results
    rm -rf test-results/
    
    success "Build artifacts cleaned"
}

# Build Go binaries
build_go_services() {
    log "Building Go services..."
    
    cd "$PROJECT_ROOT"
    
    # Download dependencies
    log "Downloading Go dependencies..."
    go mod download
    go mod verify
    
    # Build services
    local services=("ad-api" "ad-processor" "migrate")
    
    for service in "${services[@]}"; do
        if [ -d "cmd/$service" ]; then
            log "Building $service binary..."
            
            # Set build flags based on mode
            local ldflags="-s -w"
            local tags=""
            
            if [ "$BUILD_MODE" = "production" ]; then
                ldflags="$ldflags -X main.Version=$VERSION -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
                tags="netgo"
            fi
            
            # Build binary
            CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
                -ldflags "$ldflags" \
                -tags "$tags" \
                -o "bin/$service" \
                "./cmd/$service"
            
            success "Built $service binary"
        else
            warn "Service directory cmd/$service not found, skipping"
        fi
    done
    
    # Verify binaries
    log "Verifying built binaries..."
    for binary in bin/*; do
        if [ -f "$binary" ]; then
            local size=$(du -h "$binary" | cut -f1)
            info "  $(basename "$binary"): $size"
        fi
    done
    
    success "Go services built successfully"
}

# Build Docker images
build_docker_images() {
    log "Building Docker images..."
    
    cd "$PROJECT_ROOT"
    
    # Define services and their Dockerfiles
    local services=(
        "migrate::Dockerfile.migrate"
        "ad-api::Dockerfile.ad-api"
        "ad-processor::Dockerfile.ad-processor"
    )
    
    for service_entry in "${services[@]}"; do
        local service_name="${service_entry%%::*}"
        local dockerfile="${service_entry##*::}"
        
        if [ -f "$dockerfile" ]; then
            log "Building Docker image for $service_name..."
            
            # Build image with proper tagging
            docker build \
                -f "$dockerfile" \
                -t "$REGISTRY/$service_name:$VERSION" \
                -t "$REGISTRY/$service_name:latest" \
                --build-arg VERSION="$VERSION" \
                --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                . --quiet
            
            success "Built $REGISTRY/$service_name:$VERSION"
        else
            warn "$dockerfile not found, skipping $service_name"
        fi
    done
    
    # Build AI agent if available
    if [ -d "ai-agent" ] && [ -f "ai-agent/Dockerfile" ]; then
        log "Building AI agent Docker image..."
        
        docker build \
            -f ai-agent/Dockerfile \
            -t "$REGISTRY/ai-agent:$VERSION" \
            -t "$REGISTRY/ai-agent:latest" \
            --build-arg VERSION="$VERSION" \
            ./ai-agent --quiet
        
        success "Built $REGISTRY/ai-agent:$VERSION"
    else
        warn "AI agent Dockerfile not found, skipping"
    fi
    
    success "Docker images built successfully"
}

# Run tests before build
run_pre_build_tests() {
    log "Running pre-build tests..."
    
    cd "$PROJECT_ROOT"
    
    # Run unit tests
    if go test -short ./internal/... ./pkg/... > /dev/null 2>&1; then
        success "Unit tests passed"
    else
        error "Unit tests failed"
        if [ "$BUILD_MODE" = "production" ]; then
            exit 1
        else
            warn "Continuing build despite test failures (development mode)"
        fi
    fi
    
    # Run go vet
    if go vet ./... > /dev/null 2>&1; then
        success "Go vet passed"
    else
        warn "Go vet found issues"
    fi
    
    # Run go fmt check
    local fmt_files=$(gofmt -l . | grep -v vendor || true)
    if [ -z "$fmt_files" ]; then
        success "Go formatting check passed"
    else
        warn "Files need formatting: $fmt_files"
        if [ "$BUILD_MODE" = "production" ]; then
            error "Please run 'go fmt ./...' to fix formatting"
            exit 1
        fi
    fi
}

# Verify Docker images
verify_docker_images() {
    log "Verifying Docker images..."
    
    # List built images
    info "Built images:"
    docker images | grep "$REGISTRY" | while read -r line; do
        info "  $line"
    done
    
    # Test image functionality
    local test_services=("ad-api" "ad-processor")
    
    for service in "${test_services[@]}"; do
        if docker images | grep -q "$REGISTRY/$service"; then
            log "Testing $service image..."
            
            # Test that the image starts without errors
            local container_id=$(docker run -d "$REGISTRY/$service:$VERSION" --help 2>/dev/null || echo "")
            
            if [ -n "$container_id" ]; then
                # Wait a moment and check if container exited cleanly
                sleep 2
                local exit_code=$(docker inspect "$container_id" --format='{{.State.ExitCode}}' 2>/dev/null || echo "1")
                docker rm "$container_id" >/dev/null 2>&1
                
                if [ "$exit_code" = "0" ]; then
                    success "✓ $service image verification passed"
                else
                    warn "⚠ $service image verification unclear (exit code: $exit_code)"
                fi
            else
                warn "⚠ Could not test $service image"
            fi
        fi
    done
}

# Generate build report
generate_build_report() {
    log "Generating build report..."
    
    local report_file="$PROJECT_ROOT/build-report.md"
    
    cat > "$report_file" << EOF
# Build Report - Agentic Ad Processing Queue

**Generated:** $(date -u +"%Y-%m-%d %H:%M:%S UTC")
**Version:** $VERSION  
**Registry:** $REGISTRY
**Build Mode:** $BUILD_MODE

## Built Artifacts

### Go Binaries
EOF
    
    # List binaries
    if [ -d "$PROJECT_ROOT/bin" ]; then
        find "$PROJECT_ROOT/bin" -type f -executable | while read -r binary; do
            local name=$(basename "$binary")
            local size=$(du -h "$binary" | cut -f1)
            echo "- **$name**: $size" >> "$report_file"
        done
    fi
    
    cat >> "$report_file" << EOF

### Docker Images
EOF
    
    # List Docker images
    docker images --format "table {{.Repository}}:{{.Tag}}\t{{.Size}}" | grep "$REGISTRY" | while read -r line; do
        echo "- $line" >> "$report_file"
    done
    
    cat >> "$report_file" << EOF

## Usage Instructions

### Running Services Locally
\`\`\`bash
# Development mode
./scripts/dev-start.sh

# Docker Compose mode  
./scripts/docker-start.sh

# Kubernetes mode
./scripts/k8s-deploy.sh
\`\`\`

### Testing
\`\`\`bash
# Run comprehensive tests
./scripts/comprehensive-test.sh

# Check system status
./scripts/status.sh
\`\`\`

### Registry Operations
\`\`\`bash
# Push to registry
docker push $REGISTRY/ad-api:$VERSION
docker push $REGISTRY/ad-processor:$VERSION
docker push $REGISTRY/ai-agent:$VERSION

# Pull from registry
docker pull $REGISTRY/ad-api:$VERSION
\`\`\`

---
*Build completed at $(date)*
EOF
    
    success "Build report generated: $report_file"
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS] [VERSION]"
    echo ""
    echo "Build all services and Docker images for the Agentic Ad Processing Queue."
    echo ""
    echo "Arguments:"
    echo "  VERSION            Build version (default: git short hash or 'dev')"
    echo ""
    echo "Options:"
    echo "  -h, --help         Show this help message"
    echo "  --clean            Clean build artifacts before building"
    echo "  --no-tests         Skip pre-build tests"
    echo "  --no-docker        Skip Docker image building"
    echo "  --registry REGISTRY Docker registry (default: localhost)"
    echo "  --mode MODE        Build mode: development|production (default: production)"
    echo "  --push             Push images to registry after building"
    echo ""
    echo "Environment Variables:"
    echo "  REGISTRY           Docker registry prefix"
    echo "  BUILD_MODE         Build mode (development|production)"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Build with auto-detected version"
    echo "  $0 v1.0.0                             # Build specific version"
    echo "  $0 --clean --mode development         # Clean development build"
    echo "  $0 --registry my-registry.com --push  # Build and push to registry"
    echo ""
}

# Main execution
main() {
    local clean_build_flag=false
    local skip_tests=false
    local skip_docker=false
    local push_images=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            --clean)
                clean_build_flag=true
                shift
                ;;
            --no-tests)
                skip_tests=true
                shift
                ;;
            --no-docker)
                skip_docker=true
                shift
                ;;
            --registry)
                REGISTRY="$2"
                shift 2
                ;;
            --mode)
                BUILD_MODE="$2"
                shift 2
                ;;
            --push)
                push_images=true
                shift
                ;;
            v*)
                VERSION="$1"
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
    check_prerequisites
    
    if [ "$clean_build_flag" = true ]; then
        clean_build
    fi
    
    if [ "$skip_tests" = false ]; then
        run_pre_build_tests
    fi
    
    build_go_services
    
    if [ "$skip_docker" = false ]; then
        build_docker_images
        verify_docker_images
    fi
    
    if [ "$push_images" = true ]; then
        log "Pushing images to registry..."
        local services=("ad-api" "ad-processor" "ai-agent" "migrate")
        
        for service in "${services[@]}"; do
            if docker images | grep -q "$REGISTRY/$service"; then
                log "Pushing $REGISTRY/$service:$VERSION..."
                docker push "$REGISTRY/$service:$VERSION"
                docker push "$REGISTRY/$service:latest"
                success "Pushed $service"
            fi
        done
    fi
    
    generate_build_report
    
    echo ""
    echo -e "${PURPLE}============================================================================="
    echo "                        🎉 BUILD COMPLETED SUCCESSFULLY"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${GREEN}All services built and packaged!${NC}"
    echo ""
    echo -e "${CYAN}📦 Built Artifacts:${NC}"
    echo "  - Go binaries in bin/ directory"
    echo "  - Docker images tagged with version $VERSION"
    echo "  - Build report: build-report.md"
    echo ""
    echo -e "${CYAN}🚀 Next Steps:${NC}"
    echo "  - Test locally: ./scripts/dev-start.sh"
    echo "  - Deploy to Docker: ./scripts/docker-start.sh"
    echo "  - Deploy to K8s: ./scripts/k8s-deploy.sh"
    echo "  - Run tests: ./scripts/comprehensive-test.sh"
    echo ""
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi