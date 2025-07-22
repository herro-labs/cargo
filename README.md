# Cargo

A production-ready gRPC framework for Go with comprehensive MongoDB integration, authentication, observability, and performance features. Built for scalable microservices with enterprise-grade capabilities.

## Features

- **Type-Safe gRPC Services** with automatic registration and lifecycle hooks
- **MongoDB Integration** with CRUD operations, transactions, aggregations, and migrations
- **Authentication & Authorization** with JWT and configurable middleware
- **Observability** with structured logging, metrics collection, and health monitoring
- **Performance** with intelligent caching, rate limiting, and enhanced streaming
- **Resilience** with circuit breakers, request tracing, and error handling
- **Security** with TLS/SSL support, mutual TLS, and request validation
- **protoc-gen-go-bson** plugin support for optimized MongoDB serialization

## Installation

```bash
go get github.com/herro-labs/cargo@v1.0.0
go install github.com/herro-labs/protoc-gen-go-bson@latest
```

## Getting Started

### 1. Define Your Service

```protobuf
// proto/todo.proto
syntax = "proto3";
package app;
option go_package = "./proto";

service TodoService {
  rpc Create (Todo) returns (Todo);
  rpc Get (Todo) returns (Todo);
  rpc List (Todo) returns (stream Todo);
  rpc Delete (Todo) returns (Todo);
}

message Todo {
  string id = 1;
  string title = 2;
  string description = 3;
  string user_id = 4;
}
```

### 2. Generate Code

```bash
protoc --proto_path=proto \
  --go_out=proto --go-grpc_out=proto \
  --go-bson_out=proto \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  --go-bson_opt=paths=source_relative \
  proto/*.proto
```

### 3. Implement Your Application

```go
package main

import (
    "context"
    "github.com/herro-labs/cargo"
    "your-project/proto"
)

func main() {
    app := cargo.New()

    // Add middleware
    app.Use(
        cargo.Logger(),
        cargo.Recovery(),
        cargo.Auth(),
    )

    // Configure typed service handlers
    todoHandler := cargo.NewServiceHandler[proto.Todo](app, "TodoService", &TodoHooks{})

    // Register gRPC services
    server := app.GetServer()
    proto.RegisterTodoServiceServer(server, &TodoServiceImpl{handler: todoHandler})

    app.Run(":50051")
}

type TodoHooks struct{}

func (h *TodoHooks) BeforeCreate(ctx *cargo.Context, todo *proto.Todo) error {
    claims := ctx.Auth()
    todo.UserId = claims.ID
    return nil
}

func (h *TodoHooks) AfterRead(ctx *cargo.Context, todo *proto.Todo) error {
    cargo.Info("Todo retrieved", "id", todo.Id, "title", todo.Title)
    return nil
}

type TodoServiceImpl struct {
    proto.UnimplementedTodoServiceServer
    handler *cargo.ServiceHandler[proto.Todo]
}

func (s *TodoServiceImpl) Create(ctx context.Context, req *proto.Todo) (*proto.Todo, error) {
    return s.handler.Create(ctx, req)
}

func (s *TodoServiceImpl) List(req *proto.Todo, stream grpc.ServerStreamingServer[proto.Todo]) error {
    todos, err := s.handler.List(stream.Context(), req)
    if err != nil {
        return err
    }
    
    for _, todo := range todos {
        if err := stream.Send(&todo); err != nil {
            return err
        }
    }
    return nil
}
```

## Core Features

### Type-Safe Service Handlers

Cargo provides type-safe service handlers that eliminate reflection overhead and provide compile-time type checking.

```go
// Create a typed handler for your message type
handler := cargo.NewServiceHandler[proto.Todo](app, "TodoService", hooks)

// Add validation
handler = handler.WithValidator(myValidator)

// The handler provides type-safe CRUD operations
todo, err := handler.Create(ctx, &proto.Todo{Title: "Learn Cargo"})
todos, err := handler.List(ctx, &proto.Todo{})
```

### Configuration Management

Comprehensive configuration with environment variable support.

```go
config := cargo.DefaultConfig()
config.Server.Port = ":8080"
config.Database.MongoDB.URI = "mongodb://localhost:27017"
config.Auth.JWT.Secret = "your-secret-key"

app := cargo.New().WithConfig(config)
```

Environment variables:
```bash
CARGO_PORT=:8080
MONGODB_URI=mongodb://localhost:27017
JWT_SECRET=your-secret-key
LOG_LEVEL=info
CACHE_BACKEND=memory
```

### Request Validation

Built-in validation system with custom rules.

```go
type TodoValidator struct{}

func (v *TodoValidator) Validate(value interface{}, fieldName string) error {
    todo := value.(*proto.Todo)
    if len(todo.Title) < 3 {
        return cargo.NewValidationError("title must be at least 3 characters")
    }
    return nil
}

handler := cargo.NewServiceHandler[proto.Todo](app, "TodoService", hooks).
    WithValidator(&TodoValidator{})
```

### Error Handling

Structured error handling with gRPC status codes.

```go
// Create structured errors
err := cargo.NewCargoError("User not found", cargo.StatusNotFound, nil)

// Add context
err = err.WithContext(map[string]interface{}{
    "user_id": userID,
    "operation": "get_user",
})

return err
```

## Database Features

### MongoDB Operations

Advanced MongoDB operations with automatic collection management.

```go
// Basic CRUD (handled automatically by service handlers)
todo, err := handler.Create(ctx, todo)

// Advanced operations
bulkWriter := app.NewBulkWriter("todos")
bulkWriter.Insert(todo1, todo2, todo3)
result, err := bulkWriter.Execute(ctx)

// Aggregation pipelines
aggregation := app.NewAggregationBuilder("todos").
    Match(bson.M{"user_id": userID}).
    Group(bson.M{"_id": "$status", "count": bson.M{"$sum": 1}})
results, err := aggregation.Execute(ctx)

// Transactions
txManager := app.NewTransactionManager()
err = txManager.Execute(ctx, func(txCtx context.Context) error {
    // Multiple operations in transaction
    return nil
})
```

### Database Migrations

Version-controlled database migrations.

```go
migrationManager := app.NewMigrationManager()

// Add collection creation migration
migrationManager.Add(cargo.CreateCollectionMigration(
    "001_create_users",
    "Create users collection with indexes",
    "users",
    []mongo.IndexModel{
        {Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
    },
))

// Run migrations
err := migrationManager.RunMigrations(ctx)
```

### Enhanced Querying

Flexible query building for complex MongoDB operations.

```go
query := app.NewEnhancedQuery("todos").
    Filter(bson.M{"user_id": userID}).
    Sort(bson.D{{Key: "created_at", Value: -1}}).
    Limit(10).
    Skip(20)

// Execute with different patterns
todos, err := query.Find(ctx)
todo, err := query.FindOne(ctx)
count, err := query.Count(ctx)
```

## Observability Features

### Structured Logging

JSON-formatted logging with contextual information.

```go
// Configure logging
logger := cargo.NewLogger(cargo.LoggingConfig{
    Level:  "info",
    Format: "json",
    Output: "stdout",
})
cargo.SetGlobalLogger(logger)

// Use contextual logging
cargo.Info("User created", "user_id", userID, "email", user.Email)
cargo.Error("Database error", "error", err.Error(), "operation", "create_user")

// Add request context
logger.WithContext(ctx).Info("Processing request", "method", methodName)
```

### Metrics Collection

Comprehensive metrics for monitoring application performance.

```go
metrics := cargo.NewMetricsCollector()
cargo.SetGlobalMetrics(metrics)

// Built-in metrics
metrics.IncrementCounter("requests_total", map[string]string{"method": "create"})
metrics.SetGauge("active_connections", nil, 42)
metrics.RecordDuration("request_duration_ms", map[string]string{"endpoint": "/api/users"}, duration)

// Custom metrics
metrics.RecordValue("custom_metric", map[string]string{"type": "business"}, value)
```

### Health Monitoring

Multi-component health checking system.

```go
healthManager := app.NewHealthManager()

// Add built-in health checks
healthManager.AddCheck(cargo.NewMongoHealthCheck(app.Mongo()))
healthManager.AddCheck(cargo.NewServiceHealthCheck("api", "localhost:8080", 5*time.Second))

// Add custom health checks
healthManager.AddCheck(cargo.NewHealthCheckFunc("custom", func(ctx context.Context) cargo.HealthCheckResult {
    return cargo.HealthCheckResult{
        Status:  cargo.HealthStatusHealthy,
        Message: "All systems operational",
    }
}))

// Get health status
report := healthManager.GetOverallHealth(ctx)
```

## Performance Features

### Intelligent Caching

Multi-strategy caching system with automatic invalidation.

```go
// Configure caching middleware
app.UseCaching(cargo.CacheStrategyReadOnly,
    cargo.CacheRule{
        Pattern: "TodoService.*",
        TTL:     5 * time.Minute,
        Enabled: true,
    },
)

// Direct cache usage
cache := app.GetDefaultCache()
cache.Set(ctx, "user:123", userData, time.Hour)
data, found := cache.Get(ctx, "user:123")

// Cache management
manager := app.GetCacheManager()
manager.AddCache("sessions", sessionCache)
stats := cache.Stats()
```

### Enhanced Streaming

High-performance streaming with monitoring and backpressure.

```go
// Enhanced server streaming
func (s *TodoServiceImpl) List(req *proto.Todo, stream grpc.ServerStreamingServer[proto.Todo]) error {
    streamID := cargo.GenerateStreamID()
    streamManager := cargo.GetGlobalStreamManager()
    
    enhancedStream := cargo.NewEnhancedServerStream(stream, streamID, streamManager, streamConfig)
    defer enhancedStream.Close()
    
    for _, todo := range todos {
        if err := enhancedStream.Send(&todo); err != nil {
            return err
        }
    }
    return nil
}

// Stream statistics
stats := app.GetAllStreamStats()
```

### Rate Limiting

Configurable rate limiting with multiple algorithms.

```go
// Add rate limiting middleware
app.Use(cargo.RateLimitByIP(100, time.Minute))          // 100 requests per minute per IP
app.Use(cargo.RateLimitByUser(1000, time.Hour))         // 1000 requests per hour per user
app.Use(cargo.RateLimitByService("TodoService", 500, time.Minute))  // 500 requests per minute for service

// Custom rate limiter
rateLimiter := cargo.NewTokenBucket(100, time.Minute)
app.Use(cargo.RateLimitMiddleware(rateLimiter))
```

## Resilience Features

### Circuit Breakers

Fault tolerance with configurable circuit breakers.

```go
// Configure circuit breaker middleware
app.Use(cargo.CircuitBreakerMiddleware(cargo.DefaultCircuitBreakerConfig()))

// Manual circuit breaker usage
cb := cargo.NewCircuitBreaker(cargo.CircuitBreakerConfig{
    Name:           "database",
    MaxRequests:    100,
    Interval:       time.Minute,
    Timeout:        30 * time.Second,
    FailureRatio:   0.6,
})

err := cb.Call(func() error {
    // Protected operation
    return riskyDatabaseOperation()
})
```

### Request Monitoring

Comprehensive request/response monitoring and tracing.

```go
// Add monitoring middleware
monitor := cargo.GetGlobalMonitor()
app.Use(cargo.MonitoringMiddleware(monitor))

// Get monitoring data
traces := monitor.GetTraces()
stats := monitor.GetStats()

// Custom events
monitor.RecordEvent("custom_operation", map[string]interface{}{
    "user_id": userID,
    "duration": duration,
})
```

## Security Features

### TLS/SSL Support

Secure connections with certificate management.

```go
// Basic TLS
app.WithTLS("server.crt", "server.key")

// Mutual TLS (client certificate verification)
app.WithMutualTLS("server.crt", "server.key", "ca.crt")

app.Run(":443")
```

### JWT Authentication

Flexible JWT authentication with configurable claims.

```go
// Configure JWT
config.Auth.JWT.Secret = "your-secret-key"
config.Auth.JWT.ExpiryDuration = 24 * time.Hour

// Implement login hook
func BeforeLogin(ctx *cargo.Context, req *proto.LoginRequest) (*cargo.AuthIdentity, error) {
    user := authenticateUser(req.Email, req.Password)
    if user == nil {
        return nil, cargo.NewCargoError("Invalid credentials", cargo.StatusUnauthorized, nil)
    }
    
    return &cargo.AuthIdentity{
        ID: user.Id,
        Claims: map[string]interface{}{
            "email": user.Email,
            "role":  user.Role,
        },
    }, nil
}

// Access authentication in hooks
func BeforeCreate(ctx *cargo.Context, todo *proto.Todo) error {
    claims := ctx.Auth()
    todo.UserId = claims.ID
    return nil
}
```

## protoc-gen-go-bson Integration

Optimized MongoDB serialization with BSON types.

```go
// BSON types provide optimized MongoDB operations
func beforeCreateUser(ctx *cargo.Context, user *proto.UserBSON) error {
    // Automatic BSON serialization/deserialization
    // Optimized ObjectID handling
    // Collection name inference: UserBSON -> users
    return nil
}

// BSON utilities
isBSON := cargo.BSON.IsBSONType(reflect.TypeOf(&proto.UserBSON{}))
protoUser := cargo.BSON.ExtractProtoType(userBSON)
collectionName := cargo.BSON.GetBSONCollectionName(&proto.UserBSON{})
```

## Configuration Reference

### Environment Variables

```bash
# Server Configuration
CARGO_PORT=:50051
CARGO_HOST=0.0.0.0
CARGO_READ_TIMEOUT=30s
CARGO_WRITE_TIMEOUT=30s

# Database Configuration
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=myapp
MONGODB_TIMEOUT=10s

# Authentication
JWT_SECRET=your-secret-key
JWT_EXPIRY_DURATION=24h

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
LOG_OUTPUT=stdout

# Caching
CACHE_BACKEND=memory
CACHE_DEFAULT_TTL=1h
CACHE_MAX_SIZE=10000

# Streaming
STREAMING_BUFFER_SIZE=100
STREAMING_MAX_CONCURRENT_ITEMS=1000
STREAMING_ENABLE_METRICS=true
```

### Programmatic Configuration

```go
config := &cargo.Config{
    Server: cargo.ServerConfig{
        Port:         ":50051",
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
    },
    Database: cargo.DatabaseConfig{
        MongoDB: cargo.MongoConfig{
            URI:      "mongodb://localhost:27017",
            Database: "myapp",
            Timeout:  10 * time.Second,
        },
    },
    Auth: cargo.AuthConfig{
        JWT: cargo.JWTConfig{
            Secret:         "your-secret-key",
            ExpiryDuration: 24 * time.Hour,
        },
    },
    Cache: cargo.CacheConfig{
        Backend:    "memory",
        DefaultTTL: time.Hour,
        MaxSize:    10000,
    },
    Streaming: cargo.StreamConfig{
        BufferSize:         100,
        MaxConcurrentItems: 1000,
        EnableMetrics:      true,
    },
}

app := cargo.New().WithConfig(config)
```

## Production Deployment

### Docker Support

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
CMD ["./main"]
```

### Monitoring Integration

```go
// Prometheus metrics
metrics := cargo.NewMetricsCollector()
http.Handle("/metrics", promhttp.HandlerFor(
    prometheus.DefaultGatherer,
    promhttp.HandlerOpts{},
))

// Health check endpoint
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    report := healthManager.GetOverallHealth(r.Context())
    json.NewEncoder(w).Encode(report)
})
```

### Scaling Considerations

- Use external MongoDB replica set for production
- Configure Redis for distributed caching
- Implement horizontal pod autoscaling based on metrics
- Set up load balancing for multiple service instances
- Monitor streaming performance and adjust buffer sizes

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions are welcome! Please read our contributing guidelines and submit pull requests to our repository.

## Support

- Documentation: [GitHub Wiki](https://github.com/herro-labs/cargo/wiki)
- Issues: [GitHub Issues](https://github.com/herro-labs/cargo/issues)
- Discussions: [GitHub Discussions](https://github.com/herro-labs/cargo/discussions) 