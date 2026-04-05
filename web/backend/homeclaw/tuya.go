package homeclaw

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync"

	go2rtcTuya "github.com/AlexxIT/go2rtc/pkg/tuya"
	"github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/tuya"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// TuyaManager handles Tuya API operations
type TuyaManager struct {
	mu         sync.Mutex
	clients    map[string]*tuya.Client // keyed by region
	store      *data.JSONStore
	tokenStore tuya.TokenStore
}

// NewTuyaManager creates a new TuyaManager instance
func NewTuyaManager() *TuyaManager {
	// Create data directory for tuya
	dataDir := filepath.Join(config.GetPicoclawHome(), "tuya")
	store, err := data.NewJSONStore(dataDir)
	if err != nil {
		logger.ErrorC("tuya", "Failed to create Tuya data store: "+err.Error())
		return &TuyaManager{
			clients: make(map[string]*tuya.Client),
		}
	}

	tokenStore, err := tuya.NewTokenStore(store)
	if err != nil {
		logger.ErrorC("tuya", "Failed to create Tuya token store: "+err.Error())
		return &TuyaManager{
			clients: make(map[string]*tuya.Client),
			store:   store,
		}
	}

	return &TuyaManager{
		clients:    make(map[string]*tuya.Client),
		store:      store,
		tokenStore: tokenStore,
	}
}

// RegisterRoutes binds Tuya API endpoints to the ServeMux
func (m *TuyaManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tuya/regions", m.handleGetRegions)
	mux.HandleFunc("GET /api/tuya/status", m.handleGetStatus)
	mux.HandleFunc("POST /api/tuya/login", m.handleLogin)
	mux.HandleFunc("POST /api/tuya/logout", m.handleLogout)
	mux.HandleFunc("DELETE /api/tuya/credentials", m.handleDeleteCredentials)
	// Token-based auth endpoints
	mux.HandleFunc("POST /api/tuya/token", m.handleSaveToken)
	mux.HandleFunc("DELETE /api/tuya/token", m.handleDeleteToken)
}

// handleGetRegions returns available Tuya regions
func (m *TuyaManager) handleGetRegions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"regions": go2rtcTuya.AvailableRegions,
	})
}

// handleGetStatus returns the current Tuya login status
func (m *TuyaManager) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Check token-based auth first
	if m.tokenStore != nil && m.tokenStore.Exists() {
		json.NewEncoder(w).Encode(map[string]any{
			"logged_in": true,
			"auth_type": "token",
		})
		return
	}

	// Fall back to credential-based auth
	client, err := tuya.NewClient(m.store)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"logged_in": false,
			"error":     err.Error(),
		})
		return
	}

	if !client.HasStoredCredentials() {
		json.NewEncoder(w).Encode(map[string]any{
			"logged_in": false,
		})
		return
	}

	secretData, err := client.GetStoredCredentials()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"logged_in": false,
			"error":     err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"logged_in": true,
		"auth_type": "credentials",
		"region":    secretData.Region,
		"username":  secretData.UserName,
	})
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Region   string `json:"region"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin performs Tuya login
func (m *TuyaManager) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Region == "" || req.Username == "" || req.Password == "" {
		http.Error(w, "region, username and password are required", http.StatusBadRequest)
		return
	}

	// Validate region
	region := tuya.GetRegionByName(req.Region)
	if region == nil {
		http.Error(w, "Invalid region", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create client with credentials
	client, err := tuya.NewClient(m.store,
		tuya.WithCredentials(req.Region, req.Username, req.Password),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Perform login
	loginResult, err := client.Login()
	if err != nil {
		logger.ErrorC("tuya", "Login failed: "+err.Error())
		http.Error(w, "Login failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Save credentials after successful login
	if err := client.SaveCredentials(); err != nil {
		logger.ErrorC("tuya", "Failed to save credentials: "+err.Error())
		// Don't fail the request, just log the error
	}

	// Cache the client
	m.clients[req.Region] = client

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"user": map[string]any{
			"uid":      loginResult.Uid,
			"username": loginResult.Username,
			"nickname": loginResult.Nickname,
			"email":    loginResult.Email,
			"timezone": loginResult.Timezone,
		},
		"region": req.Region,
	})
}

// handleLogout logs out from Tuya
func (m *TuyaManager) handleLogout(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all clients
	for _, client := range m.clients {
		client.Close()
	}
	m.clients = make(map[string]*tuya.Client)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}

// handleDeleteCredentials removes stored credentials
func (m *TuyaManager) handleDeleteCredentials(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all clients
	for _, client := range m.clients {
		client.Close()
	}
	m.clients = make(map[string]*tuya.Client)

	// Create a client to access the secret store
	client, err := tuya.NewClient(m.store)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete credentials
	if err := client.DeleteCredentials(); err != nil {
		// It's OK if there were no credentials
		logger.ErrorC("tuya", "Failed to delete credentials: "+err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}

// SaveTokenRequest represents the token save request body
type SaveTokenRequest struct {
	Token string `json:"token"`
}

// handleSaveToken saves a Tuya Open Platform API token
func (m *TuyaManager) handleSaveToken(w http.ResponseWriter, r *http.Request) {
	var req SaveTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tokenStore == nil {
		http.Error(w, "token store not initialized", http.StatusInternalServerError)
		return
	}

	if err := m.tokenStore.Save(req.Token); err != nil {
		logger.ErrorC("tuya", "Failed to save token: "+err.Error())
		http.Error(w, "Failed to save token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}

// handleDeleteToken removes the stored API token
func (m *TuyaManager) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tokenStore != nil {
		if err := m.tokenStore.Delete(); err != nil {
			logger.ErrorC("tuya", "Failed to delete token: "+err.Error())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}

// GetClient returns a Tuya client for the given region
// If the client doesn't exist, it loads credentials and creates one
func (m *TuyaManager) GetClient(region string) (*tuya.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if client exists
	if client, ok := m.clients[region]; ok {
		return client, nil
	}

	// Try to load credentials and create client
	client, err := tuya.NewClient(m.store)
	if err != nil {
		return nil, err
	}

	if err := client.LoadCredentials(); err != nil {
		return nil, err
	}

	// Cache the client
	m.clients[region] = client
	return client, nil
}

// Stop closes all Tuya clients
func (m *TuyaManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		client.Close()
	}
	m.clients = make(map[string]*tuya.Client)
}
