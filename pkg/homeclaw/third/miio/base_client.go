package miio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// baseClient 提供通用的 MIoT HTTP 客户端能力
//
// 封装了认证头构建、GET/POST 请求发送及响应解析逻辑。
// CloudClient 和 XiaomiCameraClient 均嵌入此结构体复用 HTTP 基础能力。
type baseClient struct {
	mu          sync.RWMutex
	host        string
	baseURL     string
	clientID    string
	accessToken string
	httpClient  *http.Client
}

// newBaseClient 初始化 baseClient
func newBaseClient(host, clientID, accessToken string) baseClient {
	return baseClient{
		host:        host,
		baseURL:     "https://" + host,
		clientID:    clientID,
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: MIHomeHTTPAPITimeout},
	}
}

// updateCredentials 更新认证信息（线程安全）
//
// 任意参数为空字符串时忽略该参数。
func (b *baseClient) updateCredentials(host, clientID, accessToken string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if host != "" {
		b.host = host
		b.baseURL = "https://" + host
	}
	if clientID != "" {
		b.clientID = clientID
	}
	if accessToken != "" {
		b.accessToken = accessToken
	}
}

// apiHeaders 构建认证请求头（线程安全）
func (b *baseClient) apiHeaders() map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return map[string]string{
		"Host":           b.host,
		"X-Client-BizId": "haapi",
		"Content-Type":   "application/json",
		// 参照 Python 原文: f'Bearer{self._access_token}' —— 中间无空格
		"Authorization":  "Bearer" + b.accessToken,
		"X-Client-AppId": b.clientID,
	}
}

// hlsHeaders 构建 HLS 直播流请求头（线程安全）
//
// 用于访问小米直播转码服务（livestreaming.io.mi.com）时携带认证信息。
func (b *baseClient) hlsHeaders() map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return map[string]string{
		"Authorization":  "Bearer" + b.accessToken,
		"X-Client-AppId": b.clientID,
		"X-Client-BizId": "haapi",
	}
}

// baseURLSnapshot 获取当前 baseURL 快照（线程安全）
func (b *baseClient) baseURLSnapshot() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.baseURL
}

// doGet 发起 GET 请求并解析 JSON 响应
func (b *baseClient) doGet(urlPath string, params map[string]string) (map[string]interface{}, error) {
	reqURL := b.baseURLSnapshot() + urlPath
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, &HTTPError{Message: "create request failed: " + err.Error()}
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	for k, v := range b.apiHeaders() {
		req.Header.Set(k, v)
	}

	return b.execRequest(req, urlPath)
}

// doPost 发起 POST 请求并解析 JSON 响应
func (b *baseClient) doPost(urlPath string, data map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, &HTTPError{Message: "marshal request failed: " + err.Error()}
	}

	req, err := http.NewRequest(http.MethodPost, b.baseURLSnapshot()+urlPath, bytes.NewReader(body))
	if err != nil {
		return nil, &HTTPError{Message: "create request failed: " + err.Error()}
	}

	for k, v := range b.apiHeaders() {
		req.Header.Set(k, v)
	}

	return b.execRequest(req, urlPath)
}

// execRequest 执行 HTTP 请求，统一处理状态码及 JSON 响应码校验
func (b *baseClient) execRequest(req *http.Request, urlPath string) (map[string]interface{}, error) {
	resp, err := b.httpClient.Do(req)
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
