package variably

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketClient provides real-time configuration updates via WebSocket
type WebSocketClient struct {
	config     WSConfig
	token      string
	baseURL    string
	logger     Logger
	conn       *websocket.Conn
	
	// Connection management
	connMutex     sync.RWMutex
	state         ConnectionState
	reconnectAttempts int
	
	// Event handling
	stateCallbacks  []ConnectionStateCallback
	configCallbacks map[string][]ConfigChangeCallback
	errorCallbacks  []ErrorCallback
	callbackMutex   sync.RWMutex
	
	// Subscriptions
	subscriptions map[string]bool
	subMutex      sync.RWMutex
	
	// Control channels
	stopCh        chan struct{}
	reconnectCh   chan struct{}
	heartbeatCh   chan struct{}
	
	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(baseURL, token string, config WSConfig, logger Logger) *WebSocketClient {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WebSocketClient{
		config:          config,
		token:           token,
		baseURL:         baseURL,
		logger:          logger,
		state:           Disconnected,
		configCallbacks: make(map[string][]ConfigChangeCallback),
		subscriptions:   make(map[string]bool),
		stopCh:          make(chan struct{}),
		reconnectCh:     make(chan struct{}, 1),
		heartbeatCh:     make(chan struct{}, 1),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Connect establishes a WebSocket connection
func (ws *WebSocketClient) Connect() error {
	ws.connMutex.Lock()
	defer ws.connMutex.Unlock()
	
	if ws.state == Connected {
		return nil
	}
	
	ws.setState(Connecting)
	ws.logger.Info("Connecting to WebSocket", map[string]interface{}{
		"url": ws.getWebSocketURL(),
	})
	
	// Set up WebSocket dialer
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = ws.config.ConnectionTimeout
	
	// Add authentication header
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", ws.token))
	
	// Connect to WebSocket
	conn, resp, err := dialer.Dial(ws.getWebSocketURL(), headers)
	if err != nil {
		ws.setState(Error)
		if resp != nil {
			return NewConnectionError(
				fmt.Sprintf("WebSocket connection failed: %v (HTTP %d)", err, resp.StatusCode),
				true,
				err,
			)
		}
		return NewConnectionError(fmt.Sprintf("WebSocket connection failed: %v", err), true, err)
	}
	
	ws.conn = conn
	ws.setState(Connected)
	ws.reconnectAttempts = 0
	
	// Start message handling goroutines
	go ws.readMessages()
	go ws.heartbeatLoop()
	
	// Start auto-reconnect if enabled
	if ws.config.AutoReconnect {
		go ws.reconnectLoop()
	}
	
	ws.logger.Info("WebSocket connected successfully", nil)
	return nil
}

// Disconnect closes the WebSocket connection
func (ws *WebSocketClient) Disconnect() {
	ws.logger.Info("Disconnecting WebSocket", nil)
	
	// Cancel context to stop all goroutines
	ws.cancel()
	
	// Close stop channel
	close(ws.stopCh)
	
	// Close WebSocket connection
	ws.connMutex.Lock()
	if ws.conn != nil {
		ws.conn.Close()
		ws.conn = nil
	}
	ws.connMutex.Unlock()
	
	ws.setState(Disconnected)
}

// IsConnected returns true if the WebSocket is connected
func (ws *WebSocketClient) IsConnected() bool {
	ws.connMutex.RLock()
	defer ws.connMutex.RUnlock()
	return ws.state == Connected && ws.conn != nil
}

// Subscribe to configuration updates for a project or specific config
func (ws *WebSocketClient) Subscribe(projectID string, configKey ...string) error {
	ws.subMutex.Lock()
	defer ws.subMutex.Unlock()
	
	var channel string
	if len(configKey) > 0 && configKey[0] != "" {
		channel = fmt.Sprintf("config_updates:%s:%s", projectID, configKey[0])
	} else {
		channel = fmt.Sprintf("config_updates:%s", projectID)
	}
	
	// Track subscription
	ws.subscriptions[channel] = true
	
	// Send subscription message if connected
	if ws.IsConnected() {
		message := WebSocketMessage{
			Type:      "subscribe",
			Channel:   channel,
			Timestamp: time.Now(),
		}
		
		if err := ws.sendMessage(message); err != nil {
			delete(ws.subscriptions, channel)
			return NewConnectionError("Failed to send subscription message", false, err)
		}
		
		ws.logger.Debug("Subscribed to channel", map[string]interface{}{
			"channel": channel,
		})
	}
	
	return nil
}

// Unsubscribe from configuration updates
func (ws *WebSocketClient) Unsubscribe(projectID string, configKey ...string) error {
	ws.subMutex.Lock()
	defer ws.subMutex.Unlock()
	
	var channel string
	if len(configKey) > 0 && configKey[0] != "" {
		channel = fmt.Sprintf("config_updates:%s:%s", projectID, configKey[0])
	} else {
		channel = fmt.Sprintf("config_updates:%s", projectID)
	}
	
	// Remove subscription tracking
	delete(ws.subscriptions, channel)
	
	// Send unsubscription message if connected
	if ws.IsConnected() {
		message := WebSocketMessage{
			Type:      "unsubscribe",
			Channel:   channel,
			Timestamp: time.Now(),
		}
		
		if err := ws.sendMessage(message); err != nil {
			return NewConnectionError("Failed to send unsubscription message", false, err)
		}
		
		ws.logger.Debug("Unsubscribed from channel", map[string]interface{}{
			"channel": channel,
		})
	}
	
	return nil
}

// OnConnectionState registers a callback for connection state changes
func (ws *WebSocketClient) OnConnectionState(callback ConnectionStateCallback) {
	ws.callbackMutex.Lock()
	defer ws.callbackMutex.Unlock()
	ws.stateCallbacks = append(ws.stateCallbacks, callback)
}

// OnConfigUpdate registers a callback for configuration updates
func (ws *WebSocketClient) OnConfigUpdate(configKey string, callback ConfigChangeCallback) {
	ws.callbackMutex.Lock()
	defer ws.callbackMutex.Unlock()
	ws.configCallbacks[configKey] = append(ws.configCallbacks[configKey], callback)
}

// OnError registers a callback for errors
func (ws *WebSocketClient) OnError(callback ErrorCallback) {
	ws.callbackMutex.Lock()
	defer ws.callbackMutex.Unlock()
	ws.errorCallbacks = append(ws.errorCallbacks, callback)
}

// getWebSocketURL constructs the WebSocket URL
func (ws *WebSocketClient) getWebSocketURL() string {
	scheme := "ws"
	if ws.baseURL[:5] == "https" {
		scheme = "wss"
	}
	
	// Replace http/https with ws/wss
	url := scheme + ws.baseURL[4:] + "/api/v1/sdk/ws"
	return url
}

// setState updates the connection state and notifies callbacks
func (ws *WebSocketClient) setState(state ConnectionState) {
	ws.state = state
	
	ws.callbackMutex.RLock()
	callbacks := make([]ConnectionStateCallback, len(ws.stateCallbacks))
	copy(callbacks, ws.stateCallbacks)
	ws.callbackMutex.RUnlock()
	
	// Notify state change callbacks
	for _, callback := range callbacks {
		go func(cb ConnectionStateCallback) {
			defer func() {
				if r := recover(); r != nil {
					ws.logger.Error("Connection state callback panic", map[string]interface{}{
						"error": r,
					})
				}
			}()
			cb(state)
		}(callback)
	}
}

// sendMessage sends a message over WebSocket
func (ws *WebSocketClient) sendMessage(message WebSocketMessage) error {
	ws.connMutex.RLock()
	conn := ws.conn
	ws.connMutex.RUnlock()
	
	if conn == nil {
		return NewConnectionError("WebSocket not connected", false, nil)
	}
	
	data, err := json.Marshal(message)
	if err != nil {
		return NewConnectionError("Failed to marshal message", false, err)
	}
	
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return NewConnectionError("Failed to send message", true, err)
	}
	
	return nil
}

// readMessages handles incoming WebSocket messages
func (ws *WebSocketClient) readMessages() {
	defer func() {
		if r := recover(); r != nil {
			ws.logger.Error("WebSocket read goroutine panic", map[string]interface{}{
				"error": r,
			})
		}
	}()
	
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ws.stopCh:
			return
		default:
			ws.connMutex.RLock()
			conn := ws.conn
			ws.connMutex.RUnlock()
			
			if conn == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			
			// Set read deadline
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					ws.logger.Error("WebSocket read error", map[string]interface{}{
						"error": err.Error(),
					})
					ws.notifyError(NewConnectionError("WebSocket read error", true, err))
				}
				
				ws.setState(Disconnected)
				ws.triggerReconnect()
				return
			}
			
			if messageType == websocket.TextMessage {
				ws.handleMessage(data)
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (ws *WebSocketClient) handleMessage(data []byte) {
	var message WebSocketMessage
	if err := json.Unmarshal(data, &message); err != nil {
		ws.logger.Error("Failed to unmarshal WebSocket message", map[string]interface{}{
			"error": err.Error(),
			"data":  string(data),
		})
		return
	}
	
	switch message.Type {
	case "config_update":
		ws.handleConfigUpdate(message)
	case "heartbeat":
		ws.handleHeartbeat()
	case "error":
		ws.handleErrorMessage(message)
	default:
		ws.logger.Debug("Unknown message type", map[string]interface{}{
			"type": message.Type,
		})
	}
}

// handleConfigUpdate processes configuration update messages
func (ws *WebSocketClient) handleConfigUpdate(message WebSocketMessage) {
	if message.Data == nil {
		ws.logger.Error("Config update message missing data", nil)
		return
	}
	
	data, err := json.Marshal(message.Data)
	if err != nil {
		ws.logger.Error("Failed to marshal config update data", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	var event ConfigUpdateEvent
	if err := json.Unmarshal(data, &event); err != nil {
		ws.logger.Error("Failed to unmarshal config update event", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("Received config update", map[string]interface{}{
		"config_key":  event.ConfigKey,
		"project_id":  event.ProjectID,
		"update_type": event.UpdateType,
		"version":     event.Version,
	})
	
	// Create result object
	result := &DynamicConfigResult{
		Key:            event.ConfigKey,
		Value:          event.NewValue,
		Reason:         "realtime_update",
		ETag:           event.ETag,
		Version:        &event.Version,
		CacheHit:       false,
		RealTimeUpdate: true,
		RetrievedAt:    time.Now(),
	}
	
	if event.Timestamp != "" {
		if updatedAt, err := time.Parse(time.RFC3339, event.Timestamp); err == nil {
			result.UpdatedAt = &updatedAt
		}
	}
	
	// Notify callbacks
	ws.notifyConfigCallbacks(event.ConfigKey, result)
}

// handleHeartbeat processes heartbeat messages
func (ws *WebSocketClient) handleHeartbeat() {
	ws.logger.Debug("Received heartbeat", nil)
	select {
	case ws.heartbeatCh <- struct{}{}:
	default:
	}
}

// handleErrorMessage processes error messages from server
func (ws *WebSocketClient) handleErrorMessage(message WebSocketMessage) {
	errorMsg := "Unknown server error"
	if message.Data != nil {
		if str, ok := message.Data.(string); ok {
			errorMsg = str
		}
	}
	
	ws.logger.Error("Server error", map[string]interface{}{
		"error": errorMsg,
	})
	
	ws.notifyError(NewVariablyError("Server error: "+errorMsg, nil))
}

// notifyConfigCallbacks notifies configuration change callbacks
func (ws *WebSocketClient) notifyConfigCallbacks(configKey string, result *DynamicConfigResult) {
	ws.callbackMutex.RLock()
	
	// Get specific callbacks
	specificCallbacks := make([]ConfigChangeCallback, len(ws.configCallbacks[configKey]))
	copy(specificCallbacks, ws.configCallbacks[configKey])
	
	// Get wildcard callbacks
	wildcardCallbacks := make([]ConfigChangeCallback, len(ws.configCallbacks["*"]))
	copy(wildcardCallbacks, ws.configCallbacks["*"])
	
	ws.callbackMutex.RUnlock()
	
	// Notify specific callbacks
	for _, callback := range specificCallbacks {
		go func(cb ConfigChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
					ws.logger.Error("Config callback panic", map[string]interface{}{
						"config_key": configKey,
						"error":      r,
					})
				}
			}()
			cb(result)
		}(callback)
	}
	
	// Notify wildcard callbacks
	for _, callback := range wildcardCallbacks {
		go func(cb ConfigChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
					ws.logger.Error("Wildcard config callback panic", map[string]interface{}{
						"config_key": configKey,
						"error":      r,
					})
				}
			}()
			cb(result)
		}(callback)
	}
}

// notifyError notifies error callbacks
func (ws *WebSocketClient) notifyError(err error) {
	ws.callbackMutex.RLock()
	callbacks := make([]ErrorCallback, len(ws.errorCallbacks))
	copy(callbacks, ws.errorCallbacks)
	ws.callbackMutex.RUnlock()
	
	for _, callback := range callbacks {
		go func(cb ErrorCallback) {
			defer func() {
				if r := recover(); r != nil {
					ws.logger.Error("Error callback panic", map[string]interface{}{
						"error": r,
					})
				}
			}()
			cb(err)
		}(callback)
	}
}

// heartbeatLoop sends periodic heartbeat messages
func (ws *WebSocketClient) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ws.stopCh:
			return
		case <-ticker.C:
			if ws.IsConnected() {
				message := WebSocketMessage{
					Type:      "heartbeat",
					Timestamp: time.Now(),
				}
				
				if err := ws.sendMessage(message); err != nil {
					ws.logger.Error("Failed to send heartbeat", map[string]interface{}{
						"error": err.Error(),
					})
				}
			}
		case <-ws.heartbeatCh:
			// Reset heartbeat timer when we receive a heartbeat
			ticker.Reset(30 * time.Second)
		}
	}
}

// reconnectLoop handles automatic reconnection
func (ws *WebSocketClient) reconnectLoop() {
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ws.stopCh:
			return
		case <-ws.reconnectCh:
			if ws.reconnectAttempts >= ws.config.MaxReconnectAttempts {
				ws.logger.Error("Max reconnect attempts reached", map[string]interface{}{
					"attempts": ws.reconnectAttempts,
				})
				ws.setState(Error)
				continue
			}
			
			ws.reconnectAttempts++
			ws.logger.Info("Attempting to reconnect", map[string]interface{}{
				"attempt": ws.reconnectAttempts,
				"max":     ws.config.MaxReconnectAttempts,
			})
			
			// Wait before reconnecting
			time.Sleep(ws.config.ReconnectInterval)
			
			if err := ws.Connect(); err != nil {
				ws.logger.Error("Reconnection failed", map[string]interface{}{
					"error": err.Error(),
				})
				
				// Trigger another reconnect attempt
				select {
				case ws.reconnectCh <- struct{}{}:
				default:
				}
			} else {
				// Reconnected successfully, resubscribe to channels
				ws.resubscribe()
			}
		}
	}
}

// triggerReconnect triggers a reconnection attempt
func (ws *WebSocketClient) triggerReconnect() {
	if !ws.config.AutoReconnect {
		return
	}
	
	select {
	case ws.reconnectCh <- struct{}{}:
	default:
	}
}

// resubscribe re-establishes subscriptions after reconnection
func (ws *WebSocketClient) resubscribe() {
	ws.subMutex.RLock()
	channels := make([]string, 0, len(ws.subscriptions))
	for channel := range ws.subscriptions {
		channels = append(channels, channel)
	}
	ws.subMutex.RUnlock()
	
	for _, channel := range channels {
		message := WebSocketMessage{
			Type:      "subscribe",
			Channel:   channel,
			Timestamp: time.Now(),
		}
		
		if err := ws.sendMessage(message); err != nil {
			ws.logger.Error("Failed to resubscribe to channel", map[string]interface{}{
				"channel": channel,
				"error":   err.Error(),
			})
		} else {
			ws.logger.Debug("Resubscribed to channel", map[string]interface{}{
				"channel": channel,
			})
		}
	}
}