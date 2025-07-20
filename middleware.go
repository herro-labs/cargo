package cargo

import (
	"log"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Logger returns a logging middleware
func Logger() MiddlewareFunc {
	return func(ctx *Context) error {
		start := time.Now()
		log.Printf("[%s] Started", ctx.Value("method"))

		defer func() {
			duration := time.Since(start)
			log.Printf("[%s] Completed in %v", ctx.Value("method"), duration)
		}()

		return nil
	}
}

// Recovery returns a panic recovery middleware
func Recovery() MiddlewareFunc {
	return func(ctx *Context) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic recovered: %v\n%s", r, debug.Stack())
			}
		}()

		return nil
	}
}

// Auth returns an authentication middleware
func Auth() MiddlewareFunc {
	return func(ctx *Context) error {
		// Skip auth for AuthService and ContactService methods
		if method, ok := ctx.Value("method").(string); ok {
			if strings.Contains(method, "AuthService") || strings.Contains(method, "ContactService") {
				return nil
			}
		}

		md, ok := metadata.FromIncomingContext(ctx.Context)
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeaders := md["authorization"]
		if len(authHeaders) == 0 {
			return status.Error(codes.Unauthenticated, "missing authorization header")
		}

		tokenString := strings.TrimPrefix(authHeaders[0], "Bearer ")
		claims, err := validateJWT(tokenString)
		if err != nil {
			return status.Error(codes.Unauthenticated, "invalid token")
		}

		ctx.setAuth(claims)
		return nil
	}
}
