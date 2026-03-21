// Package miio 提供小米 MIoT HTTP 云端客户端实现
package miio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"
)

const (
	// getPropAggregateInterval 属性聚合请求间隔
	getPropAggregateInterval = 200 * time.Millisecond
	// getPropMaxReqCount 单次最多请求属性数
	getPropMaxReqCount = 150
)

// HTTPError 表示 MIoT HTTP 客户端错误
type HTTPError struct {
	Message string
	Code    int // 0 表示无特定错误码
}

func (e *HTTPError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("MIoTHttpError(code=%d): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("MIoTHttpError: %s", e.Message)
}

// propFuture 代表一个等待中的属性获取请求
type propFuture struct {
	param  map[string]interface{}
	tagged bool
	ch     chan interface{} // 结果通道，nil 表示失败
}

// CloudClient MIoT HTTP 云端客户端
//
// 对应 Python 版 MIoTHttpClient，提供以下能力：
//   - 获取用户信息 / 家庭信息 / 设备列表
//   - 批量读写设备属性（支持聚合请求）
//   - 执行设备 Action
//   - 获取中央证书
type CloudClient struct {
	host        string
	baseURL     string
	clientID    string
	accessToken string
	httpClient  *http.Client

	mu          sync.Mutex
	getPropList map[string]*propFuture // key: "did.siid.piid"
	getPropStop chan struct{}          // 关闭时通知定时器 goroutine 退出
}

// NewCloudClient 创建 CloudClient
//
// 参数:
//   - cloudServer: 服务器区域，"cn" 表示中国大陆，其他如 "de"/"us"/"sg"/"ru"/"i2"
//   - clientID:    OAuth2 客户端 ID
//   - accessToken: 访问令牌
func NewCloudClient(cloudServer, clientID, accessToken string) (*CloudClient, error) {
	if cloudServer == "" || clientID == "" || accessToken == "" {
		return nil, &HTTPError{Message: "invalid params"}
	}

	host := DefaultOAuth2APIHost
	if cloudServer != "cn" {
		host = cloudServer + "." + DefaultOAuth2APIHost
	}

	return &CloudClient{
		host:        host,
		baseURL:     "https://" + host,
		clientID:    clientID,
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: MIHomeHTTPAPITimeout},
		getPropList: make(map[string]*propFuture),
		getPropStop: make(chan struct{}),
	}, nil
}

// UpdateHTTPHeader 更新认证头信息
//
// 任意参数为空字符串时忽略该参数。
func (c *CloudClient) UpdateHTTPHeader(cloudServer, clientID, accessToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cloudServer != "" {
		if cloudServer != "cn" {
			c.host = cloudServer + "." + DefaultOAuth2APIHost
		} else {
			c.host = DefaultOAuth2APIHost
		}
		c.baseURL = "https://" + c.host
	}
	if clientID != "" {
		c.clientID = clientID
	}
	if accessToken != "" {
		c.accessToken = accessToken
	}
}

// Close 释放资源，停止内部 goroutine
func (c *CloudClient) Close() {
	select {
	case <-c.getPropStop:
		// already closed
	default:
		close(c.getPropStop)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, fut := range c.getPropList {
		select {
		case fut.ch <- nil:
		default:
		}
	}
	c.getPropList = make(map[string]*propFuture)
}

// ---------- 内部 HTTP 辅助方法 ----------

func (c *CloudClient) apiHeaders() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]string{
		"Host":           c.host,
		"X-Client-BizId": "haapi",
		"Content-Type":   "application/json",
		// Python 原文: f'Bearer{self._access_token}' —— 注意中间无空格
		"Authorization":  "Bearer" + c.accessToken,
		"X-Client-AppId": c.clientID,
	}
}

func (c *CloudClient) baseURLSnapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseURL
}

func (c *CloudClient) doGet(urlPath string, params map[string]string) (map[string]interface{}, error) {
	reqURL := c.baseURLSnapshot() + urlPath
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &HTTPError{Message: "create request failed: " + err.Error()}
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	for k, v := range c.apiHeaders() {
		req.Header.Set(k, v)
	}

	return c.execRequest(req, urlPath)
}

func (c *CloudClient) doPost(urlPath string, data map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, &HTTPError{Message: "marshal request failed: " + err.Error()}
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURLSnapshot()+urlPath, bytes.NewReader(body))
	if err != nil {
		return nil, &HTTPError{Message: "create request failed: " + err.Error()}
	}

	for k, v := range c.apiHeaders() {
		req.Header.Set(k, v)
	}

	return c.execRequest(req, urlPath)
}

func (c *CloudClient) execRequest(req *http.Request, urlPath string) (map[string]interface{}, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &HTTPError{Message: "http request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &HTTPError{
			Message: "mihome api request failed, unauthorized(401)",
			Code:    401,
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			Message: fmt.Sprintf("mihome api request failed, %d, %s", resp.StatusCode, urlPath),
		}
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &HTTPError{Message: "read response failed: " + err.Error()}
	}

	var resObj map[string]interface{}
	if err := json.Unmarshal(raw, &resObj); err != nil {
		return nil, &HTTPError{Message: "decode response failed: " + err.Error()}
	}

	code, _ := resObj["code"].(float64)
	if int(code) != 0 {
		msg, _ := resObj["message"].(string)
		return nil, &HTTPError{
			Message: fmt.Sprintf("invalid response code, %d, %s", int(code), msg),
		}
	}

	return resObj, nil
}

// ---------- 公开 API ----------

// GetUserInfo 获取用户基本信息（昵称等）
//
// 返回 map 中包含 miliaoNick 等字段。
func (c *CloudClient) GetUserInfo() (map[string]interface{}, error) {
	c.mu.Lock()
	cid := c.clientID
	tok := c.accessToken
	c.mu.Unlock()

	req, err := http.NewRequest(http.MethodGet,
		"https://open.account.xiaomi.com/user/profile", nil)
	if err != nil {
		return nil, &HTTPError{Message: "create request failed: " + err.Error()}
	}
	q := req.URL.Query()
	q.Set("clientId", cid)
	q.Set("token", tok)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &HTTPError{Message: "http request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &HTTPError{Message: "read response failed: " + err.Error()}
	}

	var resObj map[string]interface{}
	if err := json.Unmarshal(raw, &resObj); err != nil {
		return nil, &OAuthError{Code: -1, Message: fmt.Sprintf("invalid http response, %s", string(raw))}
	}

	code, _ := resObj["code"].(float64)
	data, hasData := resObj["data"].(map[string]interface{})
	if int(code) != 0 || !hasData {
		return nil, &OAuthError{Code: -1, Message: fmt.Sprintf("invalid http response, %s", string(raw))}
	}
	if _, ok := data["miliaoNick"]; !ok {
		return nil, &OAuthError{Code: -1, Message: fmt.Sprintf("invalid http response, %s", string(raw))}
	}

	return data, nil
}

// GetCentralCert 获取中央证书
//
// 参数:
//   - csr: PEM 格式的证书签名请求字符串
//
// 返回:
//   - cert: 签名后的证书字符串
func (c *CloudClient) GetCentralCert(csr string) (string, error) {
	if csr == "" {
		return "", &HTTPError{Message: "invalid params"}
	}

	resObj, err := c.doPost("/app/v2/ha/oauth/get_central_crt", map[string]interface{}{
		"csr": base64.StdEncoding.EncodeToString([]byte(csr)),
	})
	if err != nil {
		return "", err
	}

	result, ok := resObj["result"].(map[string]interface{})
	if !ok {
		return "", &HTTPError{Message: "invalid response result"}
	}
	cert, ok := result["cert"].(string)
	if !ok || cert == "" {
		return "", &HTTPError{Message: "invalid cert"}
	}

	return cert, nil
}

// DevRoomPageResult 家庭房间分页结果，key 为 home_id
type DevRoomPageResult map[string]*HomeRoomInfo

// HomeRoomInfo 家庭及房间设备信息
type HomeRoomInfo struct {
	DIDs     []string             `json:"dids"`
	RoomInfo map[string]*RoomDIDs `json:"room_info"`
}

// RoomDIDs 房间设备 ID 列表
type RoomDIDs struct {
	DIDs []string `json:"dids"`
}

// getDevRoomPage 内部分页获取家庭房间设备信息
func (c *CloudClient) getDevRoomPage(maxID string) (DevRoomPageResult, error) {
	data := map[string]interface{}{
		"limit": 150,
	}
	if maxID != "" {
		data["start_id"] = maxID
	}

	resObj, err := c.doPost("/app/v2/homeroom/get_dev_room_page", data)
	if err != nil {
		return nil, err
	}

	result, ok := resObj["result"].(map[string]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result"}
	}
	info, ok := result["info"].([]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result: missing info"}
	}

	homeList := make(DevRoomPageResult)
	for _, rawHome := range info {
		home, ok := rawHome.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := home["id"]
		if !ok {
			continue
		}
		homeID := fmt.Sprintf("%v", id)

		entry := &HomeRoomInfo{
			DIDs:     extractStrSlice(home["dids"]),
			RoomInfo: make(map[string]*RoomDIDs),
		}

		if roomlist, ok := home["roomlist"].([]interface{}); ok {
			for _, rawRoom := range roomlist {
				room, ok := rawRoom.(map[string]interface{})
				if !ok {
					continue
				}
				rid, ok := room["id"]
				if !ok {
					continue
				}
				entry.RoomInfo[fmt.Sprintf("%v", rid)] = &RoomDIDs{
					DIDs: extractStrSlice(room["dids"]),
				}
			}
		}

		homeList[homeID] = entry
	}

	// 翻页
	hasMore, _ := result["has_more"].(bool)
	nextMaxID, _ := result["max_id"].(string)
	if hasMore && nextMaxID != "" {
		nextList, err := c.getDevRoomPage(nextMaxID)
		if err != nil {
			return nil, err
		}
		for hid, info := range nextList {
			if existing, ok := homeList[hid]; ok {
				existing.DIDs = append(existing.DIDs, info.DIDs...)
				for rid, rinfo := range info.RoomInfo {
					if existingRoom, ok := existing.RoomInfo[rid]; ok {
						existingRoom.DIDs = append(existingRoom.DIDs, rinfo.DIDs...)
					} else {
						existing.RoomInfo[rid] = rinfo
					}
				}
			} else {
				homeList[hid] = info
			}
		}
	}

	return homeList, nil
}

// HomeInfoResult GetHomeInfos 返回结构
type HomeInfoResult struct {
	UID           string                 `json:"uid"`
	HomeList      map[string]*HomeDetail `json:"home_list"`
	ShareHomeList map[string]*HomeDetail `json:"share_home_list"`
}

// HomeDetail 单个家庭详情
type HomeDetail struct {
	HomeID    string                 `json:"home_id"`
	HomeName  string                 `json:"home_name"`
	CityID    interface{}            `json:"city_id"`
	Longitude interface{}            `json:"longitude"`
	Latitude  interface{}            `json:"latitude"`
	Address   interface{}            `json:"address"`
	DIDs      []string               `json:"dids"`
	RoomInfo  map[string]*RoomDetail `json:"room_info"`
	GroupID   string                 `json:"group_id"`
	UID       string                 `json:"uid"`
}

// RoomDetail 房间详情
type RoomDetail struct {
	RoomID   string   `json:"room_id"`
	RoomName string   `json:"room_name"`
	DIDs     []string `json:"dids"`
}

// GetHomeInfos 获取家庭信息（包含自有和共享家庭）
func (c *CloudClient) GetHomeInfos() (*HomeInfoResult, error) {
	resObj, err := c.doPost("/app/v2/homeroom/gethome", map[string]interface{}{
		"limit":           150,
		"fetch_share":     true,
		"fetch_share_dev": true,
		"plat_form":       0,
		"app_ver":         9,
	})
	if err != nil {
		return nil, err
	}

	result, ok := resObj["result"].(map[string]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result"}
	}

	var uid string
	homeInfos := map[string]map[string]*HomeDetail{
		"homelist":        {},
		"share_home_list": {},
	}

	for _, deviceSource := range []string{"homelist", "share_home_list"} {
		rawList, _ := result[deviceSource].([]interface{})
		for _, rawHome := range rawList {
			home, ok := rawHome.(map[string]interface{})
			if !ok {
				continue
			}
			if _, ok := home["id"]; !ok {
				continue
			}
			if _, ok := home["name"]; !ok {
				continue
			}
			if _, ok := home["roomlist"]; !ok {
				continue
			}

			homeIDRaw := home["id"]
			homeID := fmt.Sprintf("%v", homeIDRaw)
			uidRaw := intValStr(home["uid"])

			if uid == "" && deviceSource == "homelist" {
				uid = uidRaw
			}

			roomInfo := make(map[string]*RoomDetail)
			if roomlist, ok := home["roomlist"].([]interface{}); ok {
				for _, rawRoom := range roomlist {
					room, ok := rawRoom.(map[string]interface{})
					if !ok {
						continue
					}
					if _, ok := room["id"]; !ok {
						continue
					}
					rid := fmt.Sprintf("%v", room["id"])
					rname, _ := room["name"].(string)
					roomInfo[rid] = &RoomDetail{
						RoomID:   rid,
						RoomName: rname,
						DIDs:     extractStrSlice(room["dids"]),
					}
				}
			}

			homeInfos[deviceSource][homeID] = &HomeDetail{
				HomeID:    homeID,
				HomeName:  fmt.Sprintf("%v", home["name"]),
				CityID:    home["city_id"],
				Longitude: home["longitude"],
				Latitude:  home["latitude"],
				Address:   home["address"],
				DIDs:      extractStrSlice(home["dids"]),
				RoomInfo:  roomInfo,
				GroupID:   calcGroupID(uidRaw, homeID),
				UID:       uidRaw,
			}
		}
	}

	// 翻页
	hasMore, _ := result["has_more"].(bool)
	maxID, _ := result["max_id"].(string)
	if hasMore && maxID != "" {
		moreList, err := c.getDevRoomPage(maxID)
		if err != nil {
			return nil, err
		}
		for _, deviceSource := range []string{"homelist", "share_home_list"} {
			for hid, info := range moreList {
				detail, exists := homeInfos[deviceSource][hid]
				if !exists {
					continue
				}
				detail.DIDs = append(detail.DIDs, info.DIDs...)
				for rid, rinfo := range info.RoomInfo {
					if existing, ok := detail.RoomInfo[rid]; ok {
						existing.DIDs = append(existing.DIDs, rinfo.DIDs...)
					} else {
						detail.RoomInfo[rid] = &RoomDetail{
							RoomID:   rid,
							RoomName: "",
							DIDs:     rinfo.DIDs,
						}
					}
				}
			}
		}
	}

	return &HomeInfoResult{
		UID:           uid,
		HomeList:      homeInfos["homelist"],
		ShareHomeList: homeInfos["share_home_list"],
	}, nil
}

// GetUID 获取当前用户 UID
func (c *CloudClient) GetUID() (string, error) {
	info, err := c.GetHomeInfos()
	if err != nil {
		return "", err
	}
	return info.UID, nil
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	DID          string                 `json:"did"`
	UID          string                 `json:"uid"`
	Name         string                 `json:"name"`
	URN          string                 `json:"urn"`
	Model        string                 `json:"model"`
	ConnectType  int                    `json:"connect_type"`
	Token        string                 `json:"token"`
	Online       bool                   `json:"online"`
	Icon         string                 `json:"icon"`
	ParentID     string                 `json:"parent_id"`
	Manufacturer string                 `json:"manufacturer"`
	VoiceCtrl    int                    `json:"voice_ctrl"`
	RSSI         interface{}            `json:"rssi"`
	Owner        interface{}            `json:"owner"`
	PID          interface{}            `json:"pid"`
	LocalIP      string                 `json:"local_ip"`
	SSID         string                 `json:"ssid"`
	BSSID        string                 `json:"bssid"`
	OrderTime    int64                  `json:"order_time"`
	FWVersion    string                 `json:"fw_version"`
	SubDevices   map[string]*DeviceInfo `json:"sub_devices,omitempty"`

	// 家庭/房间信息（由 GetDevices 填充）
	HomeID   string `json:"home_id,omitempty"`
	HomeName string `json:"home_name,omitempty"`
	RoomID   string `json:"room_id,omitempty"`
	RoomName string `json:"room_name,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
}

// unsupportedModels 不支持的设备型号（参考 Python 原版）
var unsupportedModels = map[string]bool{}

// getDeviceListPage 内部分页获取设备列表
func (c *CloudClient) getDeviceListPage(dids []string, startDID string) (map[string]*DeviceInfo, error) {
	reqData := map[string]interface{}{
		"limit":            200,
		"get_split_device": true,
		"get_third_device": true,
		"dids":             dids,
	}
	if startDID != "" {
		reqData["start_did"] = startDID
	}

	resObj, err := c.doPost("/app/v2/home/device_list_page", reqData)
	if err != nil {
		return nil, err
	}

	result, ok := resObj["result"].(map[string]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result"}
	}

	deviceInfos := make(map[string]*DeviceInfo)
	deviceList, _ := result["list"].([]interface{})
	for _, rawDev := range deviceList {
		device, ok := rawDev.(map[string]interface{})
		if !ok {
			continue
		}
		did, _ := device["did"].(string)
		name, _ := device["name"].(string)
		urn, _ := device["spec_type"].(string)
		model, _ := device["model"].(string)

		if did == "" || name == "" {
			continue
		}
		if urn == "" || model == "" {
			continue
		}

		// 忽略 miwifi.* 路由器
		if len(did) > 7 && did[:7] == "miwifi." {
			continue
		}
		// 忽略不支持的型号
		if unsupportedModels[model] {
			continue
		}

		manufacturer := model
		if idx := indexByte(model, '.'); idx >= 0 {
			manufacturer = model[:idx]
		}

		extra, _ := device["extra"].(map[string]interface{})
		fwVersion, _ := extra["fw_version"].(string)

		pid := device["pid"]
		connectType := 0
		if pidFloat, ok := pid.(float64); ok {
			connectType = int(pidFloat)
		} else {
			connectType = -1
		}

		voiceCtrl := 0
		if vc, ok := device["voice_ctrl"].(float64); ok {
			voiceCtrl = int(vc)
		}

		orderTime := int64(0)
		if ot, ok := device["orderTime"].(float64); ok {
			orderTime = int64(ot)
		}

		deviceInfos[did] = &DeviceInfo{
			DID:          did,
			UID:          intValStr(device["uid"]),
			Name:         name,
			URN:          urn,
			Model:        model,
			ConnectType:  connectType,
			Token:        strVal(device["token"]),
			Online:       boolVal(device["isOnline"]),
			Icon:         strVal(device["icon"]),
			ParentID:     strVal(device["parent_id"]),
			Manufacturer: manufacturer,
			VoiceCtrl:    voiceCtrl,
			RSSI:         device["rssi"],
			Owner:        device["owner"],
			PID:          pid,
			LocalIP:      strVal(device["local_ip"]),
			SSID:         strVal(device["ssid"]),
			BSSID:        strVal(device["bssid"]),
			OrderTime:    orderTime,
			FWVersion:    fwVersion,
		}
	}

	// 翻页
	hasMore, _ := result["has_more"].(bool)
	nextStartDID, _ := result["next_start_did"].(string)
	if hasMore && nextStartDID != "" {
		nextPage, err := c.getDeviceListPage(dids, nextStartDID)
		if err != nil {
			return nil, err
		}
		for k, v := range nextPage {
			deviceInfos[k] = v
		}
	}

	return deviceInfos, nil
}

// GetDevicesWithDIDs 根据 DID 列表批量获取设备信息（每批最多 150 个）
func (c *CloudClient) GetDevicesWithDIDs(dids []string) (map[string]*DeviceInfo, error) {
	devices := make(map[string]*DeviceInfo)
	for i := 0; i < len(dids); i += 150 {
		end := i + 150
		if end > len(dids) {
			end = len(dids)
		}
		batch, err := c.getDeviceListPage(dids[i:end], "")
		if err != nil {
			return nil, err
		}
		if batch == nil {
			return nil, &HTTPError{Message: "get devices failed"}
		}
		for k, v := range batch {
			devices[k] = v
		}
	}
	return devices, nil
}

// DevicesResult GetDevices 返回结构
type DevicesResult struct {
	UID     string                            `json:"uid"`
	Homes   map[string]map[string]interface{} `json:"homes"`
	Devices map[string]*DeviceInfo            `json:"devices"`
}

// GetDevices 获取所有设备信息（含家庭/房间归属）
//
// homeIDs 为 nil 时获取所有家庭的设备；否则仅获取指定家庭 ID 的设备。
func (c *CloudClient) GetDevices(homeIDs []string) (*DevicesResult, error) {
	homeinfos, err := c.GetHomeInfos()
	if err != nil {
		return nil, err
	}

	homes := map[string]map[string]interface{}{
		"home_list":       {},
		"share_home_list": {},
	}
	devices := make(map[string]*DeviceInfo)

	buildDevices := func(deviceType string, homeMap map[string]*HomeDetail) {
		for homeID, homeDetail := range homeMap {
			if homeIDs != nil && !containsStr(homeIDs, homeID) {
				continue
			}
			homes[deviceType][homeID] = map[string]interface{}{
				"home_name": homeDetail.HomeName,
				"uid":       homeDetail.UID,
				"group_id":  homeDetail.GroupID,
				"room_info": map[string]string{},
			}
			// 家庭级别直属设备（未归入房间的）
			for _, did := range homeDetail.DIDs {
				devices[did] = &DeviceInfo{
					HomeID:   homeID,
					HomeName: homeDetail.HomeName,
					RoomID:   homeID,
					RoomName: homeDetail.HomeName,
					GroupID:  homeDetail.GroupID,
				}
			}
			// 房间级别设备
			for roomID, room := range homeDetail.RoomInfo {
				if ri, ok := homes[deviceType][homeID].(map[string]interface{}); ok {
					if riMap, ok := ri["room_info"].(map[string]string); ok {
						riMap[roomID] = room.RoomName
					}
				}
				for _, did := range room.DIDs {
					devices[did] = &DeviceInfo{
						HomeID:   homeID,
						HomeName: homeDetail.HomeName,
						RoomID:   roomID,
						RoomName: room.RoomName,
						GroupID:  homeDetail.GroupID,
					}
				}
			}
		}
	}

	buildDevices("home_list", homeinfos.HomeList)
	buildDevices("share_home_list", homeinfos.ShareHomeList)

	// 获取独立共享设备
	separatedShared, err := c.GetSeparatedSharedDevices()
	if err != nil {
		return nil, err
	}
	if len(separatedShared) > 0 {
		homes["separated_shared_list"] = make(map[string]interface{})
		for did, owner := range separatedShared {
			ownerMap, ok := owner.(map[string]interface{})
			if !ok {
				continue
			}
			ownerID := strVal(ownerMap["userid"])
			nickname := strVal(ownerMap["nickname"])
			if _, exists := homes["separated_shared_list"][ownerID]; !exists {
				homes["separated_shared_list"][ownerID] = map[string]interface{}{
					"home_name": nickname,
					"uid":       ownerID,
					"group_id":  "NotSupport",
					"room_info": map[string]string{"shared_device": "shared_device"},
				}
			}
			devices[did] = &DeviceInfo{
				HomeID:   ownerID,
				HomeName: nickname,
				RoomID:   "shared_device",
				RoomName: "shared_device",
				GroupID:  "NotSupport",
			}
		}
	}

	// 批量获取设备详情
	didList := make([]string, 0, len(devices))
	for did := range devices {
		didList = append(didList, did)
	}

	results, err := c.GetDevicesWithDIDs(didList)
	if err != nil {
		return nil, &HTTPError{Message: "get devices failed: " + err.Error()}
	}

	// 合并设备详情，处理子设备
	subDeviceRe := regexp.MustCompile(`\.s\d+$`)
	for _, did := range didList {
		detail, ok := results[did]
		if !ok {
			delete(devices, did)
			continue
		}
		// 合并房间/家庭信息到详情
		if loc, ok := devices[did]; ok {
			detail.HomeID = loc.HomeID
			detail.HomeName = loc.HomeName
			detail.RoomID = loc.RoomID
			detail.RoomName = loc.RoomName
			detail.GroupID = loc.GroupID
		}
		devices[did] = detail

		// 处理子设备（did 以 .sN 结尾）
		match := subDeviceRe.FindString(did)
		if match == "" {
			continue
		}
		parentDID := did[:len(did)-len(match)]
		parent, parentExists := devices[parentDID]
		if !parentExists {
			continue
		}
		if parent.SubDevices == nil {
			parent.SubDevices = make(map[string]*DeviceInfo)
		}
		parent.SubDevices[match[1:]] = devices[did]
		delete(devices, did)
	}

	return &DevicesResult{
		UID:     homeinfos.UID,
		Homes:   homes,
		Devices: devices,
	}, nil
}

// GetSeparatedSharedDevices 获取独立共享设备（非家庭共享）
//
// 返回 map[did]owner，owner 为包含 userid/nickname 字段的 map。
func (c *CloudClient) GetSeparatedSharedDevices() (map[string]interface{}, error) {
	deviceList, err := c.getDeviceListPage([]string{}, "")
	if err != nil {
		return nil, err
	}
	result := make(map[string]interface{})
	for did, dev := range deviceList {
		if dev.Owner == nil {
			continue
		}
		ownerMap, ok := dev.Owner.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasUID := ownerMap["userid"]; !hasUID {
			continue
		}
		if _, hasNick := ownerMap["nickname"]; !hasNick {
			continue
		}
		result[did] = dev.Owner
	}
	return result, nil
}

// PropParam 属性参数
type PropParam struct {
	DID  string `json:"did"`
	SIID int    `json:"siid"`
	PIID int    `json:"piid"`
}

// PropResult 单个属性读取结果
type PropResult struct {
	DID   string      `json:"did"`
	SIID  int         `json:"siid"`
	PIID  int         `json:"piid"`
	Value interface{} `json:"value"`
	Code  int         `json:"code"`
}

// GetProps 批量获取设备属性
//
// 示例:
//
//	params := []PropParam{{DID: "xxxx", SIID: 2, PIID: 1}, {DID: "xxxx", SIID: 2, PIID: 2}}
func (c *CloudClient) GetProps(params []PropParam) ([]PropResult, error) {
	paramList := make([]map[string]interface{}, len(params))
	for i, p := range params {
		paramList[i] = map[string]interface{}{
			"did":  p.DID,
			"siid": p.SIID,
			"piid": p.PIID,
		}
	}

	resObj, err := c.doPost("/app/v2/miotspec/prop/get", map[string]interface{}{
		"datasource": 1,
		"params":     paramList,
	})
	if err != nil {
		return nil, err
	}

	rawResult, ok := resObj["result"].([]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result"}
	}

	results := make([]PropResult, 0, len(rawResult))
	for _, rawItem := range rawResult {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}
		pr := PropResult{
			DID:   strVal(item["did"]),
			Value: item["value"],
		}
		if siid, ok := item["siid"].(float64); ok {
			pr.SIID = int(siid)
		}
		if piid, ok := item["piid"].(float64); ok {
			pr.PIID = int(piid)
		}
		if code, ok := item["code"].(float64); ok {
			pr.Code = int(code)
		}
		results = append(results, pr)
	}

	return results, nil
}

// GetProp 获取单个设备属性（立即模式）
func (c *CloudClient) GetProp(did string, siid, piid int) (interface{}, error) {
	results, err := c.GetProps([]PropParam{{DID: did, SIID: siid, PIID: piid}})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0].Value, nil
}

// GetPropAggregated 聚合获取单个属性（与其他请求合并批量发送）
//
// 在 getPropAggregateInterval 时间窗口内，多个调用会被合并为一次批量请求。
// 若属性已在队列中则等待同一 Future 返回。
func (c *CloudClient) GetPropAggregated(did string, siid, piid int) (interface{}, error) {
	key := fmt.Sprintf("%s.%d.%d", did, siid, piid)

	c.mu.Lock()
	fut, exists := c.getPropList[key]
	if !exists {
		fut = &propFuture{
			param: map[string]interface{}{"did": did, "siid": siid, "piid": piid},
			ch:    make(chan interface{}, 1),
		}
		c.getPropList[key] = fut
		// 首次加入：启动聚合定时器
		go c.schedulePropHandler()
	}
	ch := fut.ch
	c.mu.Unlock()

	val := <-ch
	return val, nil
}

// schedulePropHandler 等待聚合窗口后触发批量请求
func (c *CloudClient) schedulePropHandler() {
	select {
	case <-time.After(getPropAggregateInterval):
		c.flushPropRequests()
	case <-c.getPropStop:
	}
}

// flushPropRequests 批量执行队列中的属性请求
func (c *CloudClient) flushPropRequests() {
	c.mu.Lock()
	if len(c.getPropList) == 0 {
		c.mu.Unlock()
		return
	}

	// 取出未处理的请求（最多 getPropMaxReqCount 个）
	paramList := make([]PropParam, 0, getPropMaxReqCount)
	keys := make([]string, 0, getPropMaxReqCount)
	for key, item := range c.getPropList {
		if item.tagged {
			continue
		}
		if len(paramList) >= getPropMaxReqCount {
			break
		}
		item.tagged = true
		did, _ := item.param["did"].(string)
		siid, _ := item.param["siid"].(int)
		piid, _ := item.param["piid"].(int)
		// param 存储为 float64（JSON 反序列化原因），也可能是 int
		if s, ok := item.param["siid"].(float64); ok {
			siid = int(s)
		}
		if p, ok := item.param["piid"].(float64); ok {
			piid = int(p)
		}
		paramList = append(paramList, PropParam{DID: did, SIID: siid, PIID: piid})
		keys = append(keys, key)
	}
	c.mu.Unlock()

	if len(paramList) == 0 {
		return
	}

	results, err := c.GetProps(paramList)
	satisfied := make(map[string]bool)

	if err == nil {
		for _, result := range results {
			if result.Value == nil {
				continue
			}
			key := fmt.Sprintf("%s.%d.%d", result.DID, result.SIID, result.PIID)
			c.mu.Lock()
			if fut, ok := c.getPropList[key]; ok {
				fut.ch <- result.Value
				delete(c.getPropList, key)
				satisfied[key] = true
			}
			c.mu.Unlock()
		}
	}

	// 未满足的请求发送 nil
	c.mu.Lock()
	for _, key := range keys {
		if !satisfied[key] {
			if fut, ok := c.getPropList[key]; ok {
				fut.ch <- nil
				delete(c.getPropList, key)
			}
		}
	}
	// 若队列中还有未处理项，继续调度
	remaining := len(c.getPropList) > 0
	c.mu.Unlock()

	if remaining {
		go c.schedulePropHandler()
	}
}

// SetProps 批量设置设备属性
//
// 示例:
//
//	params := []map[string]interface{}{{"did":"xxxx","siid":2,"piid":1,"value":false}}
func (c *CloudClient) SetProps(params []map[string]interface{}) ([]interface{}, error) {
	resObj, err := c.doPost("/app/v2/miotspec/prop/set", map[string]interface{}{
		"params": params,
	})
	if err != nil {
		return nil, err
	}

	rawResult, ok := resObj["result"].([]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result"}
	}
	return rawResult, nil
}

// Action 执行设备 Action
//
// 示例:
//
//	result, err := client.Action("xxxx", 2, 1, []map[string]interface{}{{"value": 42}})
func (c *CloudClient) Action(did string, siid, aiid int, inList []map[string]interface{}) (map[string]interface{}, error) {
	values := make([]interface{}, len(inList))
	for i, item := range inList {
		values[i] = item["value"]
	}

	resObj, err := c.doPost("/app/v2/miotspec/action", map[string]interface{}{
		"params": map[string]interface{}{
			"did":  did,
			"siid": siid,
			"aiid": aiid,
			"in":   values,
		},
	})
	if err != nil {
		return nil, err
	}

	result, ok := resObj["result"].(map[string]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response result"}
	}
	return result, nil
}

// ---------- 辅助函数 ----------

// calcGroupID 计算家庭 group_id（与 Python 版 calc_group_id 对应）
// 实际算法根据项目需要实现，此处使用 uid_homeID 格式作为占位
func calcGroupID(uid, homeID string) string {
	return uid + "_" + homeID
}

// extractStrSlice 从 interface{} 提取字符串切片
func extractStrSlice(raw interface{}) []string {
	list, ok := raw.([]interface{})
	if !ok || list == nil {
		return []string{}
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// strVal 安全地将 interface{} 转为字符串
func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// intValStr 将 interface{} 转为整数字符串（避免科学计数法）
// 用于处理 JSON 反序列化后的 float64 类型大整数
func intValStr(v interface{}) string {
	if v == nil {
		return ""
	}
	// JSON numbers are unmarshaled as float64
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%.0f", f)
	}
	if f, ok := v.(float32); ok {
		return fmt.Sprintf("%.0f", f)
	}
	if i, ok := v.(int64); ok {
		return fmt.Sprintf("%d", i)
	}
	if i, ok := v.(int); ok {
		return fmt.Sprintf("%d", i)
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// boolVal 安全地将 interface{} 转为 bool
func boolVal(v interface{}) bool {
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// indexByte 查找字节在字符串中的首次出现位置
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// containsStr 判断字符串切片中是否包含指定值
func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
