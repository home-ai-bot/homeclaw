// Package miio 提供小米 MIoT 统一客户端实现
// 对应 Python 版 miot/miot_client.py 中的 MIoTClient
// 整合云端 HTTP、本地网关 MIPS、局域网 LAN 三种控制方式
package miio

import (
	"fmt"
	"sync"
	"time"
)

// CtrlMode 控制模式
type CtrlMode int

const (
	// CtrlModeAuto 自动模式（本地优先）
	CtrlModeAuto CtrlMode = iota
	// CtrlModeCloud 仅云端模式
	CtrlModeCloud
)

// String 返回控制模式字符串
func (m CtrlMode) String() string {
	switch m {
	case CtrlModeAuto:
		return "auto"
	case CtrlModeCloud:
		return "cloud"
	default:
		return "unknown"
	}
}

// ParseCtrlMode 解析控制模式字符串
func ParseCtrlMode(mode string) (CtrlMode, error) {
	switch mode {
	case "auto":
		return CtrlModeAuto, nil
	case "cloud":
		return CtrlModeCloud, nil
	default:
		return CtrlModeAuto, fmt.Errorf("unknown ctrl mode: %s", mode)
	}
}

// MClientConfig MClient 配置
type MClientConfig struct {
	EntryID          string
	UID              string
	CloudServer      string
	CtrlMode         CtrlMode
	OAuthRedirectURL string
	UUID             string
	VirtualDID       string
	HomeSelected     map[string]*HomeInfo
}

// HomeInfo 家庭信息
type HomeInfo struct {
	HomeID   string `json:"home_id"`
	HomeName string `json:"home_name"`
	GroupID  string `json:"group_id"`
}

// GatewayDeviceInfo 网关设备信息
type GatewayDeviceInfo struct {
	*DeviceInfo
	SpecV2Access bool   `json:"specv2_access"`
	GroupID      string `json:"group_id"`
}

// LANDeviceInfo 局域网设备信息
type LANDeviceInfo struct {
	*DeviceInfo
	PushAvailable bool `json:"push_available"`
}

// MClient MIoT 统一客户端
// 整合云端 HTTP、本地网关 MIPS、局域网 LAN 三种控制方式
type MClient struct {
	config *MClientConfig

	// 子客户端
	oauth     *MIoTOauthClient
	http      *CloudClient
	mipsCloud *MipsCloudClient
	mipsLocal map[string]*MipsLocalClient // key=group_id
	lan       *LANClient

	// 设备列表
	deviceListCache   map[string]*DeviceInfo
	deviceListCloud   map[string]*DeviceInfo
	deviceListGateway map[string]*GatewayDeviceInfo
	deviceListLAN     map[string]*LANDeviceInfo
	devicesMu         sync.RWMutex

	// 网络状态
	networkStatus   bool
	networkStatusMu sync.RWMutex

	// 状态订阅
	deviceStateSubs map[string][]func(did string, state map[string]interface{})
	stateSubsMu     sync.RWMutex

	// 定时器
	refreshTokenTimer *time.Timer
	refreshCertTimer  *time.Timer
	refreshPropsTimer *time.Timer
	timersMu          sync.Mutex

	// 刷新属性队列
	refreshPropsList map[string]*PropKey
	refreshPropsMu   sync.Mutex

	// 日志
	logger Logger
}

// PropKey 属性键
type PropKey struct {
	DID  string
	SIID int
	PIID int
}

// NewMClient 创建 MIoT 统一客户端
func NewMClient(config *MClientConfig) (*MClient, error) {
	if config.UID == "" || config.CloudServer == "" {
		return nil, fmt.Errorf("uid and cloud_server are required")
	}

	client := &MClient{
		config:            config,
		mipsLocal:         make(map[string]*MipsLocalClient),
		deviceListCache:   make(map[string]*DeviceInfo),
		deviceListCloud:   make(map[string]*DeviceInfo),
		deviceListGateway: make(map[string]*GatewayDeviceInfo),
		deviceListLAN:     make(map[string]*LANDeviceInfo),
		deviceStateSubs:   make(map[string][]func(did string, state map[string]interface{})),
		refreshPropsList:  make(map[string]*PropKey),
	}

	return client, nil
}

// SetLogger 设置日志器
func (c *MClient) SetLogger(logger Logger) {
	c.logger = logger
}

// logDebug 记录调试日志
func (c *MClient) logDebug(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Debug(fmt.Sprintf("[MClient:%s] ", c.config.UID)+msg, args...)
	}
}

// logInfo 记录信息日志
func (c *MClient) logInfo(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Info(fmt.Sprintf("[MClient:%s] ", c.config.UID)+msg, args...)
	}
}

// logError 记录错误日志
func (c *MClient) logError(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Error(fmt.Sprintf("[MClient:%s] ", c.config.UID)+msg, args...)
	}
}

// Init 初始化客户端
func (c *MClient) Init(oauth *MIoTOauthClient, http *CloudClient, mipsCloud *MipsCloudClient, lan *LANClient) error {
	c.oauth = oauth
	c.http = http
	c.mipsCloud = mipsCloud
	c.lan = lan

	// 订阅云端 MIPS 状态
	if c.mipsCloud != nil {
		c.mipsCloud.SubMipsState(fmt.Sprintf("%s-%s", c.config.UID, c.config.CloudServer),
			func(key string, connected bool) {
				c.logInfo("mips cloud state changed: %v", connected)
			})
	}

	// 自动模式下初始化本地网关和 LAN
	if c.config.CtrlMode == CtrlModeAuto {
		// 初始化 LAN 控制
		if c.lan != nil {
			if err := c.lan.Init(); err != nil {
				c.logError("failed to init lan client: %v", err)
			}
		}
	}

	c.logInfo("initialized, ctrl_mode=%s", c.config.CtrlMode)
	return nil
}

// Deinit 清理客户端资源
func (c *MClient) Deinit() {
	c.timersMu.Lock()
	if c.refreshTokenTimer != nil {
		c.refreshTokenTimer.Stop()
	}
	if c.refreshCertTimer != nil {
		c.refreshCertTimer.Stop()
	}
	if c.refreshPropsTimer != nil {
		c.refreshPropsTimer.Stop()
	}
	c.timersMu.Unlock()

	// 断开本地网关连接
	for _, mips := range c.mipsLocal {
		mips.Disconnect()
	}
	c.mipsLocal = make(map[string]*MipsLocalClient)

	// 断开云端 MQTT
	if c.mipsCloud != nil {
		c.mipsCloud.Disconnect()
	}

	// 停止 LAN
	if c.lan != nil {
		c.lan.Deinit()
	}

	c.logInfo("deinitialized")
}

// AddMipsLocalClient 添加本地网关客户端
func (c *MClient) AddMipsLocalClient(groupID string, client *MipsLocalClient) {
	c.mipsLocal[groupID] = client

	// 订阅网关状态
	client.SubMipsState(groupID, func(key string, connected bool) {
		c.logInfo("mips local state changed: group_id=%s, connected=%v", groupID, connected)
	})

	// 设置设备列表变更回调
	client.SetOnDevListChanged(func(mips *MipsLocalClient, devList []string) {
		c.logDebug("gateway device list changed: %v", devList)
	})

	// 连接
	if err := client.Connect(); err != nil {
		c.logError("failed to connect mips local: %v", err)
	}
}

// RemoveMipsLocalClient 移除本地网关客户端
func (c *MClient) RemoveMipsLocalClient(groupID string) {
	if client, exists := c.mipsLocal[groupID]; exists {
		client.Disconnect()
		delete(c.mipsLocal, groupID)
	}
}

// UpdateDeviceList 更新设备列表
func (c *MClient) UpdateDeviceList(devices map[string]*DeviceInfo) {
	c.devicesMu.Lock()
	c.deviceListCache = devices
	c.devicesMu.Unlock()

	// 更新 LAN 设备
	if c.lan != nil {
		lanDevices := make(map[string]map[string]interface{})
		for did, dev := range devices {
			if dev.Token != "" && dev.LocalIP != "" {
				lanDevices[did] = map[string]interface{}{
					"token": dev.Token,
					"ip":    dev.LocalIP,
					"model": dev.Model,
				}
			}
		}
		c.lan.UpdateDevices(lanDevices)
	}
}

// UpdateGatewayDevices 更新网关设备列表
func (c *MClient) UpdateGatewayDevices(devices map[string]*GatewayDeviceInfo) {
	c.devicesMu.Lock()
	c.deviceListGateway = devices
	c.devicesMu.Unlock()
}

// UpdateLANDevices 更新局域网设备列表
func (c *MClient) UpdateLANDevices(devices map[string]*LANDeviceInfo) {
	c.devicesMu.Lock()
	c.deviceListLAN = devices
	c.devicesMu.Unlock()
}

// GetDevice 获取设备信息
func (c *MClient) GetDevice(did string) (*DeviceInfo, bool) {
	c.devicesMu.RLock()
	defer c.devicesMu.RUnlock()
	dev, exists := c.deviceListCache[did]
	return dev, exists
}

// SetProp 设置设备属性（自动选择控制方式）
//
// 控制优先级:
// 1. 网关控制（如果设备通过中枢网关在线）
// 2. 局域网控制（如果设备在局域网在线）
// 3. 云端控制（最后 fallback）
func (c *MClient) SetProp(did string, siid, piid int, value interface{}) error {
	// 检查设备是否存在
	if _, exists := c.GetDevice(did); !exists {
		return fmt.Errorf("device not found: %s", did)
	}

	// 自动模式：优先本地控制
	if c.config.CtrlMode == CtrlModeAuto {
		// 尝试网关控制
		c.devicesMu.RLock()
		gwDev, exists := c.deviceListGateway[did]
		c.devicesMu.RUnlock()

		if exists && gwDev.Online && gwDev.SpecV2Access && gwDev.GroupID != "" {
			if mips, ok := c.mipsLocal[gwDev.GroupID]; ok {
				result, err := mips.SetPropAsync(did, siid, piid, value, 10000)
				if err != nil {
					c.logError("gateway set prop failed: %v", err)
				} else {
					rc := getInt(result, "code", -1)
					c.logDebug("gateway set prop: %s.%d.%d = %v, rc=%d", did, siid, piid, value, rc)
					if rc == 0 || rc == 1 {
						return nil
					}
				}
			} else {
				c.logError("no gateway route for device: %s", did)
			}
		}

		// 尝试局域网控制
		c.devicesMu.RLock()
		lanDev, exists := c.deviceListLAN[did]
		c.devicesMu.RUnlock()

		if exists && lanDev.Online && c.lan != nil {
			result, err := c.lan.SetPropAsync(did, siid, piid, value, 10000)
			if err != nil {
				c.logError("lan set prop failed: %v", err)
			} else {
				rc := getInt(result, "code", -1)
				c.logDebug("lan set prop: %s.%d.%d = %v, rc=%d", did, siid, piid, value, rc)
				if rc == 0 || rc == 1 {
					return nil
				}
			}
		}
	}

	// 云端控制
	if c.http != nil {
		params := []map[string]interface{}{
			{"did": did, "siid": siid, "piid": piid, "value": value},
		}
		result, err := c.http.SetProps(params)
		if err != nil {
			return fmt.Errorf("cloud set prop failed: %w", err)
		}

		if len(result) > 0 {
			resultMap, ok := result[0].(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid result format")
			}
			rc := getInt(resultMap, "code", -1)
			c.logDebug("cloud set prop: %s.%d.%d = %v, rc=%d", did, siid, piid, value, rc)
			if rc == 0 || rc == 1 {
				return nil
			}
			if rc == -704010000 || rc == -704042011 {
				c.logError("device may be removed or offline: %s", did)
			}
			return fmt.Errorf("set prop failed with code: %d", rc)
		}
	}

	return fmt.Errorf("no available control method")
}

// GetProp 获取设备属性
//
// 获取优先级:
// 1. 云端缓存（优先，避免频繁请求设备）
// 2. 网关控制
// 3. 局域网控制
func (c *MClient) GetProp(did string, siid, piid int) (interface{}, error) {
	// 检查设备是否存在
	if _, exists := c.GetDevice(did); !exists {
		return nil, fmt.Errorf("device not found: %s", did)
	}

	// 优先从云端获取
	if c.http != nil && c.getNetworkStatus() {
		result, err := c.http.GetProp(did, siid, piid)
		if err != nil {
			c.logError("get prop from cloud failed: %v", err)
		} else if result != nil {
			return result, nil
		}
	}

	// 自动模式：尝试本地获取
	if c.config.CtrlMode == CtrlModeAuto {
		// 尝试网关控制
		c.devicesMu.RLock()
		gwDev, exists := c.deviceListGateway[did]
		c.devicesMu.RUnlock()

		if exists && gwDev.Online && gwDev.SpecV2Access && gwDev.GroupID != "" {
			if mips, ok := c.mipsLocal[gwDev.GroupID]; ok {
				result, err := mips.GetPropAsync(did, siid, piid, 10000)
				if err != nil {
					c.logError("gateway get prop failed: %v", err)
				} else {
					c.logDebug("gateway get prop: %s.%d.%d = %v", did, siid, piid, result)
					return result, nil
				}
			}
		}

		// 尝试局域网控制
		c.devicesMu.RLock()
		lanDev, exists := c.deviceListLAN[did]
		c.devicesMu.RUnlock()

		if exists && lanDev.Online && c.lan != nil {
			result, err := c.lan.GetPropAsync(did, siid, piid, 10000)
			if err != nil {
				c.logError("lan get prop failed: %v", err)
			} else {
				c.logDebug("lan get prop: %s.%d.%d = %v", did, siid, piid, result)
				return result, nil
			}
		}
	}

	return nil, nil
}

// Action 执行设备动作
//
// 控制优先级:
// 1. 网关控制
// 2. 局域网控制
// 3. 云端控制
func (c *MClient) Action(did string, siid, aiid int, inList []interface{}) ([]interface{}, error) {
	// 检查设备是否存在
	if _, exists := c.GetDevice(did); !exists {
		return nil, fmt.Errorf("device not found: %s", did)
	}

	c.devicesMu.RLock()
	gwDev, _ := c.deviceListGateway[did]
	c.devicesMu.RUnlock()

	// 自动模式：优先本地控制
	if c.config.CtrlMode == CtrlModeAuto {
		// 尝试网关控制
		if gwDev != nil && gwDev.Online && gwDev.SpecV2Access && gwDev.GroupID != "" {
			if mips, ok := c.mipsLocal[gwDev.GroupID]; ok {
				result, err := mips.ActionAsync(did, siid, aiid, inList, 10000)
				if err != nil {
					c.logError("gateway action failed: %v", err)
				} else {
					rc := getInt(result, "code", -1)
					c.logDebug("gateway action: %s.%d.%d, rc=%d", did, siid, aiid, rc)
					if rc == 0 || rc == 1 {
						out, _ := result["out"].([]interface{})
						return out, nil
					}
				}
			} else {
				c.logError("no gateway route for device: %s", did)
			}
		}

		// 尝试局域网控制
		c.devicesMu.RLock()
		lanDev, exists := c.deviceListLAN[did]
		c.devicesMu.RUnlock()

		if exists && lanDev.Online && c.lan != nil {
			result, err := c.lan.ActionAsync(did, siid, aiid, inList, 10000)
			if err != nil {
				c.logError("lan action failed: %v", err)
			} else {
				rc := getInt(result, "code", -1)
				c.logDebug("lan action: %s.%d.%d, rc=%d", did, siid, aiid, rc)
				if rc == 0 || rc == 1 {
					out, _ := result["out"].([]interface{})
					return out, nil
				}
			}
		}
	}

	// 云端控制
	if c.http != nil {
		// 转换 inList 格式
		inListMaps := make([]map[string]interface{}, len(inList))
		for i, v := range inList {
			inListMaps[i] = map[string]interface{}{"value": v}
		}
		result, err := c.http.Action(did, siid, aiid, inListMaps)
		if err != nil {
			return nil, fmt.Errorf("cloud action failed: %w", err)
		}

		rc := getInt(result, "code", -1)
		c.logDebug("cloud action: %s.%d.%d, rc=%d", did, siid, aiid, rc)
		if rc == 0 || rc == 1 {
			out, _ := result["out"].([]interface{})
			return out, nil
		}
		if rc == -704010000 || rc == -704042011 {
			c.logError("device removed or offline: %s", did)
		}
		return nil, fmt.Errorf("action failed with code: %d", rc)
	}

	return nil, fmt.Errorf("no available control method")
}

// SubProp 订阅设备属性变更
func (c *MClient) SubProp(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, piid *int, ctx interface{}) error {
	// 检查设备是否存在
	if _, exists := c.GetDevice(did); !exists {
		return fmt.Errorf("device not found: %s", did)
	}

	// 订阅云端推送
	if c.mipsCloud != nil {
		if err := c.mipsCloud.SubProp(did, handler, siid, piid, ctx); err != nil {
			c.logError("failed to sub prop from cloud: %v", err)
		}
	}

	// 自动模式下订阅本地推送
	if c.config.CtrlMode == CtrlModeAuto {
		// 订阅网关推送
		c.devicesMu.RLock()
		gwDev, _ := c.deviceListGateway[did]
		c.devicesMu.RUnlock()

		if gwDev != nil && gwDev.GroupID != "" {
			if mips, ok := c.mipsLocal[gwDev.GroupID]; ok {
				if err := mips.SubProp(did, handler, siid, piid, ctx); err != nil {
					c.logError("failed to sub prop from gateway: %v", err)
				}
			}
		}

		// 订阅 LAN 推送
		if c.lan != nil {
			c.lan.SubProp(did, handler, ctx, siid, piid)
		}
	}

	return nil
}

// UnsubProp 取消订阅设备属性变更
func (c *MClient) UnsubProp(did string, siid, piid *int) error {
	if c.mipsCloud != nil {
		c.mipsCloud.UnsubProp(did, siid, piid)
	}

	for _, mips := range c.mipsLocal {
		mips.UnsubProp(did, siid, piid)
	}

	if c.lan != nil {
		c.lan.UnsubProp(did, siid, piid)
	}

	return nil
}

// SubEvent 订阅设备事件
func (c *MClient) SubEvent(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, eiid *int, ctx interface{}) error {
	// 检查设备是否存在
	if _, exists := c.GetDevice(did); !exists {
		return fmt.Errorf("device not found: %s", did)
	}

	// 订阅云端推送
	if c.mipsCloud != nil {
		if err := c.mipsCloud.SubEvent(did, handler, siid, eiid, ctx); err != nil {
			c.logError("failed to sub event from cloud: %v", err)
		}
	}

	// 自动模式下订阅本地推送
	if c.config.CtrlMode == CtrlModeAuto {
		c.devicesMu.RLock()
		gwDev, _ := c.deviceListGateway[did]
		c.devicesMu.RUnlock()

		if gwDev != nil && gwDev.GroupID != "" {
			if mips, ok := c.mipsLocal[gwDev.GroupID]; ok {
				if err := mips.SubEvent(did, handler, siid, eiid, ctx); err != nil {
					c.logError("failed to sub event from gateway: %v", err)
				}
			}
		}
	}

	return nil
}

// UnsubEvent 取消订阅设备事件
func (c *MClient) UnsubEvent(did string, siid, eiid *int) error {
	if c.mipsCloud != nil {
		c.mipsCloud.UnsubEvent(did, siid, eiid)
	}

	for _, mips := range c.mipsLocal {
		mips.UnsubEvent(did, siid, eiid)
	}

	return nil
}

// SubDeviceState 订阅设备在线状态
func (c *MClient) SubDeviceState(did string, handler func(did string, state MIoTDeviceState, ctx interface{}), ctx interface{}) error {
	if c.mipsCloud != nil {
		if err := c.mipsCloud.SubDeviceState(did, handler, ctx); err != nil {
			c.logError("failed to sub device state from cloud: %v", err)
		}
	}

	if c.config.CtrlMode == CtrlModeAuto {
		c.devicesMu.RLock()
		gwDev, _ := c.deviceListGateway[did]
		c.devicesMu.RUnlock()

		if gwDev != nil && gwDev.GroupID != "" {
			if mips, ok := c.mipsLocal[gwDev.GroupID]; ok {
				if err := mips.SubDeviceState(did, handler, ctx); err != nil {
					c.logError("failed to sub device state from gateway: %v", err)
				}
			}
		}
	}

	return nil
}

// UnsubDeviceState 取消订阅设备在线状态
func (c *MClient) UnsubDeviceState(did string) error {
	if c.mipsCloud != nil {
		c.mipsCloud.UnsubDeviceState(did)
	}

	for _, mips := range c.mipsLocal {
		mips.UnsubDeviceState(did)
	}

	return nil
}

// RefreshToken 刷新 OAuth Token
func (c *MClient) RefreshToken(refreshToken string) error {
	if c.oauth == nil {
		return fmt.Errorf("oauth client not initialized")
	}

	authInfo, err := c.oauth.RefreshAccessToken(refreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	// 更新 HTTP 客户端令牌
	if c.http != nil {
		c.http.UpdateHTTPHeader("", "", authInfo.AccessToken)
	}

	// 更新云端 MQTT 令牌
	if c.mipsCloud != nil {
		c.mipsCloud.UpdateAccessToken(authInfo.AccessToken)
	}

	c.logInfo("token refreshed successfully")
	return nil
}

// SetNetworkStatus 设置网络状态
func (c *MClient) SetNetworkStatus(online bool) {
	c.networkStatusMu.Lock()
	c.networkStatus = online
	c.networkStatusMu.Unlock()
}

// getNetworkStatus 获取网络状态
func (c *MClient) getNetworkStatus() bool {
	c.networkStatusMu.RLock()
	defer c.networkStatusMu.RUnlock()
	return c.networkStatus
}

// GetUID 获取用户 ID
func (c *MClient) GetUID() string {
	return c.config.UID
}

// GetCloudServer 获取云服务器区域
func (c *MClient) GetCloudServer() string {
	return c.config.CloudServer
}

// GetCtrlMode 获取控制模式
func (c *MClient) GetCtrlMode() CtrlMode {
	return c.config.CtrlMode
}

// GetOAuthClient 获取 OAuth 客户端
func (c *MClient) GetOAuthClient() *MIoTOauthClient {
	return c.oauth
}

// GetHTTPClient 获取 HTTP 客户端
func (c *MClient) GetHTTPClient() *CloudClient {
	return c.http
}

// GetMipsCloudClient 获取云端 MIPS 客户端
func (c *MClient) GetMipsCloudClient() *MipsCloudClient {
	return c.mipsCloud
}

// GetMipsLocalClient 获取本地网关客户端
func (c *MClient) GetMipsLocalClient(groupID string) (*MipsLocalClient, bool) {
	client, exists := c.mipsLocal[groupID]
	return client, exists
}

// GetLANClient 获取 LAN 客户端
func (c *MClient) GetLANClient() *LANClient {
	return c.lan
}

// getInt 从 map 中获取 int 值
func getInt(m map[string]interface{}, key string, defaultVal int) int {
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return defaultVal
	}
}
