package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/miio"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// syncTimestamp 用于标记本次同步时间，用于识别已删除的项目
var syncTimestamp = time.Now().Unix()

// tokenRefreshThreshold 距离过期不足此时长时自动刷新
const tokenRefreshThreshold = 5 * time.Hour

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
	acc, err := t.store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return tools.SilentResult(`{"account":null,"reason":"not_configured"}`)
		}
		return tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	// token 为空视为未配置
	if acc.AccessToken == "" || acc.RefreshToken == "" {
		return tools.SilentResult(`{"account":null,"reason":"token_missing"}`)
	}

	now := time.Now()

	// 已过期
	if !acc.TokenExpiresAt.IsZero() && now.After(acc.TokenExpiresAt) {
		return tools.SilentResult(`{"account":null,"reason":"token_expired"}`)
	}

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
	}

	b, _ := json.Marshal(acc)
	return tools.SilentResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// mi_update_token
// ─────────────────────────────────────────────────────────────────────────────

// UpdateXiaomiTokenTool 更新小米账号的 token 相关字段
// （access_token / refresh_token / expires_in / token_expires_at）。
type UpdateXiaomiTokenTool struct {
	store data.XiaomiAccountStore
}

func NewUpdateXiaomiTokenTool(store data.XiaomiAccountStore) *UpdateXiaomiTokenTool {
	return &UpdateXiaomiTokenTool{store: store}
}

func (t *UpdateXiaomiTokenTool) Name() string { return "mi_update_token" }

func (t *UpdateXiaomiTokenTool) Description() string {
	return "Update the Xiaomi account token fields (access_token, refresh_token, expires_in). " +
		"token_expires_at is calculated automatically from expires_in. " +
		"The account must already exist (use mi_get_account first)."
}

func (t *UpdateXiaomiTokenTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"access_token": map[string]any{
				"type":        "string",
				"description": "New access token",
			},
			"refresh_token": map[string]any{
				"type":        "string",
				"description": "New refresh token",
			},
			"expires_in": map[string]any{
				"type":        "integer",
				"description": "Token lifetime in seconds",
			},
		},
		"required": []string{"access_token", "refresh_token", "expires_in"},
	}
}

func (t *UpdateXiaomiTokenTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	accessToken, ok := args["access_token"].(string)
	if !ok || accessToken == "" {
		return tools.ErrorResult("access_token is required")
	}
	refreshToken, ok := args["refresh_token"].(string)
	if !ok || refreshToken == "" {
		return tools.ErrorResult("refresh_token is required")
	}

	var expiresIn int
	switch v := args["expires_in"].(type) {
	case float64:
		expiresIn = int(v)
	case int:
		expiresIn = v
	default:
		return tools.ErrorResult("expires_in must be an integer")
	}
	if expiresIn <= 0 {
		return tools.ErrorResult("expires_in must be positive")
	}

	acc, err := t.store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return tools.ErrorResult("xiaomi account not configured, cannot update token")
		}
		return tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	acc.AccessToken = accessToken
	acc.RefreshToken = refreshToken
	acc.ExpiresIn = expiresIn
	acc.TokenExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)

	if err := t.store.Save(*acc); err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to save xiaomi account: %v", err))
	}
	return tools.NewToolResult(fmt.Sprintf("xiaomi token updated, expires at %s", acc.TokenExpiresAt.Format(time.RFC3339)))
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
	store       data.XiaomiAccountStore
	oauthClient *miio.MIoTOauthClient
}

func NewSyncXiaomiHomesTool(store data.XiaomiAccountStore, oauthClient *miio.MIoTOauthClient) *SyncXiaomiHomesTool {
	return &SyncXiaomiHomesTool{store: store, oauthClient: oauthClient}
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
	acc, err := t.store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return tools.SilentResult(`{"success":false,"reason":"account_not_configured"}`)
		}
		return tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	if acc.AccessToken == "" {
		return tools.SilentResult(`{"success":false,"reason":"token_missing"}`)
	}

	// 创建 CloudClient
	client, err := miio.NewCloudClient("cn", t.oauthClient.GetClientID(), acc.AccessToken)
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
	store       data.XiaomiAccountStore
	spaceStore  data.SpaceStore
	oauthClient *miio.MIoTOauthClient
}

func NewSyncXiaomiRoomsTool(store data.XiaomiAccountStore, spaceStore data.SpaceStore, oauthClient *miio.MIoTOauthClient) *SyncXiaomiRoomsTool {
	return &SyncXiaomiRoomsTool{store: store, spaceStore: spaceStore, oauthClient: oauthClient}
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

	acc, err := t.store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return tools.SilentResult(`{"synced":false,"reason":"account_not_configured"}`)
		}
		return tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	if acc.AccessToken == "" {
		return tools.SilentResult(`{"synced":false,"reason":"token_missing"}`)
	}

	// 创建 CloudClient
	client, err := miio.NewCloudClient("cn", t.oauthClient.GetClientID(), acc.AccessToken)
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
	existingRoomIDs := make(map[string]bool)
	for _, space := range existingSpaces {
		if space.Type == "room" {
			existingRoomIDs[space.ID] = true
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
		delete(existingRoomIDs, roomID)

		// 查找是否已存在
		existing, _ := t.spaceStore.GetByID(roomID)
		if existing == nil {
			// 新建房间空间
			newSpace := data.Space{
				ID:     roomID,
				Name:   roomDetail.RoomName,
				Type:   "room",
				Source: "xiaomi",
			}
			if err := t.spaceStore.Save(newSpace); err != nil {
				continue
			}
			syncedCount++
		} else {
			// 更新现有空间
			existing.Name = roomDetail.RoomName
			existing.Source = "xiaomi"
			if err := t.spaceStore.Save(*existing); err != nil {
				continue
			}
			updatedCount++
		}
	}

	// 删除不再存在的房间（仅删除来源为 xiaomi 的）
	for roomID := range existingRoomIDs {
		if space, err := t.spaceStore.GetByID(roomID); err == nil && space.Source == "xiaomi" {
			if err := t.spaceStore.Delete(roomID); err == nil {
				removedCount++
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
	oauthClient *miio.MIoTOauthClient
}

func NewSyncXiaomiDevicesTool(store data.XiaomiAccountStore, deviceStore data.DeviceStore, oauthClient *miio.MIoTOauthClient) *SyncXiaomiDevicesTool {
	return &SyncXiaomiDevicesTool{store: store, deviceStore: deviceStore, oauthClient: oauthClient}
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

	acc, err := t.store.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			return tools.SilentResult(`{"synced":false,"reason":"account_not_configured"}`)
		}
		return tools.ErrorResult(fmt.Sprintf("failed to get xiaomi account: %v", err))
	}

	if acc.AccessToken == "" {
		return tools.SilentResult(`{"synced":false,"reason":"token_missing"}`)
	}

	// 创建 CloudClient
	client, err := miio.NewCloudClient("cn", t.oauthClient.GetClientID(), acc.AccessToken)
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
		if dev.Brand == "mijia" {
			existingDeviceIDs[dev.ID] = true
		}
	}

	// 同步设备
	now := time.Now()
	syncedCount := 0
	updatedCount := 0
	removedCount := 0

	for did, deviceInfo := range devicesResult.Devices {
		delete(existingDeviceIDs, did)

		// 查找是否已存在
		existing, _ := t.deviceStore.GetByID(did)
		if existing == nil {
			// 创建设备
			newDevice := data.Device{
				ID:           did,
				Name:         deviceInfo.Name,
				Brand:        "mijia",
				Protocol:     "miio",
				Model:        deviceInfo.Model,
				SpaceID:      deviceInfo.RoomID,
				IP:           deviceInfo.LocalIP,
				Token:        deviceInfo.Token,
				Props:        map[string]string{"online": fmt.Sprintf("%v", deviceInfo.Online)},
				LastSeen:     now,
				AddedAt:      now,
				DID:          deviceInfo.DID,
				UID:          deviceInfo.UID,
				URN:          deviceInfo.URN,
				ConnectType:  deviceInfo.ConnectType,
				Online:       deviceInfo.Online,
				Icon:         deviceInfo.Icon,
				ParentID:     deviceInfo.ParentID,
				Manufacturer: deviceInfo.Manufacturer,
				VoiceCtrl:    deviceInfo.VoiceCtrl,
				SSID:         deviceInfo.SSID,
				BSSID:        deviceInfo.BSSID,
				OrderTime:    deviceInfo.OrderTime,
				FWVersion:    deviceInfo.FWVersion,
				RoomID:       deviceInfo.RoomID,
				RoomName:     deviceInfo.RoomName,
				GroupID:      deviceInfo.GroupID,
			}
			if err := t.deviceStore.Save(newDevice); err != nil {
				continue
			}
			syncedCount++
		} else {
			// 更新设备
			existing.Name = deviceInfo.Name
			existing.Model = deviceInfo.Model
			existing.SpaceID = deviceInfo.RoomID
			existing.IP = deviceInfo.LocalIP
			existing.Token = deviceInfo.Token
			existing.Props = map[string]string{"online": fmt.Sprintf("%v", deviceInfo.Online)}
			existing.LastSeen = now
			existing.DID = deviceInfo.DID
			existing.UID = deviceInfo.UID
			existing.URN = deviceInfo.URN
			existing.ConnectType = deviceInfo.ConnectType
			existing.Online = deviceInfo.Online
			existing.Icon = deviceInfo.Icon
			existing.ParentID = deviceInfo.ParentID
			existing.Manufacturer = deviceInfo.Manufacturer
			existing.VoiceCtrl = deviceInfo.VoiceCtrl
			existing.SSID = deviceInfo.SSID
			existing.BSSID = deviceInfo.BSSID
			existing.OrderTime = deviceInfo.OrderTime
			existing.FWVersion = deviceInfo.FWVersion
			existing.RoomID = deviceInfo.RoomID
			existing.RoomName = deviceInfo.RoomName
			existing.GroupID = deviceInfo.GroupID
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
