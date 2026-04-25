package homeclaw

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// DeviceOpsManager handles device operations API
type DeviceOpsManager struct {
	mu            sync.Mutex
	deviceStore   data.DeviceStore
	deviceOpStore data.DeviceOpStore
	workspacePath string
	initialized   bool
	initErr       error
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

	m.initialized = true

	logger.InfoC("device-ops", "DeviceOpsManager initialized successfully")
	return nil
}

// RegisterRoutes binds DeviceOps API endpoints to the ServeMux
func (m *DeviceOpsManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/device-ops/list", m.handleListDeviceOps)
	mux.HandleFunc("POST /api/device-ops/execute", m.handleExecuteDeviceOp)
	mux.HandleFunc("POST /api/device-ops/clear", m.handleClearDeviceOps)
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

// handleListDeviceOps returns all DeviceOp objects for a given device
func (m *DeviceOpsManager) handleListDeviceOps(w http.ResponseWriter, r *http.Request) {
	if err := m.Initialize(m.workspacePath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to initialize device ops service",
		})
		return
	}

	fromID := r.URL.Query().Get("from_id")
	from := r.URL.Query().Get("from")

	if fromID == "" || from == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Missing required parameters: from_id, from",
		})
		return
	}

	// Lookup the device by from_id and from to get its URN
	devices, err := m.deviceStore.GetAll()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to get devices",
		})
		return
	}

	var urn string
	for _, device := range devices {
		if device.FromID == fromID && device.From == from {
			urn = device.URN
			break
		}
	}

	if urn == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Device not found",
		})
		return
	}

	// Get all DeviceOps and filter by URN and From
	allOps, err := m.deviceOpStore.GetAll()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to get device operations",
		})
		return
	}

	var filteredOps []data.DeviceOp
	for _, op := range allOps {
		if op.URN == urn && op.From == from {
			filteredOps = append(filteredOps, op)
		}
	}

	if filteredOps == nil {
		filteredOps = []data.DeviceOp{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"urn":     urn,
		"from":    from,
		"ops":     filteredOps,
	})
}

// handleClearDeviceOps clears all device operations and device ops field for a given brand
func (m *DeviceOpsManager) handleClearDeviceOps(w http.ResponseWriter, r *http.Request) {
	if err := m.Initialize(m.workspacePath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to initialize device ops service",
		})
		return
	}

	var req struct {
		Brand string `json:"brand"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Invalid request body",
		})
		return
	}

	if req.Brand == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Missing required parameter: brand",
		})
		return
	}

	// Get all devices and filter by brand
	devices, err := m.deviceStore.GetAll()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to get devices",
		})
		return
	}

	// Clear ops field from devices of this brand
	var updatedDevices []data.Device
	var clearedCount int
	for _, device := range devices {
		if device.From == req.Brand {
			if len(device.Ops) > 0 {
				device.Ops = nil
				clearedCount++
			}
			updatedDevices = append(updatedDevices, device)
		}
	}

	// Save updated devices if any were modified
	if len(updatedDevices) > 0 {
		if err := m.deviceStore.Save(updatedDevices...); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "Failed to save updated devices",
			})
			return
		}
	}

	// Get all DeviceOps and filter by brand (from field)
	allOps, err := m.deviceOpStore.GetAll()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Failed to get device operations",
		})
		return
	}

	// Delete all ops for this brand
	var deletedCount int
	for _, op := range allOps {
		if op.From == req.Brand {
			if err := m.deviceOpStore.Delete(op.URN, op.From, op.Ops); err != nil {
				logger.ErrorC("device-ops", "Failed to delete op: "+err.Error())
				continue
			}
			deletedCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":         true,
		"brand":           req.Brand,
		"devices_cleared": clearedCount,
		"ops_deleted":     deletedCount,
		"message":         "Cleared all device operations for brand: " + req.Brand,
	})
}
