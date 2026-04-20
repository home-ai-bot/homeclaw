package homeclaw

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/service"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// DeviceOpsManager handles device operations API
type DeviceOpsManager struct {
	mu               sync.Mutex
	deviceOpsService *service.DeviceOpsService
	deviceStore      data.DeviceStore
	deviceOpStore    data.DeviceOpStore
	workspacePath    string
	initialized      bool
	initErr          error
}

// NewDeviceOpsManager creates a new DeviceOpsManager instance
func NewDeviceOpsManager() *DeviceOpsManager {
	return &DeviceOpsManager{}
}

// Initialize lazily initializes the DeviceOpsService with required stores
func (m *DeviceOpsManager) Initialize(workspacePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return m.initErr
	}

	m.workspacePath = workspacePath

	// Initialize JSONStore
	dataDir := workspacePath + "/data"
	jsonStore, err := data.NewJSONStore(dataDir)
	if err != nil {
		m.initErr = err
		logger.ErrorC("device-ops", "Failed to initialize JSONStore: "+err.Error())
		return err
	}

	// Initialize DeviceStore
	m.deviceStore, err = data.NewDeviceStore(jsonStore)
	if err != nil {
		m.initErr = err
		logger.ErrorC("device-ops", "Failed to initialize DeviceStore: "+err.Error())
		return err
	}

	// Initialize DeviceOpStore
	m.deviceOpStore, err = data.NewDeviceOpStore(jsonStore, m.deviceStore)
	if err != nil {
		m.initErr = err
		logger.ErrorC("device-ops", "Failed to initialize DeviceOpStore: "+err.Error())
		return err
	}

	// Initialize DeviceOpsService
	m.deviceOpsService = service.NewDeviceOpsService(m.deviceStore, m.deviceOpStore)
	m.initialized = true

	logger.InfoC("device-ops", "DeviceOpsManager initialized successfully")
	return nil
}

// RegisterRoutes binds DeviceOps API endpoints to the ServeMux
func (m *DeviceOpsManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/device-ops/execute", m.handleExecuteDeviceOp)
	mux.HandleFunc("POST /api/device-ops/mark-no-action", m.handleMarkDeviceAsNoAction)
}

// handleExecuteDeviceOp executes a device operation by sending command to gateway via Pico channel
type executeDeviceOpRequest struct {
	FromID  string `json:"from_id"`
	From    string `json:"from"`
	OpsName string `json:"ops_name"`
}

func (m *DeviceOpsManager) handleExecuteDeviceOp(w http.ResponseWriter, r *http.Request) {
	if err := m.Initialize(m.workspacePath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to initialize device ops service",
		})
		return
	}

	var req executeDeviceOpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Invalid request body",
		})
		return
	}

	if req.FromID == "" || req.From == "" || req.OpsName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Missing required parameters: from_id, from, ops_name",
		})
		return
	}

	// Build the CLI command using the 'exe' method.
	// execExe in cli_tool.go looks up the DeviceOpStore internally via
	// {from_id, from, ops} and handles getProps/setProps/execute dispatch itself.
	cliCommand := map[string]any{
		"brand":  req.From,
		"method": "exe",
		"params": map[string]any{
			"from_id": req.FromID,
			"from":    req.From,
			"ops":     req.OpsName,
		},
	}

	// Marshal to JSON string for hc_cli tool
	commandJSON, err := json.Marshal(cliCommand)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to marshal command",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":      true,
		"from_id":      req.FromID,
		"from":         req.From,
		"ops_name":     req.OpsName,
		"cli_method":   "exe",
		"command_json": string(commandJSON),
		"message":      "Command ready to be sent to gateway via Pico channel",
	})
}

// handleMarkDeviceAsNoAction marks a device as non-operable
type markNoActionRequest struct {
	FromID string `json:"from_id"`
	From   string `json:"from"`
}

func (m *DeviceOpsManager) handleMarkDeviceAsNoAction(w http.ResponseWriter, r *http.Request) {
	if err := m.Initialize(m.workspacePath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to initialize device ops service",
		})
		return
	}

	var req markNoActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Invalid request body",
		})
		return
	}

	if req.FromID == "" || req.From == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Missing required parameters: from_id, from",
		})
		return
	}

	if err := m.deviceOpsService.MarkDeviceAsNoAction(req.FromID, req.From); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Device marked as non-operable",
	})
}
