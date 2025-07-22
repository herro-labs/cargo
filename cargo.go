package cargo

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// App represents the Cargo application
type App struct {
	server          *grpc.Server
	mongoClient     *mongo.Client
	middleware      []MiddlewareFunc
	hooks           *HookRegistry
	authHooks       AuthHooks
	config          *Config
	registeredSvcs  []*grpc.ServiceDesc // Track registered services
	startupCallback func(*App) error    // Called after MongoDB connection is established
	cacheManager    *CacheManager       // Cache management
	streamManager   *StreamManager      // Stream management
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

// New creates a new Cargo application instance
func New() *App {
	config := DefaultConfig()

	// Initialize cache manager
	cacheManager := NewCacheManager()
	defaultCache := NewMemoryCache(config.Cache)
	cacheManager.AddCache("default", defaultCache)
	SetGlobalCacheManager(cacheManager)

	// Initialize stream manager
	streamManager := NewStreamManager(config.Streaming)
	SetGlobalStreamManager(streamManager)

	return &App{
		hooks:          NewHookRegistry(),
		authHooks:      AuthHooks{},
		config:         config,
		middleware:     []MiddlewareFunc{},
		registeredSvcs: []*grpc.ServiceDesc{}, // Initialize service tracking
		cacheManager:   cacheManager,
		streamManager:  streamManager,
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
	// Store the service descriptor for tracking
	a.registeredSvcs = append(a.registeredSvcs, serviceDesc)
	log.Printf("Registered service: %s", serviceDesc.ServiceName)

	// Note: The service will be registered later when the server is initialized
	// and the actual service implementation is provided by the application
}

// WithConfig sets the configuration for the application
func (a *App) WithConfig(config *Config) *App {
	a.config = config
	return a
}

// WithTLS configures the app to use TLS with the provided certificate and key files
func (a *App) WithTLS(certFile, keyFile string) *App {
	a.config.TLS.CertFile = certFile
	a.config.TLS.KeyFile = keyFile
	return a
}

// WithMutualTLS configures the app to use mutual TLS (client certificate verification)
func (a *App) WithMutualTLS(certFile, keyFile, clientCA string) *App {
	a.config.TLS.CertFile = certFile
	a.config.TLS.KeyFile = keyFile
	a.config.TLS.ClientCA = clientCA
	return a
}

// WithStartupCallback sets a callback to be executed after the MongoDB connection is established
func (a *App) WithStartupCallback(callback func(*App) error) *App {
	a.startupCallback = callback
	return a
}

// Run starts the gRPC server
func (a *App) Run(addr ...string) {
	// Allow override of port via parameter
	if len(addr) > 0 {
		a.config.Server.Port = addr[0]
	}

	// Validate configuration
	if err := a.config.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	if a.server == nil {
		a.initServer()
	}

	// Initialize MongoDB connection
	if err := a.connectMongoDB(); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Run startup callback if configured
	if a.startupCallback != nil {
		if err := a.startupCallback(a); err != nil {
			log.Fatalf("Startup callback failed: %v", err)
		}
	}

	listener, err := a.createSecureListener()
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", a.config.Server.Port, err)
	}

	// Enable reflection for development
	reflection.Register(a.server)

	// Start server in goroutine
	go func() {
		if a.config.TLS.CertFile != "" {
			log.Printf("Cargo server starting with TLS on %s", a.config.Server.Port)
		} else {
			log.Printf("Cargo server starting on %s", a.config.Server.Port)
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
	unaryInterceptor := a.createUnaryInterceptor()
	streamInterceptor := StreamInterceptor(a.streamManager)

	var opts []grpc.ServerOption
	opts = append(opts, grpc.UnaryInterceptor(unaryInterceptor))
	opts = append(opts, grpc.StreamInterceptor(streamInterceptor))

	// Add TLS credentials if configured
	if a.config.TLS.CertFile != "" && a.config.TLS.KeyFile != "" {
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
	ctx, cancel := context.WithTimeout(context.Background(), a.config.Database.MongoDB.ConnectTimeout)
	defer cancel()

	// Build MongoDB client options
	clientOptions := options.Client().
		ApplyURI(a.config.Database.MongoDB.URI).
		SetMaxPoolSize(a.config.Database.MongoDB.MaxPoolSize).
		SetMinPoolSize(a.config.Database.MongoDB.MinPoolSize).
		SetMaxConnIdleTime(a.config.Database.MongoDB.MaxConnIdleTime).
		SetServerSelectionTimeout(a.config.Database.MongoDB.ServerSelectionTimeout).
		SetRetryWrites(a.config.Database.MongoDB.RetryWrites).
		SetRetryReads(a.config.Database.MongoDB.RetryReads)

	client, err := mongo.Connect(ctx, clientOptions)
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

// createUnaryInterceptor creates the gRPC unary interceptor for middleware
func (a *App) createUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Create Cargo context
		cargoCtx := a.newContext(ctx)
		cargoCtx = cargoCtx.WithValue("method", info.FullMethod)

		// Run middleware
		for _, mw := range a.middleware {
			if err := mw(cargoCtx); err != nil {
				if a.authHooks.OnFailure != nil {
					a.authHooks.OnFailure(cargoCtx, err)
				}
				return nil, ToGRPCError(err)
			}
		}

		// Call auth success hook if authenticated
		if cargoCtx.Auth() != nil && a.authHooks.OnSuccess != nil {
			a.authHooks.OnSuccess(cargoCtx, cargoCtx.Auth())
		}

		// Call the original handler (service implementation)
		return handler(ctx, req)
	}
}

// GetServer returns the gRPC server instance for manual service registration
func (a *App) GetServer() *grpc.Server {
	if a.server == nil {
		a.initServer()
	}
	return a.server
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

// Cache accessor methods

// GetCacheManager returns the cache manager instance
func (a *App) GetCacheManager() *CacheManager {
	return a.cacheManager
}

// GetCache returns a specific cache instance by name
func (a *App) GetCache(name string) (Cache, bool) {
	return a.cacheManager.GetCache(name)
}

// GetDefaultCache returns the default cache instance
func (a *App) GetDefaultCache() Cache {
	return a.cacheManager.GetDefault()
}

// Stream accessor methods

// GetStreamManager returns the stream manager instance
func (a *App) GetStreamManager() *StreamManager {
	return a.streamManager
}

// GetStreamStats returns statistics for a specific stream
func (a *App) GetStreamStats(streamID string) (*StreamStats, bool) {
	return a.streamManager.GetStreamStats(streamID)
}

// GetAllStreamStats returns statistics for all streams
func (a *App) GetAllStreamStats() []*StreamStats {
	return a.streamManager.GetAllStreamStats()
}

// GetConfig returns the application configuration
func (a *App) GetConfig() *Config {
	return a.config
}

// UseCaching enables caching middleware with the specified strategy
func (a *App) UseCaching(strategy CacheStrategy, rules ...CacheRule) {
	config := CacheMiddlewareConfig{
		Cache:        a.cacheManager.GetDefault(),
		Strategy:     strategy,
		DefaultTTL:   a.config.Cache.DefaultTTL,
		Rules:        rules,
		KeyPrefix:    a.config.Cache.Prefix,
		MaxKeyLength: 250, // Reasonable default
	}
	a.Use(CacheMiddleware(config))
}
