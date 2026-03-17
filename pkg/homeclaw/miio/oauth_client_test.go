// Package miio 提供小米 IoT OAuth2 客户端的单元测试
package miio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewMIoTOauthClient(t *testing.T) {
	tests := []struct {
		name        string
		clientID    string
		redirectURL string
		cloudServer string
		uuid        string
		wantErr     bool
	}{
		{
			name:        "使用默认参数创建",
			clientID:    "",
			redirectURL: "",
			cloudServer: "",
			uuid:        "",
			wantErr:     false,
		},
		{
			name:        "使用自定义参数创建",
			clientID:    "2882303761520251711",
			redirectURL: "http://homeassistant.local:8123",
			cloudServer: "cn",
			uuid:        "test-uuid-123",
			wantErr:     false,
		},
		{
			name:        "使用海外服务器",
			clientID:    "2882303761520251711",
			redirectURL: "http://homeassistant.local:8123",
			cloudServer: "de",
			uuid:        "test-uuid-456",
			wantErr:     false,
		},
		{
			name:        "无效的clientID",
			clientID:    "invalid-client-id",
			redirectURL: "http://homeassistant.local:8123",
			cloudServer: "cn",
			uuid:        "test-uuid",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewMIoTOauthClient(tt.clientID, tt.redirectURL, tt.cloudServer, tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMIoTOauthClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// 验证客户端创建成功
			if client == nil {
				t.Error("NewMIoTOauthClient() returned nil client")
				return
			}

			// 验证 deviceID 格式
			if !strings.HasPrefix(client.GetDeviceID(), "ha.") {
				t.Errorf("deviceID should start with 'ha.', got %s", client.GetDeviceID())
			}

			// 验证 state 不为空
			if client.GetState() == "" {
				t.Error("state should not be empty")
			}

			// 验证海外服务器主机名
			if tt.cloudServer != "" && tt.cloudServer != "cn" {
				expectedHost := tt.cloudServer + "." + DefaultOAuth2APIHost
				if client.oauthHost != expectedHost {
					t.Errorf("oauthHost = %s, want %s", client.oauthHost, expectedHost)
				}
			}
		})
	}
}

func TestMIoTOauthClient_GetState(t *testing.T) {
	client, _ := NewMIoTOauthClient("", "", "cn", "test-uuid")
	state := client.GetState()

	if state == "" {
		t.Error("GetState() returned empty string")
	}

	// 验证 state 是40位十六进制字符串（SHA1长度）
	if len(state) != 40 {
		t.Errorf("GetState() returned string of length %d, want 40", len(state))
	}
}

func TestMIoTOauthClient_GetDeviceID(t *testing.T) {
	uuid := "my-test-uuid"
	client, _ := NewMIoTOauthClient("", "", "cn", uuid)
	deviceID := client.GetDeviceID()

	expected := "ha." + uuid
	if deviceID != expected {
		t.Errorf("GetDeviceID() = %s, want %s", deviceID, expected)
	}
}

func TestMIoTOauthClient_SetRedirectURL(t *testing.T) {
	client, _ := NewMIoTOauthClient("", "http://homeassistant.local:8123", "cn", "test-uuid")

	tests := []struct {
		name        string
		redirectURL string
		wantErr     bool
	}{
		{
			name:        "设置有效的回调地址",
			redirectURL: "https://example.com/callback",
			wantErr:     false,
		},
		{
			name:        "设置空地址应该失败",
			redirectURL: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetRedirectURL(tt.redirectURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetRedirectURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && client.redirectURL != tt.redirectURL {
				t.Errorf("redirectURL = %s, want %s", client.redirectURL, tt.redirectURL)
			}
		})
	}
}

func TestMIoTOauthClient_GenAuthURL(t *testing.T) {
	client, _ := NewMIoTOauthClient("2882303761520251711", "http://homeassistant.local:8123", "cn", "test-uuid")

	tests := []struct {
		name        string
		redirectURL string
		state       string
		scope       []string
		skipConfirm bool
		wantContain []string
	}{
		{
			name:        "生成基本授权URL",
			redirectURL: "",
			state:       "",
			scope:       nil,
			skipConfirm: false,
			wantContain: []string{
				"https://account.xiaomi.com/oauth2/authorize",
				"client_id=2882303761520251711",
				"response_type=code",
				"redirect_uri=",
				"device_id=ha.test-uuid",
				"skip_confirm=false",
			},
		},
		{
			name:        "使用自定义回调地址",
			redirectURL: "https://example.com/callback",
			state:       "",
			scope:       nil,
			skipConfirm: false,
			wantContain: []string{
				"redirect_uri=https%3A%2F%2Fexample.com%2Fcallback",
			},
		},
		{
			name:        "使用自定义state",
			redirectURL: "",
			state:       "my-custom-state",
			scope:       nil,
			skipConfirm: false,
			wantContain: []string{
				"state=my-custom-state",
			},
		},
		{
			name:        "使用scope参数",
			redirectURL: "",
			state:       "",
			scope:       []string{"1", "3", "6"},
			skipConfirm: false,
			wantContain: []string{
				"scope=1+3+6",
			},
		},
		{
			name:        "跳过确认页面",
			redirectURL: "",
			state:       "",
			scope:       nil,
			skipConfirm: true,
			wantContain: []string{
				"skip_confirm=true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := client.GenAuthURL(tt.redirectURL, tt.state, tt.scope, tt.skipConfirm)
			t.Log(url)
			for _, want := range tt.wantContain {
				if !strings.Contains(url, want) {
					t.Errorf("GenAuthURL() = %s, should contain %s", url, want)
				}
			}
		})
	}

}
func TestMIoTOauthClient_GetAccessToken2(t *testing.T) {
	client, _ := NewMIoTOauthClient("2882303761520251711", "http://homeassistant.local:8123", "cn", "ha.test-uuid")
	t.Logf("url: %v", client.GenAuthURL("", "", nil, true))
	tokenInfo, err := client.GetAccessToken("C3_51E008FEA8F0C27F2D0A75A8A971F671")
	if err != nil {
		t.Logf("Error: %v", err)
		return
	}
	t.Logf("AccessToken: %s", tokenInfo.AccessToken)
	t.Logf("RefreshToken: %s", tokenInfo.RefreshToken)
	t.Logf("ExpiresIn: %d", tokenInfo.ExpiresIn)
	t.Logf("ExpiresTs: %d", tokenInfo.ExpiresTs)
}

func TestMIoTOauthClient_GetAccessToken(t *testing.T) {
	// 创建模拟服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求参数
		dataParam := r.URL.Query().Get("data")
		if dataParam == "" {
			t.Error("missing data parameter")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(dataParam), &data); err != nil {
			t.Errorf("failed to unmarshal data: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 验证必要的字段
		if _, ok := data["client_id"]; !ok {
			t.Error("missing client_id in data")
		}
		if _, ok := data["redirect_uri"]; !ok {
			t.Error("missing redirect_uri in data")
		}

		// 返回模拟的token响应
		response := map[string]interface{}{
			"code": 0,
			"result": map[string]interface{}{
				"access_token":  "test-access-token-12345",
				"refresh_token": "test-refresh-token-67890",
				"expires_in":    3600,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	client, _ := NewMIoTOauthClient("2882303761520251711", "http://homeassistant.local:8123", "cn", "test-uuid")
	// 替换为模拟服务器地址
	client.oauthHost = strings.TrimPrefix(mockServer.URL, "http://")

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "使用有效的code获取token",
			code:    "valid-auth-code",
			wantErr: false,
		},
		{
			name:    "使用空的code应该失败",
			code:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenInfo, err := client.GetAccessToken(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAccessToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// 验证返回的token信息
			if tokenInfo.AccessToken != "test-access-token-12345" {
				t.Errorf("AccessToken = %s, want test-access-token-12345", tokenInfo.AccessToken)
			}
			if tokenInfo.RefreshToken != "test-refresh-token-67890" {
				t.Errorf("RefreshToken = %s, want test-refresh-token-67890", tokenInfo.RefreshToken)
			}
			if tokenInfo.ExpiresIn != 3600 {
				t.Errorf("ExpiresIn = %d, want 3600", tokenInfo.ExpiresIn)
			}
			// 验证 ExpiresTs 被正确计算
			expectedExpiresTs := time.Now().Unix() + int64(3600*tokenExpiresTSRatio)
			if tokenInfo.ExpiresTs < expectedExpiresTs-5 || tokenInfo.ExpiresTs > expectedExpiresTs+5 {
				t.Errorf("ExpiresTs = %d, expected around %d", tokenInfo.ExpiresTs, expectedExpiresTs)
			}
		})
	}
}

func TestMIoTOauthClient_RefreshAccessToken(t *testing.T) {
	// 创建模拟服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataParam := r.URL.Query().Get("data")
		var data map[string]interface{}
		json.Unmarshal([]byte(dataParam), &data)

		// 验证 refresh_token 字段存在
		if _, ok := data["refresh_token"]; !ok {
			t.Error("missing refresh_token in data")
		}

		response := map[string]interface{}{
			"code": 0,
			"result": map[string]interface{}{
				"access_token":  "new-access-token-11111",
				"refresh_token": "new-refresh-token-22222",
				"expires_in":    7200,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	client, _ := NewMIoTOauthClient("2882303761520251711", "http://homeassistant.local:8123", "cn", "test-uuid")
	client.oauthHost = strings.TrimPrefix(mockServer.URL, "http://")

	tests := []struct {
		name         string
		refreshToken string
		wantErr      bool
	}{
		{
			name:         "使用有效的refresh_token刷新",
			refreshToken: "valid-refresh-token",
			wantErr:      false,
		},
		{
			name:         "使用空的refresh_token应该失败",
			refreshToken: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenInfo, err := client.RefreshAccessToken(tt.refreshToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("RefreshAccessToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if tokenInfo.AccessToken != "new-access-token-11111" {
				t.Errorf("AccessToken = %s, want new-access-token-11111", tokenInfo.AccessToken)
			}
			if tokenInfo.RefreshToken != "new-refresh-token-22222" {
				t.Errorf("RefreshToken = %s, want new-refresh-token-22222", tokenInfo.RefreshToken)
			}
			if tokenInfo.ExpiresIn != 7200 {
				t.Errorf("ExpiresIn = %d, want 7200", tokenInfo.ExpiresIn)
			}
		})
	}
}

func TestMIoTOauthClient_getToken_ErrorCases(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		wantErrMsg string
	}{
		{
			name:       "401未授权",
			statusCode: http.StatusUnauthorized,
			response:   nil,
			wantErrMsg: "unauthorized(401)",
		},
		{
			name:       "API返回错误码",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code":   1001,
				"result": map[string]interface{}{},
			},
			wantErrMsg: "api error code: 1001",
		},
		{
			name:       "缺少access_token",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code": 0,
				"result": map[string]interface{}{
					"refresh_token": "test-refresh",
					"expires_in":    3600,
				},
			},
			wantErrMsg: "missing access_token",
		},
		{
			name:       "缺少refresh_token",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"code": 0,
				"result": map[string]interface{}{
					"access_token": "test-access",
					"expires_in":   3600,
				},
			},
			wantErrMsg: "missing refresh_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer mockServer.Close()

			client, _ := NewMIoTOauthClient("2882303761520251711", "http://homeassistant.local:8123", "cn", "test-uuid")
			client.oauthHost = strings.TrimPrefix(mockServer.URL, "http://")

			_, err := client.GetAccessToken("test-code")
			if err == nil {
				t.Error("expected error, got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error message = %s, should contain %s", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestGenerateState(t *testing.T) {
	deviceID := "ha.test-device"
	state1 := generateState(deviceID)
	state2 := generateState(deviceID)

	// 相同的 deviceID 应该生成相同的 state
	if state1 != state2 {
		t.Error("generateState should return consistent result for same input")
	}

	// state 应该是40位十六进制字符串
	if len(state1) != 40 {
		t.Errorf("generateState returned string of length %d, want 40", len(state1))
	}

	// 不同的 deviceID 应该生成不同的 state
	state3 := generateState("ha.different-device")
	if state1 == state3 {
		t.Error("generateState should return different result for different input")
	}
}

func TestOAuthError(t *testing.T) {
	err := &OAuthError{
		Code:    401,
		Message: "unauthorized",
	}

	expected := "OAuthError(code=401): unauthorized"
	if err.Error() != expected {
		t.Errorf("OAuthError.Error() = %s, want %s", err.Error(), expected)
	}
}

// 集成测试示例（需要真实环境，默认跳过）
func TestIntegration_RealServer(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试，使用 -short 标志运行单元测试")
	}

	// 注意：此测试需要真实的OAuth流程，仅作为示例
	t.Skip("集成测试需要手动执行OAuth流程")

	client, err := NewMIoTOauthClient("", "http://homeassistant.local:8123", "cn", "integration-test")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// 生成授权URL
	authURL := client.GenAuthURL("", "", nil, false)
	fmt.Printf("请访问授权URL: %s\n", authURL)
	fmt.Println("授权后，从回调URL中获取 code_value 参数值")
}
