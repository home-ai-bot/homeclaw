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
	homeclawtool "github.com/sipeed/picoclaw/pkg/homeclaw/tool"
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
	jsonStore       *hcd.JSONStore
	rootJSONStore   *hcd.JSONStore
	miDeviceStore   miio.MiDeviceStore
	miHomeStore     miio.MiHomeStore
	cloud           *xiaomi.Cloud
	miClient        *miio.MiClient
	specFetcher     *miio.SpecFetcher
	tuyaTokenStore  tuya.TokenStore
	tuyaSecretStore tuya.SecretStore
	tuyaClient      *tuya.TuyaClient
	cliTool         *homeclawtool.CLITool
	cliToolMu       sync.Mutex

	// Initialization tracking
	storeOnce      sync.Once
	storeErr       error
	rootStoreOnce  sync.Once
	rootStoreErr   error
	tuyaTokenOnce  sync.Once
	tuyaTokenErr   error
	tuyaSecretOnce sync.Once
	tuyaSecretErr  error
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

// GetRootJSONStore returns the singleton root JSONStore instance (lazy initialized).
// It points to picoclawHome/tuya, which is the same directory used by the web backend TuyaManager.
func (f *ThirdFactory) GetRootJSONStore() (*hcd.JSONStore, error) {
	f.rootStoreOnce.Do(func() {
		f.rootJSONStore, f.rootStoreErr = hcd.NewJSONStore(filepath.Join(hcc.GetPicoclawHome(), "tuya"))
	})
	return f.rootJSONStore, f.rootStoreErr
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

// GetSpecFetcher returns the singleton SpecFetcher instance (lazy initialized).
func (f *ThirdFactory) GetSpecFetcher() (*miio.SpecFetcher, error) {
	if f.specFetcher != nil {
		return f.specFetcher, nil
	}
	fetcher, err := miio.NewSpecFetcher(filepath.Join(f.Workspace, "third"))
	if err != nil {
		return nil, fmt.Errorf("failed to create spec fetcher: %w", err)
	}
	f.specFetcher = fetcher
	return f.specFetcher, nil
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

	specFetcher, err := f.GetSpecFetcher()
	if err != nil {
		return nil, fmt.Errorf("get spec fetcher: %w", err)
	}

	f.miClient = miio.NewMiClient(cloud, country, f.Workspace, deviceStore, homeStore, specFetcher)
	return f.miClient, nil
}

// GetTuyaTokenStore returns the singleton TokenStore instance (lazy initialized).
func (f *ThirdFactory) GetTuyaTokenStore() (tuya.TokenStore, error) {
	f.tuyaTokenOnce.Do(func() {
		store, err := f.GetRootJSONStore()
		if err != nil {
			f.tuyaTokenErr = fmt.Errorf("get json store: %w", err)
			return
		}

		f.tuyaTokenStore, f.tuyaTokenErr = tuya.NewTokenStore(store)
	})
	return f.tuyaTokenStore, f.tuyaTokenErr
}

// GetTuyaSecretStore returns the singleton SecretStore instance (lazy initialized).
func (f *ThirdFactory) GetTuyaSecretStore() (tuya.SecretStore, error) {
	f.tuyaSecretOnce.Do(func() {
		store, err := f.GetRootJSONStore()
		if err != nil {
			f.tuyaSecretErr = fmt.Errorf("get json store: %w", err)
			return
		}

		f.tuyaSecretStore, f.tuyaSecretErr = tuya.NewSecretStore(store)
	})
	return f.tuyaSecretStore, f.tuyaSecretErr
}

// GetTuyaClient returns the singleton TuyaClient instance (lazy initialized).
// It reads the API token from the shared JSONStore via TokenStore.
// Returns nil, nil if no token is configured.
func (f *ThirdFactory) GetTuyaClient() (*tuya.TuyaClient, error) {
	f.tuyaClientOnce.Do(func() {
		tokenStore, err := f.GetTuyaTokenStore()
		if err != nil {
			f.tuyaClientErr = fmt.Errorf("get tuya token store: %w", err)
			return
		}

		// Read token if available; empty token is allowed — the client can be
		// configured later via SetAPIKey once the user provides credentials.
		var token string
		if tokenStore.Exists() {
			token, err = tokenStore.GetDecrypted()
			if err != nil {
				f.tuyaClientErr = fmt.Errorf("decrypt tuya token: %w", err)
				return
			}
		}

		// Get email, password and region from SecretStore (optional)
		var email, password, region string
		secretStore, err := f.GetTuyaSecretStore()
		if err == nil && secretStore.Exists() {
			region, email, password, err = secretStore.GetDecrypted()
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
	})
	return f.tuyaClient, f.tuyaClientErr
}

// GetCLITool returns the singleton CLITool instance (lazy initialized).
// It registers all configured brand clients (xiaomi, tuya, …) into the tool.
func (f *ThirdFactory) GetCLITool() (*homeclawtool.CLITool, error) {
	f.cliToolMu.Lock()
	defer f.cliToolMu.Unlock()
	logger.Debug("begin init CLITool-------------------------------")
	if f.cliTool != nil {
		return f.cliTool, nil
	}

	homeStore, err := f.factory.GetHomeStore()
	if err != nil {
		return nil, fmt.Errorf("get home store: %w", err)
	}

	spaceStore, err := f.factory.GetSpaceStore()
	if err != nil {
		return nil, fmt.Errorf("get space store: %w", err)
	}

	deviceStore, err := f.factory.GetDeviceStore()
	if err != nil {
		return nil, fmt.Errorf("get device store: %w", err)
	}

	deviceOpStore, err := f.factory.GetDeviceOpStore()
	if err != nil {
		return nil, fmt.Errorf("get device op store: %w", err)
	}

	clients := make(map[string]third.Client)

	// Register Xiaomi client if available
	miClient, miErr := f.GetMiClient("cn")
	if miErr == nil && miClient != nil {
		logger.Debug("set xiaomi to CLITool-------------------------------")
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
		logger.Debug("set tuya to CLITool-------------------------------")
		clients[tuyaClient.Brand()] = tuyaClient
	}

	if len(clients) == 0 {
		// No brand configured yet; still create the tool so it can be populated later
	}

	f.cliTool = homeclawtool.NewCLITool(clients, homeStore, spaceStore, deviceStore, deviceOpStore)
	return f.cliTool, nil
}
