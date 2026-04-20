package homeclaw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sync"

	go2rtcTuya "github.com/AlexxIT/go2rtc/pkg/tuya"
	"github.com/sipeed/picoclaw/pkg/homeclaw/config"
	hcd "github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// GetRegionByHost finds a region by its host
func GetRegionByHost(host string) *go2rtcTuya.Region {
	for _, r := range go2rtcTuya.AvailableRegions {
		if r.Host == host {
			return &r
		}
	}
	return nil
}

// GetRegionByName finds a region by its name
func GetRegionByName(name string) *go2rtcTuya.Region {
	for _, r := range go2rtcTuya.AvailableRegions {
		if r.Name == name {
			return &r
		}
	}
	return nil
}

// TuyaClient handles Tuya API operations with credential storage
type TuyaClient struct {
	httpClient  *http.Client
	baseURL     string
	countryCode string
	authStore   hcd.AuthStore
	region      *go2rtcTuya.Region
	email       string
	password    string
	loginResult *go2rtcTuya.LoginResult
}

// NewTuyaClient creates a new Tuya client
func NewTuyaClient(authStore hcd.AuthStore, region, email, password string) (*TuyaClient, error) {
	client := &TuyaClient{
		authStore: authStore,
		email:     email,
		password:  password,
	}

	if region != "" {
		r := GetRegionByName(region)
		if r == nil {
			return nil, fmt.Errorf("invalid region: %s", region)
		}
		client.region = r
		client.baseURL = r.Host
		client.countryCode = r.Continent
	}

	client.httpClient = go2rtcTuya.CreateHTTPClientWithSession()

	return client, nil
}

// Login performs the Tuya login flow
func (c *TuyaClient) Login() (*go2rtcTuya.LoginResult, error) {
	if c.httpClient == nil {
		return nil, errors.New("http client not initialized")
	}
	if c.baseURL == "" || c.email == "" || c.password == "" {
		return nil, errors.New("credentials not set")
	}

	// Step 1: Get login token
	tokenResp, err := c.getLoginToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get login token: %w", err)
	}

	// Step 2: Encrypt password
	encryptedPassword, err := go2rtcTuya.EncryptPassword(c.password, tokenResp.Result.PbKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	// Step 3: Perform login
	loginResp, err := c.performLogin(tokenResp.Result.Token, encryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	if !loginResp.Success {
		return nil, errors.New(loginResp.ErrorMsg)
	}

	c.loginResult = &loginResp.Result
	return &loginResp.Result, nil
}

// getLoginToken fetches the login token from Tuya API
func (c *TuyaClient) getLoginToken() (*go2rtcTuya.LoginTokenResponse, error) {
	url := fmt.Sprintf("https://%s/api/login/token", c.baseURL)

	tokenReq := go2rtcTuya.LoginTokenRequest{
		CountryCode: c.countryCode,
		Username:    c.email,
		IsUid:       false,
	}

	jsonData, err := json.Marshal(tokenReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.baseURL))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/login", c.baseURL))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp go2rtcTuya.LoginTokenResponse
	if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	if !tokenResp.Success {
		return nil, errors.New("tuya: " + tokenResp.Msg)
	}

	return &tokenResp, nil
}

// performLogin sends the login request with encrypted password
func (c *TuyaClient) performLogin(token, encryptedPassword string) (*go2rtcTuya.PasswordLoginResponse, error) {
	var loginURL string

	loginReq := go2rtcTuya.PasswordLoginRequest{
		CountryCode: c.countryCode,
		Passwd:      encryptedPassword,
		Token:       token,
		IfEncrypt:   1,
		Options:     `{"group":1}`,
	}

	if go2rtcTuya.IsEmailAddress(c.email) {
		loginURL = fmt.Sprintf("https://%s/api/private/email/login", c.baseURL)
		loginReq.Email = c.email
	} else {
		loginURL = fmt.Sprintf("https://%s/api/private/phone/login", c.baseURL)
		loginReq.Mobile = c.email
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return nil, err
	}
	logger.Info(string(jsonData))
	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.baseURL))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/login", c.baseURL))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Read body content for logging and decoding
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	logger.Infof("resp.Body: %s", string(bodyBytes))
	var loginResp go2rtcTuya.PasswordLoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, err
	}

	return &loginResp, nil
}

// SaveCredentials saves the credentials to the auth store
func (c *TuyaClient) SaveCredentials() error {
	if c.region == nil || c.email == "" || c.password == "" {
		return errors.New("credentials not set")
	}
	return c.authStore.SaveBrand("tuya_pass", c.region.Name, c.email, c.password, nil)
}

// LoadCredentials loads stored credentials from the auth store
func (c *TuyaClient) LoadCredentials() error {
	region, email, password, _, err := c.authStore.GetDecryptedBrand("tuya_pass")
	if err != nil {
		return err
	}

	r := GetRegionByName(region)
	if r == nil {
		return fmt.Errorf("invalid stored region: %s", region)
	}

	c.region = r
	c.baseURL = r.Host
	c.countryCode = r.Continent
	c.email = email
	c.password = password
	return nil
}

// HasStoredCredentials checks if there are stored credentials
func (c *TuyaClient) HasStoredCredentials() bool {
	return c.authStore.Exists("tuya_pass")
}

// DeleteCredentials removes stored credentials
func (c *TuyaClient) DeleteCredentials() error {
	return c.authStore.DeleteBrand("tuya_pass")
}

// GetStoredCredentials returns the stored credentials (encrypted)
func (c *TuyaClient) GetStoredCredentials() (*hcd.BrandAuthData, error) {
	return c.authStore.GetBrand("tuya_pass")
}

// GetLoginResult returns the last login result
func (c *TuyaClient) GetLoginResult() *go2rtcTuya.LoginResult {
	return c.loginResult
}

// Close closes the client and releases resources
func (c *TuyaClient) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

// TuyaManager handles Tuya API operations
type TuyaManager struct {
	mu            sync.Mutex
	clients       map[string]*TuyaClient // keyed by region
	store         *hcd.JSONStore
	authStore     hcd.AuthStore // Lazy-initialized shared auth store
	authStoreOnce sync.Once
}

// NewTuyaManager creates a new TuyaManager instance
func NewTuyaManager() *TuyaManager {
	// Create data directory for tuya
	dataDir := filepath.Join(config.GetPicoclawHome(), "tuya")
	store, err := hcd.NewJSONStore(dataDir)
	if err != nil {
		logger.ErrorC("tuya", "Failed to create Tuya data store: "+err.Error())
		return &TuyaManager{
			clients: make(map[string]*TuyaClient),
		}
	}

	return &TuyaManager{
		clients: make(map[string]*TuyaClient),
		store:   store,
	}
}

// getAuthStore returns the shared AuthStore instance (lazy initialized)
func (m *TuyaManager) getAuthStore() hcd.AuthStore {
	m.authStoreOnce.Do(func() {
		// Get workspace path from config
		workspace := ""
		if cfg, err := config.LoadConfig(); err == nil {
			workspace = cfg.WorkspacePath()
		}
		if workspace == "" {
			logger.WarnC("tuya", "Workspace path not configured, auth store will not be available")
			return
		}

		authDir := filepath.Join(workspace, "auth")
		authJSONStore, err := hcd.NewJSONStore(authDir)
		if err != nil {
			logger.ErrorC("tuya", "Failed to create auth store: "+err.Error())
			return
		}

		authStore, err := hcd.NewAuthStore(authJSONStore)
		if err != nil {
			logger.ErrorC("tuya", "Failed to initialize auth store: "+err.Error())
			return
		}

		m.authStore = authStore
	})
	return m.authStore
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
// Note: This now returns a placeholder. The actual status should be fetched
// via WebSocket using hc_cli.getAuthStatus method.
func (m *TuyaManager) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	// Return placeholder - frontend should use WebSocket to get actual status
	// via hc_cli.getAuthStatus method
	json.NewEncoder(w).Encode(map[string]any{
		"logged_in": false,
		"message":   "Use WebSocket hc_cli.getAuthStatus to check authentication status",
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
	region := GetRegionByName(req.Region)
	if region == nil {
		http.Error(w, "Invalid region", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create client with credentials
	authStore := m.getAuthStore()
	if authStore == nil {
		http.Error(w, "auth store not initialized", http.StatusInternalServerError)
		return
	}

	client, err := NewTuyaClient(authStore, req.Region, req.Username, req.Password)
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
	m.clients = make(map[string]*TuyaClient)

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
	m.clients = make(map[string]*TuyaClient)

	// Create a client to access the secret store
	authStore := m.getAuthStore()
	if authStore == nil {
		http.Error(w, "auth store not initialized", http.StatusInternalServerError)
		return
	}

	client, err := NewTuyaClient(authStore, "", "", "")
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
// Note: This is deprecated. Use WebSocket hc_cli.saveAuth instead.
func (m *TuyaManager) handleSaveToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	json.NewEncoder(w).Encode(map[string]any{
		"error":   "Deprecated: Use WebSocket hc_cli.saveAuth method instead",
		"message": "Please use callTool with hc_cli.saveAuth to save tokens",
	})
}

// handleDeleteToken removes the stored API token
// Note: This is deprecated. Use WebSocket hc_cli.deleteAuth instead.
func (m *TuyaManager) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	json.NewEncoder(w).Encode(map[string]any{
		"error":   "Deprecated: Use WebSocket hc_cli.deleteAuth method instead",
		"message": "Please use callTool with hc_cli.deleteAuth to delete tokens",
	})
}

// GetClient returns a Tuya client for the given region
// If the client doesn't exist, it loads credentials and creates one
func (m *TuyaManager) GetClient(region string) (*TuyaClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if client exists
	if client, ok := m.clients[region]; ok {
		return client, nil
	}

	// Try to load credentials and create client
	authStore := m.getAuthStore()
	if authStore == nil {
		return nil, fmt.Errorf("auth store not initialized")
	}

	client, err := NewTuyaClient(authStore, "", "", "")
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
	m.clients = make(map[string]*TuyaClient)
}
