package cargo

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType represents different types of metrics
type MetricType int

const (
	MetricTypeCounter MetricType = iota
	MetricTypeGauge
	MetricTypeHistogram
	MetricTypeSummary
)

// Metric represents a single metric
type Metric interface {
	Name() string
	Type() MetricType
	Labels() map[string]string
	Value() interface{}
	Reset()
}

// Counter represents a monotonic counter metric
type Counter struct {
	name   string
	labels map[string]string
	value  int64
}

func NewCounter(name string, labels map[string]string) *Counter {
	if labels == nil {
		labels = make(map[string]string)
	}
	return &Counter{
		name:   name,
		labels: labels,
		value:  0,
	}
}

func (c *Counter) Name() string {
	return c.name
}

func (c *Counter) Type() MetricType {
	return MetricTypeCounter
}

func (c *Counter) Labels() map[string]string {
	return c.labels
}

func (c *Counter) Value() interface{} {
	return atomic.LoadInt64(&c.value)
}

func (c *Counter) Reset() {
	atomic.StoreInt64(&c.value, 0)
}

func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

func (c *Counter) Add(value int64) {
	atomic.AddInt64(&c.value, value)
}

// Gauge represents a gauge metric that can go up and down
type Gauge struct {
	name   string
	labels map[string]string
	value  int64
}

func NewGauge(name string, labels map[string]string) *Gauge {
	if labels == nil {
		labels = make(map[string]string)
	}
	return &Gauge{
		name:   name,
		labels: labels,
		value:  0,
	}
}

func (g *Gauge) Name() string {
	return g.name
}

func (g *Gauge) Type() MetricType {
	return MetricTypeGauge
}

func (g *Gauge) Labels() map[string]string {
	return g.labels
}

func (g *Gauge) Value() interface{} {
	return atomic.LoadInt64(&g.value)
}

func (g *Gauge) Reset() {
	atomic.StoreInt64(&g.value, 0)
}

func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

func (g *Gauge) Add(value int64) {
	atomic.AddInt64(&g.value, value)
}

// Histogram represents a histogram metric for timing and size distributions
type Histogram struct {
	name    string
	labels  map[string]string
	buckets []float64
	counts  []int64
	sum     int64
	count   int64
	mutex   sync.RWMutex
}

func NewHistogram(name string, labels map[string]string, buckets []float64) *Histogram {
	if labels == nil {
		labels = make(map[string]string)
	}
	if buckets == nil {
		// Default buckets for response times in milliseconds
		buckets = []float64{0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}
	}

	return &Histogram{
		name:    name,
		labels:  labels,
		buckets: buckets,
		counts:  make([]int64, len(buckets)+1), // +1 for the +Inf bucket
		sum:     0,
		count:   0,
	}
}

func (h *Histogram) Name() string {
	return h.name
}

func (h *Histogram) Type() MetricType {
	return MetricTypeHistogram
}

func (h *Histogram) Labels() map[string]string {
	return h.labels
}

func (h *Histogram) Value() interface{} {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return map[string]interface{}{
		"buckets": h.buckets,
		"counts":  h.counts,
		"sum":     h.sum,
		"count":   h.count,
	}
}

func (h *Histogram) Reset() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for i := range h.counts {
		h.counts[i] = 0
	}
	h.sum = 0
	h.count = 0
}

func (h *Histogram) Observe(value float64) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Find the appropriate bucket
	for i, bucket := range h.buckets {
		if value <= bucket {
			atomic.AddInt64(&h.counts[i], 1)
		}
	}
	// Always increment the +Inf bucket
	atomic.AddInt64(&h.counts[len(h.buckets)], 1)

	atomic.AddInt64(&h.sum, int64(value))
	atomic.AddInt64(&h.count, 1)
}

// MetricRegistry manages all metrics
type MetricRegistry struct {
	metrics map[string]Metric
	mutex   sync.RWMutex
}

func NewMetricRegistry() *MetricRegistry {
	return &MetricRegistry{
		metrics: make(map[string]Metric),
	}
}

func (r *MetricRegistry) Register(metric Metric) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := r.metricKey(metric.Name(), metric.Labels())
	r.metrics[key] = metric
}

func (r *MetricRegistry) GetOrCreateCounter(name string, labels map[string]string) *Counter {
	key := r.metricKey(name, labels)

	r.mutex.RLock()
	if metric, exists := r.metrics[key]; exists {
		r.mutex.RUnlock()
		if counter, ok := metric.(*Counter); ok {
			return counter
		}
	} else {
		r.mutex.RUnlock()
	}

	// Create new counter
	counter := NewCounter(name, labels)
	r.Register(counter)
	return counter
}

func (r *MetricRegistry) GetOrCreateGauge(name string, labels map[string]string) *Gauge {
	key := r.metricKey(name, labels)

	r.mutex.RLock()
	if metric, exists := r.metrics[key]; exists {
		r.mutex.RUnlock()
		if gauge, ok := metric.(*Gauge); ok {
			return gauge
		}
	} else {
		r.mutex.RUnlock()
	}

	// Create new gauge
	gauge := NewGauge(name, labels)
	r.Register(gauge)
	return gauge
}

func (r *MetricRegistry) GetOrCreateHistogram(name string, labels map[string]string, buckets []float64) *Histogram {
	key := r.metricKey(name, labels)

	r.mutex.RLock()
	if metric, exists := r.metrics[key]; exists {
		r.mutex.RUnlock()
		if histogram, ok := metric.(*Histogram); ok {
			return histogram
		}
	} else {
		r.mutex.RUnlock()
	}

	// Create new histogram
	histogram := NewHistogram(name, labels, buckets)
	r.Register(histogram)
	return histogram
}

func (r *MetricRegistry) GetAllMetrics() []Metric {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	metrics := make([]Metric, 0, len(r.metrics))
	for _, metric := range r.metrics {
		metrics = append(metrics, metric)
	}
	return metrics
}

func (r *MetricRegistry) metricKey(name string, labels map[string]string) string {
	key := name
	for k, v := range labels {
		key += fmt.Sprintf(",%s=%s", k, v)
	}
	return key
}

// MetricsCollector provides high-level metrics collection
type MetricsCollector struct {
	registry *MetricRegistry
	enabled  bool
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		registry: NewMetricRegistry(),
		enabled:  true,
	}
}

func (mc *MetricsCollector) Enable() {
	mc.enabled = true
}

func (mc *MetricsCollector) Disable() {
	mc.enabled = false
}

func (mc *MetricsCollector) IsEnabled() bool {
	return mc.enabled
}

// Counter methods
func (mc *MetricsCollector) IncrementCounter(name string, labels map[string]string) {
	if !mc.enabled {
		return
	}
	counter := mc.registry.GetOrCreateCounter(name, labels)
	counter.Inc()
}

func (mc *MetricsCollector) AddToCounter(name string, labels map[string]string, value int64) {
	if !mc.enabled {
		return
	}
	counter := mc.registry.GetOrCreateCounter(name, labels)
	counter.Add(value)
}

// Gauge methods
func (mc *MetricsCollector) SetGauge(name string, labels map[string]string, value int64) {
	if !mc.enabled {
		return
	}
	gauge := mc.registry.GetOrCreateGauge(name, labels)
	gauge.Set(value)
}

func (mc *MetricsCollector) IncrementGauge(name string, labels map[string]string) {
	if !mc.enabled {
		return
	}
	gauge := mc.registry.GetOrCreateGauge(name, labels)
	gauge.Inc()
}

func (mc *MetricsCollector) DecrementGauge(name string, labels map[string]string) {
	if !mc.enabled {
		return
	}
	gauge := mc.registry.GetOrCreateGauge(name, labels)
	gauge.Dec()
}

// Histogram methods
func (mc *MetricsCollector) RecordDuration(name string, labels map[string]string, duration time.Duration) {
	if !mc.enabled {
		return
	}
	histogram := mc.registry.GetOrCreateHistogram(name, labels, nil)
	histogram.Observe(float64(duration.Milliseconds()))
}

func (mc *MetricsCollector) RecordValue(name string, labels map[string]string, value float64) {
	if !mc.enabled {
		return
	}
	histogram := mc.registry.GetOrCreateHistogram(name, labels, nil)
	histogram.Observe(value)
}

// Built-in application metrics
func (mc *MetricsCollector) RecordRequest(method, service, status string, duration time.Duration) {
	labels := map[string]string{
		"method":  method,
		"service": service,
		"status":  status,
	}

	mc.IncrementCounter("cargo_requests_total", labels)
	mc.RecordDuration("cargo_request_duration_ms", labels, duration)
}

func (mc *MetricsCollector) RecordDBOperation(operation, collection, status string, duration time.Duration) {
	labels := map[string]string{
		"operation":  operation,
		"collection": collection,
		"status":     status,
	}

	mc.IncrementCounter("cargo_db_operations_total", labels)
	mc.RecordDuration("cargo_db_operation_duration_ms", labels, duration)
}

func (mc *MetricsCollector) RecordValidationError(service, field string) {
	labels := map[string]string{
		"service": service,
		"field":   field,
	}

	mc.IncrementCounter("cargo_validation_errors_total", labels)
}

func (mc *MetricsCollector) RecordAuthentication(success bool) {
	status := "success"
	if !success {
		status = "failure"
	}

	labels := map[string]string{
		"status": status,
	}

	mc.IncrementCounter("cargo_auth_attempts_total", labels)
}

func (mc *MetricsCollector) UpdateActiveConnections(count int64) {
	mc.SetGauge("cargo_active_connections", nil, count)
}

func (mc *MetricsCollector) GetRegistry() *MetricRegistry {
	return mc.registry
}

// Global metrics collector
var globalMetrics *MetricsCollector

func init() {
	globalMetrics = NewMetricsCollector()
}

// SetGlobalMetrics sets the global metrics collector
func SetGlobalMetrics(collector *MetricsCollector) {
	globalMetrics = collector
}

// GetGlobalMetrics returns the global metrics collector
func GetGlobalMetrics() *MetricsCollector {
	return globalMetrics
}

// Convenience functions for global metrics
func IncrementCounter(name string, labels map[string]string) {
	globalMetrics.IncrementCounter(name, labels)
}

func SetGauge(name string, labels map[string]string, value int64) {
	globalMetrics.SetGauge(name, labels, value)
}

func RecordDuration(name string, labels map[string]string, duration time.Duration) {
	globalMetrics.RecordDuration(name, labels, duration)
}

func RecordRequest(method, service, status string, duration time.Duration) {
	globalMetrics.RecordRequest(method, service, status, duration)
}

func RecordDBOperation(operation, collection, status string, duration time.Duration) {
	globalMetrics.RecordDBOperation(operation, collection, status, duration)
}
