// Package miio 提供小米 MIoT Spec 规范获取与缓存功能
package miio

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

const (
	// SpecInstanceURL MIoT Spec instance API URL
	SpecInstanceURL = "https://miot-spec.org/miot-spec-v2/instance"
	// SpecCacheDir 本地 Spec 缓存目录
	SpecCacheDir = "xiao-spec"
	// SpecCacheEffectiveTime 缓存有效期（14天）
	SpecCacheEffectiveTime = 3600 * 24 * 14 * time.Second
)

// SpecFetcher Spec 获取器
type SpecFetcher struct {
	fileCache  *data.FileCache
	httpClient *http.Client
}

// NewSpecFetcher 创建 SpecFetcher
//
// 参数:
//   - basePath: 项目根目录，用于构建本地缓存路径
func NewSpecFetcher(basePath string) (*SpecFetcher, error) {
	cacheDir := filepath.Join(basePath, SpecCacheDir)

	fileCache, err := data.NewFileCache(data.FileCacheConfig{
		CacheDir:  cacheDir,
		TTL:       SpecCacheEffectiveTime,
		EncodeKey: true, // URN contains special characters like ":"
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create file cache: %w", err)
	}

	return &SpecFetcher{
		fileCache:  fileCache,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// GetSpec 获取设备 Spec JSON
//
// 优先从本地缓存读取,如果没有则从云端获取并缓存
// 参数:
//   - urn: 设备 URN (如: urn:miot-spec-v2:device:light:0000A001:yeelink-v1)
//
// 返回:
//   - string: Spec JSON 原始数据
//   - error: 错误信息
func (f *SpecFetcher) GetSpec(urn string) (string, error) {
	if urn == "" {
		return "", fmt.Errorf("urn is empty")
	}

	// 1. 检查文件缓存
	data, err := f.fileCache.GetAsString(urn)
	if err == nil && data != "" {
		return data, nil
	}

	// 2. 从云端获取
	data, err = f.fetchFromCloud(urn)
	if err != nil {
		return "", fmt.Errorf("fetch spec from cloud failed: %w", err)
	}

	// 3. 保存到本地缓存
	if err := f.fileCache.SetString(urn, data); err != nil {
		// 缓存失败不返回错误,继续执行
		_ = err
	}

	return data, nil
}

// fetchFromCloud 从云端获取 Spec
func (f *SpecFetcher) fetchFromCloud(urn string) (string, error) {
	url := fmt.Sprintf("%s?type=%s", SpecInstanceURL, urn)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SaveProcessedSpec saves the processed spec (device commands JSON) to _{mode}_new.json file
// This stores the simplified/processed version of the spec for quick access
// Parameters:
//   - urn: device URN
//   - mode: "read" or "write", determines file suffix (read_new.json or write_new.json)
//   - processedJSON: processed spec JSON string
func (f *SpecFetcher) SaveProcessedSpec(urn string, mode string, processedJSON string) error {
	if urn == "" {
		return fmt.Errorf("urn is empty")
	}

	key := urn + "_" + mode + "_new"
	return f.fileCache.SetString(key, processedJSON)
}

// GetProcessedSpec reads the processed spec from _{mode}_new.json file
// Returns the processed device commands JSON if available
// Parameters:
//   - urn: device URN
//   - mode: "read" or "write", determines file suffix (read_new.json or write_new.json)
func (f *SpecFetcher) GetProcessedSpec(urn string, mode string) (string, error) {
	if urn == "" {
		return "", fmt.Errorf("urn is empty")
	}

	key := urn + "_" + mode + "_new"
	return f.fileCache.GetAsString(key)
}

// ClearCache 清除所有缓存
func (f *SpecFetcher) ClearCache() error {
	return f.fileCache.Clear()
}
