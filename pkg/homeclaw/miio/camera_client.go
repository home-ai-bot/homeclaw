package miio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// cameraAPIHost 小米智能摄像头 API 主机
	cameraAPIHost = "business.smartcamera.api.io.mi.com"
)

// XiaomiCameraClient 小米摄像头 HTTP 客户端
//
// 嵌入 baseClient 以复用认证头构建和 HTTP 请求能力。
// 摄像头 API 使用独立的 API 主机（smartcamera），需通过 AccessToken 认证。
type XiaomiCameraClient struct {
	baseClient
}

// NewXiaomiCameraClient 创建 XiaomiCameraClient
//
// 参数:
//   - server:      服务器区域，"cn" 表示中国大陆，其他如 "de"/"us"/"sg"/"ru"/"i2"
//   - clientID:    OAuth2 客户端 ID
//   - accessToken: 访问令牌
func NewXiaomiCameraClient(server, clientID, accessToken string) *XiaomiCameraClient {
	host := cameraAPIHost
	if server != "" && server != "cn" {
		host = server + "." + cameraAPIHost
	}
	return &XiaomiCameraClient{
		baseClient: newBaseClient(host, clientID, accessToken),
	}
}

// UpdateServer 切换服务器区域（线程安全）
func (c *XiaomiCameraClient) UpdateServer(server, clientID, accessToken string) {
	var host string
	if server != "" {
		if server != "cn" {
			host = server + "." + cameraAPIHost
		} else {
			host = cameraAPIHost
		}
	}
	c.updateCredentials(host, clientID, accessToken)
}

// GetEventList 获取摄像头事件列表
//
// 参数:
//   - did:       设备 DID
//   - model:     设备型号
//   - beginTime: 开始时间（毫秒时间戳）
//   - endTime:   结束时间（毫秒时间戳）
//   - limit:     最多返回条数
func (c *XiaomiCameraClient) GetEventList(did, model string, beginTime, endTime int64, limit int) ([]map[string]interface{}, error) {
	apiPath := "/common/app/get/eventlist"
	params := map[string]string{
		"did":       did,
		"model":     model,
		"doorBell":  "false",
		"eventType": "Default",
		"needMerge": "true",
		"sortType":  "DESC",
		"region":    "CN",
		"beginTime": fmt.Sprintf("%d", beginTime),
		"endTime":   fmt.Sprintf("%d", endTime),
		"limit":     fmt.Sprintf("%d", limit),
	}

	resObj, err := c.doGet(apiPath, params)
	if err != nil {
		return nil, err
	}

	data, ok := resObj["data"].(map[string]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response: missing data field"}
	}
	events, ok := data["thirdPartPlayUnits"].([]interface{})
	if !ok {
		return nil, &HTTPError{Message: "invalid response: missing thirdPartPlayUnits field"}
	}

	eventList := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		if m, ok := e.(map[string]interface{}); ok {
			eventList = append(eventList, m)
		}
	}
	return eventList, nil
}

// GetM3U8URL 获取事件视频的 M3U8 播放地址
//
// 参数:
//   - did:        设备 DID
//   - model:      设备型号
//   - fileID:     事件文件 ID
//   - isAlarm:    是否为告警事件
//   - videoCodec: 视频编码格式（如 "H265"）
func (c *XiaomiCameraClient) GetM3U8URL(did, model, fileID string, isAlarm bool, videoCodec string) (string, error) {
	apiPath := "/common/app/m3u8"
	params := map[string]string{
		"did":        did,
		"model":      model,
		"fileId":     fileID,
		"isAlarm":    fmt.Sprintf("%v", isAlarm),
		"videoCodec": videoCodec,
	}

	reqURL := c.baseURLSnapshot() + apiPath
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", &HTTPError{Message: "create request failed: " + err.Error()}
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	for k, v := range c.apiHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &HTTPError{Message: "http request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &HTTPError{Message: "read response failed: " + err.Error()}
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "application/vnd.apple.mpegurl" ||
		contentType == "application/x-mpegURL" {
		return string(body), nil
	}

	// 可能返回 JSON 包含 URL
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err == nil {
		if u, ok := result["url"].(string); ok {
			return u, nil
		}
	}

	return string(body), nil
}

// GetM3U8Playlist 获取并解析 M3U8 播放列表内容
func (c *XiaomiCameraClient) GetM3U8Playlist(m3u8URL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, m3u8URL, nil)
	if err != nil {
		return "", &HTTPError{Message: "create request failed: " + err.Error()}
	}
	for k, v := range c.apiHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &HTTPError{Message: "http request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &HTTPError{Message: "read response failed: " + err.Error()}
	}
	return string(body), nil
}
