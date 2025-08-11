#!/bin/bash

# 📊 System Status and Health Check Script
# Monitor all services and system health

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
    echo "               📊 AGENTIC AD PROCESSING QUEUE - SYSTEM STATUS"
    echo "============================================================================="
    echo -e "${NC}"
    echo -e "${CYAN}Timestamp: $(date)${NC}"
    echo ""
}

# Check service health via HTTP
check_http_service() {
    local name="$1"
    local url="$2"
    local expected_status="${3:-200}"
    local timeout="${4:-5}"
    
    local status_code=$(curl -s -w "%{http_code}" --max-time "$timeout" "$url" -o /dev/null 2>/dev/null || echo "000")
    
    if [[ "$status_code" == "$expected_status" ]]; then
        success "✓ $name (HTTP $status_code)"
        return 0
    else
        error "✗ $name (HTTP $status_code)"
        return 1
    fi
}

# Check TCP service
check_tcp_service() {
    local name="$1"
    local host="$2"
    local port="$3"
    local timeout="${4:-5}"
    
    if timeout "$timeout" bash -c "</dev/tcp/$host/$port" 2>/dev/null; then
        success "✓ $name ($host:$port)"
        return 0
    else
        error "✗ $name ($host:$port)"
        return 1
    fi
}

# Check development environment
check_development_status() {
    echo -e "${PURPLE}=== DEVELOPMENT ENVIRONMENT ===${NC}"
    
    local services_healthy=0
    local services_total=0
    
    # Check if PID files exist
    cd "$PROJECT_ROOT"
    
    if [ -d ".pids" ]; then
        local pids=($(find .pids -name "*.pid" 2>/dev/null))
        if [ ${#pids[@]} -gt 0 ]; then
            info "Checking development services..."
            
            for pid_file in "${pids[@]}"; do
                local service_name=$(basename "$pid_file" .pid)
                ((services_total++))
                
                if [ -f "$pid_file" ]; then
                    local pid=$(cat "$pid_file")
                    if kill -0 "$pid" 2>/dev/null; then
                        success "✓ $service_name (PID: $pid)"
                        ((services_healthy++))
                    else
                        error "✗ $service_name (stale PID: $pid)"
                    fi
                else
                    error "✗ $service_name (no PID file)"
                fi
            done
        else
            warn "No development services running (no PID files found)"
        fi
    else
        warn "Development services not started (.pids directory not found)"
        info "Start with: ./scripts/dev-start.sh"
    fi
    
    # Check API endpoints if services are running
    if [ $services_healthy -gt 0 ]; then
        echo ""
        info "Checking API endpoints..."
        
        local api_urls=(
            "Ad API::http://localhost:8080/health"
            "AI Agent::http://localhost:8000/health"
        )
        
        for url_entry in "${api_urls[@]}"; do
            local name="${url_entry%%::*}"
            local url="${url_entry##*::}"
            check_http_service "$name" "$url" "200" 3 || true
        done
    fi
    
    echo ""
    info "Development Status: $services_healthy/$services_total services healthy"
}

# Check Docker environment
check_docker_status() {
    echo -e "${PURPLE}=== DOCKER ENVIRONMENT ===${NC}"
    
    cd "$PROJECT_ROOT"
    
    # Check if Docker is running
    if ! docker info &>/dev/null; then
        error "Docker is not running"
        return 1
    fi
    
    # Check Docker Compose services
    if [ -f "docker-compose.yml" ]; then
        local compose_services=$(docker-compose ps --services 2>/dev/null || echo "")
        
        if [ -n "$compose_services" ]; then
            info "Checking Docker Compose services..."
            
            local healthy=0
            local total=0
            
            while IFS= read -r service; do
                ((total++))
                local status=$(docker-compose ps -q "$service" 2>/dev/null | xargs -I {} docker inspect -f '{{.State.Status}}' {} 2>/dev/null || echo "not_found")
                
                if [[ "$status" == "running" ]]; then
                    success "✓ $service"
                    ((healthy++))
                else
                    error "✗ $service ($status)"
                fi
            done <<< "$compose_services"
            
            echo ""
            
            # Check service endpoints
            if [ $healthy -gt 0 ]; then
                info "Checking service endpoints..."
                
                local endpoints=(
                    "Ad API::http://localhost:8443/health"
                    "AI Agent::http://localhost:8080/health"
                    "Prometheus::http://localhost:9090/-/ready"
                    "Grafana::http://localhost:3000/api/health"
                )
                
                for endpoint in "${endpoints[@]}"; do
                    local name="${endpoint%%::*}"
                    local url="${endpoint##*::}"
                    check_http_service "$name" "$url" "200" 5 || true
                done
                
                # Check database services
                echo ""
                info "Checking database services..."
                check_tcp_service "PostgreSQL" "localhost" "5433" 3 || true
                check_tcp_service "Redis" "localhost" "6380" 3 || true
            fi
            
            echo ""
            info "Docker Status: $healthy/$total services healthy"
        else
            warn "No Docker Compose services defined or docker-compose.yml not found"
        fi
    else
        warn "docker-compose.yml not found"
    fi
}

# Check Kubernetes environment
check_kubernetes_status() {
    echo -e "${PURPLE}=== KUBERNETES ENVIRONMENT ===${NC}"
    
    # Check if kubectl is available
    if ! command -v kubectl &>/dev/null; then
        warn "kubectl not found"
        return 1
    fi
    
    # Check cluster connection
    if ! kubectl cluster-info &>/dev/null; then
        warn "Not connected to a Kubernetes cluster"
        return 1
    fi
    
    local context=$(kubectl config current-context)
    info "Connected to cluster: $context"
    
    # Check namespace
    local namespace="ad-processing"
    if ! kubectl get namespace "$namespace" &>/dev/null; then
        warn "Namespace '$namespace' not found"
        info "Deploy with: ./scripts/k8s-deploy.sh"
        return 1
    fi
    
    # Check deployments
    info "Checking Kubernetes deployments..."
    
    local deployments=$(kubectl get deployments -n "$namespace" -o name 2>/dev/null | sed 's|deployment.apps/||')
    local healthy=0
    local total=0
    
    if [ -n "$deployments" ]; then
        while IFS= read -r deployment; do
            if [ -n "$deployment" ]; then
                ((total++))
                
                # Get deployment status using a more reliable method
                local status_line=$(kubectl get deployment "$deployment" -n "$namespace" --no-headers 2>/dev/null)
                if [ -n "$status_line" ]; then
                    # Parse the READY column (format: ready/desired)
                    local ready_status=$(echo "$status_line" | awk '{print $2}')
                    local ready=$(echo "$ready_status" | cut -d'/' -f1)
                    local desired=$(echo "$ready_status" | cut -d'/' -f2)
                    
                    if [ "$ready" = "$desired" ] && [ "$ready" != "0" ]; then
                        success "✓ $deployment ($ready/$desired ready)"
                        ((healthy++))
                    else
                        error "✗ $deployment ($ready/$desired ready)"
                    fi
                else
                    error "✗ $deployment (not found)"
                fi
            fi
        done <<< "$deployments"
        
        echo ""
        
        # Check service endpoints through NodePort/LoadBalancer
        if [ $healthy -gt 0 ]; then
            info "Checking service endpoints..."
            
            # Try to access services via NodePort
            local endpoints=(
                "Ad API::http://localhost:30443/health"
                "AI Agent::http://localhost:30080/health"
                "Prometheus::http://localhost:30090/-/ready"
                "Grafana::http://localhost:30000/api/health"
            )
            
            for endpoint in "${endpoints[@]}"; do
                local name="${endpoint%%::*}"
                local url="${endpoint##*::}"
                check_http_service "$name" "$url" "200" 5 || true
            done
            
            echo ""
            
            # Check pods status
            info "Pod status:"
            kubectl get pods -n "$namespace" --no-headers | while read -r line; do
                local pod_name=$(echo "$line" | awk '{print $1}')
                local status=$(echo "$line" | awk '{print $3}')
                local ready=$(echo "$line" | awk '{print $2}')
                
                if [[ "$status" == "Running" ]]; then
                    success "✓ $pod_name ($ready, $status)"
                else
                    warn "⚠ $pod_name ($ready, $status)"
                fi
            done
        fi
        
        echo ""
        info "Kubernetes Status: $healthy/$total deployments healthy"
    else
        warn "No deployments found in namespace $namespace"
    fi
}

# Check system resources
check_system_resources() {
    echo -e "${PURPLE}=== SYSTEM RESOURCES ===${NC}"
    
    # Check disk space
    local disk_usage=$(df -h / | tail -1 | awk '{print $5}' | sed 's/%//')
    if [ "$disk_usage" -lt 80 ]; then
        success "✓ Disk space: ${disk_usage}% used"
    elif [ "$disk_usage" -lt 90 ]; then
        warn "⚠ Disk space: ${disk_usage}% used"
    else
        error "✗ Disk space: ${disk_usage}% used (critical)"
    fi
    
    # Check memory usage
    if command -v free &>/dev/null; then
        local mem_usage=$(free | grep Mem | awk '{printf "%.0f", $3/$2 * 100.0}')
        if [ "$mem_usage" -lt 80 ]; then
            success "✓ Memory usage: ${mem_usage}%"
        elif [ "$mem_usage" -lt 90 ]; then
            warn "⚠ Memory usage: ${mem_usage}%"
        else
            error "✗ Memory usage: ${mem_usage}% (critical)"
        fi
    fi
    
    # Check Docker resources if Docker is running
    if docker info &>/dev/null; then
        echo ""
        info "Docker resources:"
        local containers_running=$(docker ps -q | wc -l | tr -d ' ')
        local images_count=$(docker images -q | wc -l | tr -d ' ')
        info "  Running containers: $containers_running"
        info "  Total images: $images_count"
        
        # Check Docker disk usage
        local docker_space=$(docker system df --format "table {{.Type}}\t{{.TotalCount}}\t{{.Size}}" 2>/dev/null | tail -n +2 | awk '{size+=$3} END {print size}')
        if [ -n "$docker_space" ]; then
            info "  Disk usage: ${docker_space}B"
        fi
    fi
}

# Get API statistics
get_api_statistics() {
    echo -e "${PURPLE}=== API STATISTICS ===${NC}"
    
    # Try different API endpoints
    local api_urls=("http://localhost:8080" "http://localhost:8443" "http://localhost:30443")
    local working_api=""
    
    for url in "${api_urls[@]}"; do
        if curl -s --max-time 3 "$url/health" &>/dev/null; then
            working_api="$url"
            break
        fi
    done
    
    if [ -n "$working_api" ]; then
        success "API accessible at: $working_api"
        
        # Get queue statistics
        local queue_stats=$(curl -s --max-time 5 "$working_api/api/v1/ads/queue/stats" 2>/dev/null)
        if [ -n "$queue_stats" ] && echo "$queue_stats" | grep -q "total\|distribution"; then
            info "Queue statistics:"
            if command -v jq &>/dev/null; then
                echo "$queue_stats" | jq -r '
                    "  Total ads in queue: " + (.total // "0" | tostring),
                    "  Priority distribution:",
                    (.distribution // {} | to_entries[] | "    Priority " + .key + ": " + (.value | tostring))
                ' 2>/dev/null || echo "  $queue_stats"
            else
                echo "  $queue_stats"
            fi
        else
            warn "Unable to retrieve queue statistics"
        fi
        
        # Test basic API functionality
        echo ""
        info "Testing API functionality:"
        
        # Test health endpoint
        if curl -s --max-time 5 "$working_api/health" | grep -q "healthy\|service"; then
            success "✓ Health endpoint working"
        else
            warn "⚠ Health endpoint not responding properly"
        fi
        
        # Test readiness endpoint
        if curl -s --max-time 5 "$working_api/ready" &>/dev/null; then
            success "✓ Readiness endpoint working"
        else
            warn "⚠ Readiness endpoint not available"
        fi
        
    else
        warn "API not accessible on standard ports"
        info "Checked ports: 8080, 8443, 30443"
    fi
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Check system status and health of Agentic Ad Processing Queue services."
    echo ""
    echo "Options:"
    echo "  -h, --help         Show this help message"
    echo "  --dev              Check development environment only"
    echo "  --docker           Check Docker environment only"  
    echo "  --k8s              Check Kubernetes environment only"
    echo "  --resources        Check system resources only"
    echo "  --api              Check API statistics only"
    echo "  --watch INTERVAL   Watch mode - refresh every N seconds"
    echo ""
    echo "Examples:"
    echo "  $0                 # Check all environments"
    echo "  $0 --dev           # Check development only"
    echo "  $0 --watch 30      # Monitor with 30-second refresh"
    echo ""
}

# Watch mode
watch_status() {
    local interval="$1"
    
    while true; do
        clear
        print_header
        main_check
        echo ""
        echo -e "${CYAN}Refreshing in ${interval}s... (Press Ctrl+C to exit)${NC}"
        sleep "$interval"
    done
}

# Main check function
main_check() {
    check_development_status
    echo ""
    check_docker_status
    echo ""
    check_kubernetes_status
    echo ""
    check_system_resources
    echo ""
    get_api_statistics
}

# Main execution
main() {
    local check_dev=true
    local check_docker=true
    local check_k8s=true
    local check_resources=true
    local check_api=true
    local watch_interval=""
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            --dev)
                check_dev=true
                check_docker=false
                check_k8s=false
                check_resources=false
                check_api=false
                shift
                ;;
            --docker)
                check_dev=false
                check_docker=true
                check_k8s=false
                check_resources=false
                check_api=false
                shift
                ;;
            --k8s)
                check_dev=false
                check_docker=false
                check_k8s=true
                check_resources=false
                check_api=false
                shift
                ;;
            --resources)
                check_dev=false
                check_docker=false
                check_k8s=false
                check_resources=true
                check_api=false
                shift
                ;;
            --api)
                check_dev=false
                check_docker=false
                check_k8s=false
                check_resources=false
                check_api=true
                shift
                ;;
            --watch)
                watch_interval="$2"
                if [[ ! "$watch_interval" =~ ^[0-9]+$ ]] || [ "$watch_interval" -lt 1 ]; then
                    error "Invalid watch interval: $watch_interval"
                    exit 1
                fi
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
    
    # Handle watch mode
    if [ -n "$watch_interval" ]; then
        watch_status "$watch_interval"
        return
    fi
    
    # Run requested checks
    if [ "$check_dev" = true ]; then
        check_development_status
        echo ""
    fi
    
    if [ "$check_docker" = true ]; then
        check_docker_status
        echo ""
    fi
    
    if [ "$check_k8s" = true ]; then
        check_kubernetes_status
        echo ""
    fi
    
    if [ "$check_resources" = true ]; then
        check_system_resources
        echo ""
    fi
    
    if [ "$check_api" = true ]; then
        get_api_statistics
        echo ""
    fi
    
    echo -e "${PURPLE}============================================================================="
    echo "                           📊 STATUS CHECK COMPLETE"
    echo "=============================================================================${NC}"
    echo ""
    echo -e "${CYAN}💡 Tips:${NC}"
    echo "  - Use --watch 30 for continuous monitoring"
    echo "  - Check logs with: ./scripts/logs.sh"
    echo "  - Run tests with: ./scripts/comprehensive-test.sh"
    echo ""
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi