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
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio"
	midata "github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/data"
)

// ThirdFactory is the central factory for creating and managing third-party
// smart home platform components. It follows the singleton pattern for components
// that should exist only once per application lifecycle.
type ThirdFactory struct {
	Workspace string
	cfg       *config.Config
	hcfg      *hcc.HomeclawConfig
	// Singleton instances - lazy loaded
	jsonStore     *hcd.JSONStore
	miDeviceStore midata.MiDeviceStore
	cloud         *xiaomi.Cloud
	miClient      *miio.MiClient
	specFetcher   *miio.SpecFetcher

	// Initialization tracking
	storeOnce sync.Once
	storeErr  error
}

// NewThirdFactory creates a new ThirdFactory instance.
// workspace is the data root used for all third-party data files.
func NewThirdFactory(workspace string, cfg *config.Config, hcfg *hcc.HomeclawConfig) *ThirdFactory {
	return &ThirdFactory{
		Workspace: workspace,
		cfg:       cfg,
		hcfg:      hcfg,
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

	f.miClient = miio.NewMiClient(cloud, country, f.Workspace, deviceStore)
	return f.miClient, nil
}
