package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/event"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// CloudClientFactory defines the interface for creating CloudClient
type CloudClientFactory interface {
	GetCloudClient() (*miio.CloudClient, error)
}

// SpecFetcherFactory defines the interface for creating SpecFetcher
type SpecFetcherFactory interface {
	GetSpecFetcher() *miio.SpecFetcher
}

// PasswordConnectorFactory defines the interface for accessing the singleton PasswordConnector
type PasswordConnectorFactory interface {
	GetPasswordConnector() *miio.PasswordConnector
	SetPasswordConnectorCredentials(username, password string)
}

// syncTimestamp 用于标记本次同步时间，用于识别已删除的项目
var syncTimestamp = time.Now().Unix()

// tokenRefreshThreshold 距离过期不足此时长时自动刷新
const tokenRefreshThreshold = 5 * time.Hour

// checkToken 从 store 获取小米账号并校验 token 有效性。
// 返回 (acc, nil) 表示 token 有效；返回 (nil, result) 表示校验失败，调用方应直接返回 result。
const authGuide = "Please follow these steps to authorize: " +
	"1) Call mi_get_oauth_url to get the login URL. " +
	"2) Open the URL in a browser and complete the Xiaomi login. " +
	"3) After login, you will be redirected to a callback URL that contains a 'code_value' parameter. " +
	"4) Call mi_get_access_token with that code to save the token."

func checkToken(store data.XiaomiAccountStore) (*data.XiaomiAccount, *tools.ToolResult) {
	acc, err := store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return nil, tools.SilentResult(`{"account":null,"reason":"not_configured","guide":"` + authGuide + `"}`)
		}
		return nil, tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	// token 为空视为未配置
	if acc.AccessToken == "" || acc.RefreshToken == "" {
		return nil, tools.SilentResult(`{"account":null,"reason":"token_missing","guide":"` + authGuide + `"}`)
	}

	now := time.Now()

	// 已过期
	if !acc.TokenExpiresAt.IsZero() && now.After(acc.TokenExpiresAt) {
		return nil, tools.SilentResult(`{"account":null,"reason":"token_expired","guide":"` + authGuide + `"}`)
	}

	return acc, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_get_account
// ─────────────────────────────────────────────────────────────────────────────

// GetXiaomiAccountTool 读取小米账号信息。
// - 账号不存在 / token 为空 / token 已过期 → 返回 account:null
// - token 距过期不足 5 小时 → 自动调用 RefreshAccessToken 刷新并保存，返回最新账号
// - token 有效 → 直接返回账号
type GetXiaomiAccountTool struct {
	store       data.XiaomiAccountStore
	oauthClient *miio.MIoTOauthClient
}

func NewGetXiaomiAccountTool(store data.XiaomiAccountStore, oauthClient *miio.MIoTOauthClient) *GetXiaomiAccountTool {
	return &GetXiaomiAccountTool{store: store, oauthClient: oauthClient}
}

func (t *GetXiaomiAccountTool) Name() string { return "mi_get_account" }

func (t *GetXiaomiAccountTool) Description() string {
	return "Get the stored Xiaomi (Mi Home) account information including tokens and home binding. " +
		"Returns account:null if not configured, token is missing, or token has already expired. " +
		"When the token will expire within 5 hours it is refreshed automatically; the returned account always contains a valid token."
}

func (t *GetXiaomiAccountTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *GetXiaomiAccountTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	acc, result := checkToken(t.store)
	if result != nil {
		return result
	}

	now := time.Now()

	// 距过期不足 5 小时 → 自动刷新
	if !acc.TokenExpiresAt.IsZero() && acc.TokenExpiresAt.Sub(now) < tokenRefreshThreshold {
		refreshed, err := t.oauthClient.RefreshAccessToken(acc.RefreshToken)
		if err != nil {
			// 刷新失败不阻断，记录原因后返回空（避免用过期 token 操作）
			return tools.SilentResult(fmt.Sprintf(`{"account":null,"reason":"refresh_failed","error":%q}`, err.Error()))
		}
		acc.AccessToken = refreshed.AccessToken
		acc.RefreshToken = refreshed.RefreshToken
		acc.ExpiresIn = refreshed.ExpiresIn
		acc.TokenExpiresAt = time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
		if saveErr := t.store.Save(*acc); saveErr != nil {
			return tools.ErrorResult(fmt.Sprintf("token refreshed but failed to save: %v", saveErr))
		}

		// 发送 token 更新事件
		evt := event.NewEvent(event.EventTypeToken, "mi_get_account", &event.TokenData{
			AccessToken:    acc.AccessToken,
			RefreshToken:   acc.RefreshToken,
			TokenExpiresAt: acc.TokenExpiresAt,
		})
		event.GetCenter().Publish(evt)
	}

	b, _ := json.Marshal(acc)
	return tools.SilentResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_update_home
// ─────────────────────────────────────────────────────────────────────────────

// UpdateXiaomiHomeTool 更新小米账号绑定的家庭信息（home_id / home_name）。
type UpdateXiaomiHomeTool struct {
	store data.XiaomiAccountStore
}

func NewUpdateXiaomiHomeTool(store data.XiaomiAccountStore) *UpdateXiaomiHomeTool {
	return &UpdateXiaomiHomeTool{store: store}
}

func (t *UpdateXiaomiHomeTool) Name() string { return "mi_update_home" }

func (t *UpdateXiaomiHomeTool) Description() string {
	return "Update the Mi Home binding (home_id and home_name) for the stored Xiaomi account. " +
		"The account must already exist."
}

func (t *UpdateXiaomiHomeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"home_id": map[string]any{
				"type":        "string",
				"description": "Mi Home family/home ID",
			},
			"home_name": map[string]any{
				"type":        "string",
				"description": "Mi Home family/home display name",
			},
		},
		"required": []string{"home_id", "home_name"},
	}
}

func (t *UpdateXiaomiHomeTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	homeID, ok := args["home_id"].(string)
	if !ok || homeID == "" {
		return tools.ErrorResult("home_id is required")
	}
	homeName, ok := args["home_name"].(string)
	if !ok || homeName == "" {
		return tools.ErrorResult("home_name is required")
	}

	acc, err := t.store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return tools.ErrorResult("xiaomi account not configured, cannot update home info")
		}
		return tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	acc.HomeID = homeID
	acc.HomeName = homeName

	if err := t.store.Save(*acc); err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to save xiaomi account: %v", err))
	}
	return tools.NewToolResult(fmt.Sprintf("xiaomi home updated: id=%q name=%q", homeID, homeName))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_get_oauth_url
// ─────────────────────────────────────────────────────────────────────────────

// GetXiaomiOAuthURLTool 生成小米 OAuth2 授权 URL，引导用户在浏览器中完成授权。
type GetXiaomiOAuthURLTool struct {
	oauthClient *miio.MIoTOauthClient
}

func NewGetXiaomiOAuthURLTool(oauthClient *miio.MIoTOauthClient) *GetXiaomiOAuthURLTool {
	return &GetXiaomiOAuthURLTool{oauthClient: oauthClient}
}

func (t *GetXiaomiOAuthURLTool) Name() string { return "mi_get_oauth_url" }

func (t *GetXiaomiOAuthURLTool) Description() string {
	return "Generate the Xiaomi OAuth2 authorization URL. " +
		"The user should open this URL in a browser to authorize access. " +
		"After authorization, the callback URL will contain a code_value parameter to be used with mi_get_access_token."
}

func (t *GetXiaomiOAuthURLTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"redirect_url": map[string]any{
				"type":        "string",
				"description": "OAuth2 callback URL (optional, uses default if empty)",
			},
			"scope": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Permission scope IDs (optional), e.g. [\"1\",\"3\",\"6\"]",
			},
			"skip_confirm": map[string]any{
				"type":        "boolean",
				"description": "Skip confirmation page if already authorized (optional, default false)",
			},
		},
		"required": []string{},
	}
}

func (t *GetXiaomiOAuthURLTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	redirectURL, _ := args["redirect_url"].(string)

	var scope []string
	if raw, ok := args["scope"].([]any); ok {
		for _, s := range raw {
			if str, ok := s.(string); ok {
				scope = append(scope, str)
			}
		}
	}

	skipConfirm, _ := args["skip_confirm"].(bool)

	authURL := t.oauthClient.GenAuthURL(redirectURL, "", scope, skipConfirm)

	type result struct {
		URL      string `json:"url"`
		DeviceID string `json:"device_id"`
		State    string `json:"state"`
	}
	b, _ := json.Marshal(result{
		URL:      authURL,
		DeviceID: t.oauthClient.GetDeviceID(),
		State:    t.oauthClient.GetState(),
	})
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_get_access_token
// ─────────────────────────────────────────────────────────────────────────────

// GetXiaomiAccessTokenTool 使用授权码换取 access_token，并将 token 保存到账号。
type GetXiaomiAccessTokenTool struct {
	store       data.XiaomiAccountStore
	oauthClient *miio.MIoTOauthClient
}

func NewGetXiaomiAccessTokenTool(store data.XiaomiAccountStore, oauthClient *miio.MIoTOauthClient) *GetXiaomiAccessTokenTool {
	return &GetXiaomiAccessTokenTool{store: store, oauthClient: oauthClient}
}

func (t *GetXiaomiAccessTokenTool) Name() string { return "mi_get_access_token" }

func (t *GetXiaomiAccessTokenTool) Description() string {
	return "Exchange the OAuth2 authorization code for an access token and save it to the Xiaomi account. " +
		"The code_value comes from the callback URL after the user completes authorization via mi_get_oauth_url."
}

func (t *GetXiaomiAccessTokenTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code": map[string]any{
				"type":        "string",
				"description": "Authorization code from the OAuth2 callback URL (code_value parameter)",
			},
		},
		"required": []string{"code"},
	}
}

func (t *GetXiaomiAccessTokenTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return tools.ErrorResult("code is required")
	}

	tokenInfo, err := t.oauthClient.GetAccessToken(code)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to get access token: %v", err))
	}

	// 读取已有账号；不存在则新建
	acc, err := t.store.Get()
	if err != nil {
		acc = &data.XiaomiAccount{}
	}

	acc.AccessToken = tokenInfo.AccessToken
	acc.RefreshToken = tokenInfo.RefreshToken
	acc.ExpiresIn = tokenInfo.ExpiresIn
	acc.TokenExpiresAt = time.Now().Add(time.Duration(tokenInfo.ExpiresIn) * time.Second)

	if err := t.store.Save(*acc); err != nil {
		return tools.ErrorResult(fmt.Sprintf("token obtained but failed to save: %v", err))
	}

	b, _ := json.Marshal(acc)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_sync_homes
// ─────────────────────────────────────────────────────────────────────────────

// SyncXiaomiHomesTool 查询小米家庭列表
type SyncXiaomiHomesTool struct {
	store   data.XiaomiAccountStore
	factory CloudClientFactory
}

func NewSyncXiaomiHomesTool(store data.XiaomiAccountStore, factory CloudClientFactory) *SyncXiaomiHomesTool {
	return &SyncXiaomiHomesTool{store: store, factory: factory}
}

func (t *SyncXiaomiHomesTool) Name() string { return "mi_sync_homes" }

func (t *SyncXiaomiHomesTool) Description() string {
	return "Query all homes from Xiaomi Mi Home cloud. " +
		"Returns a list of home_id and home_name for user to select which home to bind. " +
		"Note: One device can only be bound to one home."
}

func (t *SyncXiaomiHomesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *SyncXiaomiHomesTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	_, tokenErr := checkToken(t.store)
	if tokenErr != nil {
		return tokenErr
	}

	// 创建 CloudClient
	client, err := t.factory.GetCloudClient()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create cloud client: %v", err))
	}
	defer client.Close()

	// 获取家庭信息
	homeInfos, err := client.GetHomeInfos()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to get home infos: %v", err))
	}

	// 构建家庭列表（仅查询，不写入 Space 表）
	type homeInfo struct {
		HomeID   string `json:"home_id"`
		HomeName string `json:"home_name"`
		Type     string `json:"type"`
	}

	var homes []homeInfo

	// 处理自有家庭
	for homeID, homeDetail := range homeInfos.HomeList {
		homes = append(homes, homeInfo{
			HomeID:   homeID,
			HomeName: homeDetail.HomeName,
			Type:     "own",
		})
	}

	// 处理共享家庭
	for homeID, homeDetail := range homeInfos.ShareHomeList {
		homes = append(homes, homeInfo{
			HomeID:   homeID,
			HomeName: homeDetail.HomeName,
			Type:     "shared",
		})
	}

	result := map[string]interface{}{
		"success":      true,
		"uid":          homeInfos.UID,
		"homes":        homes,
		"homes_total":  len(homes),
		"homes_own":    len(homeInfos.HomeList),
		"homes_shared": len(homeInfos.ShareHomeList),
	}
	b, _ := json.Marshal(result)
	return tools.SilentResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_sync_rooms
// ─────────────────────────────────────────────────────────────────────────────

// SyncXiaomiRoomsTool 同步指定家庭的房间信息
type SyncXiaomiRoomsTool struct {
	store      data.XiaomiAccountStore
	spaceStore data.SpaceStore
	factory    CloudClientFactory
}

func NewSyncXiaomiRoomsTool(store data.XiaomiAccountStore, spaceStore data.SpaceStore, factory CloudClientFactory) *SyncXiaomiRoomsTool {
	return &SyncXiaomiRoomsTool{store: store, spaceStore: spaceStore, factory: factory}
}

func (t *SyncXiaomiRoomsTool) Name() string { return "mi_sync_rooms" }

func (t *SyncXiaomiRoomsTool) Description() string {
	return "Sync room information for a specific home from Xiaomi Mi Home cloud. " +
		"Fetches all rooms in the specified home and creates/updates corresponding spaces. " +
		"Rooms that no longer exist are removed. Rooms are created with source='xiaomi'."
}

func (t *SyncXiaomiRoomsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"home_id": map[string]any{
				"type":        "string",
				"description": "Mi Home ID to sync rooms from",
			},
		},
		"required": []string{"home_id"},
	}
}

func (t *SyncXiaomiRoomsTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	homeID, ok := args["home_id"].(string)
	if !ok || homeID == "" {
		return tools.ErrorResult("home_id is required")
	}

	_, tokenErr := checkToken(t.store)
	if tokenErr != nil {
		return tokenErr
	}

	// 创建 CloudClient
	client, err := t.factory.GetCloudClient()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create cloud client: %v", err))
	}
	defer client.Close()

	// 获取家庭信息（包含房间）
	homeInfos, err := client.GetHomeInfos()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to get home infos: %v", err))
	}

	// 查找指定家庭
	var targetHome *miio.HomeDetail
	for _, home := range homeInfos.HomeList {
		if home.HomeID == homeID {
			targetHome = home
			break
		}
	}
	if targetHome == nil {
		for _, home := range homeInfos.ShareHomeList {
			if home.HomeID == homeID {
				targetHome = home
				break
			}
		}
	}
	if targetHome == nil {
		return tools.ErrorResult(fmt.Sprintf("home_id %s not found", homeID))
	}

	// 获取现有所有空间用于比较
	existingSpaces, _ := t.spaceStore.GetAll()
	existingXiaomiRoomIDs := make(map[string]bool)
	for _, space := range existingSpaces {
		if fromID, ok := space.From["id"]; ok && space.From["name"] == "小米" {
			existingXiaomiRoomIDs[fromID] = true
		}
	}

	// 同步房间
	now := time.Now()
	syncedCount := 0
	updatedCount := 0
	removedCount := 0
	currentRoomIDs := make(map[string]bool)

	for roomID, roomDetail := range targetHome.RoomInfo {
		currentRoomIDs[roomID] = true
		delete(existingXiaomiRoomIDs, roomID)

		// 查找是否已存在（通过名称匹配）
		var existing *data.Space
		for _, sp := range existingSpaces {
			if strings.EqualFold(sp.Name, roomDetail.RoomName) {
				existing = &sp
				break
			}
		}
		if existing == nil {
			// 新建房间空间
			newSpace := data.Space{
				Name: roomDetail.RoomName,
				From: map[string]string{"name": "小米", "id": roomID},
			}
			if err := t.spaceStore.Save(newSpace); err != nil {
				continue
			}
			syncedCount++
		} else {
			// 更新现有空间
			existing.From = map[string]string{"name": "小米", "id": roomID}
			if err := t.spaceStore.Save(*existing); err != nil {
				continue
			}
			updatedCount++
		}
	}

	// 删除不再存在的房间（仅删除来源为小米的）
	if len(existingXiaomiRoomIDs) > 0 {
		allSpaces, _ := t.spaceStore.GetAll()
		for _, sp := range allSpaces {
			if fromID, ok := sp.From["id"]; ok && sp.From["name"] == "小米" && existingXiaomiRoomIDs[fromID] {
				if err := t.spaceStore.Delete(sp.Name); err == nil {
					removedCount++
				}
			}
		}
	}

	result := map[string]interface{}{
		"synced":      true,
		"home_id":     homeID,
		"home_name":   targetHome.HomeName,
		"rooms_total": len(targetHome.RoomInfo),
		"created":     syncedCount,
		"updated":     updatedCount,
		"removed":     removedCount,
		"timestamp":   now.Format(time.RFC3339),
	}
	b, _ := json.Marshal(result)
	return tools.SilentResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_sync_devices
// ─────────────────────────────────────────────────────────────────────────────

// SyncXiaomiDevicesTool 同步指定家庭的设备信息
type SyncXiaomiDevicesTool struct {
	store       data.XiaomiAccountStore
	deviceStore data.DeviceStore
	factory     CloudClientFactory
}

func NewSyncXiaomiDevicesTool(store data.XiaomiAccountStore, deviceStore data.DeviceStore, factory CloudClientFactory) *SyncXiaomiDevicesTool {
	return &SyncXiaomiDevicesTool{store: store, deviceStore: deviceStore, factory: factory}
}

func (t *SyncXiaomiDevicesTool) Name() string { return "mi_sync_devices" }

func (t *SyncXiaomiDevicesTool) Description() string {
	return "Sync device information for a specific home from Xiaomi Mi Home cloud. " +
		"Fetches all devices in the specified home and creates/updates them. " +
		"Devices that no longer exist are removed."
}

func (t *SyncXiaomiDevicesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"home_id": map[string]any{
				"type":        "string",
				"description": "Mi Home ID to sync devices from",
			},
		},
		"required": []string{"home_id"},
	}
}

func (t *SyncXiaomiDevicesTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	homeID, ok := args["home_id"].(string)
	if !ok || homeID == "" {
		return tools.ErrorResult("home_id is required")
	}

	_, tokenErr := checkToken(t.store)
	if tokenErr != nil {
		return tokenErr
	}

	// 创建 CloudClient
	client, err := t.factory.GetCloudClient()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create cloud client: %v", err))
	}
	defer client.Close()

	// 获取设备信息
	devicesResult, err := client.GetDevices([]string{homeID})
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to get devices: %v", err))
	}

	// 获取现有设备用于比较
	existingDevices, _ := t.deviceStore.GetAll()
	existingDeviceIDs := make(map[string]bool)
	for _, dev := range existingDevices {
		if dev.From == "mijia" {
			existingDeviceIDs[dev.FromID] = true
		}
	}

	// 同步设备
	now := time.Now()
	syncedCount := 0
	updatedCount := 0
	removedCount := 0

	for did, deviceInfo := range devicesResult.Devices {
		delete(existingDeviceIDs, did)

		// 查找是否已存在（通过 FromID 匹配）
		var existing *data.Device
		for i, dev := range existingDevices {
			if dev.FromID == did {
				existing = &existingDevices[i]
				break
			}
		}
		if existing == nil {
			// 创建设备
			newDevice := data.Device{
				FromID:    did,
				From:      "mijia",
				Name:      deviceInfo.Name,
				Type:      deviceInfo.Model,
				IP:        deviceInfo.LocalIP,
				Token:     deviceInfo.Token,
				URN:       deviceInfo.URN,
				SpaceName: deviceInfo.RoomName,
			}
			if err := t.deviceStore.Save(newDevice); err != nil {
				continue
			}
			syncedCount++
		} else {
			// 更新设备
			existing.Name = deviceInfo.Name
			existing.Type = deviceInfo.Model
			existing.IP = deviceInfo.LocalIP
			existing.Token = deviceInfo.Token
			existing.URN = deviceInfo.URN
			existing.SpaceName = deviceInfo.RoomName
			if err := t.deviceStore.Save(*existing); err != nil {
				continue
			}
			updatedCount++
		}
	}

	// 删除不再存在的设备（仅删除 brand=mijia 的）
	for did := range existingDeviceIDs {
		if err := t.deviceStore.Delete(did); err == nil {
			removedCount++
		}
	}

	result := map[string]interface{}{
		"synced":        true,
		"home_id":       homeID,
		"devices_total": len(devicesResult.Devices),
		"created":       syncedCount,
		"updated":       updatedCount,
		"removed":       removedCount,
		"timestamp":     now.Format(time.RFC3339),
	}
	b, _ := json.Marshal(result)
	return tools.SilentResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_get_spec
// ─────────────────────────────────────────────────────────────────────────────

// GetXiaomiSpecTool 获取设备 MIoT Spec 规范
type GetXiaomiSpecTool struct {
	specFetcher *miio.SpecFetcher
}

// NewGetXiaomiSpecTool creates a new GetXiaomiSpecTool
func NewGetXiaomiSpecTool(specFetcher *miio.SpecFetcher) *GetXiaomiSpecTool {
	return &GetXiaomiSpecTool{specFetcher: specFetcher}
}

func (t *GetXiaomiSpecTool) Name() string { return "mi_get_spec" }

func (t *GetXiaomiSpecTool) Description() string {
	return "Get the MIoT specification for a Xiaomi device by its URN. " +
		"Returns the raw JSON spec containing services, properties, actions and events. " +
		"The result is cached locally for 14 days."
}

func (t *GetXiaomiSpecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"urn": map[string]any{
				"type":        "string",
				"description": "Device URN (e.g., urn:miot-spec-v2:device:light:0000A001:yeelink-v1)",
			},
		},
		"required": []string{"urn"},
	}
}

func (t *GetXiaomiSpecTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	urn, ok := args["urn"].(string)
	if !ok || urn == "" {
		return tools.ErrorResult("urn is required")
	}

	specJSON, err := t.specFetcher.GetSpec(urn)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to get spec: %v", err))
	}

	return tools.SilentResult(specJSON)
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_action
// ─────────────────────────────────────────────────────────────────────────────

// XiaomiActionTool executes a device action via Mi Home cloud
type XiaomiActionTool struct {
	store   data.XiaomiAccountStore
	factory CloudClientFactory
}

func NewXiaomiActionTool(store data.XiaomiAccountStore, factory CloudClientFactory) *XiaomiActionTool {
	return &XiaomiActionTool{store: store, factory: factory}
}

func (t *XiaomiActionTool) Name() string { return "mi_action" }

func (t *XiaomiActionTool) Description() string {
	return "Trigger a predefined action (task) on a Xiaomi Mi Home device. " +
		"Actions are collections of operations designed for specific scenarios, encapsulating multiple steps into one command. " +
		"Typical scenarios: start robot vacuum cleaning, start camera video stream, factory reset device. " +
		"Operation: Tell the device 'execute start-cleaning action'. The device auto-executes internal steps like checking battery, lowering brushes, starting fan and wheels. " +
		"Example: action(siid=3,aiid=1) start-cleaning may internally check battery, set fan property, set wheel property, etc. " +
		"For directly setting individual states (e.g., turn on/off, set brightness level), use mi_set_prop instead. " +
		"Parameters: did (device ID), siid (service ID), aiid (action ID), and optional input parameters."
}

func (t *XiaomiActionTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"did": map[string]any{
				"type":        "string",
				"description": "Device ID (DID)",
			},
			"siid": map[string]any{
				"type":        "integer",
				"description": "Service ID (SIID)",
			},
			"aiid": map[string]any{
				"type":        "integer",
				"description": "Action ID (AIID)",
			},
			"in": map[string]any{
				"type":        "array",
				"description": "Action input parameters (optional)",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": map[string]any{
							"description": "Parameter value",
						},
					},
				},
			},
		},
		"required": []string{"did", "siid", "aiid"},
	}
}

func (t *XiaomiActionTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	did, ok := args["did"].(string)
	if !ok || did == "" {
		return tools.ErrorResult("did is required")
	}

	siidFloat, ok := args["siid"].(float64)
	if !ok {
		return tools.ErrorResult("siid is required and must be an integer")
	}
	siid := int(siidFloat)

	aiidFloat, ok := args["aiid"].(float64)
	if !ok {
		return tools.ErrorResult("aiid is required and must be an integer")
	}
	aiid := int(aiidFloat)

	var inList []map[string]interface{}
	if rawIn, ok := args["in"].([]any); ok && len(rawIn) > 0 {
		inList = make([]map[string]interface{}, 0, len(rawIn))
		for _, item := range rawIn {
			if m, ok := item.(map[string]any); ok {
				inList = append(inList, m)
			}
		}
	}
	if inList == nil {
		inList = []map[string]interface{}{}
	}

	_, tokenErr := checkToken(t.store)
	if tokenErr != nil {
		return tokenErr
	}

	// Create CloudClient
	client, err := t.factory.GetCloudClient()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create cloud client: %v", err))
	}
	defer client.Close()

	// Execute action
	result, err := client.Action(did, siid, aiid, []interface{}{})
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to execute action: %v", err))
	}

	b, _ := json.Marshal(result)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_set_prop
// ─────────────────────────────────────────────────────────────────────────────

// SetXiaomiPropTool 设置设备属性
// 包装 CloudClient.SetProps 方法
type SetXiaomiPropTool struct {
	store   data.XiaomiAccountStore
	factory CloudClientFactory
}

func NewSetXiaomiPropTool(store data.XiaomiAccountStore, factory CloudClientFactory) *SetXiaomiPropTool {
	return &SetXiaomiPropTool{store: store, factory: factory}
}

func (t *SetXiaomiPropTool) Name() string { return "mi_set_prop" }

func (t *SetXiaomiPropTool) Description() string {
	return "Directly set a single property (state) on a Xiaomi Mi Home device. " +
		"Properties are the smallest unit describing device current state. " +
		"Typical scenarios: turn on/off a lamp, adjust fan speed, set air purifier target humidity. " +
		"Operation: Tell the device 'set switch property to on' or 'set brightness to 80'. " +
		"Examples: set prop(siid=2,piid=1) switch to true; set prop(siid=2,piid=2) brightness to 60. " +
		"For triggering multi-step tasks (e.g., start cleaning, start video stream), use mi_action instead. " +
		"Parameters: did (device ID), siid (service ID), piid (property ID), value, and valueType (must match the actual property type)."
}

func (t *SetXiaomiPropTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"did": map[string]any{
				"type":        "string",
				"description": "Device ID (DID)",
			},
			"siid": map[string]any{
				"type":        "integer",
				"description": "Service ID (SIID)",
			},
			"piid": map[string]any{
				"type":        "integer",
				"description": "Property ID (PIID)",
			},
			"value": map[string]any{
				"type":        "any",
				"description": "Property value to set",
			},
			"valueType": map[string]any{
				"type":        "string",
				"description": "The MIoT property format mapped to Go type. Use \"bool\" for bool, \"int\" for uint8/uint16/uint32/int, \"float\" for float/double, \"string\" for string. Must match the device spec format to avoid type errors.",
				"enum":        []string{"bool", "int", "float", "string"},
			},
		},
		"required": []string{"did", "siid", "piid", "value", "valueType"},
	}
}

func (t *SetXiaomiPropTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	did, ok := args["did"].(string)
	if !ok || did == "" {
		return tools.ErrorResult("did is required")
	}

	siidFloat, ok := args["siid"].(float64)
	if !ok {
		return tools.ErrorResult("siid is required and must be an integer")
	}
	siid := int(siidFloat)

	piidFloat, ok := args["piid"].(float64)
	if !ok {
		return tools.ErrorResult("piid is required and must be an integer")
	}
	piid := int(piidFloat)

	rawValue, ok := args["value"]
	if !ok {
		return tools.ErrorResult("value is required")
	}

	valueType, ok := args["valueType"].(string)
	if !ok || valueType == "" {
		return tools.ErrorResult("valueType is required and must be one of: bool, int, float, string")
	}

	var value interface{}
	switch valueType {
	case "bool":
		v, ok := rawValue.(bool)
		if !ok {
			// JSON numbers decoded as float64; treat 0 as false, non-zero as true
			if f, fok := rawValue.(float64); fok {
				v = f != 0
				ok = true
			}
			if s, sok := rawValue.(string); sok {
				v = s == "true" || s == "1"
				ok = true
			}
		}
		if !ok {
			return tools.ErrorResult(fmt.Sprintf("value cannot be cast to bool: %v", rawValue))
		}
		value = v
	case "int":
		f, ok := rawValue.(float64)
		if !ok {
			return tools.ErrorResult(fmt.Sprintf("value cannot be cast to int: %v", rawValue))
		}
		value = int(f)
	case "float":
		f, ok := rawValue.(float64)
		if !ok {
			return tools.ErrorResult(fmt.Sprintf("value cannot be cast to float: %v", rawValue))
		}
		value = f
	case "string":
		switch v := rawValue.(type) {
		case string:
			value = v
		default:
			value = fmt.Sprintf("%v", rawValue)
		}
	default:
		return tools.ErrorResult(fmt.Sprintf("unsupported valueType %q, must be one of: bool, int, float, string", valueType))
	}

	_, tokenErr := checkToken(t.store)
	if tokenErr != nil {
		return tokenErr
	}

	// Create CloudClient
	client, err := t.factory.GetCloudClient()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create cloud client: %v", err))
	}
	defer client.Close()

	// Build params and execute setProps
	params := []map[string]interface{}{
		{
			"did":   did,
			"siid":  siid,
			"piid":  piid,
			"value": value,
		},
	}

	result, err := client.SetProps(params)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to set property: %v", err))
	}

	b, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"result":  result,
	})
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_send_email_code
// ─────────────────────────────────────────────────────────────────────────────

// MiSendEmailCodeTool initiates Xiaomi password login and triggers 2FA email code delivery.
// The caller must supply the same username/password pair when calling MiLoginEmailTool.
type MiSendEmailCodeTool struct {
	store   data.XiaomiAccountStore
	factory PasswordConnectorFactory
}

func NewMiSendEmailCodeTool(store data.XiaomiAccountStore, factory PasswordConnectorFactory) *MiSendEmailCodeTool {
	return &MiSendEmailCodeTool{store: store, factory: factory}
}

func (t *MiSendEmailCodeTool) Name() string { return "mi_send_email_code" }

func (t *MiSendEmailCodeTool) Description() string {
	return "Start the Xiaomi App login flow with username and password. " +
		"If the account has two-factor authentication (2FA) enabled, a verification code will be sent to the registered email address. " +
		"After calling this tool, ask the user to check their email and provide the code, then call mi_login_email to complete login. " +
		"On success without 2FA the login completes immediately and user_id, ssecurity, service_token are saved to the account."
}

func (t *MiSendEmailCodeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"username": map[string]any{
				"type":        "string",
				"description": "Xiaomi account username (email or phone number)",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "Xiaomi account password",
			},
		},
		"required": []string{"username", "password"},
	}
}

func (t *MiSendEmailCodeTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	username, ok := args["username"].(string)
	if !ok || username == "" {
		return tools.ErrorResult("username is required")
	}
	password, ok := args["password"].(string)
	if !ok || password == "" {
		return tools.ErrorResult("password is required")
	}

	connector := t.factory.GetPasswordConnector()
	t.factory.SetPasswordConnectorCredentials(username, password)
	result, err := connector.Login()

	// 2FA required — email code has been sent
	var tfa *miio.ErrTwoFactorRequired
	if errors.As(err, &tfa) {
		b, _ := json.Marshal(map[string]interface{}{
			"status":  "2fa_required",
			"message": "A verification code has been sent to your registered email address. Please provide the code to mi_login_email to complete login.",
		})
		return tools.NewToolResult(string(b))
	}

	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("login failed: %v", err))
	}

	// No 2FA — login completed directly; save credentials
	if saveErr := t.saveLoginResult(result); saveErr != nil {
		return tools.ErrorResult(fmt.Sprintf("login succeeded but failed to save credentials: %v", saveErr))
	}

	b, _ := json.Marshal(map[string]interface{}{
		"status":  "ok",
		"user_id": result.UserID,
	})
	return tools.NewToolResult(string(b))
}

func (t *MiSendEmailCodeTool) saveLoginResult(r *miio.AppLoginResult) error {
	acc, err := t.store.Get()
	if err != nil {
		acc = &data.XiaomiAccount{}
	}
	acc.UserID = r.UserID
	acc.SSecurity = r.SSecurity
	acc.ServiceToken = r.ServiceToken
	return t.store.Save(*acc)
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_login_email
// ─────────────────────────────────────────────────────────────────────────────

// MiLoginEmailTool completes the Xiaomi 2FA login by submitting the email verification code.
// Must be called on the same PasswordConnector instance started by MiSendEmailCodeTool,
// so the factory must return the same connector for the same username/password within a session.
type MiLoginEmailTool struct {
	store   data.XiaomiAccountStore
	factory PasswordConnectorFactory
}

func NewMiLoginEmailTool(store data.XiaomiAccountStore, factory PasswordConnectorFactory) *MiLoginEmailTool {
	return &MiLoginEmailTool{store: store, factory: factory}
}

func (t *MiLoginEmailTool) Name() string { return "mi_login_email" }

func (t *MiLoginEmailTool) Description() string {
	return "Complete the Xiaomi 2FA login by submitting the email verification code received after calling mi_send_email_code. " +
		"On success, user_id, ssecurity and service_token are saved to the Xiaomi account."
}

func (t *MiLoginEmailTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code": map[string]any{
				"type":        "string",
				"description": "Email verification code received from Xiaomi",
			},
		},
		"required": []string{"code"},
	}
}

func (t *MiLoginEmailTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return tools.ErrorResult("code is required")
	}

	connector := t.factory.GetPasswordConnector()
	result, err := connector.CompleteTwoFactor(code)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("2FA verification failed: %v", err))
	}

	// Save credentials
	acc, getErr := t.store.Get()
	if getErr != nil {
		acc = &data.XiaomiAccount{}
	}
	acc.UserID = result.UserID
	acc.SSecurity = result.SSecurity
	acc.ServiceToken = result.ServiceToken
	if saveErr := t.store.Save(*acc); saveErr != nil {
		return tools.ErrorResult(fmt.Sprintf("login succeeded but failed to save credentials: %v", saveErr))
	}

	b, _ := json.Marshal(map[string]interface{}{
		"status":  "ok",
		"user_id": result.UserID,
	})
	return tools.NewToolResult(string(b))
}
