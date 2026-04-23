// Package ioc provides the ThirdFactory for creating and managing
// third-party smart home platform components (e.g., Xiaomi MIoT).
package ioc

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi"
	"github.com/sipeed/picoclaw/pkg/config"
	hcc "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	hcd "github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/ioc"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/tuya"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// ThirdFactory is the central factory for creating and managing third-party
// smart home platform components. It follows the singleton pattern for components
// that should exist only once per application lifecycle.
type ThirdFactory struct {
	Workspace string
	cfg       *config.Config
	hcfg      *hcc.HomeclawConfig
	factory   *ioc.Factory
	// Singleton instances - lazy loaded
	jsonStore     *hcd.JSONStore
	miDeviceStore miio.MiDeviceStore
	miHomeStore   miio.MiHomeStore
	cloud         *xiaomi.Cloud
	miClient      *miio.MiClient
	tuyaClient    *tuya.TuyaClient
	clients       map[string]third.Client
	clientsMu     sync.Mutex

	// Initialization tracking
	storeOnce      sync.Once
	storeErr       error
	tuyaClientOnce sync.Once
	tuyaClientErr  error
}

// NewThirdFactory creates a new ThirdFactory instance.
// workspace is the data root used for all third-party data files.
func NewThirdFactory(factory *ioc.Factory) *ThirdFactory {
	return &ThirdFactory{
		Workspace: factory.Workspace,
		cfg:       factory.Cfg,
		hcfg:      factory.Hcfg,
		factory:   factory,
	}
}

// GetJSONStore returns the singleton JSONStore instance (lazy initialized).
func (f *ThirdFactory) GetJSONStore() (*hcd.JSONStore, error) {
	f.storeOnce.Do(func() {
		f.jsonStore, f.storeErr = hcd.NewJSONStore(filepath.Join(f.Workspace, "third"))
	})
	return f.jsonStore, f.storeErr
}

// GetAuthStore returns the singleton AuthStore instance (lazy initialized).
// It delegates to the main factory's GetAuthStore.
func (f *ThirdFactory) GetAuthStore() (hcd.AuthStore, error) {
	return f.factory.GetAuthStore()
}

// GetMiDeviceStore returns the singleton MiDeviceStore instance (lazy initialized).
func (f *ThirdFactory) GetMiDeviceStore() (miio.MiDeviceStore, error) {
	if f.miDeviceStore != nil {
		return f.miDeviceStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, fmt.Errorf("get json store: %w", err)
	}

	f.miDeviceStore, err = miio.NewMiDeviceStore(store)
	if err != nil {
		return nil, fmt.Errorf("mi device store init failed: %w", err)
	}
	return f.miDeviceStore, nil
}

// GetMiHomeStore returns the singleton MiHomeStore instance (lazy initialized).
func (f *ThirdFactory) GetMiHomeStore() (miio.MiHomeStore, error) {
	if f.miHomeStore != nil {
		return f.miHomeStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, fmt.Errorf("get json store: %w", err)
	}

	f.miHomeStore, err = miio.NewMiHomeStore(store)
	if err != nil {
		return nil, fmt.Errorf("mi home store init failed: %w", err)
	}
	return f.miHomeStore, nil
}

// GetCloud returns the singleton Cloud instance (lazy initialized).
// The sid parameter defaults to "xiaomiio" if empty.
func (f *ThirdFactory) GetCloud(sid string) *xiaomi.Cloud {
	if f.cloud != nil {
		return f.cloud
	}
	if sid == "" {
		sid = "xiaomiio"
	}
	f.cloud = xiaomi.NewCloud(sid)
	var Xiaomi struct {
		Cfg map[string]string `yaml:"xiaomi"`
	}

	hcc.LoadGo2RTCConfig(&Xiaomi)

	// Get first key-value pair: userId=key, token=value
	var userId, token string
	for k, v := range Xiaomi.Cfg {
		userId = k
		token = v
		break
	}
	f.cloud.LoginWithToken(userId, token)
	return f.cloud
}

// GetMiClient returns the singleton MiClient instance (lazy initialized).
//
// Parameters:
//   - country: region code (cn, de, ru, sg, i2, us, etc.)
func (f *ThirdFactory) GetMiClient(country string) (*miio.MiClient, error) {
	if f.miClient != nil {
		return f.miClient, nil
	}

	cloud := f.GetCloud("xiaomiio")

	deviceStore, err := f.GetMiDeviceStore()
	if err != nil {
		return nil, fmt.Errorf("get mi device store: %w", err)
	}

	homeStore, err := f.GetMiHomeStore()
	if err != nil {
		return nil, fmt.Errorf("get mi home store: %w", err)
	}

	// Use workspace/third path for xiao-spec cache (consistent with homekit-spec)
	cacheBase := ""
	if f.Workspace != "" {
		cacheBase = filepath.Join(f.Workspace, "third")
	}

	f.miClient = miio.NewMiClient(cloud, country, cacheBase, deviceStore, homeStore)
	return f.miClient, nil
}

// GetTuyaClient returns the singleton TuyaClient instance (lazy initialized).
// It reads the API token from the AuthStore.
// Returns nil, nil if no token is configured.
func (f *ThirdFactory) GetTuyaClient() (*tuya.TuyaClient, error) {
	f.tuyaClientOnce.Do(func() {
		authStore, err := f.GetAuthStore()
		if err != nil {
			f.tuyaClientErr = fmt.Errorf("get auth store: %w", err)
			return
		}

		// Read token if available
		var token string
		if authStore.Exists("tuya_token") {
			_, _, token, _, err = authStore.GetDecryptedBrand("tuya_token")
			if err != nil {
				f.tuyaClientErr = fmt.Errorf("decrypt tuya token: %w", err)
				return
			}
		}

		// Get email, password and region from AuthStore (optional)
		var email, password, region string
		if authStore.Exists("tuya_pass") {
			region, email, password, _, err = authStore.GetDecryptedBrand("tuya_pass")
			if err != nil {
				// Log warning but continue with empty credentials
				email = ""
				password = ""
				region = ""
			}
		}

		store, err := f.GetJSONStore()
		if err != nil {
			f.tuyaClientErr = fmt.Errorf("get json store: %w", err)
			return
		}

		f.tuyaClient, f.tuyaClientErr = tuya.NewTuyaClient(store, token, email, password, region)
		if f.tuyaClientErr != nil {
			return
		}

		// Set auth store to enable lazy token loading
		f.tuyaClient.SetAuthStore(authStore)
	})
	return f.tuyaClient, f.tuyaClientErr
}

// ResetTuyaClient clears the cached Tuya client so it can be recreated with fresh credentials.
// This should be called after saving or deleting Tuya authentication.
func (f *ThirdFactory) ResetTuyaClient() {
	f.tuyaClientOnce = sync.Once{}
	f.tuyaClient = nil
	f.tuyaClientErr = nil
}

// SetClients builds and sets brand clients (xiaomi, tuya, …) for CLI and LLM tools.
// This method initializes all available brand clients and injects them into the tools.
func (f *ThirdFactory) SetClients() error {
	f.clientsMu.Lock()
	defer f.clientsMu.Unlock()

	// Reset Tuya client to allow re-initialization with fresh credentials
	f.ResetTuyaClient()

	clients := make(map[string]third.Client)

	// Register Xiaomi client if available
	miClient, miErr := f.GetMiClient("cn")
	if miErr == nil && miClient != nil {
		logger.Debug("set xiaomi client-------------------------------")
		clients[miClient.Brand()] = miClient
	}

	// Register Tuya client if available
	tuyaClient, tuyaErr := f.GetTuyaClient()
	if tuyaErr != nil {
		logger.Warnf("init tuya client err %v -----------------------", tuyaErr)
	} else if tuyaClient == nil {
		logger.Debug("tuya client is nil (unexpected), skipping-------------------------------")
	} else if tuyaClient.GetAPIKey() == "" {
		logger.Debug("tuya client created without token (not yet configured), registered for later use-------------------------------")
		clients[tuyaClient.Brand()] = tuyaClient
	} else {
		logger.Debug("set tuya client-------------------------------")
		clients[tuyaClient.Brand()] = tuyaClient
	}

	if len(clients) == 0 {
		logger.Debug("no brand clients configured-------------------------------")
	}

	f.clients = clients

	// Inject clients into CLI tool and LLM tool
	f.factory.SetCLIToolClients(clients)

	if llmTool, err := f.factory.GetLLMTool(); err == nil && llmTool != nil {
		llmTool.SetClients(clients)
	}

	return nil
}

// GetClients returns the map of brand clients.
func (f *ThirdFactory) GetClients() map[string]third.Client {
	f.clientsMu.Lock()
	defer f.clientsMu.Unlock()
	return f.clients
}
