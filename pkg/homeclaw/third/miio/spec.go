// Package miio 提供小米 MIoT Spec 规范获取与缓存功能
package miio

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// SpecInstanceURL MIoT Spec instance API URL
	SpecInstanceURL = "https://miot-spec.org/miot-spec-v2/instance"
	// SpecCacheDir 本地 Spec 缓存目录
	SpecCacheDir = "spec"
	// SpecCacheEffectiveTime 缓存有效期（14天）
	SpecCacheEffectiveTime = 3600 * 24 * 14 * time.Second
)

// SpecFetcher Spec 获取器
type SpecFetcher struct {
	cacheDir    string
	httpClient  *http.Client
	memoryCache map[string]*cacheEntry
	mu          sync.RWMutex
}

// cacheEntry 内存缓存条目
type cacheEntry struct {
	data      string
	timestamp time.Time
}

// NewSpecFetcher 创建 SpecFetcher
//
// 参数:
//   - basePath: 项目根目录，用于构建本地缓存路径
func NewSpecFetcher(basePath string) *SpecFetcher {
	return &SpecFetcher{
		cacheDir:    filepath.Join(basePath, SpecCacheDir),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		memoryCache: make(map[string]*cacheEntry),
	}
}

// GetSpec 获取设备 Spec JSON
//
// 优先从本地缓存读取，如果没有则从云端获取并缓存
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

	// 1. 检查内存缓存
	f.mu.RLock()
	if entry, ok := f.memoryCache[urn]; ok {
		if time.Since(entry.timestamp) < SpecCacheEffectiveTime {
			f.mu.RUnlock()
			return entry.data, nil
		}
	}
	f.mu.RUnlock()

	// 2. 检查本地文件缓存
	data, err := f.loadFromLocalCache(urn)
	if err == nil && data != "" {
		// 更新内存缓存
		f.mu.Lock()
		f.memoryCache[urn] = &cacheEntry{
			data:      data,
			timestamp: time.Now(),
		}
		f.mu.Unlock()
		return data, nil
	}

	// 3. 从云端获取
	data, err = f.fetchFromCloud(urn)
	if err != nil {
		return "", fmt.Errorf("fetch spec from cloud failed: %w", err)
	}

	// 4. 保存到本地缓存
	if err := f.saveToLocalCache(urn, data); err != nil {
		// 缓存失败不返回错误，继续执行
		_ = err
	}

	// 5. 更新内存缓存
	f.mu.Lock()
	f.memoryCache[urn] = &cacheEntry{
		data:      data,
		timestamp: time.Now(),
	}
	f.mu.Unlock()

	return data, nil
}

// loadFromLocalCache 从本地缓存加载 Spec
func (f *SpecFetcher) loadFromLocalCache(urn string) (string, error) {
	filename := f.getCacheFilename(urn)
	filepath := filepath.Join(f.cacheDir, filename)

	// 检查文件是否存在
	info, err := os.Stat(filepath)
	if err != nil {
		return "", err
	}

	// 检查缓存是否过期
	if time.Since(info.ModTime()) > SpecCacheEffectiveTime {
		return "", fmt.Errorf("cache expired")
	}

	// 读取文件
	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// saveToLocalCache 保存 Spec 到本地缓存
func (f *SpecFetcher) saveToLocalCache(urn string, data string) error {
	// 确保目录存在
	if err := os.MkdirAll(f.cacheDir, 0755); err != nil {
		return err
	}

	filename := f.getCacheFilename(urn)
	filepath := filepath.Join(f.cacheDir, filename)

	// 直接写入原始 JSON 数据
	return os.WriteFile(filepath, []byte(data), 0644)
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

// getCacheFilename 获取缓存文件名
// 使用 URN 的 base64 编码作为文件名
func (f *SpecFetcher) getCacheFilename(urn string) string {
	// 将 URN 进行 base64 编码
	encoded := base64.URLEncoding.EncodeToString([]byte(urn))
	return encoded + ".json"
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

	// Ensure directory exists
	if err := os.MkdirAll(f.cacheDir, 0755); err != nil {
		return err
	}

	filename := f.getProcessedCacheFilename(urn, mode)
	filepath := filepath.Join(f.cacheDir, filename)

	return os.WriteFile(filepath, []byte(processedJSON), 0644)
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

	filename := f.getProcessedCacheFilename(urn, mode)
	filepath := filepath.Join(f.cacheDir, filename)

	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getProcessedCacheFilename returns the filename for processed spec cache
// Uses URN's base64 encoding + "_{mode}_new.json" suffix (e.g., "_read_new.json" or "_write_new.json")
func (f *SpecFetcher) getProcessedCacheFilename(urn string, mode string) string {
	encoded := base64.URLEncoding.EncodeToString([]byte(urn))
	return encoded + "_" + mode + "_new.json"
}

// ClearCache 清除所有缓存
func (f *SpecFetcher) ClearCache() error {
	f.mu.Lock()
	f.memoryCache = make(map[string]*cacheEntry)
	f.mu.Unlock()

	// 删除本地缓存文件
	entries, err := os.ReadDir(f.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			if err := os.Remove(filepath.Join(f.cacheDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}
