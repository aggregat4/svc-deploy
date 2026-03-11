GO ?= go
.DEFAULT_GOAL := build

BINARY_NAME := svc-deploy
CMD_PACKAGE := ./cmd/svc-deploy
BUILD_DIR := build
OUTPUT := $(BUILD_DIR)/$(BINARY_NAME)
TOOLS_DIR := .tools/bin

GOLANGCI_LINT_VERSION := v1.60.3
GOFUMPT_VERSION := v0.7.0

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
GOFUMPT := $(TOOLS_DIR)/gofumpt

.PHONY: help build test test-race vet lint fmt fmt-check coverage clean tools

help:
	@printf '%s\n' \
		'Targets:' \
		'  make build      Build ./build/$(BINARY_NAME)' \
		'  make test       Run go test ./...' \
		'  make test-race  Run go test -race ./...' \
		'  make vet        Run go vet ./...' \
		'  make lint       Run golangci-lint with pinned version' \
		'  make fmt        Format code with pinned gofumpt' \
		'  make fmt-check  Fail if formatting drift exists' \
		'  make coverage   Write coverage artifacts under ./coverage' \
		'  make clean      Remove build and tool artifacts'

build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(OUTPUT) $(CMD_PACKAGE)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --timeout=5m ./...

fmt: $(GOFUMPT)
	$(GOFUMPT) -w .

fmt-check: $(GOFUMPT)
	@files="$$( $(GOFUMPT) -l . )"; \
	if [ -n "$$files" ]; then \
		printf 'Formatting drift detected:\n%s\n' "$$files"; \
		exit 1; \
	fi

coverage:
	@mkdir -p coverage
	$(GO) test -coverprofile=coverage/coverage.out ./...
	$(GO) tool cover -func=coverage/coverage.out > coverage/coverage.txt
	$(GO) tool cover -html=coverage/coverage.out -o coverage/coverage.html

clean:
	rm -rf $(BUILD_DIR) coverage .tools
	$(GO) clean -cache -testcache

tools: $(GOLANGCI_LINT) $(GOFUMPT)

$(TOOLS_DIR):
	mkdir -p $(TOOLS_DIR)

$(GOLANGCI_LINT): | $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOFUMPT): | $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) $(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
