package cargo

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// CacheStrategy defines when and how to cache requests
type CacheStrategy int

const (
	CacheStrategyNone     CacheStrategy = iota
	CacheStrategyReadOnly               // Cache only read operations
	CacheStrategyAll                    // Cache all successful operations
	CacheStrategyCustom                 // Use custom rules
)

// CacheRule defines a caching rule for specific methods or services
type CacheRule struct {
	Pattern     string                                                                       `json:"pattern"` // Method pattern (e.g., "TodoService.Get*")
	TTL         time.Duration                                                                `json:"ttl"`     // Cache TTL
	Enabled     bool                                                                         `json:"enabled"` // Whether caching is enabled
	KeyFunc     func(ctx context.Context, req interface{}) string                            `json:"-"`       // Custom key function
	ShouldCache func(ctx context.Context, req interface{}, resp interface{}, err error) bool `json:"-"`       // Custom cache condition
}

// CacheMiddlewareConfig holds cache middleware configuration
type CacheMiddlewareConfig struct {
	Cache           Cache                             `json:"-"`
	Strategy        CacheStrategy                     `json:"strategy"`
	DefaultTTL      time.Duration                     `json:"default_ttl"`
	Rules           []CacheRule                       `json:"rules"`
	KeyPrefix       string                            `json:"key_prefix"`
	IgnoreHeaders   []string                          `json:"ignore_headers"`
	MaxKeyLength    int                               `json:"max_key_length"`
	SerializeFunc   func(interface{}) ([]byte, error) `json:"-"`
	DeserializeFunc func([]byte, interface{}) error   `json:"-"`
}

// CacheMiddleware creates a caching middleware
func CacheMiddleware(config CacheMiddlewareConfig) MiddlewareFunc {
	if config.Cache == nil {
		config.Cache = GetDefaultCache()
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 5 * time.Minute
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = "cargo:cache"
	}
	if config.MaxKeyLength == 0 {
		config.MaxKeyLength = 250
	}
	if config.SerializeFunc == nil {
		config.SerializeFunc = MarshalJSON
	}
	if config.DeserializeFunc == nil {
		config.DeserializeFunc = UnmarshalJSON
	}

	return func(ctx *Context) error {
		// Skip caching based on strategy
		if config.Strategy == CacheStrategyNone {
			return nil
		}

		method := getMethodFromContext(ctx)
		if method == "" {
			return nil
		}

		// Check if method should be cached
		rule := findMatchingRule(method, config.Rules)
		if rule != nil && !rule.Enabled {
			return nil
		}

		// For read-only strategy, only cache read operations
		if config.Strategy == CacheStrategyReadOnly && !isReadOperation(method) {
			return nil
		}

		// This middleware is called before the actual service call
		// We need to integrate with the interceptor for full request/response caching
		return nil
	}
}

// CacheInterceptor creates a gRPC interceptor for automatic caching
func CacheInterceptor(config CacheMiddlewareConfig) grpc.UnaryServerInterceptor {
	if config.Cache == nil {
		config.Cache = GetDefaultCache()
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 5 * time.Minute
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = "cargo:cache"
	}
	if config.SerializeFunc == nil {
		config.SerializeFunc = MarshalJSON
	}
	if config.DeserializeFunc == nil {
		config.DeserializeFunc = UnmarshalJSON
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip caching based on strategy
		if config.Strategy == CacheStrategyNone {
			return handler(ctx, req)
		}

		method := info.FullMethod

		// Check if method should be cached
		rule := findMatchingRule(method, config.Rules)
		if rule != nil && !rule.Enabled {
			return handler(ctx, req)
		}

		// For read-only strategy, only cache read operations
		if config.Strategy == CacheStrategyReadOnly && !isReadOperation(method) {
			return handler(ctx, req)
		}

		// Generate cache key
		cacheKey := generateCacheKey(ctx, method, req, config, rule)

		// Try to get from cache first
		if cached, err := config.Cache.Get(ctx, cacheKey); err == nil {
			// Cache hit - deserialize and return
			var cachedResp CachedResponse
			if err := config.DeserializeFunc(cached, &cachedResp); err == nil {
				// Record cache hit metrics
				if metrics := GetGlobalMetrics(); metrics != nil {
					metrics.IncrementCounter("cargo_cache_hits_total", map[string]string{
						"method": extractServiceMethod(method),
					})
				}

				Info("Cache hit", "method", method, "key", cacheKey)

				// Deserialize the actual response
				respType := reflect.TypeOf(cachedResp.Response)
				if respType.Kind() == reflect.Ptr {
					respType = respType.Elem()
				}
				resp := reflect.New(respType).Interface()
				if err := config.DeserializeFunc(cachedResp.ResponseData, resp); err == nil {
					return resp, nil
				}
			}
		}

		// Cache miss - call the handler
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// Record cache miss metrics
		if metrics := GetGlobalMetrics(); metrics != nil {
			metrics.IncrementCounter("cargo_cache_misses_total", map[string]string{
				"method": extractServiceMethod(method),
			})
		}

		// Determine if we should cache this response
		shouldCache := shouldCacheResponse(ctx, req, resp, err, config, rule)

		if shouldCache && err == nil {
			// Serialize and cache the response
			if respData, serErr := config.SerializeFunc(resp); serErr == nil {
				cachedResp := CachedResponse{
					Method:       method,
					Response:     resp,
					ResponseData: respData,
					CachedAt:     time.Now(),
					TTL:          getTTLForRule(rule, config.DefaultTTL),
				}

				if cachedData, cacheErr := config.SerializeFunc(cachedResp); cacheErr == nil {
					ttl := getTTLForRule(rule, config.DefaultTTL)
					if setErr := config.Cache.Set(ctx, cacheKey, cachedData, ttl); setErr == nil {
						// Record cache set metrics
						if metrics := GetGlobalMetrics(); metrics != nil {
							metrics.IncrementCounter("cargo_cache_sets_total", map[string]string{
								"method": extractServiceMethod(method),
							})
						}

						Info("Response cached",
							"method", method,
							"key", cacheKey,
							"ttl", ttl,
							"duration_ms", duration.Milliseconds(),
						)
					}
				}
			}
		}

		return resp, err
	}
}

// CachedResponse represents a cached response with metadata
type CachedResponse struct {
	Method       string        `json:"method"`
	Response     interface{}   `json:"response"`
	ResponseData []byte        `json:"response_data"`
	CachedAt     time.Time     `json:"cached_at"`
	TTL          time.Duration `json:"ttl"`
}

// generateCacheKey creates a cache key for the request
func generateCacheKey(ctx context.Context, method string, req interface{}, config CacheMiddlewareConfig, rule *CacheRule) string {
	var keyParts []string

	// Start with prefix
	keyParts = append(keyParts, config.KeyPrefix)

	// Add method
	keyParts = append(keyParts, method)

	// Use custom key function if available
	if rule != nil && rule.KeyFunc != nil {
		customKey := rule.KeyFunc(ctx, req)
		keyParts = append(keyParts, customKey)
	} else {
		// Generate key from request data
		if reqData, err := config.SerializeFunc(req); err == nil {
			hash := md5.Sum(reqData)
			keyParts = append(keyParts, hex.EncodeToString(hash[:]))
		}

		// Add user context if available
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			// Add user ID to key for user-specific caching
			if userIDs := md.Get("user-id"); len(userIDs) > 0 {
				keyParts = append(keyParts, "user", userIDs[0])
			}

			// Add relevant headers (excluding ignored ones)
			for key, values := range md {
				if len(values) > 0 && !isIgnoredHeader(key, config.IgnoreHeaders) {
					keyParts = append(keyParts, key, values[0])
				}
			}
		}
	}

	key := strings.Join(keyParts, ":")

	// Truncate if too long
	if len(key) > config.MaxKeyLength {
		hash := md5.Sum([]byte(key))
		key = config.KeyPrefix + ":" + hex.EncodeToString(hash[:])
	}

	return key
}

// findMatchingRule finds a cache rule that matches the method
func findMatchingRule(method string, rules []CacheRule) *CacheRule {
	for i := range rules {
		if matchesPattern(method, rules[i].Pattern) {
			return &rules[i]
		}
	}
	return nil
}

// matchesPattern checks if a method matches a pattern
func matchesPattern(method, pattern string) bool {
	if pattern == "*" {
		return true
	}

	// Simple wildcard matching
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(method, parts[0]) && strings.HasSuffix(method, parts[1])
		}
	}

	return method == pattern
}

// isReadOperation determines if a method is a read operation
func isReadOperation(method string) bool {
	readMethods := []string{"Get", "List", "Find", "Search", "Query"}
	methodName := extractMethodName(method)

	for _, readMethod := range readMethods {
		if strings.Contains(methodName, readMethod) {
			return true
		}
	}

	return false
}

// shouldCacheResponse determines if a response should be cached
func shouldCacheResponse(ctx context.Context, req interface{}, resp interface{}, err error, config CacheMiddlewareConfig, rule *CacheRule) bool {
	// Don't cache errors
	if err != nil {
		return false
	}

	// Check gRPC status
	if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
		return false
	}

	// Use custom cache condition if available
	if rule != nil && rule.ShouldCache != nil {
		return rule.ShouldCache(ctx, req, resp, err)
	}

	// Default: cache successful responses
	return true
}

// getTTLForRule gets the TTL for a cache rule
func getTTLForRule(rule *CacheRule, defaultTTL time.Duration) time.Duration {
	if rule != nil && rule.TTL > 0 {
		return rule.TTL
	}
	return defaultTTL
}

// isIgnoredHeader checks if a header should be ignored in cache key generation
func isIgnoredHeader(header string, ignoredHeaders []string) bool {
	header = strings.ToLower(header)
	for _, ignored := range ignoredHeaders {
		if strings.ToLower(ignored) == header {
			return true
		}
	}

	// Always ignore certain headers
	defaultIgnored := []string{"authorization", "user-agent", "x-request-id", "x-trace-id"}
	for _, ignored := range defaultIgnored {
		if header == ignored {
			return true
		}
	}

	return false
}

// extractServiceMethod extracts service and method name from full method
func extractServiceMethod(fullMethod string) string {
	// Format: "/package.Service/Method"
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 3 {
		return parts[1] + "/" + parts[2]
	}
	return fullMethod
}

// extractMethodName extracts just the method name
func extractMethodName(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return fullMethod
}

// getMethodFromContext extracts method from context
func getMethodFromContext(ctx *Context) string {
	if method := ctx.Value("method"); method != nil {
		if methodStr, ok := method.(string); ok {
			return methodStr
		}
	}
	return ""
}

// Cache invalidation helpers

// InvalidateCache removes cache entries based on patterns
func InvalidateCache(ctx context.Context, cache Cache, patterns ...string) error {
	for _, pattern := range patterns {
		if err := cache.DeletePattern(ctx, pattern); err != nil {
			return fmt.Errorf("failed to invalidate cache pattern %s: %w", pattern, err)
		}
	}
	return nil
}

// InvalidateUserCache invalidates all cache entries for a specific user
func InvalidateUserCache(ctx context.Context, cache Cache, userID string) error {
	pattern := fmt.Sprintf("*:user:%s:*", userID)
	return cache.DeletePattern(ctx, pattern)
}

// InvalidateServiceCache invalidates all cache entries for a specific service
func InvalidateServiceCache(ctx context.Context, cache Cache, service string) error {
	pattern := fmt.Sprintf("*:%s/*", service)
	return cache.DeletePattern(ctx, pattern)
}

// Cache warming helpers

// WarmCache pre-populates cache with common requests
func WarmCache(ctx context.Context, cache Cache, warmupRequests []CacheWarmupRequest) error {
	for _, req := range warmupRequests {
		if data, err := MarshalJSON(req.Response); err == nil {
			if err := cache.Set(ctx, req.Key, data, req.TTL); err != nil {
				Warn("Failed to warm cache", "key", req.Key, "error", err.Error())
			}
		}
	}
	return nil
}

// CacheWarmupRequest represents a cache warmup request
type CacheWarmupRequest struct {
	Key      string        `json:"key"`
	Response interface{}   `json:"response"`
	TTL      time.Duration `json:"ttl"`
}

// Convenience functions for common cache patterns

// ReadThroughCache implements read-through caching pattern
func ReadThroughCache[T any](
	ctx context.Context,
	cache Cache,
	key string,
	ttl time.Duration,
	loader func(ctx context.Context) (*T, error),
) (*T, error) {
	// Try cache first
	if cached, err := cache.Get(ctx, key); err == nil {
		var result T
		if err := UnmarshalJSON(cached, &result); err == nil {
			return &result, nil
		}
	}

	// Load from source
	result, err := loader(ctx)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := MarshalJSON(result); err == nil {
		cache.Set(ctx, key, data, ttl)
	}

	return result, nil
}

// WriteThroughCache implements write-through caching pattern
func WriteThroughCache[T any](
	ctx context.Context,
	cache Cache,
	key string,
	value *T,
	ttl time.Duration,
	writer func(ctx context.Context, value *T) error,
) error {
	// Write to source first
	if err := writer(ctx, value); err != nil {
		return err
	}

	// Update cache
	if data, err := MarshalJSON(value); err == nil {
		cache.Set(ctx, key, data, ttl)
	}

	return nil
}

// WriteAroundCache implements write-around caching pattern
func WriteAroundCache[T any](
	ctx context.Context,
	cache Cache,
	key string,
	value *T,
	writer func(ctx context.Context, value *T) error,
) error {
	// Write to source
	if err := writer(ctx, value); err != nil {
		return err
	}

	// Invalidate cache to ensure consistency
	cache.Delete(ctx, key)

	return nil
}
