package cargo

import (
	"context"
	"sync"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// String returns string representation of circuit breaker state
func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitBreakerClosed:
		return "closed"
	case CircuitBreakerOpen:
		return "open"
	case CircuitBreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	MaxFailures      int           `json:"max_failures" yaml:"max_failures"`
	Timeout          time.Duration `json:"timeout" yaml:"timeout"`
	ResetTimeout     time.Duration `json:"reset_timeout" yaml:"reset_timeout"`
	SuccessThreshold int           `json:"success_threshold" yaml:"success_threshold"`
}

// CircuitBreakerStats holds statistics for a circuit breaker
type CircuitBreakerStats struct {
	State           CircuitBreakerState `json:"state"`
	Failures        int                 `json:"failures"`
	Successes       int                 `json:"successes"`
	Requests        int                 `json:"requests"`
	LastFailureTime time.Time           `json:"last_failure_time"`
	LastSuccessTime time.Time           `json:"last_success_time"`
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config        CircuitBreakerConfig
	state         CircuitBreakerState
	failures      int
	successes     int
	requests      int
	lastFailure   time.Time
	lastSuccess   time.Time
	nextAttempt   time.Time
	mutex         sync.RWMutex
	onStateChange func(from, to CircuitBreakerState)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 60 * time.Second
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 30 * time.Second
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}

	return &CircuitBreaker{
		config:      config,
		state:       CircuitBreakerClosed,
		failures:    0,
		successes:   0,
		requests:    0,
		nextAttempt: time.Now(),
	}
}

// WithStateChangeCallback sets a callback for state changes
func (cb *CircuitBreaker) WithStateChangeCallback(callback func(from, to CircuitBreakerState)) *CircuitBreaker {
	cb.onStateChange = callback
	return cb
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.allowRequest() {
		return NewCircuitBreakerOpenError()
	}

	err := fn()
	cb.recordResult(err == nil)
	return err
}

// CallWithContext executes a function with circuit breaker protection and context
func (cb *CircuitBreaker) CallWithContext(ctx context.Context, fn func(context.Context) error) error {
	if !cb.allowRequest() {
		return NewCircuitBreakerOpenError()
	}

	// Create a timeout context if configured
	if cb.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.config.Timeout)
		defer cancel()
	}

	err := fn(ctx)
	cb.recordResult(err == nil)
	return err
}

// allowRequest checks if a request should be allowed
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		if now.After(cb.nextAttempt) {
			cb.setState(CircuitBreakerHalfOpen)
			return true
		}
		return false
	case CircuitBreakerHalfOpen:
		return true
	default:
		return false
	}
}

// recordResult records the result of a request
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.requests++

	if success {
		cb.successes++
		cb.lastSuccess = time.Now()
		cb.onSuccess()
	} else {
		cb.failures++
		cb.lastFailure = time.Now()
		cb.onFailure()
	}
}

// onSuccess handles successful requests
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case CircuitBreakerHalfOpen:
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(CircuitBreakerClosed)
			cb.reset()
		}
	case CircuitBreakerClosed:
		// Reset failure count on success
		cb.failures = 0
	}
}

// onFailure handles failed requests
func (cb *CircuitBreaker) onFailure() {
	switch cb.state {
	case CircuitBreakerClosed:
		if cb.failures >= cb.config.MaxFailures {
			cb.setState(CircuitBreakerOpen)
			cb.nextAttempt = time.Now().Add(cb.config.ResetTimeout)
		}
	case CircuitBreakerHalfOpen:
		cb.setState(CircuitBreakerOpen)
		cb.nextAttempt = time.Now().Add(cb.config.ResetTimeout)
	}
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(newState CircuitBreakerState) {
	if cb.state != newState {
		oldState := cb.state
		cb.state = newState

		if cb.onStateChange != nil {
			cb.onStateChange(oldState, newState)
		}

		// Log state change
		if logger := GetGlobalLogger(); logger != nil {
			logger.Info("Circuit breaker state changed",
				"from", oldState.String(),
				"to", newState.String(),
				"failures", cb.failures,
				"successes", cb.successes,
			)
		}

		// Record metrics
		if globalMetrics != nil {
			globalMetrics.IncrementCounter("cargo_circuit_breaker_state_changes_total", map[string]string{
				"from": oldState.String(),
				"to":   newState.String(),
			})
		}
	}
}

// reset resets the circuit breaker counters
func (cb *CircuitBreaker) reset() {
	cb.failures = 0
	cb.successes = 0
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return CircuitBreakerStats{
		State:           cb.state,
		Failures:        cb.failures,
		Successes:       cb.successes,
		Requests:        cb.requests,
		LastFailureTime: cb.lastFailure,
		LastSuccessTime: cb.lastSuccess,
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.setState(CircuitBreakerClosed)
	cb.reset()
	cb.nextAttempt = time.Now()
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
	mutex    sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(config CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetCircuitBreaker gets or creates a circuit breaker for a given key
func (cbm *CircuitBreakerManager) GetCircuitBreaker(key string) *CircuitBreaker {
	cbm.mutex.RLock()
	cb, exists := cbm.breakers[key]
	cbm.mutex.RUnlock()

	if exists {
		return cb
	}

	cbm.mutex.Lock()
	defer cbm.mutex.Unlock()

	// Double-check locking
	if cb, exists = cbm.breakers[key]; exists {
		return cb
	}

	// Create new circuit breaker
	cb = NewCircuitBreaker(cbm.config)
	cb.WithStateChangeCallback(func(from, to CircuitBreakerState) {
		if logger := GetGlobalLogger(); logger != nil {
			logger.Info("Circuit breaker state changed",
				"key", key,
				"from", from.String(),
				"to", to.String(),
			)
		}
	})

	cbm.breakers[key] = cb
	return cb
}

// GetAllStats returns statistics for all circuit breakers
func (cbm *CircuitBreakerManager) GetAllStats() map[string]CircuitBreakerStats {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	stats := make(map[string]CircuitBreakerStats)
	for key, cb := range cbm.breakers {
		stats[key] = cb.GetStats()
	}
	return stats
}

// Reset resets all circuit breakers
func (cbm *CircuitBreakerManager) Reset() {
	cbm.mutex.RLock()
	defer cbm.mutex.RUnlock()

	for _, cb := range cbm.breakers {
		cb.Reset()
	}
}

// CircuitBreakerMiddleware creates middleware that protects service calls with circuit breakers
func CircuitBreakerMiddleware(config CircuitBreakerConfig) MiddlewareFunc {
	manager := NewCircuitBreakerManager(config)

	return func(ctx *Context) error {
		// Extract service name for circuit breaker key
		method := "unknown"
		if methodValue := ctx.Value("method"); methodValue != nil {
			if methodStr, ok := methodValue.(string); ok {
				method = methodStr
			}
		}

		cb := manager.GetCircuitBreaker(method)

		// The actual service call happens after middleware, so we can't wrap it here
		// Instead, we check if the circuit breaker allows the request
		if !cb.allowRequest() {
			if globalMetrics != nil {
				globalMetrics.IncrementCounter("cargo_circuit_breaker_rejections_total", map[string]string{
					"method": method,
				})
			}

			return NewCircuitBreakerOpenError()
		}

		return nil
	}
}

// Circuit breaker error types
type CircuitBreakerOpenError struct {
	message string
}

func NewCircuitBreakerOpenError() *CircuitBreakerOpenError {
	return &CircuitBreakerOpenError{
		message: "circuit breaker is open",
	}
}

func (e *CircuitBreakerOpenError) Error() string {
	return e.message
}

// Helper functions for common circuit breaker patterns

// DefaultCircuitBreakerConfig returns a sensible default configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:      5,
		Timeout:          60 * time.Second,
		ResetTimeout:     30 * time.Second,
		SuccessThreshold: 3,
	}
}

// ServiceCircuitBreakerConfig returns configuration optimized for service calls
func ServiceCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:      3,
		Timeout:          30 * time.Second,
		ResetTimeout:     60 * time.Second,
		SuccessThreshold: 2,
	}
}

// DatabaseCircuitBreakerConfig returns configuration optimized for database calls
func DatabaseCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:      10,
		Timeout:          10 * time.Second,
		ResetTimeout:     120 * time.Second,
		SuccessThreshold: 5,
	}
}
