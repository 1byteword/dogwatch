.PHONY: all generate build clean test test-unit test-integration test-all test-coverage test-verbose lint build-ui-v2

CLANG ?= clang
GO ?= $(shell which go || echo /usr/local/go/bin/go)
COVERAGE_DIR ?= coverage
COVERAGE_FILE ?= $(COVERAGE_DIR)/coverage.out

all: generate build-ui-v2 build

# Generate Go bindings from BPF C code
generate:
	cd internal/probe && $(GO) generate ./...

# Build V2 UI and copy into embed directory
build-ui-v2:
	cd ui-v2 && npm ci && npx vite build
	rm -rf internal/web/v2dist/assets internal/web/v2dist/index.html
	cp -r ui-v2/dist/* internal/web/v2dist/

# Build the dogwatch binary
build:
	CGO_ENABLED=0 $(GO) build -o dogwatch ./cmd/dogwatch

# Clean build artifacts
clean:
	rm -f dogwatch
	rm -f internal/probe/bpf_*.go
	rm -f internal/probe/bpf_*.o
	rm -rf $(COVERAGE_DIR)

# Install dependencies
deps:
	$(GO) get github.com/cilium/ebpf@latest
	$(GO) get github.com/cilium/ebpf/cmd/bpf2go@latest
	$(GO) mod tidy

# Run dogwatch (requires root)
run: build
	sudo ./dogwatch

# ============================================================================
# Testing Targets
# ============================================================================

# Run all unit tests (default test target)
test: test-unit

# Run unit tests only (no integration tests, no eBPF tests requiring root)
test-unit:
	@echo "Running unit tests..."
	$(GO) test -v -race -count=1 \
		./internal/storage/... \
		./internal/trace/... \
		./internal/alerting/... \
		./internal/rbac/... \
		./internal/probe/... \
		./internal/testutil/...

# Run unit tests with short flag (skip slow tests)
test-short:
	@echo "Running short tests..."
	$(GO) test -v -short -race \
		./internal/storage/... \
		./internal/trace/... \
		./internal/alerting/... \
		./internal/rbac/... \
		./internal/probe/...

# Run integration tests (requires running dogwatch instance)
test-integration:
	@echo "Running integration tests..."
	@echo "Make sure dogwatch is running on localhost:9999"
	$(GO) test -v -count=1 ./tests/integration/...

# Run all tests (unit + integration)
test-all: test-unit test-integration

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -coverprofile=$(COVERAGE_FILE) -covermode=atomic \
		./internal/storage/... \
		./internal/trace/... \
		./internal/alerting/... \
		./internal/rbac/... \
		./internal/probe/... \
		./internal/testutil/...
	$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report generated: $(COVERAGE_DIR)/coverage.html"
	@$(GO) tool cover -func=$(COVERAGE_FILE) | tail -n 1

# Run tests with verbose output and no caching
test-verbose:
	@echo "Running tests (verbose, no cache)..."
	$(GO) test -v -count=1 -race \
		./internal/storage/... \
		./internal/trace/... \
		./internal/alerting/... \
		./internal/rbac/... \
		./internal/probe/... \
		./internal/testutil/...

# Run benchmarks
test-bench:
	@echo "Running benchmarks..."
	$(GO) test -bench=. -benchmem -run=^$$ \
		./internal/storage/... \
		./internal/trace/... \
		./internal/alerting/... \
		./internal/rbac/... \
		./internal/probe/...

# Run specific package tests
test-storage:
	$(GO) test -v -race ./internal/storage/...

test-trace:
	$(GO) test -v -race ./internal/trace/...

test-alerting:
	$(GO) test -v -race ./internal/alerting/...

test-rbac:
	$(GO) test -v -race ./internal/rbac/...

test-probe:
	$(GO) test -v -race ./internal/probe/...

# Check test coverage percentage (fails if below threshold)
test-coverage-check: test-coverage
	@echo "Checking coverage threshold..."
	@coverage=$$($(GO) tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	threshold=50; \
	if [ $$(echo "$$coverage < $$threshold" | bc -l) -eq 1 ]; then \
		echo "Coverage $$coverage% is below threshold $$threshold%"; \
		exit 1; \
	else \
		echo "Coverage $$coverage% meets threshold $$threshold%"; \
	fi

# ============================================================================
# Code Quality
# ============================================================================

# Run linter (requires golangci-lint)
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

# Format code
fmt:
	$(GO) fmt ./...

# Run go vet
vet:
	$(GO) vet ./...

# Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
