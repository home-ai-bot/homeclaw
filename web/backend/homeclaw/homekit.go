package homeclaw

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// HomeKitManager handles HomeKit device discovery and pairing
type HomeKitManager struct {
	deviceStore data.DeviceStore
}

// NewHomeKitManager creates a new HomeKitManager instance
func NewHomeKitManager(deviceStore data.DeviceStore) *HomeKitManager {
	return &HomeKitManager{
		deviceStore: deviceStore,
	}
}

// HomeKitDiscoveryResponse represents the response format for device discovery
type HomeKitDiscoveryResponse struct {
	Sources []HomeKitDeviceSource `json:"sources"`
}

// HomeKitDeviceSource represents a single discovered device
type HomeKitDeviceSource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Info     string `json:"info"`
	URL      string `json:"url"`
	Location string `json:"location"`
}

// RegisterRoutes binds HomeKit endpoints to the ServeMux
func (m *HomeKitManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/homekit/discovery", m.handleDiscovery)
	mux.HandleFunc("POST /api/homekit", m.handlePair)
	mux.HandleFunc("DELETE /api/homekit", m.handleUnpair)
}

// handleDiscovery discovers HomeKit devices on the local network
func (m *HomeKitManager) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	devices, err := m.discoverDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := HomeKitDiscoveryResponse{
		Sources: devices,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handlePair pairs a HomeKit device with the provided PIN
func (m *HomeKitManager) handlePair(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	src := r.FormValue("src")
	pin := r.FormValue("pin")

	if id == "" || src == "" || pin == "" {
		http.Error(w, "missing required parameters: id, src, pin", http.StatusBadRequest)
		return
	}

	// Parse the src to extract IP and port
	ip := src
	port := uint16(0)
	if idx := strings.LastIndex(src, ":"); idx != -1 {
		ip = src[:idx]
		if p, err := fmt.Sscanf(src[idx+1:], "%d", &port); err != nil || p != 1 {
			port = 0
		}
	}

	// Save device to DeviceStore
	device := data.Device{
		FromID: id,
		From:   "homekit",
		Name:   id,
		Type:   "homekit",
		Token:  pin,
		IP:     ip,
	}

	if err := m.deviceStore.Save(device); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save device: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// handleUnpair removes a paired HomeKit device
func (m *HomeKitManager) handleUnpair(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	if id == "" {
		http.Error(w, "missing required parameter: id", http.StatusBadRequest)
		return
	}

	// Delete device from DeviceStore
	if err := m.deviceStore.Delete(id, "homekit"); err != nil {
		if err == data.ErrRecordNotFound {
			http.Error(w, "device not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Failed to delete device: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

// discoverDevices performs mDNS discovery for HomeKit devices
func (m *HomeKitManager) discoverDevices() ([]HomeKitDeviceSource, error) {
	// Get all paired devices from DeviceStore
	pairedDevices, err := m.deviceStore.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get paired devices: %w", err)
	}

	// Build a map of paired device IDs
	pairedMap := make(map[string]bool)
	for _, device := range pairedDevices {
		if device.From == "homekit" {
			pairedMap[device.FromID] = true
		}
	}

	var devices []HomeKitDeviceSource
	deviceMap := make(map[string]*mdns.ServiceEntry)

	// Discover HomeKit devices using mDNS
	err = mdns.Discovery(mdns.ServiceHAP, func(entry *mdns.ServiceEntry) bool {
		if entry.Complete() {
			key := entry.Name
			if _, ok := deviceMap[key]; ok {
				// Update existing entry if this one is more complete
				if entry.IP != nil && entry.Port > 0 {
					deviceMap[key] = entry
				}
			} else {
				deviceMap[key] = entry
			}
		}
		return false // Continue discovery
	})

	if err != nil {
		return nil, fmt.Errorf("mDNS discovery failed: %w", err)
	}

	// Convert map to slice and format for response
	for name, entry := range deviceMap {
		isPaired := pairedMap[name]

		// Build info string similar to go2rtc format
		var infoParts []string
		for k, v := range entry.Info {
			infoParts = append(infoParts, fmt.Sprintf("%s=%s", k, v))
		}

		// Add status indicator (status=1 means unpaired, status=0 means paired)
		if isPaired {
			infoParts = append(infoParts, "status=0")
		} else {
			infoParts = append(infoParts, "status=1")
		}

		sort.Strings(infoParts)
		infoStr := strings.Join(infoParts, " ")

		device := HomeKitDeviceSource{
			ID:       name,
			Name:     entry.Name,
			Info:     infoStr,
			URL:      entry.Addr(),
			Location: entry.IP.String(),
		}

		devices = append(devices, device)
	}

	return devices, nil
}
