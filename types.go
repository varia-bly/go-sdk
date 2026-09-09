package variably

import (
	"encoding/json"
	"time"
)

// UserContext represents user information for configuration evaluation
type UserContext struct {
	UserID     string                 `json:"user_id"`
	Email      string                 `json:"email,omitempty"`
	Country    string                 `json:"country,omitempty"`
	Language   string                 `json:"language,omitempty"`
	Platform   string                 `json:"platform,omitempty"`
	Version    string                 `json:"version,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
}

// DynamicConfigResult represents the result of a configuration evaluation
type DynamicConfigResult struct {
	Key             string      `json:"key"`
	Value           interface{} `json:"value"`
	Reason          string      `json:"reason"`
	RuleID          *string     `json:"rule_id,omitempty"`
	ETag            string      `json:"etag,omitempty"`
	Version         *int64      `json:"version,omitempty"`
	UpdatedAt       *time.Time  `json:"updated_at,omitempty"`
	CacheHit        bool        `json:"cache_hit"`
	RealTimeUpdate  bool        `json:"real_time_update"`
	RetrievedAt     time.Time   `json:"retrieved_at"`
	Error           error       `json:"error,omitempty"`
}

// DynamicConfigEvaluationRequest represents a request to evaluate a configuration
type DynamicConfigEvaluationRequest struct {
	ConfigKey string      `json:"config_key"`
	Context   UserContext `json:"context"`
}

// DynamicConfigEvaluationResponse represents the response from configuration evaluation
type DynamicConfigEvaluationResponse struct {
	ConfigKey string          `json:"config_key"`
	Value     json.RawMessage `json:"value"`
	Reason    string          `json:"reason"`
	RuleID    *string         `json:"rule_id,omitempty"`
	ETag      string          `json:"etag,omitempty"`
	Version   *int64          `json:"version,omitempty"`
	UpdatedAt *string         `json:"updated_at,omitempty"`
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Channel   string      `json:"channel,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// SubscriptionRequest represents a subscription request
type SubscriptionRequest struct {
	ProjectID string   `json:"project_id"`
	ConfigKey *string  `json:"config_key,omitempty"`
	Types     []string `json:"types,omitempty"`
}

// ConfigUpdateEvent represents a real-time configuration update
type ConfigUpdateEvent struct {
	ConfigKey  string          `json:"config_key"`
	ProjectID  string          `json:"project_id"`
	NewValue   json.RawMessage `json:"new_value"`
	Enabled    bool            `json:"enabled"`
	ETag       string          `json:"etag"`
	Version    int64           `json:"version"`
	Timestamp  string          `json:"timestamp"`
	UpdateType string          `json:"update_type"` // "update", "toggle", "delete"
}

// ConnectionState represents the state of a WebSocket connection
type ConnectionState string

const (
	Connecting   ConnectionState = "connecting"
	Connected    ConnectionState = "connected"
	Disconnected ConnectionState = "disconnected"
	Error        ConnectionState = "error"
)

// ConnectionStatus provides information about the current connection
type ConnectionStatus struct {
	Mode           string `json:"mode"`             // "realtime" | "polling" | "offline"
	Connected      bool   `json:"connected"`       // WebSocket connection status
	FallbackActive bool   `json:"fallback_active"` // Whether polling fallback is active
}

// CacheConfig represents cache configuration options
type CacheConfig struct {
	TTL     time.Duration `json:"ttl"`      // Cache TTL (default: 5 minutes)
	MaxSize int           `json:"max_size"` // Maximum number of cached entries (default: 1000)
	Enabled bool          `json:"enabled"`  // Enable caching (default: true)
}

// ConfigChangeCallback is called when a configuration changes
type ConfigChangeCallback func(result *DynamicConfigResult)

// ConnectionStateCallback is called when connection state changes
type ConnectionStateCallback func(state ConnectionState)

// ErrorCallback is called when an error occurs
type ErrorCallback func(err error)

// ClientConfig represents the configuration for the dynamic config client
type ClientConfig struct {
	APIKey         string        `json:"api_key"`
	JWTToken       string        `json:"jwt_token,omitempty"`
	BaseURL        string        `json:"base_url"`
	ProjectID      string        `json:"project_id"`
	EnableRealtime bool          `json:"enable_realtime"`
	PollingInterval time.Duration `json:"polling_interval"`
	Cache          CacheConfig   `json:"cache"`
	WebSocket      WSConfig      `json:"websocket"`
	Debug          bool          `json:"debug"`
}

// WSConfig represents WebSocket configuration options
type WSConfig struct {
	ReconnectInterval     time.Duration `json:"reconnect_interval"`
	MaxReconnectAttempts  int           `json:"max_reconnect_attempts"`
	ConnectionTimeout     time.Duration `json:"connection_timeout"`
	AutoReconnect         bool          `json:"auto_reconnect"`
}

// DefaultClientConfig returns a client configuration with sensible defaults
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		// BaseURL is deliberately empty: see NewVariablyClient. Callers must set it.
		BaseURL:         "",
		EnableRealtime:  true,
		PollingInterval: 30 * time.Second,
		Cache: CacheConfig{
			TTL:     5 * time.Minute,
			MaxSize: 1000,
			Enabled: true,
		},
		WebSocket: WSConfig{
			ReconnectInterval:    5 * time.Second,
			MaxReconnectAttempts: 10,
			ConnectionTimeout:    10 * time.Second,
			AutoReconnect:        true,
		},
		Debug: false,
	}
}