package cargo

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Cargo application
type Config struct {
	Server    ServerConfig   `json:"server" yaml:"server"`
	Database  DatabaseConfig `json:"database" yaml:"database"`
	Auth      AuthConfig     `json:"auth" yaml:"auth"`
	TLS       TLSConfig      `json:"tls" yaml:"tls"`
	Logging   LoggingConfig  `json:"logging" yaml:"logging"`
	Cache     CacheConfig    `json:"cache" yaml:"cache"`
	Streaming StreamConfig   `json:"streaming" yaml:"streaming"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port            string        `json:"port" yaml:"port"`
	Host            string        `json:"host" yaml:"host"`
	ReadTimeout     time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout" yaml:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`
	MaxRecvMsgSize  int           `json:"max_recv_msg_size" yaml:"max_recv_msg_size"`
	MaxSendMsgSize  int           `json:"max_send_msg_size" yaml:"max_send_msg_size"`
}

// DatabaseConfig holds database-related configuration
type DatabaseConfig struct {
	MongoDB MongoConfig `json:"mongodb" yaml:"mongodb"`
}

// MongoConfig holds MongoDB-specific configuration
type MongoConfig struct {
	URI                    string        `json:"uri" yaml:"uri"`
	Database               string        `json:"database" yaml:"database"`
	Timeout                time.Duration `json:"timeout" yaml:"timeout"`
	MaxPoolSize            uint64        `json:"max_pool_size" yaml:"max_pool_size"`
	MinPoolSize            uint64        `json:"min_pool_size" yaml:"min_pool_size"`
	MaxConnIdleTime        time.Duration `json:"max_conn_idle_time" yaml:"max_conn_idle_time"`
	ConnectTimeout         time.Duration `json:"connect_timeout" yaml:"connect_timeout"`
	ServerSelectionTimeout time.Duration `json:"server_selection_timeout" yaml:"server_selection_timeout"`
	RetryWrites            bool          `json:"retry_writes" yaml:"retry_writes"`
	RetryReads             bool          `json:"retry_reads" yaml:"retry_reads"`
}

// AuthConfig holds authentication-related configuration
type AuthConfig struct {
	JWT JWTConfig `json:"jwt" yaml:"jwt"`
}

// JWTConfig holds JWT-specific configuration
type JWTConfig struct {
	Secret          string        `json:"secret" yaml:"secret"`
	Issuer          string        `json:"issuer" yaml:"issuer"`
	Audience        string        `json:"audience" yaml:"audience"`
	ExpiryDuration  time.Duration `json:"expiry_duration" yaml:"expiry_duration"`
	RefreshEnabled  bool          `json:"refresh_enabled" yaml:"refresh_enabled"`
	RefreshDuration time.Duration `json:"refresh_duration" yaml:"refresh_duration"`
}

// LoggingConfig holds logging-related configuration
type LoggingConfig struct {
	Level      string `json:"level" yaml:"level"`
	Format     string `json:"format" yaml:"format"`
	Output     string `json:"output" yaml:"output"`
	Structured bool   `json:"structured" yaml:"structured"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnv("CARGO_PORT", ":50051"),
			Host:            getEnv("CARGO_HOST", ""),
			ReadTimeout:     getDurationEnv("CARGO_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnv("CARGO_WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: getDurationEnv("CARGO_SHUTDOWN_TIMEOUT", 10*time.Second),
			MaxRecvMsgSize:  getIntEnv("CARGO_MAX_RECV_MSG_SIZE", 4*1024*1024), // 4MB
			MaxSendMsgSize:  getIntEnv("CARGO_MAX_SEND_MSG_SIZE", 4*1024*1024), // 4MB
		},
		Database: DatabaseConfig{
			MongoDB: MongoConfig{
				URI:                    getEnv("MONGODB_URI", "mongodb://localhost:27017"),
				Database:               getEnv("MONGODB_DATABASE", "cargo"),
				Timeout:                getDurationEnv("MONGODB_TIMEOUT", 10*time.Second),
				MaxPoolSize:            getUint64Env("MONGODB_MAX_POOL_SIZE", 100),
				MinPoolSize:            getUint64Env("MONGODB_MIN_POOL_SIZE", 5),
				MaxConnIdleTime:        getDurationEnv("MONGODB_MAX_CONN_IDLE_TIME", 5*time.Minute),
				ConnectTimeout:         getDurationEnv("MONGODB_CONNECT_TIMEOUT", 10*time.Second),
				ServerSelectionTimeout: getDurationEnv("MONGODB_SERVER_SELECTION_TIMEOUT", 30*time.Second),
				RetryWrites:            getBoolEnv("MONGODB_RETRY_WRITES", true),
				RetryReads:             getBoolEnv("MONGODB_RETRY_READS", true),
			},
		},
		Auth: AuthConfig{
			JWT: JWTConfig{
				Secret:          getEnv("JWT_SECRET", "cargo-default-secret-change-in-production"),
				Issuer:          getEnv("JWT_ISSUER", "cargo"),
				Audience:        getEnv("JWT_AUDIENCE", "cargo-users"),
				ExpiryDuration:  getDurationEnv("JWT_EXPIRY_DURATION", 24*time.Hour),
				RefreshEnabled:  getBoolEnv("JWT_REFRESH_ENABLED", false),
				RefreshDuration: getDurationEnv("JWT_REFRESH_DURATION", 7*24*time.Hour),
			},
		},
		TLS: TLSConfig{
			CertFile: getEnv("TLS_CERT_FILE", ""),
			KeyFile:  getEnv("TLS_KEY_FILE", ""),
			ClientCA: getEnv("TLS_CLIENT_CA", ""),
		},
		Logging: LoggingConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "text"),
			Output:     getEnv("LOG_OUTPUT", "stdout"),
			Structured: getBoolEnv("LOG_STRUCTURED", false),
		},
		Cache: CacheConfig{
			Backend:        getEnv("CACHE_BACKEND", "memory"),
			DefaultTTL:     getDurationEnv("CACHE_DEFAULT_TTL", 1*time.Hour),
			MaxSize:        getInt64Env("CACHE_MAX_SIZE", 10000),
			MaxMemory:      getInt64Env("CACHE_MAX_MEMORY", 100*1024*1024), // 100MB
			EvictionPolicy: getEnv("CACHE_EVICTION_POLICY", "lru"),
			RedisAddr:      getEnv("CACHE_REDIS_ADDR", "localhost:6379"),
			RedisPassword:  getEnv("CACHE_REDIS_PASSWORD", ""),
			RedisDB:        getIntEnv("CACHE_REDIS_DB", 0),
			Prefix:         getEnv("CACHE_PREFIX", "cargo:"),
			Serialization:  getEnv("CACHE_SERIALIZATION", "json"),
		},
		Streaming: StreamConfig{
			BufferSize:          getIntEnv("STREAMING_BUFFER_SIZE", 100),
			MaxConcurrentItems:  getIntEnv("STREAMING_MAX_CONCURRENT_ITEMS", 1000),
			BackpressureTimeout: getDurationEnv("STREAMING_BACKPRESSURE_TIMEOUT", 30*time.Second),
			HeartbeatInterval:   getDurationEnv("STREAMING_HEARTBEAT_INTERVAL", 30*time.Second),
			MaxStreamDuration:   getDurationEnv("STREAMING_MAX_DURATION", 1*time.Hour),
			EnableMetrics:       getBoolEnv("STREAMING_ENABLE_METRICS", true),
			EnableBackpressure:  getBoolEnv("STREAMING_ENABLE_BACKPRESSURE", true),
		},
	}
}

// Helper functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getUint64Env(key string, defaultValue uint64) uint64 {
	if value := os.Getenv(key); value != "" {
		if uint64Value, err := strconv.ParseUint(value, 10, 64); err == nil {
			return uint64Value
		}
	}
	return defaultValue
}

func getInt64Env(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if int64Value, err := strconv.ParseInt(value, 10, 64); err == nil {
			return int64Value
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port == "" {
		return NewValidationError("server port cannot be empty")
	}

	// Database validation
	if c.Database.MongoDB.URI == "" {
		return NewValidationError("mongodb uri cannot be empty")
	}
	if c.Database.MongoDB.Database == "" {
		return NewValidationError("mongodb database name cannot be empty")
	}

	// Auth validation
	if c.Auth.JWT.Secret == "" || c.Auth.JWT.Secret == "cargo-default-secret-change-in-production" {
		return NewValidationError("jwt secret must be set and not use default value")
	}

	return nil
}
