package homeclaw

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi"
	hcconfig "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	// AppXiaomiHome is the service ID for Xiaomi Home
	AppXiaomiHome = "XiaomiHome"
)

// XiaomiManager handles Xiaomi login operations
type XiaomiManager struct {
	mu sync.Mutex
	// auth holds the current login session for multi-step login (captcha/verify)
	auth *xiaomi.Cloud
}

// NewXiaomiManager creates a new XiaomiManager instance
func NewXiaomiManager() *XiaomiManager {
	return &XiaomiManager{}
}

// RegisterRoutes binds Xiaomi API endpoints to the ServeMux
func (m *XiaomiManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/xiaomi/status", m.handleGetStatus)
	mux.HandleFunc("POST /api/xiaomi/auth", m.handleAuth)
	mux.HandleFunc("POST /api/xiaomi/logout", m.handleLogout)
}

// handleGetStatus returns the current Xiaomi login status
func (m *XiaomiManager) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load xiaomi config from go2rtc config
	var xiaomiCfg struct {
		Cfg map[string]string `yaml:"xiaomi"`
	}
	if err := hcconfig.LoadGo2RTCConfig(&xiaomiCfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"logged_in": false,
			"error":     err.Error(),
		})
		return
	}

	// Check if there are any stored tokens
	if len(xiaomiCfg.Cfg) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"logged_in": false,
		})
		return
	}

	// Return the first user ID found
	var userID string
	for uid := range xiaomiCfg.Cfg {
		userID = uid
		break
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"logged_in": true,
		"user_id":   userID,
	})
}

// handleAuth handles all login steps (login/captcha/verify)
func (m *XiaomiManager) handleAuth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	username := r.Form.Get("username")
	password := r.Form.Get("password")
	captcha := r.Form.Get("captcha")
	verify := r.Form.Get("verify")

	var err error

	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case username != "" || password != "":
		// Start new login session
		m.auth = xiaomi.NewCloud(AppXiaomiHome)
		err = m.auth.Login(username, password)
	case captcha != "":
		// Continue with captcha
		if m.auth == nil {
			http.Error(w, "no active login session", http.StatusBadRequest)
			return
		}
		err = m.auth.LoginWithCaptcha(captcha)
	case verify != "":
		// Continue with 2FA verify
		if m.auth == nil {
			http.Error(w, "no active login session", http.StatusBadRequest)
			return
		}
		err = m.auth.LoginWithVerify(verify)
	default:
		http.Error(w, "wrong request", http.StatusBadRequest)
		return
	}

	if err == nil {
		userID, token := m.auth.UserToken()
		m.auth = nil

		// Save token to go2rtc config
		err = hcconfig.PatchGo2RTCConfig([]string{"xiaomi", userID}, token)
		if err != nil {
			logger.ErrorC("xiaomi", "Failed to save token: "+err.Error())
		}
	}

	if err != nil {
		var login *xiaomi.LoginError
		if errors.As(err, &login) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(login)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Success response
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// handleLogout clears the current login session
func (m *XiaomiManager) handleLogout(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear the auth session
	m.auth = nil

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}

// Stop clears any active login session
func (m *XiaomiManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auth = nil
}
