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
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio"
	midata "github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/data"
	mitool "github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/tool"
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
	jsonStore           *hcd.JSONStore
	miDeviceStore       midata.MiDeviceStore
	miHomeStore         midata.MiHomeStore
	cloud               *xiaomi.Cloud
	miClient            *miio.MiClient
	specFetcher         *miio.SpecFetcher
	syncHomesTool       *mitool.SyncHomesTool
	syncDevicesTool     *mitool.SyncDevicesTool
	executeActionTool   *mitool.ExecuteActionTool
	getSpecCommandsTool *mitool.GetSpecCommandsTool

	// Initialization tracking
	storeOnce sync.Once
	storeErr  error
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

// GetMiDeviceStore returns the singleton MiDeviceStore instance (lazy initialized).
func (f *ThirdFactory) GetMiDeviceStore() (midata.MiDeviceStore, error) {
	if f.miDeviceStore != nil {
		return f.miDeviceStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, fmt.Errorf("get json store: %w", err)
	}

	f.miDeviceStore, err = midata.NewMiDeviceStore(store)
	if err != nil {
		return nil, fmt.Errorf("mi device store init failed: %w", err)
	}
	return f.miDeviceStore, nil
}

// GetMiHomeStore returns the singleton MiHomeStore instance (lazy initialized).
func (f *ThirdFactory) GetMiHomeStore() (midata.MiHomeStore, error) {
	if f.miHomeStore != nil {
		return f.miHomeStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, fmt.Errorf("get json store: %w", err)
	}

	f.miHomeStore, err = midata.NewMiHomeStore(store)
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
func (f *ThirdFactory) GetSpecFetcher() *miio.SpecFetcher {
	if f.specFetcher != nil {
		return f.specFetcher
	}
	f.specFetcher = miio.NewSpecFetcher(filepath.Join(f.Workspace, "third"))
	return f.specFetcher
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

	f.miClient = miio.NewMiClient(cloud, country, f.Workspace, deviceStore, homeStore, f.GetSpecFetcher())
	return f.miClient, nil
}

// GetSyncHomesTool returns the singleton SyncHomesTool instance (lazy initialized).
func (f *ThirdFactory) GetSyncHomesTool() (*mitool.SyncHomesTool, error) {
	if f.syncHomesTool != nil {
		return f.syncHomesTool, nil
	}
	country := "cn"
	client, err := f.GetMiClient(country)
	if err != nil {
		return nil, fmt.Errorf("get mi client: %w", err)
	}

	homeStore, err := f.factory.GetHomeStore()
	if err != nil {
		return nil, fmt.Errorf("get home store: %w", err)
	}

	f.syncHomesTool, err = mitool.NewSyncHomesTool(client, homeStore)
	if err != nil {
		return nil, fmt.Errorf("create sync homes tool: %w", err)
	}
	return f.syncHomesTool, nil
}

// GetSyncDevicesTool returns the singleton SyncDevicesTool instance (lazy initialized).
func (f *ThirdFactory) GetSyncDevicesTool() (*mitool.SyncDevicesTool, error) {
	if f.syncDevicesTool != nil {
		return f.syncDevicesTool, nil
	}
	country := "cn"
	client, err := f.GetMiClient(country)
	if err != nil {
		return nil, fmt.Errorf("get mi client: %w", err)
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

	f.syncDevicesTool = mitool.NewSyncDevicesTool(client, homeStore, spaceStore, deviceStore, f.GetSpecFetcher())
	return f.syncDevicesTool, nil
}

// GetExecuteActionTool returns the singleton ExecuteActionTool instance (lazy initialized).
func (f *ThirdFactory) GetExecuteActionTool() (*mitool.ExecuteActionTool, error) {
	if f.executeActionTool != nil {
		return f.executeActionTool, nil
	}
	country := "cn"
	client, err := f.GetMiClient(country)
	if err != nil {
		return nil, fmt.Errorf("get mi client: %w", err)
	}

	f.executeActionTool = mitool.NewExecuteActionTool(client)
	return f.executeActionTool, nil
}

// GetSpecCommandsTool returns the singleton GetSpecCommandsTool instance (lazy initialized).
func (f *ThirdFactory) GetSpecCommandsTool() (*mitool.GetSpecCommandsTool, error) {
	if f.getSpecCommandsTool != nil {
		return f.getSpecCommandsTool, nil
	}
	f.getSpecCommandsTool = mitool.NewGetSpecCommandsTool(f.GetSpecFetcher())
	return f.getSpecCommandsTool, nil
}
