package variably

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for the Variably client
type Config struct {
	// Authentication
	APIKey      string `json:"api_key" yaml:"api_key"`
	BaseURL     string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`

	// Performance
	Timeout        time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	RetryAttempts  int           `json:"retry_attempts,omitempty" yaml:"retry_attempts,omitempty"`
	MaxCacheSize   int           `json:"max_cache_size,omitempty" yaml:"max_cache_size,omitempty"`

	// Features
	EnableAnalytics    bool `json:"enable_analytics" yaml:"enable_analytics"`
	EnableOfflineMode  bool `json:"enable_offline_mode" yaml:"enable_offline_mode"`
	EnableRealTimeSync bool `json:"enable_real_time_sync" yaml:"enable_real_time_sync"`

	// Advanced Configuration
	CacheConfig   CacheConfig   `json:"cache_config,omitempty" yaml:"cache_config,omitempty"`
	PollingConfig PollingConfig `json:"polling_config,omitempty" yaml:"polling_config,omitempty"`
	LogConfig     LogConfig     `json:"log_config,omitempty" yaml:"log_config,omitempty"`

	// Custom Logger
	Logger Logger `json:"-" yaml:"-"`
}

// CacheConfig configures the caching behavior
type CacheConfig struct {
	TTL               time.Duration `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	MaxSize           int           `json:"max_size,omitempty" yaml:"max_size,omitempty"`
	EnablePersistence bool          `json:"enable_persistence" yaml:"enable_persistence"`
	PersistencePath   string        `json:"persistence_path,omitempty" yaml:"persistence_path,omitempty"`
	EvictionPolicy    string        `json:"eviction_policy,omitempty" yaml:"eviction_policy,omitempty"`
}

// PollingConfig configures real-time updates
type PollingConfig struct {
	Enabled  bool          `json:"enabled" yaml:"enabled"`
	Interval time.Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Jitter   time.Duration `json:"jitter,omitempty" yaml:"jitter,omitempty"`
}

// LogConfig configures logging behavior
type LogConfig struct {
	Level  string `json:"level,omitempty" yaml:"level,omitempty"`
	Format string `json:"format,omitempty" yaml:"format,omitempty"`
	Output string `json:"output,omitempty" yaml:"output,omitempty"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		BaseURL:     "http://localhost:8080",
		Environment: "development",
		Timeout:     5 * time.Second,
		RetryAttempts: 3,
		MaxCacheSize: 1000,

		EnableAnalytics:    true,
		EnableOfflineMode:  true,
		EnableRealTimeSync: false,

		CacheConfig: CacheConfig{
			TTL:               5 * time.Minute,
			MaxSize:           1000,
			EnablePersistence: false,
			EvictionPolicy:    "LRU",
		},

		PollingConfig: PollingConfig{
			Enabled:  false,
			Interval: 30 * time.Second,
			Jitter:   5 * time.Second,
		},

		LogConfig: LogConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
	}
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	config := DefaultConfig()

	if apiKey := os.Getenv("VARIABLY_API_KEY"); apiKey != "" {
		config.APIKey = apiKey
	}

	if baseURL := os.Getenv("VARIABLY_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}

	if env := os.Getenv("VARIABLY_ENVIRONMENT"); env != "" {
		config.Environment = env
	}

	if timeout := os.Getenv("VARIABLY_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.Timeout = d
		}
	}

	if cacheTTL := os.Getenv("VARIABLY_CACHE_TTL"); cacheTTL != "" {
		if d, err := time.ParseDuration(cacheTTL); err == nil {
			config.CacheConfig.TTL = d
		}
	}

	if logLevel := os.Getenv("VARIABLY_LOG_LEVEL"); logLevel != "" {
		config.LogConfig.Level = logLevel
	}

	return config
}

// LoadConfigFromFile loads configuration from a YAML file
func LoadConfigFromFile(filename string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	if c.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}

	if c.Environment == "" {
		return fmt.Errorf("environment is required")
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}

	if c.RetryAttempts < 0 {
		return fmt.Errorf("retry attempts must be non-negative")
	}

	if c.CacheConfig.TTL <= 0 {
		c.CacheConfig.TTL = 5 * time.Minute
	}

	if c.CacheConfig.MaxSize <= 0 {
		c.CacheConfig.MaxSize = 1000
	}

	validEvictionPolicies := map[string]bool{
		"LRU": true,
		"LFU": true,
		"TTL": true,
	}
	if !validEvictionPolicies[c.CacheConfig.EvictionPolicy] {
		c.CacheConfig.EvictionPolicy = "LRU"
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogConfig.Level] {
		c.LogConfig.Level = "info"
	}

	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[c.LogConfig.Format] {
		c.LogConfig.Format = "text"
	}

	return nil
}

// Copy creates a deep copy of the configuration
func (c *Config) Copy() *Config {
	copy := *c
	return &copy
}