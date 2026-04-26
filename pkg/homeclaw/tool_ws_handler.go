package homeclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ToolCallHandler is the interface for handling direct tool call WebSocket connections.
// It decouples the WebSocket transport layer (PicoChannel) from the tool execution logic (homeclaw).
type ToolCallHandler interface {
	// HandleToolWebSocket handles an incoming WebSocket connection for direct tool calls.
	// The HTTP connection will be upgraded to WebSocket inside this method.
	HandleToolWebSocket(w http.ResponseWriter, r *http.Request)
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool WebSocket connection
// ─────────────────────────────────────────────────────────────────────────────

// toolWSConn wraps a WebSocket connection for tool calls.
type toolWSConn struct {
	id      string
	conn    *websocket.Conn
	writeMu sync.Mutex
	closed  atomic.Bool
}

func (tc *toolWSConn) writeJSON(v any) error {
	if tc.closed.Load() {
		return fmt.Errorf("connection closed")
	}
	tc.writeMu.Lock()
	defer tc.writeMu.Unlock()
	return tc.conn.WriteJSON(v)
}

func (tc *toolWSConn) close() {
	if tc.closed.CompareAndSwap(false, true) {
		tc.conn.Close()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool WebSocket message (Pico Protocol compatible)
// ─────────────────────────────────────────────────────────────────────────────

// toolWSMessage represents a Pico Protocol message for the tool WebSocket.
// It is wire-compatible with the PicoMessage format so the frontend can use
// the same sendAndWait logic without modification.
type toolWSMessage struct {
	Type      string         `json:"type"`
	ID        string         `json:"id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func newToolWSMessage(msgType string, payload map[string]any) toolWSMessage {
	return toolWSMessage{
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
}

func newToolWSError(code, message string) toolWSMessage {
	return newToolWSMessage("error", map[string]any{
		"code":    code,
		"message": message,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// ToolWSHandler — core implementation
// ─────────────────────────────────────────────────────────────────────────────

// ToolWSHandler implements ToolCallHandler. It handles WebSocket connections
// for direct tool execution, bypassing the agent loop entirely.
// All tool call logic (parse, execute, respond) lives here in the homeclaw package.
type ToolWSHandler struct {
	homeclaw     *HomeClaw
	toolRegistry *tools.ToolRegistry
	upgrader     websocket.Upgrader
	ctx          context.Context
	cancel       context.CancelFunc
	picoToken    string
}

// NewToolWSHandler creates a new ToolWSHandler.
func NewToolWSHandler(hc *HomeClaw, toolRegistry *tools.ToolRegistry, picoConfig *config.Config) *ToolWSHandler {
	// Extract Pico channel settings
	var allowOrigins []string
	var picoToken string
	if picoChannel, ok := picoConfig.Channels[config.ChannelPico]; ok && picoChannel != nil {
		if decoded, err := picoChannel.GetDecoded(); err == nil && decoded != nil {
			if picoSettings, ok := decoded.(*config.PicoSettings); ok {
				allowOrigins = picoSettings.AllowOrigins
				picoToken = picoSettings.Token.String()
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ToolWSHandler{
		homeclaw:     hc,
		toolRegistry: toolRegistry,
		picoToken:    picoToken,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				if len(allowOrigins) == 0 {
					return true
				}
				origin := r.Header.Get("Origin")
				for _, allowed := range allowOrigins {
					if allowed == "*" || allowed == origin {
						return true
					}
				}
				return false
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// HandleToolWebSocket upgrades the HTTP connection and handles tool calls directly.
func (h *ToolWSHandler) HandleToolWebSocket(w http.ResponseWriter, r *http.Request) {
	logger.Infof("[ToolWSHandler.HandleToolWebSocket] called")
	if h.homeclaw == nil || h.toolRegistry == nil {
		logger.Errorf("[ToolWSHandler.HandleToolWebSocket] not initialized")
		http.Error(w, "tool handler not initialized", http.StatusServiceUnavailable)
		return
	}

	// Echo the matched subprotocol back so the browser accepts the upgrade.
	proto := h.matchedSubprotocol(r)
	logger.Infof("[ToolWSHandler.HandleToolWebSocket] matchedSubprotocol=%s, picoToken=%s", proto, h.picoToken)
	logger.Infof("[ToolWSHandler.HandleToolWebSocket] subprotocols from request: %v", websocket.Subprotocols(r))
	var responseHeader http.Header
	if proto != "" {
		responseHeader = http.Header{"Sec-WebSocket-Protocol": {proto}}
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		logger.ErrorCF("homeclaw", "Tool WS upgrade failed", map[string]any{"error": err.Error()})
		return
	}

	logger.Infof("[ToolWSHandler.HandleToolWebSocket] WebSocket upgraded successfully")

	tc := &toolWSConn{
		id:   uuid.New().String(),
		conn: conn,
	}

	logger.InfoCF("homeclaw", "Tool WebSocket client connected", map[string]any{
		"conn_id": tc.id,
	})

	go h.toolReadLoop(tc)
}

// Close cleans up the handler resources.
func (h *ToolWSHandler) Close() {
	if h.cancel != nil {
		h.cancel()
	}
}

// matchedSubprotocol returns the "token.<value>" subprotocol that matches
// the configured pico token, or "" if none do.
func (h *ToolWSHandler) matchedSubprotocol(r *http.Request) string {
	if h.picoToken == "" {
		return ""
	}
	for _, proto := range websocket.Subprotocols(r) {
		if after, ok := strings.CutPrefix(proto, "token."); ok && after == h.picoToken {
			return proto
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Read loop — reads messages and dispatches to executeToolCall
// ─────────────────────────────────────────────────────────────────────────────

func (h *ToolWSHandler) toolReadLoop(tc *toolWSConn) {
	defer func() {
		tc.close()
		logger.InfoCF("homeclaw", "Tool WebSocket client disconnected", map[string]any{
			"conn_id": tc.id,
		})
	}()

	readTimeout := 60 * time.Second
	_ = tc.conn.SetReadDeadline(time.Now().Add(readTimeout))
	tc.conn.SetPongHandler(func(string) error {
		_ = tc.conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})

	// Start ping ticker
	pingInterval := 30 * time.Second
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()
	go func() {
		for range pingTicker.C {
			if tc.closed.Load() {
				return
			}
			tc.writeMu.Lock()
			err := tc.conn.WriteMessage(websocket.PingMessage, nil)
			tc.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		_, raw, err := tc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.DebugCF("homeclaw", "Tool WS read error", map[string]any{"error": err.Error()})
			}
			return
		}

		_ = tc.conn.SetReadDeadline(time.Now().Add(readTimeout))

		var msg toolWSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			tc.writeJSON(newToolWSError("invalid_message", "failed to parse message"))
			continue
		}

		h.executeToolCall(tc, msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool execution — parse, execute, respond
// ─────────────────────────────────────────────────────────────────────────────

func (h *ToolWSHandler) executeToolCall(tc *toolWSConn, msg toolWSMessage) {
	content, _ := msg.Payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		return
	}

	// Parse tool command using HomeClaw's existing ParseToolCommand
	toolName, commandJSON, ok := h.homeclaw.ParseToolCommand(content)
	if !ok {
		h.sendToolResponse(tc, msg.ID, "not a valid tool command", true)
		return
	}

	logger.InfoCF("homeclaw", "Direct tool call received", map[string]any{
		"tool_name": toolName,
		"msg_id":    msg.ID,
	})

	// Get tool from registry
	tool, found := h.toolRegistry.Get(toolName)
	if !found {
		logger.ErrorCF("homeclaw", "Tool not found in registry", map[string]any{
			"tool_name":       toolName,
			"available_tools": h.toolRegistry.List(),
		})
		h.sendToolResponse(tc, msg.ID, fmt.Sprintf("tool '%s' not registered", toolName), true)
		return
	}

	// Execute tool directly
	toolArgs := map[string]any{"commandJson": commandJSON}
	result := tool.Execute(h.ctx, toolArgs)

	if result.IsError {
		logger.ErrorCF("homeclaw", "Direct tool execution failed", map[string]any{
			"tool_name": toolName,
			"error":     result.ForLLM,
		})
		h.sendToolResponse(tc, msg.ID, result.ForLLM, true)
		return
	}

	logger.InfoCF("homeclaw", "Direct tool executed successfully", map[string]any{
		"tool_name":     toolName,
		"result_length": len(result.ForLLM),
	})

	h.sendToolResponse(tc, msg.ID, result.ForLLM, false)
}

func (h *ToolWSHandler) sendToolResponse(tc *toolWSConn, originalMsgID string, content string, isError bool) {
	response := newToolWSMessage("message.create", map[string]any{
		"content":  content,
		"is_error": isError,
	})
	response.ID = "tool-response-" + originalMsgID

	if err := tc.writeJSON(response); err != nil {
		logger.ErrorCF("homeclaw", "Failed to send tool response", map[string]any{
			"error": err.Error(),
		})
	}
}
