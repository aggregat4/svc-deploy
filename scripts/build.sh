#!/bin/bash
set -euo pipefail

# svc-deploy build script
# Usage: ./scripts/build.sh [command]
# Commands:
#   build       - Build the binary (default)
#   test        - Run tests
#   test-race   - Run tests with race detector
#   lint        - Run linters
#   fmt         - Format code
#   vet         - Run go vet
#   tidy        - Tidy and verify go.mod
#   coverage    - Generate coverage report
#   check       - Run all checks (fmt, vet, lint, test)
#   clean       - Clean build artifacts
#   all         - Run full CI pipeline

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BINARY_NAME="svc-deploy"
BUILD_DIR="${PROJECT_ROOT}/build"
COVERAGE_DIR="${PROJECT_ROOT}/coverage"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Ensure we're in the project root
cd "${PROJECT_ROOT}"

# Check Go version
check_go_version() {
    local required_version="1.26"
    local current_version
    current_version=$(go version | awk '{print $3}' | sed 's/go//')
    
    if [[ "${current_version%%.*}" -lt "${required_version%%.*}" ]]; then
        log_error "Go version ${current_version} is too old. Requires ${required_version}+"
        exit 1
    fi
    
    log_info "Using Go version: ${current_version}"
}

# Build the binary
cmd_build() {
    log_info "Building ${BINARY_NAME}..."
    
    mkdir -p "${BUILD_DIR}"
    
    local ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
    
    go build -ldflags "${ldflags}" -o "${BUILD_DIR}/${BINARY_NAME}" ./cmd/svc-deploy
    
    log_info "Build complete: ${BUILD_DIR}/${BINARY_NAME}"
}

# Run tests
cmd_test() {
    log_info "Running tests..."
    go test -v -count=1 ./...
}

# Run tests with race detector
cmd_test_race() {
    log_info "Running tests with race detector..."
    go test -race -count=1 ./...
}

# Run linters
cmd_lint() {
    log_info "Running linters..."
    
    local GOLANGCI_LINT="golangci-lint"
    
    # Check if golangci-lint is installed
    if ! command -v golangci-lint &> /dev/null; then
        # Check common install locations
        if [[ -x "${HOME}/go/bin/golangci-lint" ]]; then
            GOLANGCI_LINT="${HOME}/go/bin/golangci-lint"
        elif [[ -n "${GOPATH:-}" && -x "${GOPATH}/bin/golangci-lint" ]]; then
            GOLANGCI_LINT="${GOPATH}/bin/golangci-lint"
        elif [[ -x "$(go env GOPATH)/bin/golangci-lint" ]]; then
            GOLANGCI_LINT="$(go env GOPATH)/bin/golangci-lint"
        else
            log_warn "golangci-lint not found. Installing..."
            go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
            # Use the newly installed binary
            GOLANGCI_LINT="$(go env GOPATH)/bin/golangci-lint"
        fi
    fi
    
    log_info "Using golangci-lint: ${GOLANGCI_LINT}"
    "${GOLANGCI_LINT}" run --timeout=5m ./...
}

# Format code
cmd_fmt() {
    log_info "Formatting code..."
    
    # Check if gofumpt is available (stricter formatter)
    if command -v gofumpt &> /dev/null; then
        gofumpt -w .
    else
        gofmt -s -w .
    fi
    
    # Check for unformatted files
    local unformatted
    unformatted=$(gofmt -l .)
    if [[ -n "${unformatted}" ]]; then
        log_error "The following files need formatting:"
        echo "${unformatted}"
        exit 1
    fi
    
    log_info "Code formatting OK"
}

# Run go vet
cmd_vet() {
    log_info "Running go vet..."
    go vet ./...
}

# Tidy go.mod
cmd_tidy() {
    log_info "Tidying go.mod..."
    go mod tidy
    go mod verify
}

# Generate coverage report
cmd_coverage() {
    log_info "Generating coverage report..."
    
    mkdir -p "${COVERAGE_DIR}"
    
    # Run tests with coverage
    go test -race -coverprofile="${COVERAGE_DIR}/coverage.out" -covermode=atomic ./...
    
    # Generate HTML report
    go tool cover -html="${COVERAGE_DIR}/coverage.out" -o "${COVERAGE_DIR}/coverage.html"
    
    # Generate function coverage
    go tool cover -func="${COVERAGE_DIR}/coverage.out" | tee "${COVERAGE_DIR}/coverage.txt"
    
    # Calculate total coverage percentage
    local total_coverage
    total_coverage=$(go tool cover -func="${COVERAGE_DIR}/coverage.out" | grep total | awk '{print $3}')
    
    log_info "Total coverage: ${total_coverage}"
    log_info "Coverage report: ${COVERAGE_DIR}/coverage.html"
}

# Run all checks
cmd_check() {
    log_info "Running all checks..."
    cmd_fmt
    cmd_vet
    cmd_lint
    cmd_test
    log_info "All checks passed!"
}

# Clean build artifacts
cmd_clean() {
    log_info "Cleaning build artifacts..."
    rm -rf "${BUILD_DIR}"
    rm -rf "${COVERAGE_DIR}"
    go clean -cache -testcache
}

# Full CI pipeline
cmd_all() {
    log_info "Running full CI pipeline..."
    check_go_version
    cmd_tidy
    cmd_fmt
    cmd_vet
    cmd_lint
    cmd_test_race
    cmd_build
    cmd_coverage
    log_info "Full CI pipeline completed successfully!"
}

# Main
check_go_version

COMMAND="${1:-build}"

case "${COMMAND}" in
    build)
        cmd_build
        ;;
    test)
        cmd_test
        ;;
    test-race)
        cmd_test_race
        ;;
    lint)
        cmd_lint
        ;;
    fmt)
        cmd_fmt
        ;;
    vet)
        cmd_vet
        ;;
    tidy)
        cmd_tidy
        ;;
    coverage)
        cmd_coverage
        ;;
    check)
        cmd_check
        ;;
    clean)
        cmd_clean
        ;;
    all)
        cmd_all
        ;;
    *)
        echo "Usage: $0 [command]"
        echo "Commands:"
        echo "  build       - Build the binary (default)"
        echo "  test        - Run tests"
        echo "  test-race   - Run tests with race detector"
        echo "  lint        - Run linters"
        echo "  fmt         - Format code"
        echo "  vet         - Run go vet"
        echo "  tidy        - Tidy and verify go.mod"
        echo "  coverage    - Generate coverage report"
        echo "  check       - Run all checks (fmt, vet, lint, test)"
        echo "  clean       - Clean build artifacts"
        echo "  all         - Run full CI pipeline"
        exit 1
        ;;
esac
