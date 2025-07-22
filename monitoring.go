package cargo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TraceID represents a unique identifier for a request trace
type TraceID string

// SpanID represents a unique identifier for a span within a trace
type SpanID string

// RequestInfo contains information about an incoming request
type RequestInfo struct {
	TraceID     TraceID           `json:"trace_id"`
	SpanID      SpanID            `json:"span_id"`
	Method      string            `json:"method"`
	Service     string            `json:"service"`
	UserID      string            `json:"user_id,omitempty"`
	ClientIP    string            `json:"client_ip,omitempty"`
	UserAgent   string            `json:"user_agent,omitempty"`
	StartTime   time.Time         `json:"start_time"`
	Headers     map[string]string `json:"headers,omitempty"`
	Request     interface{}       `json:"request,omitempty"`
	RequestSize int64             `json:"request_size"`
}

// ResponseInfo contains information about an outgoing response
type ResponseInfo struct {
	TraceID      TraceID       `json:"trace_id"`
	SpanID       SpanID        `json:"span_id"`
	StatusCode   int           `json:"status_code"`
	Status       string        `json:"status"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	Response     interface{}   `json:"response,omitempty"`
	ResponseSize int64         `json:"response_size"`
	Error        string        `json:"error,omitempty"`
}

// RequestTrace represents a complete request-response cycle
type RequestTrace struct {
	Request  RequestInfo       `json:"request"`
	Response ResponseInfo      `json:"response"`
	Tags     map[string]string `json:"tags,omitempty"`
	Logs     []TraceLog        `json:"logs,omitempty"`
}

// TraceLog represents a log entry within a trace
type TraceLog struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Monitor interface for request/response monitoring
type Monitor interface {
	StartRequest(ctx context.Context, method string, req interface{}) (*RequestTrace, context.Context)
	EndRequest(trace *RequestTrace, resp interface{}, err error)
	LogEvent(trace *RequestTrace, level, message string, fields map[string]interface{})
	GetTrace(traceID TraceID) (*RequestTrace, bool)
	GetActiveTraces() []*RequestTrace
}

// InMemoryMonitor implements Monitor with in-memory storage
type InMemoryMonitor struct {
	traces    map[TraceID]*RequestTrace
	mutex     sync.RWMutex
	maxTraces int
	retention time.Duration
}

// NewInMemoryMonitor creates a new in-memory monitor
func NewInMemoryMonitor(maxTraces int, retention time.Duration) *InMemoryMonitor {
	monitor := &InMemoryMonitor{
		traces:    make(map[TraceID]*RequestTrace),
		maxTraces: maxTraces,
		retention: retention,
	}

	// Start cleanup goroutine
	go monitor.cleanupOldTraces()

	return monitor
}

// StartRequest starts monitoring a new request
func (m *InMemoryMonitor) StartRequest(ctx context.Context, method string, req interface{}) (*RequestTrace, context.Context) {
	traceID := m.generateTraceID()
	spanID := m.generateSpanID()

	// Extract metadata from context
	userID := ""
	clientIP := ""
	userAgent := ""
	headers := make(map[string]string)

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userIds := md.Get("user-id"); len(userIds) > 0 {
			userID = userIds[0]
		}
		if ips := md.Get("x-forwarded-for"); len(ips) > 0 {
			clientIP = ips[0]
		} else if ips := md.Get("x-real-ip"); len(ips) > 0 {
			clientIP = ips[0]
		}
		if agents := md.Get("user-agent"); len(agents) > 0 {
			userAgent = agents[0]
		}

		// Store all headers
		for key, values := range md {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}
	}

	// Extract service name from method
	service := extractServiceName(method)

	// Calculate request size
	requestSize := m.calculateSize(req)

	trace := &RequestTrace{
		Request: RequestInfo{
			TraceID:     traceID,
			SpanID:      spanID,
			Method:      method,
			Service:     service,
			UserID:      userID,
			ClientIP:    clientIP,
			UserAgent:   userAgent,
			StartTime:   time.Now(),
			Headers:     headers,
			Request:     req,
			RequestSize: requestSize,
		},
		Tags: make(map[string]string),
		Logs: make([]TraceLog, 0),
	}

	// Store trace
	m.mutex.Lock()
	m.traces[traceID] = trace
	m.mutex.Unlock()

	// Add trace context
	ctxWithTrace := context.WithValue(ctx, "trace_id", string(traceID))
	ctxWithTrace = context.WithValue(ctxWithTrace, "span_id", string(spanID))

	// Record metrics
	if globalMetrics != nil {
		globalMetrics.IncrementCounter("cargo_requests_started_total", map[string]string{
			"method":  method,
			"service": service,
		})
	}

	// Log request start
	if logger := GetGlobalLogger(); logger != nil {
		logger.Info("Request started",
			"trace_id", string(traceID),
			"method", method,
			"service", service,
			"user_id", userID,
			"client_ip", clientIP,
		)
	}

	return trace, ctxWithTrace
}

// EndRequest completes monitoring for a request
func (m *InMemoryMonitor) EndRequest(trace *RequestTrace, resp interface{}, err error) {
	endTime := time.Now()
	duration := endTime.Sub(trace.Request.StartTime)

	// Determine status
	statusCode := 0
	statusMsg := "OK"
	errorMsg := ""

	if err != nil {
		if st, ok := status.FromError(err); ok {
			statusCode = int(st.Code())
			statusMsg = st.Code().String()
			errorMsg = st.Message()
		} else {
			statusCode = 13 // INTERNAL error
			statusMsg = "INTERNAL"
			errorMsg = err.Error()
		}
	}

	// Calculate response size
	responseSize := m.calculateSize(resp)

	// Update trace
	trace.Response = ResponseInfo{
		TraceID:      trace.Request.TraceID,
		SpanID:       trace.Request.SpanID,
		StatusCode:   statusCode,
		Status:       statusMsg,
		EndTime:      endTime,
		Duration:     duration,
		Response:     resp,
		ResponseSize: responseSize,
		Error:        errorMsg,
	}

	// Record metrics
	if globalMetrics != nil {
		globalMetrics.RecordRequest(
			trace.Request.Method,
			trace.Request.Service,
			statusMsg,
			duration,
		)

		if err != nil {
			globalMetrics.IncrementCounter("cargo_requests_failed_total", map[string]string{
				"method":  trace.Request.Method,
				"service": trace.Request.Service,
				"error":   statusMsg,
			})
		}
	}

	// Log request completion
	if logger := GetGlobalLogger(); logger != nil {
		logger.Info("Request completed",
			"trace_id", string(trace.Request.TraceID),
			"method", trace.Request.Method,
			"service", trace.Request.Service,
			"status", statusMsg,
			"duration", duration,
			"error", errorMsg,
		)
	}
}

// LogEvent adds a log event to a trace
func (m *InMemoryMonitor) LogEvent(trace *RequestTrace, level, message string, fields map[string]interface{}) {
	if trace == nil {
		return
	}

	logEntry := TraceLog{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    fields,
	}

	trace.Logs = append(trace.Logs, logEntry)
}

// GetTrace retrieves a trace by ID
func (m *InMemoryMonitor) GetTrace(traceID TraceID) (*RequestTrace, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	trace, exists := m.traces[traceID]
	return trace, exists
}

// GetActiveTraces returns all currently active traces
func (m *InMemoryMonitor) GetActiveTraces() []*RequestTrace {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	traces := make([]*RequestTrace, 0, len(m.traces))
	for _, trace := range m.traces {
		traces = append(traces, trace)
	}
	return traces
}

// cleanupOldTraces removes old traces periodically
func (m *InMemoryMonitor) cleanupOldTraces() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mutex.Lock()

		cutoff := time.Now().Add(-m.retention)
		toDelete := make([]TraceID, 0)

		for traceID, trace := range m.traces {
			if trace.Request.StartTime.Before(cutoff) {
				toDelete = append(toDelete, traceID)
			}
		}

		for _, traceID := range toDelete {
			delete(m.traces, traceID)
		}

		// Also enforce max traces limit
		if len(m.traces) > m.maxTraces {
			// Remove oldest traces
			oldest := make([]TraceID, 0)
			for traceID := range m.traces {
				oldest = append(oldest, traceID)
			}

			// Simple removal (in production, you'd sort by time)
			excess := len(m.traces) - m.maxTraces
			for i := 0; i < excess && i < len(oldest); i++ {
				delete(m.traces, oldest[i])
			}
		}

		m.mutex.Unlock()
	}
}

// Helper methods
func (m *InMemoryMonitor) generateTraceID() TraceID {
	return TraceID(fmt.Sprintf("trace-%d", time.Now().UnixNano()))
}

func (m *InMemoryMonitor) generateSpanID() SpanID {
	return SpanID(fmt.Sprintf("span-%d", time.Now().UnixNano()))
}

func (m *InMemoryMonitor) calculateSize(data interface{}) int64 {
	if data == nil {
		return 0
	}

	// Simple size calculation using JSON marshaling
	if jsonData, err := json.Marshal(data); err == nil {
		return int64(len(jsonData))
	}

	return 0
}

// extractServiceName extracts service name from gRPC method
func extractServiceName(method string) string {
	if method == "" {
		return "unknown"
	}

	// Method format: "/package.Service/Method"
	parts := strings.Split(method, "/")
	if len(parts) >= 2 {
		serviceParts := strings.Split(parts[1], ".")
		if len(serviceParts) > 1 {
			return serviceParts[len(serviceParts)-1]
		}
		return parts[1]
	}

	return method
}

// MonitoringMiddleware creates middleware for request/response monitoring
func MonitoringMiddleware(monitor Monitor) MiddlewareFunc {
	return func(ctx *Context) error {
		// Extract method from context
		method := "unknown"
		if methodValue := ctx.Value("method"); methodValue != nil {
			if methodStr, ok := methodValue.(string); ok {
				method = methodStr
			}
		}

		// Start monitoring (we can't access the actual request here, so pass nil)
		trace, _ := monitor.StartRequest(ctx.Context, method, nil)

		// Update context
		for key, value := range map[string]interface{}{
			"trace_id": string(trace.Request.TraceID),
			"span_id":  string(trace.Request.SpanID),
			"trace":    trace,
		} {
			*ctx = *ctx.WithValue(key, value)
		}

		return nil
	}
}

// Global monitor instance
var globalMonitor Monitor

func init() {
	globalMonitor = NewInMemoryMonitor(1000, time.Hour)
}

// SetGlobalMonitor sets the global monitor instance
func SetGlobalMonitor(monitor Monitor) {
	globalMonitor = monitor
}

// GetGlobalMonitor returns the global monitor instance
func GetGlobalMonitor() Monitor {
	return globalMonitor
}

// Convenience functions for global monitor
func StartRequest(ctx context.Context, method string, req interface{}) (*RequestTrace, context.Context) {
	return globalMonitor.StartRequest(ctx, method, req)
}

func EndRequest(trace *RequestTrace, resp interface{}, err error) {
	globalMonitor.EndRequest(trace, resp, err)
}

func LogEvent(trace *RequestTrace, level, message string, fields map[string]interface{}) {
	globalMonitor.LogEvent(trace, level, message, fields)
}

func GetTrace(traceID TraceID) (*RequestTrace, bool) {
	return globalMonitor.GetTrace(traceID)
}

func GetActiveTraces() []*RequestTrace {
	return globalMonitor.GetActiveTraces()
}

// Enhanced middleware that integrates with the ServiceHandler
func EnhancedMonitoringInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Start monitoring
		trace, tracedCtx := StartRequest(ctx, info.FullMethod, req)

		// Add request details now that we have access to them
		trace.Request.Request = req
		trace.Request.RequestSize = globalMonitor.(*InMemoryMonitor).calculateSize(req)

		// Call the handler
		resp, err := handler(tracedCtx, req)

		// End monitoring
		EndRequest(trace, resp, err)

		return resp, err
	}
}

// TraceContext helps extract tracing information from context
type TraceContext struct {
	TraceID TraceID
	SpanID  SpanID
	Trace   *RequestTrace
}

// GetTraceContext extracts trace context from Cargo context
func GetTraceContext(ctx *Context) *TraceContext {
	traceCtx := &TraceContext{}

	if traceID := ctx.Value("trace_id"); traceID != nil {
		if tid, ok := traceID.(string); ok {
			traceCtx.TraceID = TraceID(tid)
		}
	}

	if spanID := ctx.Value("span_id"); spanID != nil {
		if sid, ok := spanID.(string); ok {
			traceCtx.SpanID = SpanID(sid)
		}
	}

	if trace := ctx.Value("trace"); trace != nil {
		if t, ok := trace.(*RequestTrace); ok {
			traceCtx.Trace = t
		}
	}

	return traceCtx
}
