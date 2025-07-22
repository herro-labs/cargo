package cargo

import (
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RateLimiter defines the interface for rate limiting
type RateLimiter interface {
	Allow(key string) bool
	Reset(key string)
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	capacity     int64
	tokens       int64
	refillRate   int64
	refillPeriod time.Duration
	lastRefill   time.Time
	mutex        sync.Mutex
}

func NewTokenBucket(capacity, refillRate int64, refillPeriod time.Duration) *TokenBucket {
	return &TokenBucket{
		capacity:     capacity,
		tokens:       capacity,
		refillRate:   refillRate,
		refillPeriod: refillPeriod,
		lastRefill:   time.Now(),
	}
}

func (tb *TokenBucket) Allow(key string) bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Refill tokens based on elapsed time
	if elapsed >= tb.refillPeriod {
		periods := elapsed / tb.refillPeriod
		tokensToAdd := int64(periods) * tb.refillRate
		tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

func (tb *TokenBucket) Reset(key string) {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	tb.tokens = tb.capacity
	tb.lastRefill = time.Now()
}

// SlidingWindow implements a sliding window rate limiter
type SlidingWindow struct {
	limit    int64
	window   time.Duration
	requests []time.Time
	mutex    sync.Mutex
}

func NewSlidingWindow(limit int64, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0),
	}
}

func (sw *SlidingWindow) Allow(key string) bool {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	// Remove old requests
	validRequests := sw.requests[:0]
	for _, req := range sw.requests {
		if req.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	sw.requests = validRequests

	if int64(len(sw.requests)) < sw.limit {
		sw.requests = append(sw.requests, now)
		return true
	}

	return false
}

func (sw *SlidingWindow) Reset(key string) {
	sw.mutex.Lock()
	defer sw.mutex.Unlock()
	sw.requests = sw.requests[:0]
}

// FixedWindow implements a fixed window rate limiter
type FixedWindow struct {
	limit       int64
	window      time.Duration
	count       int64
	windowStart time.Time
	mutex       sync.Mutex
}

func NewFixedWindow(limit int64, window time.Duration) *FixedWindow {
	return &FixedWindow{
		limit:       limit,
		window:      window,
		count:       0,
		windowStart: time.Now(),
	}
}

func (fw *FixedWindow) Allow(key string) bool {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	now := time.Now()

	// Check if we need to reset the window
	if now.Sub(fw.windowStart) >= fw.window {
		fw.count = 0
		fw.windowStart = now
	}

	if fw.count < fw.limit {
		fw.count++
		return true
	}

	return false
}

func (fw *FixedWindow) Reset(key string) {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()
	fw.count = 0
	fw.windowStart = time.Now()
}

// MultiKeyRateLimiter manages rate limiters for multiple keys
type MultiKeyRateLimiter struct {
	limiters    map[string]RateLimiter
	factory     func() RateLimiter
	mutex       sync.RWMutex
	cleanup     time.Duration
	lastCleanup time.Time
}

func NewMultiKeyRateLimiter(factory func() RateLimiter, cleanup time.Duration) *MultiKeyRateLimiter {
	return &MultiKeyRateLimiter{
		limiters:    make(map[string]RateLimiter),
		factory:     factory,
		cleanup:     cleanup,
		lastCleanup: time.Now(),
	}
}

func (mkrl *MultiKeyRateLimiter) Allow(key string) bool {
	mkrl.mutex.RLock()
	limiter, exists := mkrl.limiters[key]
	mkrl.mutex.RUnlock()

	if !exists {
		mkrl.mutex.Lock()
		// Double-check locking
		if limiter, exists = mkrl.limiters[key]; !exists {
			limiter = mkrl.factory()
			mkrl.limiters[key] = limiter
		}
		mkrl.mutex.Unlock()
	}

	// Periodic cleanup of old limiters
	if time.Since(mkrl.lastCleanup) > mkrl.cleanup {
		go mkrl.cleanupOldLimiters()
	}

	return limiter.Allow(key)
}

func (mkrl *MultiKeyRateLimiter) Reset(key string) {
	mkrl.mutex.RLock()
	limiter, exists := mkrl.limiters[key]
	mkrl.mutex.RUnlock()

	if exists {
		limiter.Reset(key)
	}
}

func (mkrl *MultiKeyRateLimiter) cleanupOldLimiters() {
	mkrl.mutex.Lock()
	defer mkrl.mutex.Unlock()

	// Simple cleanup - remove all limiters (in a real implementation,
	// you'd track last access time and remove only old ones)
	if len(mkrl.limiters) > 1000 { // Arbitrary threshold
		mkrl.limiters = make(map[string]RateLimiter)
	}
	mkrl.lastCleanup = time.Now()
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled    bool          `json:"enabled" yaml:"enabled"`
	Algorithm  string        `json:"algorithm" yaml:"algorithm"` // "token_bucket", "sliding_window", "fixed_window"
	Limit      int64         `json:"limit" yaml:"limit"`
	Window     time.Duration `json:"window" yaml:"window"`
	BurstLimit int64         `json:"burst_limit" yaml:"burst_limit"`
	KeyFunc    string        `json:"key_func" yaml:"key_func"` // "ip", "user", "service", "method"
}

// RateLimitKeyFunc defines how to extract rate limiting keys
type RateLimitKeyFunc func(ctx *Context) string

// Built-in key functions
var (
	// ByIP extracts client IP for rate limiting
	ByIP RateLimitKeyFunc = func(ctx *Context) string {
		if md, ok := metadata.FromIncomingContext(ctx.Context); ok {
			if ips := md.Get("x-forwarded-for"); len(ips) > 0 {
				return ips[0]
			}
			if ips := md.Get("x-real-ip"); len(ips) > 0 {
				return ips[0]
			}
		}
		return "unknown"
	}

	// ByUser extracts user ID for rate limiting
	ByUser RateLimitKeyFunc = func(ctx *Context) string {
		if claims := ctx.Auth(); claims != nil {
			return claims.ID
		}
		return "anonymous"
	}

	// ByService extracts service name for rate limiting
	ByService RateLimitKeyFunc = func(ctx *Context) string {
		if method := ctx.Value("method"); method != nil {
			if methodStr, ok := method.(string); ok {
				// Extract service from method (e.g., "/package.Service/Method" -> "Service")
				parts := strings.Split(methodStr, "/")
				if len(parts) >= 2 {
					serviceParts := strings.Split(parts[1], ".")
					if len(serviceParts) > 1 {
						return serviceParts[len(serviceParts)-1]
					}
				}
			}
		}
		return "unknown"
	}

	// ByMethod extracts full method for rate limiting
	ByMethod RateLimitKeyFunc = func(ctx *Context) string {
		if method := ctx.Value("method"); method != nil {
			if methodStr, ok := method.(string); ok {
				return methodStr
			}
		}
		return "unknown"
	}
)

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(config RateLimitConfig, keyFunc RateLimitKeyFunc) MiddlewareFunc {
	if !config.Enabled {
		return func(ctx *Context) error {
			return nil // No-op if disabled
		}
	}

	// Create rate limiter factory based on algorithm
	var factory func() RateLimiter
	switch config.Algorithm {
	case "token_bucket":
		factory = func() RateLimiter {
			return NewTokenBucket(config.BurstLimit, config.Limit, config.Window)
		}
	case "sliding_window":
		factory = func() RateLimiter {
			return NewSlidingWindow(config.Limit, config.Window)
		}
	case "fixed_window":
		factory = func() RateLimiter {
			return NewFixedWindow(config.Limit, config.Window)
		}
	default:
		factory = func() RateLimiter {
			return NewTokenBucket(config.BurstLimit, config.Limit, config.Window)
		}
	}

	rateLimiter := NewMultiKeyRateLimiter(factory, time.Hour) // Cleanup every hour

	return func(ctx *Context) error {
		key := keyFunc(ctx)

		if !rateLimiter.Allow(key) {
			// Record rate limit violation
			if globalMetrics != nil {
				globalMetrics.IncrementCounter("cargo_rate_limit_violations_total", map[string]string{
					"key":       key,
					"algorithm": config.Algorithm,
				})
			}

			// Log rate limit violation
			if logger := GetGlobalLogger(); logger != nil {
				logger.Warn("Rate limit exceeded",
					"key", key,
					"algorithm", config.Algorithm,
					"limit", config.Limit,
					"window", config.Window,
				)
			}

			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		return nil
	}
}

// Convenience functions for common rate limiting patterns

// RateLimitByIP creates a rate limiter by IP address
func RateLimitByIP(limit int64, window time.Duration) MiddlewareFunc {
	config := RateLimitConfig{
		Enabled:    true,
		Algorithm:  "token_bucket",
		Limit:      limit,
		Window:     window,
		BurstLimit: limit * 2, // Allow burst up to 2x the rate
	}
	return RateLimitMiddleware(config, ByIP)
}

// RateLimitByUser creates a rate limiter by user ID
func RateLimitByUser(limit int64, window time.Duration) MiddlewareFunc {
	config := RateLimitConfig{
		Enabled:    true,
		Algorithm:  "sliding_window",
		Limit:      limit,
		Window:     window,
		BurstLimit: limit,
	}
	return RateLimitMiddleware(config, ByUser)
}

// RateLimitByService creates a rate limiter by service
func RateLimitByService(limit int64, window time.Duration) MiddlewareFunc {
	config := RateLimitConfig{
		Enabled:    true,
		Algorithm:  "fixed_window",
		Limit:      limit,
		Window:     window,
		BurstLimit: limit,
	}
	return RateLimitMiddleware(config, ByService)
}

// RateLimitByMethod creates a rate limiter by method
func RateLimitByMethod(limit int64, window time.Duration) MiddlewareFunc {
	config := RateLimitConfig{
		Enabled:    true,
		Algorithm:  "sliding_window",
		Limit:      limit,
		Window:     window,
		BurstLimit: limit,
	}
	return RateLimitMiddleware(config, ByMethod)
}

// Helper function (min is not available in older Go versions)
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
