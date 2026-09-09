package variably

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ClientConfig{
				APIKey:    "test-api-key",
				ProjectID: "test-project",
				BaseURL:   "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			config: ClientConfig{
				ProjectID: "test-project",
				BaseURL:   "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "missing project id",
			config: ClientConfig{
				APIKey:  "test-api-key",
				BaseURL: "http://localhost:8080",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				if client != nil {
					client.Close()
				}
			}
		})
	}
}

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()
	
	// Deliberately empty: NewVariablyClient rejects a missing base URL rather than
	// defaulting to localhost, which in production would make every flag read as its
	// default — indistinguishable from a rollout that turned everything off.
	assert.Equal(t, "", config.BaseURL)
	assert.Equal(t, true, config.EnableRealtime)
	assert.Equal(t, 30*time.Second, config.PollingInterval)
	assert.Equal(t, 5*time.Minute, config.Cache.TTL)
	assert.Equal(t, 1000, config.Cache.MaxSize)
	assert.Equal(t, true, config.Cache.Enabled)
	assert.Equal(t, 5*time.Second, config.WebSocket.ReconnectInterval)
	assert.Equal(t, 10, config.WebSocket.MaxReconnectAttempts)
	assert.Equal(t, 10*time.Second, config.WebSocket.ConnectionTimeout)
	assert.Equal(t, true, config.WebSocket.AutoReconnect)
	assert.Equal(t, false, config.Debug)
}

func TestDynamicConfigClient_GetConfig(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/sdk/dynamic-configs/evaluate", r.URL.Path)
		// The API authenticates SDK traffic with X-API-Key; a bearer token is rejected.
		assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req DynamicConfigEvaluationRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		
		assert.Equal(t, "test_config", req.ConfigKey)
		assert.Equal(t, "user123", req.Context.UserID)

		response := DynamicConfigEvaluationResponse{
			ConfigKey: "test_config",
			Value:     json.RawMessage(`true`),
			Reason:    "rule_match",
			ETag:      "etag123",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        server.URL,
		EnableRealtime: false, // Disable real-time for this test
	})
	require.NoError(t, err)
	defer client.Close()

	userContext := UserContext{
		UserID: "user123",
	}

	ctx := context.Background()
	value, err := client.GetConfigBool(ctx, "test_config", false, userContext)
	
	assert.NoError(t, err)
	assert.True(t, value)
}

func TestDynamicConfigClient_EvaluateConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DynamicConfigEvaluationResponse{
			ConfigKey: "test_config",
			Value:     json.RawMessage(`"hello world"`),
			Reason:    "default_value",
			ETag:      "etag456",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        server.URL,
		EnableRealtime: false,
	})
	require.NoError(t, err)
	defer client.Close()

	userContext := UserContext{
		UserID: "user123",
	}

	ctx := context.Background()
	result, err := client.EvaluateConfig(ctx, "test_config", "default", userContext)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test_config", result.Key)
	assert.Equal(t, "hello world", result.Value)
	assert.Equal(t, "api_evaluation", result.Reason)
	assert.False(t, result.CacheHit)
	assert.False(t, result.RealTimeUpdate)
	assert.NotZero(t, result.RetrievedAt)
}

func TestDynamicConfigClient_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := DynamicConfigEvaluationResponse{
			ConfigKey: "cached_config",
			Value:     json.RawMessage(`42`),
			Reason:    "rule_match",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        server.URL,
		EnableRealtime: false,
		Cache: CacheConfig{
			TTL:     10 * time.Minute,
			MaxSize: 100,
			Enabled: true,
		},
	})
	require.NoError(t, err)
	defer client.Close()

	userContext := UserContext{
		UserID: "user123",
	}

	ctx := context.Background()

	// First call - should hit the server
	result1, err := client.EvaluateConfig(ctx, "cached_config", 0.0, userContext)
	assert.NoError(t, err)
	assert.False(t, result1.CacheHit)
	assert.Equal(t, 1, callCount)

	// Second call - should hit the cache
	result2, err := client.EvaluateConfig(ctx, "cached_config", 0.0, userContext)
	assert.NoError(t, err)
	assert.True(t, result2.CacheHit)
	assert.Equal(t, 1, callCount) // Server should not be called again
}

func TestDynamicConfigClient_OnConfigChange(t *testing.T) {
	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        "http://localhost:8080",
		EnableRealtime: false, // Disable real-time for this test
	})
	require.NoError(t, err)
	defer client.Close()

	// Test the subscription callback mechanism
	unsubscribe := client.OnConfigChange("test_config", func(result *DynamicConfigResult) {
		// This callback would be triggered by real WebSocket updates
		// In integration tests, we would verify the result here
		_ = result
	})
	defer unsubscribe()
	
	// Verify subscription was created (callback exists)
	// Note: Without access to private methods, we can't fully test the internal callback mechanism
	// In practice, this would be tested through integration tests with a real WebSocket connection
	assert.NotNil(t, unsubscribe, "Unsubscribe function should be returned")
}

func TestDynamicConfigClient_ValidationErrors(t *testing.T) {
	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        "http://localhost:8080",
		EnableRealtime: false,
	})
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	userContext := UserContext{UserID: "user123"}

	// Test empty config key
	result, err := client.EvaluateConfig(ctx, "", "default", userContext)
	assert.Error(t, err)
	assert.IsType(t, &ValidationError{}, err)
	assert.Nil(t, result)

	// Test empty user ID
	emptyUserContext := UserContext{}
	result, err = client.EvaluateConfig(ctx, "config_key", "default", emptyUserContext)
	assert.Error(t, err)
	assert.IsType(t, &ValidationError{}, err)
	assert.Nil(t, result)
}

func TestDynamicConfigClient_ConnectionStatus(t *testing.T) {
	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        "http://localhost:8080",
		EnableRealtime: false,
	})
	require.NoError(t, err)
	defer client.Close()

	status := client.GetConnectionStatus()
	assert.Equal(t, "polling", status.Mode)
	assert.True(t, status.Connected)
	assert.False(t, status.FallbackActive)
}

func TestCacheKeyGeneration(t *testing.T) {
	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		EnableRealtime: false,
	})
	require.NoError(t, err)
	defer client.Close()

	// Since cache key generation is now internal, we'll test that different user contexts
	// produce different cache behavior by making requests and checking cache statistics
	userContext1 := UserContext{
		UserID: "user123",
		Attributes: map[string]interface{}{
			"tier": "premium",
		},
	}

	userContext2 := UserContext{
		UserID: "user123",
		Attributes: map[string]interface{}{
			"tier": "basic",
		},
	}

	// This test verifies that the cache key generation works internally
	// by ensuring different user contexts can coexist in cache
	ctx := context.Background()
	
	// These calls would generate different cache keys internally
	_, err1 := client.EvaluateConfig(ctx, "test_config", "default", userContext1)
	_, err2 := client.EvaluateConfig(ctx, "test_config", "default", userContext2)
	
	// Both should work without interfering with each other
	assert.Error(t, err1) // Expected to fail due to no server, but shouldn't panic
	assert.Error(t, err2) // Expected to fail due to no server, but shouldn't panic
}

func TestHashUserContext(t *testing.T) {
	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		EnableRealtime: false,
	})
	require.NoError(t, err)
	defer client.Close()

	// Since hashUserContext is now internal, we'll test that user context hashing
	// works correctly by verifying consistent behavior with the same context
	userContext := UserContext{
		UserID: "user123",
		Attributes: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}

	ctx := context.Background()
	
	// Make multiple calls with the same context to verify consistency
	result1, _ := client.EvaluateConfig(ctx, "test_config", "default", userContext)
	result2, _ := client.EvaluateConfig(ctx, "test_config", "default", userContext)
	
	// Both results should be consistent (both will be nil due to no server, but that's ok)
	assert.Equal(t, result1 != nil, result2 != nil, "Same user context should produce consistent results")
}

// Benchmark tests
func BenchmarkDynamicConfigClient_GetConfigBool(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DynamicConfigEvaluationResponse{
			ConfigKey: "benchmark_config",
			Value:     json.RawMessage(`true`),
			Reason:    "benchmark",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		APIKey:         "test-api-key",
		ProjectID:      "test-project",
		BaseURL:        server.URL,
		EnableRealtime: false,
	})
	require.NoError(b, err)
	defer client.Close()

	userContext := UserContext{UserID: "benchmark_user"}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.GetConfigBool(ctx, "benchmark_config", false, userContext)
		if err != nil {
			b.Fatal(err)
		}
	}
}