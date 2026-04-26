package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/web/backend/homeclaw"
	"github.com/sipeed/picoclaw/web/backend/launcherconfig"
)

// Handler serves HTTP API requests.

type Handler struct {
	configPath           string
	workspacePath        string
	serverPort           int
	serverPublic         bool
	serverPublicExplicit bool
	serverHostInput      string
	serverHostExplicit   bool
	serverCIDRs          []string
	debug                bool
	oauthMu              sync.Mutex
	oauthFlows           map[string]*oauthFlow
	oauthState           map[string]string
	go2rtcManager        *homeclaw.Go2RTCManager
	tuyaManager          *homeclaw.TuyaManager
	xiaomiManager        *homeclaw.XiaomiManager
	homekitManager       *homeclaw.HomeKitManager
	deviceOpsManager     *homeclaw.DeviceOpsManager
	weixinMu             sync.Mutex
	weixinFlows          map[string]*weixinFlow
	wecomMu              sync.Mutex
	wecomFlows           map[string]*wecomFlow
	deviceStoreInitOnce  sync.Once
	deviceStore          data.DeviceStore
	deviceStoreErr       error
}

// NewHandler creates an instance of the API handler.
func NewHandler(configPath string) *Handler {
	h := &Handler{
		configPath:       configPath,
		serverPort:       launcherconfig.DefaultPort,
		oauthFlows:       make(map[string]*oauthFlow),
		oauthState:       make(map[string]string),
		go2rtcManager:    homeclaw.NewGo2RTCManager(),
		tuyaManager:      homeclaw.NewTuyaManager(),
		xiaomiManager:    homeclaw.NewXiaomiManager(),
		deviceOpsManager: homeclaw.NewDeviceOpsManager(),
		weixinFlows:      make(map[string]*weixinFlow),
		wecomFlows:       make(map[string]*wecomFlow),
	}

	// Derive workspace path from config
	if configPath != "" {
		if cfg, err := config.LoadConfig(configPath); err == nil {
			h.workspacePath = cfg.WorkspacePath()
		} else {
			// Fallback to config directory if config loading fails
			h.workspacePath = filepath.Dir(configPath)
		}
	}

	return h
}

// SetServerOptions stores current backend listen options for fallback behavior.
func (h *Handler) SetServerOptions(port int, public bool, publicExplicit bool, allowedCIDRs []string) {
	h.serverPort = port
	h.serverPublic = public
	h.serverPublicExplicit = publicExplicit
	h.serverHostInput = ""
	h.serverHostExplicit = false
	h.serverCIDRs = append([]string(nil), allowedCIDRs...)
}

// SetServerBindHost stores the launcher's effective bind host.
// When explicit is true, hostInput is the normalized -host / PICOCLAW_LAUNCHER_HOST value.
func (h *Handler) SetServerBindHost(hostInput string, explicit bool) {
	h.serverHostInput = strings.TrimSpace(hostInput)
	if !explicit {
		h.serverHostInput = ""
	}
	h.serverHostExplicit = explicit
}

func (h *Handler) SetDebug(debug bool) {
	h.debug = debug
}

// initHomeKitManager lazily initializes the HomeKit manager with DeviceStore
func (h *Handler) initHomeKitManager() {
	h.deviceStoreInitOnce.Do(func() {
		if h.workspacePath == "" {
			return
		}

		// Initialize JSONStore
		dataDir := filepath.Join(h.workspacePath, "data")
		jsonStore, err := data.NewJSONStore(dataDir)
		if err != nil {
			h.deviceStoreErr = fmt.Errorf("json store init failed: %w", err)
			return
		}

		// Initialize DeviceStore
		h.deviceStore, h.deviceStoreErr = data.NewDeviceStore(jsonStore)
		if h.deviceStoreErr != nil {
			h.deviceStoreErr = fmt.Errorf("device store init failed: %w", h.deviceStoreErr)
			return
		}

		// Initialize HomeKitManager with DeviceStore
		h.homekitManager = homeclaw.NewHomeKitManager(h.deviceStore, h.workspacePath)
	})
}

// RegisterRoutes binds all API endpoint handlers to the ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Config CRUD
	h.registerConfigRoutes(mux)

	// Pico Channel (WebSocket chat)
	h.registerPicoRoutes(mux)

	// Gateway process lifecycle
	h.registerGatewayRoutes(mux)

	// Go2RTC process lifecycle (homeclaw)
	h.go2rtcManager.RegisterRoutes(mux)

	// Tuya API endpoints
	h.tuyaManager.RegisterRoutes(mux)

	// Xiaomi API endpoints
	h.xiaomiManager.RegisterRoutes(mux)

	// HomeKit API endpoints (lazy init)
	h.initHomeKitManager()
	if h.homekitManager != nil {
		h.homekitManager.RegisterRoutes(mux)
	}

	// DeviceOps API endpoints (lazy init)
	if h.workspacePath != "" {
		h.deviceOpsManager.Initialize(h.workspacePath)
		h.deviceOpsManager.RegisterRoutes(mux)
	}

	// Session history
	h.registerSessionRoutes(mux)

	// OAuth login and credential management
	h.registerOAuthRoutes(mux)

	// Model list management
	h.registerModelRoutes(mux)

	// Channel catalog (for frontend navigation/config pages)
	h.registerChannelRoutes(mux)

	// Skills and tools support/actions
	h.registerSkillRoutes(mux)
	h.registerToolRoutes(mux)

	// OS startup / launch-at-login
	h.registerStartupRoutes(mux)

	// Launcher service parameters (port/public)
	h.registerLauncherConfigRoutes(mux)

	// Self-update endpoint (requires dashboard auth)
	h.registerUpdateRoutes(mux)

	// Runtime build/version metadata
	h.registerVersionRoutes(mux)

	// WeChat QR login flow
	h.registerWeixinRoutes(mux)

	// WeCom QR login flow
	h.registerWecomRoutes(mux)
}

// Shutdown gracefully shuts down the handler, stopping the gateway and go2rtc if they were started by this handler.
func (h *Handler) Shutdown() {
	h.StopGateway()
	h.go2rtcManager.Stop()
	h.tuyaManager.Stop()
	h.xiaomiManager.Stop()
}

// TryAutoStartGo2RTC delegates to the Go2RTCManager to auto-start go2rtc.
func (h *Handler) TryAutoStartGo2RTC() {
	h.go2rtcManager.TryAutoStart()
}
