package cargo

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StreamType represents different types of streams
type StreamType int

const (
	StreamTypeServerStreaming StreamType = iota
	StreamTypeClientStreaming
	StreamTypeBidirectional
)

// StreamConfig holds streaming configuration
type StreamConfig struct {
	BufferSize          int           `json:"buffer_size"`
	MaxConcurrentItems  int           `json:"max_concurrent_items"`
	BackpressureTimeout time.Duration `json:"backpressure_timeout"`
	HeartbeatInterval   time.Duration `json:"heartbeat_interval"`
	MaxStreamDuration   time.Duration `json:"max_stream_duration"`
	EnableMetrics       bool          `json:"enable_metrics"`
	EnableBackpressure  bool          `json:"enable_backpressure"`
}

// StreamStats holds statistics for a stream
type StreamStats struct {
	StreamID      string        `json:"stream_id"`
	Type          StreamType    `json:"type"`
	StartTime     time.Time     `json:"start_time"`
	EndTime       *time.Time    `json:"end_time,omitempty"`
	Duration      time.Duration `json:"duration"`
	ItemsSent     int64         `json:"items_sent"`
	ItemsReceived int64         `json:"items_received"`
	BytesSent     int64         `json:"bytes_sent"`
	BytesReceived int64         `json:"bytes_received"`
	ErrorCount    int64         `json:"error_count"`
	LastActivity  time.Time     `json:"last_activity"`
	Active        bool          `json:"active"`
}

// StreamManager manages multiple streams and their lifecycle
type StreamManager struct {
	streams map[string]*StreamStats
	mutex   sync.RWMutex
	config  StreamConfig
}

// NewStreamManager creates a new stream manager
func NewStreamManager(config StreamConfig) *StreamManager {
	if config.BufferSize == 0 {
		config.BufferSize = 100
	}
	if config.MaxConcurrentItems == 0 {
		config.MaxConcurrentItems = 1000
	}
	if config.BackpressureTimeout == 0 {
		config.BackpressureTimeout = 30 * time.Second
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.MaxStreamDuration == 0 {
		config.MaxStreamDuration = time.Hour
	}

	return &StreamManager{
		streams: make(map[string]*StreamStats),
		config:  config,
	}
}

// RegisterStream registers a new stream
func (sm *StreamManager) RegisterStream(streamID string, streamType StreamType) *StreamStats {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	stats := &StreamStats{
		StreamID:     streamID,
		Type:         streamType,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Active:       true,
	}

	sm.streams[streamID] = stats
	return stats
}

// UnregisterStream removes a stream from management
func (sm *StreamManager) UnregisterStream(streamID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if stats, exists := sm.streams[streamID]; exists {
		now := time.Now()
		stats.EndTime = &now
		stats.Duration = now.Sub(stats.StartTime)
		stats.Active = false
	}
}

// GetStreamStats returns statistics for a specific stream
func (sm *StreamManager) GetStreamStats(streamID string) (*StreamStats, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	stats, exists := sm.streams[streamID]
	return stats, exists
}

// GetAllStreamStats returns statistics for all streams
func (sm *StreamManager) GetAllStreamStats() []*StreamStats {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := make([]*StreamStats, 0, len(sm.streams))
	for _, s := range sm.streams {
		statsCopy := *s
		stats = append(stats, &statsCopy)
	}
	return stats
}

// UpdateStreamActivity updates the last activity time for a stream
func (sm *StreamManager) UpdateStreamActivity(streamID string) {
	sm.mutex.RLock()
	stats, exists := sm.streams[streamID]
	sm.mutex.RUnlock()

	if exists {
		stats.LastActivity = time.Now()
	}
}

// IncrementSent increments the sent counter for a stream
func (sm *StreamManager) IncrementSent(streamID string, bytes int64) {
	sm.mutex.RLock()
	stats, exists := sm.streams[streamID]
	sm.mutex.RUnlock()

	if exists {
		atomic.AddInt64(&stats.ItemsSent, 1)
		atomic.AddInt64(&stats.BytesSent, bytes)
		stats.LastActivity = time.Now()
	}
}

// IncrementReceived increments the received counter for a stream
func (sm *StreamManager) IncrementReceived(streamID string, bytes int64) {
	sm.mutex.RLock()
	stats, exists := sm.streams[streamID]
	sm.mutex.RUnlock()

	if exists {
		atomic.AddInt64(&stats.ItemsReceived, 1)
		atomic.AddInt64(&stats.BytesReceived, bytes)
		stats.LastActivity = time.Now()
	}
}

// IncrementError increments the error counter for a stream
func (sm *StreamManager) IncrementError(streamID string) {
	sm.mutex.RLock()
	stats, exists := sm.streams[streamID]
	sm.mutex.RUnlock()

	if exists {
		atomic.AddInt64(&stats.ErrorCount, 1)
		stats.LastActivity = time.Now()
	}
}

// EnhancedServerStreamInterface defines interface for server streaming
type EnhancedServerStreamInterface[T any] interface {
	Send(*T) error
	Context() context.Context
}

// EnhancedServerStream wraps a server stream with enhanced capabilities
type EnhancedServerStream[T any] struct {
	stream   EnhancedServerStreamInterface[T]
	streamID string
	manager  *StreamManager
	config   StreamConfig
	buffer   chan *T
	done     chan struct{}
	errors   chan error
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewEnhancedServerStream creates a new enhanced server stream
func NewEnhancedServerStream[T any](
	stream EnhancedServerStreamInterface[T],
	streamID string,
	manager *StreamManager,
	config StreamConfig,
) *EnhancedServerStream[T] {
	ctx, cancel := context.WithCancel(stream.Context())

	enhanced := &EnhancedServerStream[T]{
		stream:   stream,
		streamID: streamID,
		manager:  manager,
		config:   config,
		buffer:   make(chan *T, config.BufferSize),
		done:     make(chan struct{}),
		errors:   make(chan error, 10),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Register with manager
	enhanced.manager.RegisterStream(streamID, StreamTypeServerStreaming)

	return enhanced
}

// Send sends a message with enhanced features
func (ess *EnhancedServerStream[T]) Send(msg *T) error {
	select {
	case <-ess.done:
		return status.Error(codes.Canceled, "stream closed")
	case <-ess.ctx.Done():
		return ess.ctx.Err()
	default:
	}

	// Calculate message size (approximate)
	bytes := int64(calculateMessageSize(msg))

	// Apply backpressure if enabled
	if ess.config.EnableBackpressure {
		select {
		case ess.buffer <- msg:
			// Message buffered successfully
		case <-time.After(ess.config.BackpressureTimeout):
			ess.manager.IncrementError(ess.streamID)
			return status.Error(codes.ResourceExhausted, "stream buffer full")
		case <-ess.ctx.Done():
			return ess.ctx.Err()
		}
	}

	// Send the message
	if err := ess.stream.Send(msg); err != nil {
		ess.manager.IncrementError(ess.streamID)
		return err
	}

	// Update metrics
	ess.manager.IncrementSent(ess.streamID, bytes)

	if ess.config.EnableMetrics {
		if metrics := GetGlobalMetrics(); metrics != nil {
			metrics.IncrementCounter("cargo_stream_messages_sent_total", map[string]string{
				"stream_id": ess.streamID,
				"type":      "server_streaming",
			})
			metrics.RecordValue("cargo_stream_message_size_bytes", map[string]string{
				"stream_id": ess.streamID,
				"direction": "sent",
			}, float64(bytes))
		}
	}

	return nil
}

// SendBatch sends multiple messages efficiently
func (ess *EnhancedServerStream[T]) SendBatch(messages []*T) error {
	for _, msg := range messages {
		if err := ess.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the enhanced stream
func (ess *EnhancedServerStream[T]) Close() error {
	close(ess.done)
	ess.cancel()
	ess.manager.UnregisterStream(ess.streamID)

	if ess.config.EnableMetrics {
		if stats, exists := ess.manager.GetStreamStats(ess.streamID); exists {
			if metrics := GetGlobalMetrics(); metrics != nil {
				metrics.RecordDuration("cargo_stream_duration_ms", map[string]string{
					"stream_id": ess.streamID,
					"type":      "server_streaming",
				}, stats.Duration)
			}
		}
	}

	return nil
}

// EnhancedClientStreamInterface defines interface for client streaming
type EnhancedClientStreamInterface[T any] interface {
	Recv() (*T, error)
	Context() context.Context
}

// EnhancedClientStream wraps a client stream with enhanced capabilities
type EnhancedClientStream[T any] struct {
	stream   EnhancedClientStreamInterface[T]
	streamID string
	manager  *StreamManager
	config   StreamConfig
	buffer   chan *T
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewEnhancedClientStream creates a new enhanced client stream
func NewEnhancedClientStream[T any](
	stream EnhancedClientStreamInterface[T],
	streamID string,
	manager *StreamManager,
	config StreamConfig,
) *EnhancedClientStream[T] {
	ctx, cancel := context.WithCancel(stream.Context())

	enhanced := &EnhancedClientStream[T]{
		stream:   stream,
		streamID: streamID,
		manager:  manager,
		config:   config,
		buffer:   make(chan *T, config.BufferSize),
		done:     make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Register with manager
	enhanced.manager.RegisterStream(streamID, StreamTypeClientStreaming)

	return enhanced
}

// Recv receives a message with enhanced features
func (ecs *EnhancedClientStream[T]) Recv() (*T, error) {
	msg, err := ecs.stream.Recv()
	if err != nil {
		if err == io.EOF {
			ecs.Close()
			return nil, err
		}
		ecs.manager.IncrementError(ecs.streamID)
		return nil, err
	}

	// Calculate message size (approximate)
	bytes := int64(calculateMessageSize(msg))

	// Update metrics
	ecs.manager.IncrementReceived(ecs.streamID, bytes)

	if ecs.config.EnableMetrics {
		if metrics := GetGlobalMetrics(); metrics != nil {
			metrics.IncrementCounter("cargo_stream_messages_received_total", map[string]string{
				"stream_id": ecs.streamID,
				"type":      "client_streaming",
			})
			metrics.RecordValue("cargo_stream_message_size_bytes", map[string]string{
				"stream_id": ecs.streamID,
				"direction": "received",
			}, float64(bytes))
		}
	}

	return msg, nil
}

// Close closes the enhanced client stream
func (ecs *EnhancedClientStream[T]) Close() error {
	close(ecs.done)
	ecs.cancel()
	ecs.manager.UnregisterStream(ecs.streamID)
	return nil
}

// BidirectionalStreamInterface defines interface for bidirectional streaming
type BidirectionalStreamInterface[TReq, TResp any] interface {
	Send(*TResp) error
	Recv() (*TReq, error)
	Context() context.Context
}

// BidirectionalStream manages bidirectional streaming with enhanced features
type BidirectionalStream[TReq, TResp any] struct {
	stream   BidirectionalStreamInterface[TReq, TResp]
	streamID string
	manager  *StreamManager
	config   StreamConfig
	sendCh   chan *TResp
	recvCh   chan *TReq
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewBidirectionalStream creates a new enhanced bidirectional stream
func NewBidirectionalStream[TReq, TResp any](
	stream BidirectionalStreamInterface[TReq, TResp],
	streamID string,
	manager *StreamManager,
	config StreamConfig,
) *BidirectionalStream[TReq, TResp] {
	ctx, cancel := context.WithCancel(stream.Context())

	enhanced := &BidirectionalStream[TReq, TResp]{
		stream:   stream,
		streamID: streamID,
		manager:  manager,
		config:   config,
		sendCh:   make(chan *TResp, config.BufferSize),
		recvCh:   make(chan *TReq, config.BufferSize),
		done:     make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Register with manager
	enhanced.manager.RegisterStream(streamID, StreamTypeBidirectional)

	return enhanced
}

// Send sends a message through the bidirectional stream
func (bs *BidirectionalStream[TReq, TResp]) Send(msg *TResp) error {
	select {
	case <-bs.done:
		return status.Error(codes.Canceled, "stream closed")
	case <-bs.ctx.Done():
		return bs.ctx.Err()
	default:
	}

	// Calculate message size
	bytes := int64(calculateMessageSize(msg))

	// Send the message
	if err := bs.stream.Send(msg); err != nil {
		bs.manager.IncrementError(bs.streamID)
		return err
	}

	// Update metrics
	bs.manager.IncrementSent(bs.streamID, bytes)

	if bs.config.EnableMetrics {
		if metrics := GetGlobalMetrics(); metrics != nil {
			metrics.IncrementCounter("cargo_stream_messages_sent_total", map[string]string{
				"stream_id": bs.streamID,
				"type":      "bidirectional",
			})
		}
	}

	return nil
}

// Recv receives a message from the bidirectional stream
func (bs *BidirectionalStream[TReq, TResp]) Recv() (*TReq, error) {
	msg, err := bs.stream.Recv()
	if err != nil {
		if err == io.EOF {
			bs.Close()
			return nil, err
		}
		bs.manager.IncrementError(bs.streamID)
		return nil, err
	}

	// Calculate message size
	bytes := int64(calculateMessageSize(msg))

	// Update metrics
	bs.manager.IncrementReceived(bs.streamID, bytes)

	if bs.config.EnableMetrics {
		if metrics := GetGlobalMetrics(); metrics != nil {
			metrics.IncrementCounter("cargo_stream_messages_received_total", map[string]string{
				"stream_id": bs.streamID,
				"type":      "bidirectional",
			})
		}
	}

	return msg, nil
}

// Close closes the bidirectional stream
func (bs *BidirectionalStream[TReq, TResp]) Close() error {
	close(bs.done)
	bs.cancel()
	bs.manager.UnregisterStream(bs.streamID)
	return nil
}

// StreamProcessor provides utilities for processing streams
type StreamProcessor[T any] struct {
	config StreamConfig
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor[T any](config StreamConfig) *StreamProcessor[T] {
	return &StreamProcessor[T]{config: config}
}

// ProcessBatch processes items in batches for better performance
func (sp *StreamProcessor[T]) ProcessBatch(
	ctx context.Context,
	stream EnhancedServerStreamInterface[T],
	items []*T,
	processor func(*T) *T,
) error {
	batchSize := sp.config.MaxConcurrentItems
	if batchSize <= 0 {
		batchSize = 100
	}

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]

		// Process batch concurrently
		var wg sync.WaitGroup
		results := make([]*T, len(batch))

		for j, item := range batch {
			wg.Add(1)
			go func(index int, item *T) {
				defer wg.Done()
				results[index] = processor(item)
			}(j, item)
		}

		wg.Wait()

		// Send batch results
		for _, result := range results {
			if err := stream.Send(result); err != nil {
				return err
			}
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// ProcessStream processes items as they arrive
func (sp *StreamProcessor[T]) ProcessStream(
	ctx context.Context,
	stream EnhancedClientStreamInterface[T],
	processor func(*T) error,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := processor(msg); err != nil {
			return err
		}
	}

	return nil
}

// Stream helpers and utilities

// generateStreamID generates a unique stream ID
func generateStreamID() string {
	return fmt.Sprintf("stream-%d", time.Now().UnixNano())
}

// calculateMessageSize calculates approximate message size
func calculateMessageSize(msg interface{}) int {
	// Simple size calculation - in production, use protobuf size methods
	if data, err := MarshalJSON(msg); err == nil {
		return len(data)
	}
	return 1024 // Default estimate
}

// StreamInterceptor creates an interceptor for streaming monitoring
func StreamInterceptor(manager *StreamManager) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		streamID := generateStreamID()

		var streamType StreamType
		if info.IsClientStream && info.IsServerStream {
			streamType = StreamTypeBidirectional
		} else if info.IsServerStream {
			streamType = StreamTypeServerStreaming
		} else {
			streamType = StreamTypeClientStreaming
		}

		// Register stream
		stats := manager.RegisterStream(streamID, streamType)
		defer manager.UnregisterStream(streamID)

		// Wrap the stream with monitoring
		wrappedStream := &MonitoredServerStream{
			ServerStream: ss,
			streamID:     streamID,
			manager:      manager,
		}

		start := time.Now()
		err := handler(srv, wrappedStream)
		duration := time.Since(start)

		// Update final stats
		stats.Duration = duration
		if err != nil {
			manager.IncrementError(streamID)
		}

		// Record metrics
		if metrics := GetGlobalMetrics(); metrics != nil {
			status := "success"
			if err != nil {
				status = "error"
			}

			metrics.RecordDuration("cargo_stream_total_duration_ms", map[string]string{
				"stream_id": streamID,
				"status":    status,
			}, duration)
		}

		return err
	}
}

// MonitoredServerStream wraps a server stream for monitoring
type MonitoredServerStream struct {
	grpc.ServerStream
	streamID string
	manager  *StreamManager
}

// SendMsg wraps the send method for monitoring
func (mss *MonitoredServerStream) SendMsg(m interface{}) error {
	err := mss.ServerStream.SendMsg(m)
	if err != nil {
		mss.manager.IncrementError(mss.streamID)
	} else {
		bytes := int64(calculateMessageSize(m))
		mss.manager.IncrementSent(mss.streamID, bytes)
	}
	return err
}

// RecvMsg wraps the receive method for monitoring
func (mss *MonitoredServerStream) RecvMsg(m interface{}) error {
	err := mss.ServerStream.RecvMsg(m)
	if err != nil && err != io.EOF {
		mss.manager.IncrementError(mss.streamID)
	} else if err == nil {
		bytes := int64(calculateMessageSize(m))
		mss.manager.IncrementReceived(mss.streamID, bytes)
	}
	return err
}

// Global stream manager
var globalStreamManager *StreamManager

func init() {
	globalStreamManager = NewStreamManager(StreamConfig{
		BufferSize:          100,
		MaxConcurrentItems:  1000,
		BackpressureTimeout: 30 * time.Second,
		HeartbeatInterval:   30 * time.Second,
		MaxStreamDuration:   time.Hour,
		EnableMetrics:       true,
		EnableBackpressure:  true,
	})
}

// SetGlobalStreamManager sets the global stream manager
func SetGlobalStreamManager(manager *StreamManager) {
	globalStreamManager = manager
}

// GetGlobalStreamManager returns the global stream manager
func GetGlobalStreamManager() *StreamManager {
	return globalStreamManager
}

// Convenience functions for global stream manager
func GetStreamStats(streamID string) (*StreamStats, bool) {
	return globalStreamManager.GetStreamStats(streamID)
}

func GetAllStreamStats() []*StreamStats {
	return globalStreamManager.GetAllStreamStats()
}
