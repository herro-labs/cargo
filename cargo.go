package cargo

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// App represents the Cargo application
type App struct {
	server         *grpc.Server
	mongoClient    *mongo.Client
	middleware     []MiddlewareFunc
	hooks          map[string]Hooks
	authHooks      AuthHooks
	port           string
	tlsConfig      *TLSConfig
	registeredSvcs []*grpc.ServiceDesc // Track registered services
}

// Context wraps the standard context with additional framework functionality
type Context struct {
	context.Context
	app    *App
	claims *JWTClaims
}

// MiddlewareFunc defines the signature for middleware functions
type MiddlewareFunc func(*Context) error

// AuthHooks contains authentication lifecycle callbacks
type AuthHooks struct {
	OnSuccess func(*Context, *JWTClaims)
	OnFailure func(*Context, error)
}

// Hooks contains lifecycle hook functions for services
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

// JWTClaims represents the JWT token claims
type JWTClaims struct {
	ID     string                 `json:"id"`
	Claims map[string]interface{} `json:"claims"`
	jwt.RegisteredClaims
}

// AuthIdentity represents the authentication result from login hooks
type AuthIdentity struct {
	ID     string                 `json:"id"`
	Claims map[string]interface{} `json:"claims"`
}

// Query provides MongoDB query building functionality
type Query struct {
	filter bson.M
	opts   *options.FindOptions
}

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "cargo-default-secret-change-in-production"
	}
	jwtSecret = []byte(secret)
}

// New creates a new Cargo application instance
func New() *App {
	return &App{
		hooks:          make(map[string]Hooks),
		authHooks:      AuthHooks{},
		port:           ":50051",
		middleware:     []MiddlewareFunc{},
		registeredSvcs: []*grpc.ServiceDesc{}, // Initialize service tracking
	}
}

// Use adds middleware to the application
func (a *App) Use(middleware ...MiddlewareFunc) {
	a.middleware = append(a.middleware, middleware...)
}

// UseAuthHooks sets authentication success and failure callbacks
func (a *App) UseAuthHooks(onSuccess func(*Context, *JWTClaims), onFailure func(*Context, error)) {
	a.authHooks.OnSuccess = onSuccess
	a.authHooks.OnFailure = onFailure
}

// RegisterService registers a gRPC service with the application
func (a *App) RegisterService(serviceDesc *grpc.ServiceDesc) {
	if a.server == nil {
		a.initServer()
	}

	// Store the service descriptor for tracking
	a.registeredSvcs = append(a.registeredSvcs, serviceDesc)

	// Don't register with gRPC directly - let the unknown handler catch it
	log.Printf("Registered service: %s (handled via cargo unknown method handler)", serviceDesc.ServiceName)
}

// RegisterHooks registers lifecycle hooks for a specific service
func (a *App) RegisterHooks(serviceName string, hooks Hooks) {
	a.hooks[serviceName] = hooks
}

// Run starts the gRPC server
func (a *App) Run(addr ...string) {
	if len(addr) > 0 {
		a.port = addr[0]
	}

	if a.server == nil {
		a.initServer()
	}

	// Initialize MongoDB connection
	if err := a.connectMongoDB(); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	listener, err := a.createSecureListener()
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", a.port, err)
	}

	// Enable reflection for development
	reflection.Register(a.server)

	// Start server in goroutine
	go func() {
		if a.tlsConfig != nil {
			log.Printf("Cargo server starting with TLS on %s", a.port)
		} else {
			log.Printf("Cargo server starting on %s", a.port)
		}
		if err := a.server.Serve(listener); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")
	a.server.GracefulStop()

	if a.mongoClient != nil {
		a.mongoClient.Disconnect(nil)
	}
}

// initServer initializes the gRPC server with interceptors
func (a *App) initServer() {
	interceptor := a.createUnaryInterceptor()
	unknownHandler := a.createUnknownMethodHandler()

	var opts []grpc.ServerOption
	opts = append(opts, grpc.UnaryInterceptor(interceptor))
	opts = append(opts, grpc.UnknownServiceHandler(unknownHandler))

	// Add TLS credentials if configured
	if a.tlsConfig != nil {
		creds, err := a.createTLSCredentials()
		if err != nil {
			log.Fatalf("Failed to create TLS credentials: %v", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	a.server = grpc.NewServer(opts...)
}

// connectMongoDB establishes connection to MongoDB
func (a *App) connectMongoDB() error {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	a.mongoClient = client
	return nil
}

// createUnaryInterceptor creates the gRPC unary interceptor
func (a *App) createUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Check if this is a registered cargo service
		if a.isCargoService(info.FullMethod) {
			// Create Cargo context
			cargoCtx := a.newContext(ctx)
			cargoCtx = cargoCtx.WithValue("method", info.FullMethod)

			// Run middleware
			for _, mw := range a.middleware {
				if err := mw(cargoCtx); err != nil {
					if a.authHooks.OnFailure != nil {
						a.authHooks.OnFailure(cargoCtx, err)
					}
					return nil, err
				}
			}

			// Call auth success hook if authenticated
			if cargoCtx.Auth() != nil && a.authHooks.OnSuccess != nil {
				a.authHooks.OnSuccess(cargoCtx, cargoCtx.Auth())
			}

			// Route to cargo service handler
			return a.handleServiceCall(cargoCtx, req, info.FullMethod)
		}

		// For non-cargo services, use the original handler
		return handler(ctx, req)
	}
}

// isCargoService checks if a method belongs to a registered cargo service
func (a *App) isCargoService(fullMethod string) bool {
	for _, serviceDesc := range a.registeredSvcs {
		// Check if the method belongs to this service
		servicePath := "/" + serviceDesc.ServiceName + "/"
		if strings.HasPrefix(fullMethod, servicePath) {
			return true
		}
	}
	return false
}

// createUnknownMethodHandler creates a handler for unknown/unregistered gRPC methods
func (a *App) createUnknownMethodHandler() grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		fullMethod, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return fmt.Errorf("failed to get method from stream")
		}

		// Check if this is a cargo service
		if a.isCargoService(fullMethod) {
			// This is a cargo service - handle it
			return a.handleUnknownServiceCall(stream, fullMethod)
		}

		// Not a cargo service
		return fmt.Errorf("unknown service method: %s", fullMethod)
	}
}

// handleUnknownServiceCall handles calls to unregistered cargo services
func (a *App) handleUnknownServiceCall(stream grpc.ServerStream, fullMethod string) error {
	// For now, return unimplemented for streaming calls
	// TODO: Implement proper streaming support
	return fmt.Errorf("cargo service %s is not fully implemented yet", fullMethod)
}

// Context methods
func (a *App) newContext(ctx context.Context) *Context {
	return &Context{
		Context: ctx,
		app:     a,
	}
}

func (c *Context) WithValue(key, value interface{}) *Context {
	return &Context{
		Context: context.WithValue(c.Context, key, value),
		app:     c.app,
		claims:  c.claims,
	}
}

func (c *Context) Auth() *JWTClaims {
	return c.claims
}

func (c *Context) setAuth(claims *JWTClaims) {
	c.claims = claims
}

func (c *Context) Mongo() *mongo.Client {
	return c.app.mongoClient
}

// Query methods
func NewQuery() *Query {
	return &Query{
		filter: bson.M{},
		opts:   options.Find(),
	}
}

func (q *Query) Filter(filter bson.M) *Query {
	for k, v := range filter {
		q.filter[k] = v
	}
	return q
}

func (q *Query) Limit(limit int64) *Query {
	q.opts.SetLimit(limit)
	return q
}

func (q *Query) Skip(skip int64) *Query {
	q.opts.SetSkip(skip)
	return q
}

func (q *Query) Sort(sort bson.D) *Query {
	q.opts.SetSort(sort)
	return q
}

func (q *Query) GetFilter() bson.M {
	return q.filter
}

func (q *Query) GetOptions() *options.FindOptions {
	return q.opts
}
