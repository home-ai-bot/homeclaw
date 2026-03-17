// Package miio 提供小米 IoT OAuth2 客户端实现
package miio

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	common "github.com/sipeed/picoclaw/pkg/homeclaw/common"
)

const (
	// tokenExpiresTSRatio Token过期时间比例（提前刷新）
	tokenExpiresTSRatio = 0.7
)

// TokenInfo 表示获取到的令牌信息
type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	ExpiresTs    int64  `json:"expires_ts"` // 建议的刷新时间戳
}

// OAuthError 表示 OAuth 相关错误
type OAuthError struct {
	Code    int
	Message string
}

func (e *OAuthError) Error() string {
	return fmt.Sprintf("OAuthError(code=%d): %s", e.Code, e.Message)
}

// MIoTOauthClient 小米 IoT OAuth2 客户端
type MIoTOauthClient struct {
	oauthHost   string
	clientID    string
	redirectURL string
	deviceID    string
	state       string
	httpClient  *http.Client
}

// NewMIoTOauthClient 创建新的 MIoT OAuth 客户端
//
// 参数:
//   - clientID: OAuth2 客户端ID，默认使用 Home Assistant 集成的ID
//   - redirectURL: 回调地址，默认 http://homeassistant.local:8123
//   - cloudServer: 服务器区域，"cn"表示中国大陆，其他如 "de", "us", "sg", "ru", "i2"
//   - uuid: 设备唯一标识，用于生成 device_id
func NewMIoTOauthClient(clientID, redirectURL, cloudServer, uuid string) (*MIoTOauthClient, error) {
	if clientID == "" {
		clientID = OAuth2ClientID
	}
	if redirectURL == "" {
		redirectURL = OAuthRedirectURL
	}
	if cloudServer == "" {
		cloudServer = DefaultCloudServer
	}
	if uuid == "" {
		uuid = common.GenerateUUID()
	}

	oauthHost := DefaultOAuth2APIHost
	if cloudServer != "cn" {
		oauthHost = cloudServer + "." + DefaultOAuth2APIHost
	}

	deviceID := "ha." + uuid
	state := generateState(deviceID)

	return &MIoTOauthClient{
		oauthHost:   oauthHost,
		clientID:    clientID,
		redirectURL: redirectURL,
		deviceID:    deviceID,
		state:       state,
		httpClient: &http.Client{
			Timeout: MIHomeHTTPAPITimeout,
		},
	}, nil
}

// GetState 获取 state 参数（用于验证回调）
func (c *MIoTOauthClient) GetState() string {
	return c.state
}

// GetDeviceID 获取设备ID
func (c *MIoTOauthClient) GetDeviceID() string {
	return c.deviceID
}

// GetClientID 获取客户端ID
func (c *MIoTOauthClient) GetClientID() string {
	return c.clientID
}

// SetRedirectURL 设置回调地址
func (c *MIoTOauthClient) SetRedirectURL(redirectURL string) error {
	if redirectURL == "" {
		return &OAuthError{Code: -1, Message: "invalid redirect_url"}
	}
	c.redirectURL = redirectURL
	return nil
}

// GenAuthURL 生成小米 OAuth2 授权URL
//
// 用户需要在浏览器中打开此URL进行授权，授权后会跳转到回调地址，
// 回调URL格式: http://homeassistant.local:8123?code_value=xxxx
//
// 参数:
//   - redirectURL: 回调地址，为空则使用初始化时的地址
//   - state: 状态参数，用于防止CSRF攻击，为空则使用默认state
//   - scope: 开放数据接口权限ID列表，如 []string{"1", "3", "6"}
//     具体值参考: https://dev.mi.com/distribute/doc/details?pId=1518
//   - skipConfirm: 是否跳过确认页面，true表示授权有效期内直接通过
func (c *MIoTOauthClient) GenAuthURL(redirectURL, state string, scope []string, skipConfirm bool) string {
	if redirectURL == "" {
		redirectURL = c.redirectURL
	}
	if state == "" {
		state = c.state
	}

	params := url.Values{}
	params.Set("redirect_uri", redirectURL)
	params.Set("client_id", c.clientID)
	params.Set("response_type", "code")
	params.Set("device_id", c.deviceID)
	params.Set("state", state)
	params.Set("skip_confirm", strconv.FormatBool(skipConfirm))

	if len(scope) > 0 {
		scopeStr := ""
		for i, s := range scope {
			if i > 0 {
				scopeStr += " "
			}
			scopeStr += s
		}
		params.Set("scope", scopeStr)
	}

	return OAuth2AuthURL + "?" + params.Encode()
}

// GetAccessToken 使用授权码获取 access_token 和 refresh_token
//
// 参数:
//   - code: 从回调URL中获取的授权码 (code_value 参数值)
//     URL格式: http://homeassistant.local:8123?code_value=xxxx
//
// 返回:
//   - TokenInfo: 包含 access_token, refresh_token, expires_in, expires_ts
func (c *MIoTOauthClient) GetAccessToken(code string) (*TokenInfo, error) {
	if code == "" {
		return nil, &OAuthError{Code: -1, Message: "invalid code"}
	}

	data := map[string]interface{}{
		"client_id":    c.clientID,
		"redirect_uri": c.redirectURL,
		"code":         code,
		"device_id":    c.deviceID,
	}

	return c.getToken(data)
}

// RefreshAccessToken 使用 refresh_token 刷新 access_token
//
// 参数:
//   - refreshToken: 之前获取的 refresh_token
//
// 返回:
//   - TokenInfo: 包含新的 access_token, refresh_token, expires_in, expires_ts
func (c *MIoTOauthClient) RefreshAccessToken(refreshToken string) (*TokenInfo, error) {
	if refreshToken == "" {
		return nil, &OAuthError{Code: -1, Message: "invalid refresh_token"}
	}

	data := map[string]interface{}{
		"client_id":     c.clientID,
		"redirect_uri":  c.redirectURL,
		"refresh_token": refreshToken,
	}

	return c.getToken(data)
}

// getToken 内部方法：向小米服务器请求token
func (c *MIoTOauthClient) getToken(data map[string]interface{}) (*TokenInfo, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, &OAuthError{Code: -1, Message: "failed to marshal data: " + err.Error()}
	}

	params := url.Values{}
	params.Set("data", string(dataJSON))

	url := fmt.Sprintf("https://%s/app/v2/ha/oauth/get_token?%s", c.oauthHost, params.Encode())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, &OAuthError{Code: -1, Message: "failed to create request: " + err.Error()}
	}

	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &OAuthError{Code: -1, Message: "http request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &OAuthError{Code: 401, Message: "unauthorized(401)"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &OAuthError{Code: resp.StatusCode, Message: fmt.Sprintf("invalid http status code: %d", resp.StatusCode)}
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &OAuthError{Code: -1, Message: "failed to decode response: " + err.Error()}
	}

	if result.Code != 0 {
		return nil, &OAuthError{Code: result.Code, Message: fmt.Sprintf("api error code: %d, message: %s", result.Code, result.Message)}
	}

	if result.Result.AccessToken == "" || result.Result.RefreshToken == "" {
		return nil, &OAuthError{Code: -1, Message: "invalid response: missing access_token or refresh_token"}
	}

	expiresTs := time.Now().Unix() + int64(float64(result.Result.ExpiresIn)*tokenExpiresTSRatio)

	return &TokenInfo{
		AccessToken:  result.Result.AccessToken,
		RefreshToken: result.Result.RefreshToken,
		ExpiresIn:    result.Result.ExpiresIn,
		ExpiresTs:    expiresTs,
	}, nil
}

// generateState 生成 state 参数
func generateState(deviceID string) string {
	h := sha1.New()
	h.Write([]byte("d=" + deviceID))
	return hex.EncodeToString(h.Sum(nil))
}
