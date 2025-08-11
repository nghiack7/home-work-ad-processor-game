package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Environment string           `mapstructure:"environment"`
	LogLevel    string           `mapstructure:"log_level"`
	Server      ServerConfig     `mapstructure:"server"`
	Database    DatabaseConfig   `mapstructure:"database"`
	Redis       RedisConfig      `mapstructure:"redis"`
	Queue       QueueConfig      `mapstructure:"queue"`
	AIAgent     AIAgentConfig    `mapstructure:"ai_agent"`
	Kafka       KafkaConfig      `mapstructure:"kafka"`
	Monitoring  MonitoringConfig `mapstructure:"monitoring"`
	Security    SecurityConfig   `mapstructure:"security"`
	Health      HealthConfig     `mapstructure:"health"`
	TLS         TLSConfig        `mapstructure:"tls"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port                int `mapstructure:"port"`
	ReadTimeoutSeconds  int `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int `mapstructure:"write_timeout_seconds"`
	IdleTimeoutSeconds  int `mapstructure:"idle_timeout_seconds"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host                   string `mapstructure:"host"`
	Port                   int    `mapstructure:"port"`
	User                   string `mapstructure:"user"`
	Password               string `mapstructure:"password"`
	Name                   string `mapstructure:"name"`
	SSLMode                string `mapstructure:"ssl_mode"`
	MaxOpenConns           int    `mapstructure:"max_open_conns"`
	MaxIdleConns           int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `mapstructure:"conn_max_lifetime_minutes"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	Password            string `mapstructure:"password"`
	DB                  int    `mapstructure:"db"`
	PoolSize            int    `mapstructure:"pool_size"`
	ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
	IdleTimeoutSeconds  int    `mapstructure:"idle_timeout_seconds"`
}

// QueueConfig holds queue configuration
type QueueConfig struct {
	AntiStarvationEnabled    bool `mapstructure:"anti_starvation_enabled"`
	MaxWaitTimeSeconds       int  `mapstructure:"max_wait_time_seconds"`
	WorkerCount              int  `mapstructure:"worker_count"`
	BatchSize                int  `mapstructure:"batch_size"`
	ProcessingTimeoutSeconds int  `mapstructure:"processing_timeout_seconds"`
	ShardCount               int  `mapstructure:"shard_count"`
}

// AIAgentConfig holds AI agent configuration
type AIAgentConfig struct {
	GoogleADK       GoogleADKConfig `mapstructure:"google_adk"`
	CacheEnabled    bool            `mapstructure:"cache_enabled"`
	CacheTTLSeconds int             `mapstructure:"cache_ttl_seconds"`
}

// GoogleADKConfig holds Google ADK specific configuration
type GoogleADKConfig struct {
	APIKey          string `mapstructure:"api_key"`
	Endpoint        string `mapstructure:"endpoint"`
	TimeoutSeconds  int    `mapstructure:"timeout_seconds"`
	RetryAttempts   int    `mapstructure:"retry_attempts"`
	FallbackEnabled bool   `mapstructure:"fallback_enabled"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing"`
	Logging LoggingConfig `mapstructure:"logging"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

type TracingConfig struct {
	Enabled        bool    `mapstructure:"enabled"`
	JaegerEndpoint string  `mapstructure:"jaeger_endpoint"`
	SampleRate     float64 `mapstructure:"sample_rate"`
}

type LoggingConfig struct {
	Structured bool   `mapstructure:"structured"`
	Format     string `mapstructure:"format"`
	Level      string `mapstructure:"level"`
	Output     string `mapstructure:"output"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	JWT          JWTConfig          `mapstructure:"jwt"`
	RateLimiting RateLimitingConfig `mapstructure:"rate_limiting"`
	CORS         CORSConfig         `mapstructure:"cors"`
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	Issuer      string `mapstructure:"issuer"`
	Audience    string `mapstructure:"audience"`
	ExpiryHours int    `mapstructure:"expiry_hours"`
}

type RateLimitingConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerMinute int  `mapstructure:"requests_per_minute"`
	Burst             int  `mapstructure:"burst"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
	MaxAgeSeconds  int      `mapstructure:"max_age_seconds"`
}

// HealthConfig holds health check configuration
type HealthConfig struct {
	CheckIntervalSeconds int              `mapstructure:"check_interval_seconds"`
	TimeoutSeconds       int              `mapstructure:"timeout_seconds"`
	Endpoints            []HealthEndpoint `mapstructure:"endpoints"`
}

type HealthEndpoint struct {
	Name    string   `mapstructure:"name"`
	URL     string   `mapstructure:"url"`
	Brokers []string `mapstructure:"brokers"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CAFile   string `mapstructure:"ca_file"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Brokers            []string `mapstructure:"brokers"`
	ProducerBatchSize  int      `mapstructure:"producer_batch_size"`
	ProducerFlushMS    int      `mapstructure:"producer_flush_ms"`
	ConsumerGroupID    string   `mapstructure:"consumer_group_id"`
	ConsumerAutoOffset string   `mapstructure:"consumer_auto_offset"`
}

// Load loads configuration from file and environment variables
func Load() (*Config, error) {
	// Determine config file name based on environment
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	if env == "" {
		env = "development"
	}

	// Set config file path
	configName := "config"
	if env == "production" {
		configName = "production"
	}

	viper.SetConfigName(configName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("../../configs")
	viper.AddConfigPath("/app/configs")

	// Read config file (it's okay if it doesn't exist)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Process the config with environment variable substitution
	processedConfig := make(map[string]interface{})
	if err := viper.Unmarshal(&processedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw config: %w", err)
	}

	// Process {ENV-default} patterns recursively
	processEnvPatterns(processedConfig)

	// Create a new viper instance with processed config
	processedViper := viper.New()
	for key, value := range processedConfig {
		processedViper.Set(key, value)
	}

	var config Config
	if err := processedViper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal processed config: %w", err)
	}

	return &config, nil
}

// processEnvPatterns processes {ENV-default} patterns recursively
func processEnvPatterns(config map[string]interface{}) {
	for key, value := range config {
		config[key] = processValue(value)
	}
}

// processValue processes a single value for environment variable substitution
func processValue(value interface{}) interface{} {
	envPattern := regexp.MustCompile(`\{([A-Z_]+)-([^}]*)\}`)

	switch v := value.(type) {
	case string:
		if matches := envPattern.FindStringSubmatch(v); len(matches) == 3 {
			envVar := matches[1]
			defaultValue := matches[2]

			// Get environment variable value or use default
			if envValue := os.Getenv(envVar); envValue != "" {
				return convertValue(envValue, defaultValue)
			} else {
				return convertValue(defaultValue, defaultValue)
			}
		}
		return v
	case map[string]interface{}:
		processEnvPatterns(v)
		return v
	case []interface{}:
		for i, item := range v {
			v[i] = processValue(item)
		}
		return v
	default:
		return v
	}
}

// convertValue converts string values to appropriate types
func convertValue(value, defaultValue string) interface{} {
	// Special case: if default is empty, always return the value as string (even if empty)
	if defaultValue == "" {
		return value
	}

	// If value is empty and default is not empty, try to convert default
	if value == "" {
		return convertToType(defaultValue)
	}

	// Convert the actual value
	return convertToType(value)
}

// convertToType converts a string to the most appropriate type
func convertToType(value string) interface{} {
	// If value is empty, return empty string
	if value == "" {
		return ""
	}

	// Try boolean conversion first
	if strings.ToLower(value) == "true" {
		return true
	}
	if strings.ToLower(value) == "false" {
		return false
	}

	// Try integer conversion
	if intVal, err := strconv.Atoi(value); err == nil {
		return intVal
	}

	// Try float conversion
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Return as string if no conversion is possible
	return value
}
