# Cargo Framework Testing Plan

A comprehensive testing strategy covering every feature in the Cargo framework with unit tests, integration tests, and performance benchmarks.

## Test Structure Overview

```
cargo/
├── test/
│   ├── unit/                    # Unit tests for individual components
│   │   ├── auth_test.go
│   │   ├── cache_test.go
│   │   ├── circuitbreaker_test.go
│   │   ├── config_test.go
│   │   ├── database_test.go
│   │   ├── errors_test.go
│   │   ├── health_test.go
│   │   ├── hooks_test.go
│   │   ├── logging_test.go
│   │   ├── metrics_test.go
│   │   ├── middleware_test.go
│   │   ├── migrations_test.go
│   │   ├── mongodb_test.go
│   │   ├── monitoring_test.go
│   │   ├── ratelimit_test.go
│   │   ├── streaming_test.go
│   │   ├── tls_test.go
│   │   └── validation_test.go
│   ├── integration/             # Integration tests
│   │   ├── app_test.go
│   │   ├── grpc_test.go
│   │   ├── mongodb_integration_test.go
│   │   └── end_to_end_test.go
│   ├── benchmarks/              # Performance benchmarks
│   │   ├── cache_bench_test.go
│   │   ├── database_bench_test.go
│   │   ├── streaming_bench_test.go
│   │   └── ratelimit_bench_test.go
│   ├── mocks/                   # Test mocks and utilities
│   │   ├── mongo_mock.go
│   │   ├── grpc_mock.go
│   │   └── cache_mock.go
│   └── fixtures/                # Test data and fixtures
│       ├── test_data.go
│       ├── proto/
│       └── certs/
├── testutils/                   # Shared testing utilities
│   ├── helpers.go
│   ├── assertions.go
│   └── test_server.go
├── Makefile                     # Test commands
└── test_coverage.sh             # Coverage analysis script
```

## Testing Categories

### 1. Core Framework Tests (`cargo_test.go`)

**Test Coverage:**
- App initialization and configuration
- Server startup and shutdown
- Middleware registration and execution
- Service registration
- Context handling
- Error propagation

**Key Test Cases:**
```go
TestNew()                        // App creation
TestWithConfig()                 // Configuration setting
TestUse()                        // Middleware registration
TestGetServer()                  // gRPC server access
TestRun()                        // Server startup
TestGracefulShutdown()           // Clean shutdown
TestContextCreation()            // Request context handling
```

### 2. Authentication Tests (`auth_test.go`)

**Test Coverage:**
- JWT token generation and validation
- Authentication middleware
- Authorization claims handling
- Token expiration and refresh
- Invalid token handling

**Key Test Cases:**
```go
TestGenerateJWT()                // Token generation
TestValidateJWT()                // Token validation
TestJWTExpiration()              // Token expiry
TestInvalidToken()               // Invalid token handling
TestAuthMiddleware()             // Middleware functionality
TestAuthClaims()                 // Claims extraction
TestAuthWithSkip()               // Skip authentication
```

### 3. Type-Safe Hooks Tests (`hooks_test.go`)

**Test Coverage:**
- Hook registration and execution
- Type safety validation
- Hook registry management
- Service handler operations
- Lifecycle hook execution

**Key Test Cases:**
```go
TestHookRegistry()               // Hook registration
TestServiceHandler()             // Handler creation
TestBeforeHooks()               // Before lifecycle hooks
TestAfterHooks()                // After lifecycle hooks
TestTypeSafety()                // Generic type checking
TestHookExecution()             // Hook execution order
TestHookErrors()                // Error handling in hooks
```

### 4. Configuration Tests (`config_test.go`)

**Test Coverage:**
- Default configuration loading
- Environment variable parsing
- Configuration validation
- Type conversion
- Error handling for invalid configs

**Key Test Cases:**
```go
TestDefaultConfig()             // Default configuration
TestConfigFromEnv()             // Environment loading
TestConfigValidation()          // Validation logic
TestInvalidConfig()             // Invalid configuration
TestGetEnv()                    // Environment helpers
TestConfigMerging()             // Configuration merging
```

### 5. Request Validation Tests (`validation_test.go`)

**Test Coverage:**
- Validation rule implementation
- Struct tag parsing
- Custom validators
- Error message formatting
- Performance validation

**Key Test Cases:**
```go
TestRequiredRule()              // Required field validation
TestEmailRule()                 // Email format validation
TestMinLengthRule()             // Minimum length validation
TestMaxLengthRule()             // Maximum length validation
TestRangeRule()                 // Numeric range validation
TestRegexRule()                 // Regular expression validation
TestOneOfRule()                 // Enum validation
TestStructValidation()          // Full struct validation
TestCustomValidator()           // Custom validation logic
```

### 6. Error Handling Tests (`errors_test.go`)

**Test Coverage:**
- Error creation and formatting
- gRPC status code mapping
- Error context and metadata
- Error wrapping and unwrapping
- Error serialization

**Key Test Cases:**
```go
TestCargoError()                // Error creation
TestErrorWithContext()          // Context addition
TestGRPCStatus()                // gRPC conversion
TestErrorWrapping()             // Error wrapping
TestErrorTypes()                // Different error types
TestErrorSerialization()        // JSON serialization
```

### 7. Database Tests (`database_test.go`)

**Test Coverage:**
- MongoDB connection management
- CRUD operations
- Collection name mapping
- Connection pooling
- Error handling

**Key Test Cases:**
```go
TestMongoConnection()           // Connection establishment
TestGetDatabase()               // Database access
TestCollectionNaming()          // Collection name logic
TestConnectionPooling()         // Pool management
TestDatabaseErrors()            // Error scenarios
```

### 8. Advanced MongoDB Tests (`mongodb_test.go`)

**Test Coverage:**
- Bulk operations
- Aggregation pipelines
- Transaction management
- Index management
- Health checking

**Key Test Cases:**
```go
TestBulkWriter()                // Bulk operations
TestAggregationBuilder()        // Aggregation pipelines
TestTransactionManager()        // Transaction handling
TestIndexManager()              // Index operations
TestMongoHealthCheck()          // Health monitoring
TestEnhancedQuery()             // Query builder
```

### 9. Database Migrations Tests (`migrations_test.go`)

**Test Coverage:**
- Migration execution
- Rollback functionality
- Migration status tracking
- Data transformations
- Version management

**Key Test Cases:**
```go
TestMigrationManager()          // Migration management
TestAddMigration()              // Adding migrations
TestRunMigrations()             // Execution logic
TestRollback()                  // Rollback functionality
TestMigrationStatus()           // Status tracking
TestDataMigration()             // Data transformations
```

### 10. Structured Logging Tests (`logging_test.go`)

**Test Coverage:**
- Logger configuration
- Log level filtering
- Structured formatting
- Context propagation
- Performance logging

**Key Test Cases:**
```go
TestNewLogger()                 // Logger creation
TestLogLevels()                 // Level filtering
TestStructuredFormat()          // JSON formatting
TestWithContext()               // Context logging
TestGlobalLogger()              // Global logger functions
TestLoggerPerformance()         // Performance impact
```

### 11. Metrics Collection Tests (`metrics_test.go`)

**Test Coverage:**
- Metric registration
- Counter operations
- Gauge operations
- Histogram operations
- Metric aggregation

**Key Test Cases:**
```go
TestMetricRegistry()            // Registry management
TestCounter()                   // Counter metrics
TestGauge()                     // Gauge metrics
TestHistogram()                 // Histogram metrics
TestMetricsCollector()          // Collection logic
TestMetricLabels()              // Label handling
TestMetricExport()              // Export functionality
```

### 12. Health Monitoring Tests (`health_test.go`)

**Test Coverage:**
- Health check registration
- Status aggregation
- Component monitoring
- Dependency checking
- Health reporting

**Key Test Cases:**
```go
TestHealthManager()             // Manager functionality
TestHealthChecks()              // Individual checks
TestHealthStatus()              // Status reporting
TestOverallHealth()             // Aggregated health
TestHealthTimeout()             // Timeout handling
TestHealthDependencies()        // Dependency checking
```

### 13. Caching Tests (`cache_test.go`)

**Test Coverage:**
- Cache implementation
- TTL handling
- Cache strategies
- Memory management
- Cache statistics

**Key Test Cases:**
```go
TestMemoryCache()               // In-memory caching
TestCacheManager()              // Manager functionality
TestCacheTTL()                  // TTL expiration
TestCacheStrategies()           // Different strategies
TestCacheMiddleware()           // Middleware integration
TestCacheStats()                // Statistics collection
TestCacheEviction()             // Memory management
```

### 14. Enhanced Streaming Tests (`streaming_test.go`)

**Test Coverage:**
- Stream management
- Backpressure handling
- Stream statistics
- Error recovery
- Performance monitoring

**Key Test Cases:**
```go
TestStreamManager()             // Manager functionality
TestEnhancedStream()            // Enhanced streaming
TestStreamStats()               // Statistics collection
TestBackpressure()              // Backpressure handling
TestStreamInterceptor()         // Interceptor logic
TestStreamErrors()              // Error handling
TestStreamPerformance()         // Performance metrics
```

### 15. Rate Limiting Tests (`ratelimit_test.go`)

**Test Coverage:**
- Rate limiter algorithms
- Key generation strategies
- Middleware integration
- Performance impact
- Configuration handling

**Key Test Cases:**
```go
TestTokenBucket()               // Token bucket algorithm
TestSlidingWindow()             // Sliding window algorithm
TestFixedWindow()               // Fixed window algorithm
TestRateLimitKeys()             // Key generation
TestRateLimitMiddleware()       // Middleware integration
TestRateLimitConfig()           // Configuration handling
```

### 16. Circuit Breaker Tests (`circuitbreaker_test.go`)

**Test Coverage:**
- State transitions
- Failure detection
- Recovery mechanisms
- Configuration validation
- Performance monitoring

**Key Test Cases:**
```go
TestCircuitBreaker()            // Basic functionality
TestStateTransitions()          // State changes
TestFailureDetection()          // Failure tracking
TestRecovery()                  // Recovery logic
TestCircuitBreakerManager()     // Manager functionality
TestCircuitBreakerMiddleware()  // Middleware integration
```

### 17. Request Monitoring Tests (`monitoring_test.go`)

**Test Coverage:**
- Request tracing
- Response monitoring
- Performance metrics
- Error tracking
- Trace correlation

**Key Test Cases:**
```go
TestRequestMonitoring()         // Request tracking
TestTraceGeneration()           // Trace ID generation
TestMonitoringMiddleware()      // Middleware integration
TestPerformanceMetrics()        // Performance tracking
TestErrorTracking()             // Error monitoring
TestTraceCorrelation()          // Request correlation
```

### 18. Middleware Tests (`middleware_test.go`)

**Test Coverage:**
- Middleware chaining
- Execution order
- Error handling
- Context propagation
- Performance impact

**Key Test Cases:**
```go
TestMiddlewareChaining()        // Execution order
TestLoggerMiddleware()          // Logging middleware
TestRecoveryMiddleware()        // Panic recovery
TestAuthMiddleware()            // Authentication
TestMiddlewareContext()         // Context handling
TestMiddlewareErrors()          // Error propagation
```

### 19. TLS Support Tests (`tls_test.go`)

**Test Coverage:**
- Certificate loading
- TLS configuration
- Mutual TLS setup
- Security validation
- Connection handling

**Key Test Cases:**
```go
TestTLSConfiguration()          // TLS setup
TestCertificateLoading()        // Certificate handling
TestMutualTLS()                 // mTLS configuration
TestTLSSecurity()               // Security validation
TestTLSConnections()            // Connection handling
```

### 20. BSON Integration Tests (`bson_test.go`)

**Test Coverage:**
- BSON type detection
- Type extraction
- Collection naming
- Serialization optimization
- Integration validation

**Key Test Cases:**
```go
TestBSONTypeDetection()         // Type checking
TestBSONExtraction()            // Type extraction
TestBSONCollectionNaming()      // Collection naming
TestBSONSerialization()         // Optimization
TestBSONIntegration()           // Framework integration
```

## Integration Tests

### Full Application Tests (`integration/app_test.go`)

**Test Scenarios:**
- Complete application lifecycle
- Multi-service registration
- End-to-end request flow
- Database integration
- Authentication flow

### gRPC Integration Tests (`integration/grpc_test.go`)

**Test Scenarios:**
- Service registration and discovery
- Request/response handling
- Streaming operations
- Error propagation
- Middleware execution

### MongoDB Integration Tests (`integration/mongodb_integration_test.go`)

**Test Scenarios:**
- Real database operations
- Transaction handling
- Migration execution
- Performance validation
- Connection management

## Performance Benchmarks

### Cache Performance (`benchmarks/cache_bench_test.go`)

```go
BenchmarkCacheSet()             // Set operations
BenchmarkCacheGet()             // Get operations
BenchmarkCacheEviction()        // Eviction performance
BenchmarkCacheConcurrency()     // Concurrent access
```

### Database Performance (`benchmarks/database_bench_test.go`)

```go
BenchmarkCRUDOperations()       // Basic CRUD
BenchmarkBulkOperations()       // Bulk operations
BenchmarkAggregation()          // Aggregation performance
BenchmarkTransactions()         // Transaction overhead
```

### Streaming Performance (`benchmarks/streaming_bench_test.go`)

```go
BenchmarkStreamThroughput()     // Message throughput
BenchmarkStreamLatency()        // Message latency
BenchmarkBackpressure()         // Backpressure handling
```

### Rate Limiting Performance (`benchmarks/ratelimit_bench_test.go`)

```go
BenchmarkTokenBucket()          // Token bucket performance
BenchmarkSlidingWindow()        // Sliding window performance
BenchmarkConcurrentRateLimit()  // Concurrent access
```

## Test Utilities and Mocks

### Shared Test Utilities (`testutils/helpers.go`)

```go
// Test server setup
func NewTestServer() *cargo.App
func StartTestServer(app *cargo.App) (string, func())
func CreateTestContext() *cargo.Context

// Test data generators
func GenerateTestUser() *proto.User
func GenerateTestTodo() *proto.Todo
func GenerateTestClaims() map[string]interface{}

// Assertions
func AssertNoError(t *testing.T, err error)
func AssertError(t *testing.T, err error, expectedType string)
func AssertEqual(t *testing.T, expected, actual interface{})
```

### MongoDB Mock (`mocks/mongo_mock.go`)

```go
type MockMongoClient struct {
    // Mock implementation for testing
}

func NewMockMongoClient() *MockMongoClient
func (m *MockMongoClient) SetupCollections()
func (m *MockMongoClient) SeedTestData()
func (m *MockMongoClient) Cleanup()
```

### Cache Mock (`mocks/cache_mock.go`)

```go
type MockCache struct {
    data map[string]interface{}
    ttl  map[string]time.Time
}

func NewMockCache() *MockCache
func (c *MockCache) Get(key string) (interface{}, bool)
func (c *MockCache) Set(key string, value interface{}, ttl time.Duration)
```

## Test Execution Strategy

### Makefile Commands

```makefile
# Run all tests
test: test-unit test-integration test-benchmarks

# Unit tests
test-unit:
	go test -v ./test/unit/...

# Integration tests (requires MongoDB)
test-integration:
	go test -v ./test/integration/...

# Benchmark tests
test-benchmarks:
	go test -bench=. ./test/benchmarks/...

# Test coverage
test-coverage:
	./test_coverage.sh

# Test with race detection
test-race:
	go test -race -v ./...

# Clean test cache
test-clean:
	go clean -testcache

# Test specific feature
test-auth:
	go test -v ./test/unit/auth_test.go

test-cache:
	go test -v ./test/unit/cache_test.go

test-hooks:
	go test -v ./test/unit/hooks_test.go
```

### Coverage Analysis Script (`test_coverage.sh`)

```bash
#!/bin/bash

# Generate coverage report
go test -coverprofile=coverage.out ./...

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Show coverage summary
go tool cover -func=coverage.out

# Check minimum coverage threshold (90%)
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
if (( $(echo "$COVERAGE < 90" | bc -l) )); then
    echo "Coverage $COVERAGE% is below minimum threshold of 90%"
    exit 1
fi

echo "Coverage: $COVERAGE%"
```

### Test Configuration

#### Environment Variables for Testing

```bash
# Test environment
CARGO_ENV=test
CARGO_LOG_LEVEL=error

# Test MongoDB (use testcontainers or local instance)
MONGODB_URI=mongodb://localhost:27017/cargo_test
MONGODB_TIMEOUT=5s

# Test authentication
JWT_SECRET=test-secret-key
JWT_EXPIRY_DURATION=1h

# Test caching
CACHE_BACKEND=memory
CACHE_MAX_SIZE=1000

# Disable features for testing
METRICS_ENABLED=false
HEALTH_CHECK_ENABLED=false
```

## Testing Standards and Requirements

### Code Coverage Requirements

- **Minimum Coverage**: 90% overall
- **Critical Components**: 95% coverage (auth, database, hooks)
- **Performance Components**: 85% coverage (cache, streaming, rate limiting)
- **Utility Components**: 80% coverage (logging, metrics, monitoring)

### Test Quality Standards

1. **Test Isolation**: Each test should be independent
2. **Deterministic**: Tests should produce consistent results
3. **Fast Execution**: Unit tests should complete in < 100ms
4. **Clear Naming**: Test names should describe the scenario
5. **Comprehensive**: Cover happy path, edge cases, and error conditions

### CI/CD Integration

```yaml
# .github/workflows/test.yml
name: Test Suite
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      mongodb:
        image: mongo:7
        ports:
          - 27017:27017
    
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.23'
      
      - name: Run tests
        run: make test
      
      - name: Check coverage
        run: make test-coverage
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3
```

### Performance Testing

#### Load Testing Scenarios

1. **High Throughput**: 10,000 requests/second
2. **Concurrent Users**: 1,000 simultaneous connections
3. **Memory Usage**: Monitor memory consumption under load
4. **Cache Performance**: Test cache hit/miss ratios
5. **Database Performance**: Test query performance with large datasets

#### Performance Benchmarks

```go
// Example benchmark requirements
func BenchmarkServiceHandler(b *testing.B) {
    // Should handle > 50,000 ops/sec
    // Should use < 100MB memory
    // Should complete in < 1ms per operation
}
```

## Test Data Management

### Test Fixtures (`test/fixtures/test_data.go`)

```go
var (
    TestUsers = []proto.User{
        {Id: "user1", Name: "John Doe", Email: "john@example.com"},
        {Id: "user2", Name: "Jane Smith", Email: "jane@example.com"},
    }
    
    TestTodos = []proto.Todo{
        {Id: "todo1", Title: "Test Todo 1", UserId: "user1"},
        {Id: "todo2", Title: "Test Todo 2", UserId: "user2"},
    }
)

func SeedTestData(db *mongo.Database) error {
    // Seed test data into database
}

func CleanupTestData(db *mongo.Database) error {
    // Clean up test data
}
```

### Test Certificates (`test/fixtures/certs/`)

```
certs/
├── ca.crt          # Test CA certificate
├── server.crt      # Test server certificate
├── server.key      # Test server private key
├── client.crt      # Test client certificate
└── client.key      # Test client private key
```

This comprehensive testing plan ensures every feature in the Cargo framework is thoroughly tested with proper coverage, performance validation, and quality standards. 