# Cargo

A lightweight gRPC framework for Go with built-in MongoDB integration and JWT authentication. Think Gin but for gRPC services.

## Directory Structure

```
cargo/
├── go.mod                 # Module definition
├── cargo.go              # Main framework (App, Context, types)
├── auth.go               # JWT authentication utilities  
├── middleware.go         # Built-in middleware (Logger, Recovery, Auth)
├── service.go            # gRPC service handling and hooks
├── database.go           # MongoDB operations
├── tls.go                # TLS/SSL support
├── bson.go               # protoc-gen-go-bson integration
├── auth/                 # Authentication package
├── middleware/           # Additional middleware
├── database/             # Database utilities
├── internal/             # Internal framework code
└── examples/             # Example applications
```

## Features

- gRPC server with automatic service registration
- MongoDB integration with automatic CRUD operations
- **protoc-gen-go-bson plugin support** for optimized MongoDB serialization
- JWT authentication with middleware
- **TLS/SSL support with certificates**
- **Mutual TLS (mTLS) support**
- Lifecycle hooks for request processing
- Built-in middleware (Logger, Recovery, Auth)
- Query building for MongoDB operations

## Installation

```bash
go get github.com/herro-labs/cargo
go install github.com/herro-labs/protoc-gen-go-bson@latest
```

## protoc-gen-go-bson Integration

Cargo is designed to work seamlessly with the [`protoc-gen-go-bson`](https://github.com/herro-labs/protoc-gen-go-bson) plugin for optimized MongoDB operations.

### Code Generation

Generate both protobuf and BSON types:

```bash
protoc --proto_path=proto \
  --go_out=proto --go-grpc_out=proto \
  --go-bson_out=proto \
  --go_opt=paths=source_relative \
  --go-grpc_opt=paths=source_relative \
  --go-bson_opt=paths=source_relative \
  proto/*.proto
```

### Example Proto File

```protobuf
syntax = "proto3";
package app;
option go_package = "./proto";

service UserService {
  rpc Create (User) returns (User);
  rpc Get (User) returns (User);
  rpc List (User) returns (stream User);
}

message User {
  string id = 1;
  string name = 2;
  string email = 3;
}
```

This generates:
- `user.pb.go` - Standard protobuf types
- `user_grpc.pb.go` - gRPC service definitions  
- `user_bson.pb.go` - BSON-optimized types (UserBSON)

### Using BSON Types with Cargo

```go
func main() {
    app := cargo.New()
    
    // Register service (works with BSON types automatically)
    app.RegisterService(&proto.UserService_ServiceDesc)
    app.RegisterHooks("UserService", cargo.Hooks{
        BeforeCreate: beforeCreateUser,
        AfterRead:    afterReadUser,
    })
    
    app.Run()
}

// Hook using BSON type for optimal MongoDB performance
func beforeCreateUser(ctx *cargo.Context, input *proto.UserBSON) error {
    // Cargo automatically handles:
    // 1. BSON serialization/deserialization
    // 2. MongoDB ObjectID mapping
    // 3. Collection name inference (UserBSON -> users)
    
    claims := ctx.Auth()
    // ... your logic here
    
    return nil
}
```

### BSON Utilities

Cargo provides utilities for working with BSON types:

```go
// Check if a type is a BSON wrapper
isBSON := cargo.BSON.IsBSONType(reflect.TypeOf(&proto.UserBSON{}))

// Extract the embedded protobuf message
protoUser := cargo.BSON.ExtractProtoType(userBSON)

// Get proper collection name for BSON types
collectionName := cargo.BSON.GetBSONCollectionName(&proto.UserBSON{})
```

## Quick Start

```go
package main

import (
    "github.com/herro-labs/cargo"
    "your-project/proto"
    "your-project/hooks"
)

func main() {
    app := cargo.New()

    // Add middleware
    app.Use(
        cargo.Logger(),
        cargo.Recovery(),
        cargo.Auth(),
    )

    // Set auth callbacks
    app.UseAuthHooks(hooks.OnAuthSuccess, hooks.OnAuthFailure)

    // Register services
    app.RegisterService(&proto.AuthService_ServiceDesc)
    app.RegisterHooks("AuthService", cargo.Hooks{
        BeforeLogin: hooks.BeforeLogin,
    })

    app.RegisterService(&proto.TodoService_ServiceDesc)
    app.RegisterHooks("TodoService", cargo.Hooks{
        BeforeCreate: hooks.BeforeCreateTodo,
        BeforeList:   hooks.BeforeListTodo,
        BeforeDelete: hooks.BeforeDeleteTodo,
        AfterRead:    hooks.AfterReadTodo,
    })

    app.Run() // Starts on :50051
}
```

## TLS/SSL Support

Cargo supports secure connections using TLS/SSL certificates:

### Basic TLS
```go
app := cargo.New()

// Configure TLS with certificate and key files
app.WithTLS("server.crt", "server.key")

app.Run(":443") // Secure HTTPS port
```

### Mutual TLS (Client Certificate Verification)
```go
app := cargo.New()

// Configure mutual TLS
app.WithMutualTLS("server.crt", "server.key", "ca.crt")

app.Run(":443")
```

### Generating Self-Signed Certificates for Development
```bash
# Generate private key
openssl genrsa -out server.key 2048

# Generate certificate
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 365
```

## Hooks

Cargo provides lifecycle hooks for request processing:

```go
type Hooks struct {
    BeforeCreate interface{} // func(*Context, *SomeType) error
    BeforeList   interface{} // func(*Context, *Query) error
    BeforeUpdate interface{} // func(*Context, *SomeType) error
    BeforeDelete interface{} // func(*Context, *SomeType) error
    BeforeLogin  interface{} // func(*Context, *SomeType) (*AuthIdentity, error)
    
    AfterCreate interface{} // func(*Context, *SomeType) error
    AfterRead   interface{} // func(*Context, *SomeType) error
    AfterUpdate interface{} // func(*Context, *SomeType) error
    AfterDelete interface{} // func(*Context, *SomeType) error
}
```

## Context

The Cargo context provides access to authentication and database:

```go
func BeforeCreateTodo(ctx *cargo.Context, input *proto.Todo) error {
    // Access authenticated user
    claims := ctx.Auth()
    input.UserId = claims.ID
    
    // Access MongoDB client
    db := ctx.Mongo()
    
    return nil
}
```

## Authentication

Implement login logic with BeforeLogin hook:

```go
func BeforeLogin(ctx *cargo.Context, input *proto.LoginRequest) (*cargo.AuthIdentity, error) {
    // Validate credentials
    user := validateUser(input.Email, input.Password)
    if user == nil {
        return nil, errors.New("invalid credentials")
    }
    
    return &cargo.AuthIdentity{
        ID: user.Id,
        Claims: map[string]any{
            "email": user.Email,
        },
    }, nil
}
```

## Environment Variables

- `MONGODB_URI`: MongoDB connection string (default: mongodb://localhost:27017)
- `JWT_SECRET`: JWT signing secret (default: cargo-default-secret-change-in-production)

## Database

Cargo automatically maps protobuf messages to MongoDB collections:
- `Todo` message → `todos` collection
- `User` message → `users` collection
- `UserBSON` type → `users` collection (with BSON optimization)
- `ContactMessage` message → `contactmessages` collection

## Security Features

- **TLS/SSL encryption** for secure communication
- **Mutual TLS (mTLS)** for client certificate verification
- **JWT authentication** with configurable secrets
- **Request/response hooks** for custom security logic
- **protoc-gen-go-bson** for optimized and secure MongoDB operations

## License

MIT 