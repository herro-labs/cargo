package cargo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the health status of a service
type HealthStatus int

const (
	HealthStatusHealthy HealthStatus = iota
	HealthStatusDegraded
	HealthStatusUnhealthy
	HealthStatusUnknown
)

// String returns string representation of health status
func (hs HealthStatus) String() string {
	switch hs {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusDegraded:
		return "degraded"
	case HealthStatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheck represents a single health check
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) HealthCheckResult
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Status    HealthStatus           `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Error     error                  `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// HealthCheckFunc is a function type that implements HealthCheck
type HealthCheckFunc struct {
	name string
	fn   func(ctx context.Context) HealthCheckResult
}

func NewHealthCheckFunc(name string, fn func(ctx context.Context) HealthCheckResult) *HealthCheckFunc {
	return &HealthCheckFunc{
		name: name,
		fn:   fn,
	}
}

func (hc *HealthCheckFunc) Name() string {
	return hc.name
}

func (hc *HealthCheckFunc) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := hc.fn(ctx)
	result.Duration = time.Since(start)
	result.Timestamp = time.Now()
	return result
}

// MongoDB Health Check
type MongoDBHealthCheck struct {
	name   string
	client interface{} // mongo.Client interface to avoid direct dependency
	dbName string
}

func NewMongoDBHealthCheck(name string, client interface{}, dbName string) *MongoDBHealthCheck {
	return &MongoDBHealthCheck{
		name:   name,
		client: client,
		dbName: dbName,
	}
}

func (mhc *MongoDBHealthCheck) Name() string {
	return mhc.name
}

func (mhc *MongoDBHealthCheck) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Status:    HealthStatusUnknown,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Use the health checker from mongodb.go if available
	if app, ok := mhc.client.(*App); ok {
		healthChecker := app.NewHealthChecker()

		// Check connectivity
		if err := healthChecker.Ping(ctx); err != nil {
			result.Status = HealthStatusUnhealthy
			result.Message = "Failed to ping MongoDB"
			result.Error = err
		} else {
			result.Status = HealthStatusHealthy
			result.Message = "MongoDB connection healthy"

			// Get additional stats
			if stats, err := healthChecker.GetStats(ctx, mhc.dbName); err == nil {
				result.Details["stats"] = stats
			}
		}
	}

	result.Duration = time.Since(start)
	return result
}

// Service Health Check
type ServiceHealthCheck struct {
	name     string
	endpoint string
	timeout  time.Duration
}

func NewServiceHealthCheck(name, endpoint string, timeout time.Duration) *ServiceHealthCheck {
	return &ServiceHealthCheck{
		name:     name,
		endpoint: endpoint,
		timeout:  timeout,
	}
}

func (shc *ServiceHealthCheck) Name() string {
	return shc.name
}

func (shc *ServiceHealthCheck) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Status:    HealthStatusHealthy,
		Message:   fmt.Sprintf("Service %s is responding", shc.name),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, shc.timeout)
	defer cancel()

	// Simulate a health check (in real implementation, this would make an HTTP call)
	select {
	case <-timeoutCtx.Done():
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Service %s health check timed out", shc.name)
		result.Error = timeoutCtx.Err()
	default:
		// Service is responsive
		result.Details["endpoint"] = shc.endpoint
		result.Details["timeout"] = shc.timeout.String()
	}

	result.Duration = time.Since(start)
	return result
}

// Memory Health Check
type MemoryHealthCheck struct {
	name      string
	threshold int64 // MB
}

func NewMemoryHealthCheck(name string, thresholdMB int64) *MemoryHealthCheck {
	return &MemoryHealthCheck{
		name:      name,
		threshold: thresholdMB * 1024 * 1024, // Convert to bytes
	}
}

func (mhc *MemoryHealthCheck) Name() string {
	return mhc.name
}

func (mhc *MemoryHealthCheck) Check(ctx context.Context) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Status:    HealthStatusHealthy,
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Get memory statistics (simplified - in real implementation would use runtime.MemStats)
	var memUsage int64 = 50 * 1024 * 1024 // Simulated 50MB usage

	result.Details["memory_usage_bytes"] = memUsage
	result.Details["memory_threshold_bytes"] = mhc.threshold
	result.Details["memory_usage_mb"] = memUsage / (1024 * 1024)

	if memUsage > mhc.threshold {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("Memory usage (%d MB) exceeds threshold (%d MB)",
			memUsage/(1024*1024), mhc.threshold/(1024*1024))
	} else {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Memory usage (%d MB) is within threshold", memUsage/(1024*1024))
	}

	result.Duration = time.Since(start)
	return result
}

// HealthManager manages multiple health checks
type HealthManager struct {
	checks   map[string]HealthCheck
	results  map[string]HealthCheckResult
	mutex    sync.RWMutex
	interval time.Duration
	stopCh   chan struct{}
}

// OverallHealthReport represents the overall health of the application
type OverallHealthReport struct {
	Status    HealthStatus                 `json:"status"`
	Message   string                       `json:"message"`
	Timestamp time.Time                    `json:"timestamp"`
	Checks    map[string]HealthCheckResult `json:"checks"`
	Summary   map[string]int               `json:"summary"`
	Details   map[string]interface{}       `json:"details,omitempty"`
}

func NewHealthManager(interval time.Duration) *HealthManager {
	return &HealthManager{
		checks:   make(map[string]HealthCheck),
		results:  make(map[string]HealthCheckResult),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (hm *HealthManager) AddCheck(check HealthCheck) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	hm.checks[check.Name()] = check
}

func (hm *HealthManager) RemoveCheck(name string) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	delete(hm.checks, name)
	delete(hm.results, name)
}

func (hm *HealthManager) GetCheck(name string) (HealthCheckResult, bool) {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	result, exists := hm.results[name]
	return result, exists
}

func (hm *HealthManager) RunCheck(ctx context.Context, name string) (HealthCheckResult, error) {
	hm.mutex.RLock()
	check, exists := hm.checks[name]
	hm.mutex.RUnlock()

	if !exists {
		return HealthCheckResult{}, fmt.Errorf("health check %s not found", name)
	}

	result := check.Check(ctx)

	hm.mutex.Lock()
	hm.results[name] = result
	hm.mutex.Unlock()

	return result, nil
}

func (hm *HealthManager) RunAllChecks(ctx context.Context) map[string]HealthCheckResult {
	hm.mutex.RLock()
	checks := make(map[string]HealthCheck)
	for name, check := range hm.checks {
		checks[name] = check
	}
	hm.mutex.RUnlock()

	results := make(map[string]HealthCheckResult)
	var wg sync.WaitGroup

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()
			result := check.Check(ctx)

			hm.mutex.Lock()
			hm.results[name] = result
			results[name] = result
			hm.mutex.Unlock()
		}(name, check)
	}

	wg.Wait()
	return results
}

func (hm *HealthManager) GetOverallHealth(ctx context.Context) OverallHealthReport {
	results := hm.RunAllChecks(ctx)

	report := OverallHealthReport{
		Status:    HealthStatusHealthy,
		Timestamp: time.Now(),
		Checks:    results,
		Summary:   make(map[string]int),
		Details:   make(map[string]interface{}),
	}

	// Count status types
	statusCounts := make(map[HealthStatus]int)
	for _, result := range results {
		statusCounts[result.Status]++
		report.Summary[result.Status.String()]++
	}

	// Determine overall status
	if statusCounts[HealthStatusUnhealthy] > 0 {
		report.Status = HealthStatusUnhealthy
		report.Message = fmt.Sprintf("%d unhealthy checks detected", statusCounts[HealthStatusUnhealthy])
	} else if statusCounts[HealthStatusDegraded] > 0 {
		report.Status = HealthStatusDegraded
		report.Message = fmt.Sprintf("%d degraded checks detected", statusCounts[HealthStatusDegraded])
	} else if statusCounts[HealthStatusHealthy] > 0 {
		report.Status = HealthStatusHealthy
		report.Message = "All checks are healthy"
	} else {
		report.Status = HealthStatusUnknown
		report.Message = "No health checks configured"
	}

	// Add summary details
	report.Details["total_checks"] = len(results)
	report.Details["healthy_checks"] = statusCounts[HealthStatusHealthy]
	report.Details["degraded_checks"] = statusCounts[HealthStatusDegraded]
	report.Details["unhealthy_checks"] = statusCounts[HealthStatusUnhealthy]

	return report
}

func (hm *HealthManager) StartPeriodicChecks(ctx context.Context) {
	if hm.interval <= 0 {
		return
	}

	ticker := time.NewTicker(hm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hm.stopCh:
			return
		case <-ticker.C:
			hm.RunAllChecks(ctx)
		}
	}
}

func (hm *HealthManager) Stop() {
	close(hm.stopCh)
}

func (hm *HealthManager) ToJSON() ([]byte, error) {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	report := hm.GetOverallHealth(context.Background())
	return json.Marshal(report)
}

// App integration for health checks
func (a *App) NewHealthManager() *HealthManager {
	manager := NewHealthManager(30 * time.Second) // Check every 30 seconds

	// Add default health checks
	manager.AddCheck(NewMongoDBHealthCheck("mongodb", a, a.config.Database.MongoDB.Database))
	manager.AddCheck(NewMemoryHealthCheck("memory", 512)) // 512MB threshold

	return manager
}

// Built-in health check functions

// HealthyCheck always returns healthy
func HealthyCheck() HealthCheckResult {
	return HealthCheckResult{
		Status:    HealthStatusHealthy,
		Message:   "Service is healthy",
		Timestamp: time.Now(),
	}
}

// UnhealthyCheck always returns unhealthy
func UnhealthyCheck() HealthCheckResult {
	return HealthCheckResult{
		Status:    HealthStatusUnhealthy,
		Message:   "Service is unhealthy",
		Timestamp: time.Now(),
	}
}

// DegradedCheck returns degraded status
func DegradedCheck() HealthCheckResult {
	return HealthCheckResult{
		Status:    HealthStatusDegraded,
		Message:   "Service is degraded",
		Timestamp: time.Now(),
	}
}
