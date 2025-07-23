# Cargo Framework Test Suite
.PHONY: test test-unit test-integration test-benchmarks test-coverage test-race test-clean help

# Default target
help:
	@echo "Cargo Framework Test Commands:"
	@echo "  test              - Run all tests (unit + integration)"
	@echo "  test-unit         - Run unit tests only"
	@echo "  test-integration  - Run integration tests (requires MongoDB)"
	@echo "  test-benchmarks   - Run performance benchmarks"
	@echo "  test-coverage     - Generate test coverage report"
	@echo "  test-race         - Run tests with race detection"
	@echo "  test-clean        - Clean test cache"
	@echo "  test-verbose      - Run tests with verbose output"
	@echo ""
	@echo "Individual Feature Tests:"
	@echo "  test-auth         - Test authentication features"
	@echo "  test-cache        - Test caching features"
	@echo "  test-hooks        - Test type-safe hooks"
	@echo "  test-config       - Test configuration management"
	@echo "  test-database     - Test database operations"
	@echo "  test-monitoring   - Test observability features"

# Run all tests
test: test-unit test-integration

# Unit tests (no external dependencies)
test-unit:
	@echo "Running unit tests..."
	go test -v ./test/unit/...

# Integration tests (requires MongoDB)
test-integration:
	@echo "Running integration tests..."
	@echo "Ensure MongoDB is running on localhost:27017"
	go test -v ./test/integration/...

# Performance benchmarks
test-benchmarks:
	@echo "Running performance benchmarks..."
	go test -bench=. -benchmem ./test/benchmarks/...

# Generate test coverage report
test-coverage:
	@echo "Generating test coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out
	@echo "Coverage report generated: coverage.html"

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	go test -race -v ./...

# Run tests with verbose output
test-verbose:
	@echo "Running all tests with verbose output..."
	go test -v -cover ./...

# Clean test cache
test-clean:
	@echo "Cleaning test cache..."
	go clean -testcache

# Individual feature tests
test-auth:
	@echo "Testing authentication features..."
	go test -v ./test/unit/auth_test.go ./testutils/

test-cache:
	@echo "Testing caching features..."
	go test -v ./test/unit/cache_test.go ./testutils/

test-hooks:
	@echo "Testing type-safe hooks..."
	go test -v ./test/unit/hooks_test.go ./testutils/

test-config:
	@echo "Testing configuration management..."
	go test -v ./test/unit/config_test.go ./testutils/

test-database:
	@echo "Testing database operations..."
	go test -v ./test/unit/database_test.go ./test/unit/mongodb_test.go ./testutils/

test-monitoring:
	@echo "Testing observability features..."
	go test -v ./test/unit/logging_test.go ./test/unit/metrics_test.go ./test/unit/health_test.go ./testutils/

# Setup test environment
test-setup:
	@echo "Setting up test environment..."
	mkdir -p test/unit test/integration test/benchmarks test/mocks test/fixtures testutils
	@echo "Test directories created"

# Quick smoke test
test-smoke:
	@echo "Running smoke tests..."
	go test -v -run "TestNew\|TestConfig\|TestAuth" ./...

# Test with coverage threshold (90%)
test-ci:
	@echo "Running CI test suite..."
	go test -coverprofile=coverage.out ./...
	@bash -c 'COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk "{print \$$3}" | sed "s/%//"); if (( $$(echo "$$COVERAGE < 90" | bc -l) )); then echo "Coverage $$COVERAGE% is below minimum threshold of 90%"; exit 1; fi; echo "Coverage: $$COVERAGE%"'

# Install test dependencies
test-deps:
	@echo "Installing test dependencies..."
	go mod download
	go install github.com/golang/mock/mockgen@latest
	@echo "Test dependencies installed" 