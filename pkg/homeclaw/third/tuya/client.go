package tuya

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	tuya "github.com/AlexxIT/go2rtc/pkg/tuya"

	rootdata "github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// Client handles Tuya API operations with credential storage
type Client struct {
	httpClient  *http.Client
	baseURL     string
	countryCode string
	secretStore SecretStore
	region      *tuya.Region
	email       string
	password    string
	loginResult *tuya.LoginResult
}

// ClientOption is a functional option for Client
type ClientOption func(*Client) error

// WithCredentials sets the credentials for the client
func WithCredentials(region, email, password string) ClientOption {
	return func(c *Client) error {
		r := GetRegionByName(region)
		if r == nil {
			return fmt.Errorf("invalid region: %s", region)
		}
		c.region = r
		c.baseURL = r.Host
		c.countryCode = r.Continent
		c.email = email
		c.password = password
		return nil
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) error {
		c.httpClient = httpClient
		return nil
	}
}

// NewClient creates a new Tuya client
func NewClient(dataStore *rootdata.JSONStore, opts ...ClientOption) (*Client, error) {
	secretStore, err := NewSecretStore(dataStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret store: %w", err)
	}

	client := &Client{
		secretStore: secretStore,
	}

	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, err
		}
	}

	if client.httpClient == nil {
		client.httpClient = tuya.CreateHTTPClientWithSession()
	}

	return client, nil
}

// Login performs the Tuya login flow
func (c *Client) Login() (*tuya.LoginResult, error) {
	if c.httpClient == nil {
		return nil, errors.New("http client not initialized")
	}
	if c.baseURL == "" || c.email == "" || c.password == "" {
		return nil, errors.New("credentials not set")
	}

	// Step 1: Get login token
	tokenResp, err := c.getLoginToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get login token: %w", err)
	}

	// Step 2: Encrypt password
	encryptedPassword, err := tuya.EncryptPassword(c.password, tokenResp.Result.PbKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	// Step 3: Perform login
	loginResp, err := c.performLogin(tokenResp.Result.Token, encryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	if !loginResp.Success {
		return nil, errors.New(loginResp.ErrorMsg)
	}

	c.loginResult = &loginResp.Result
	return &loginResp.Result, nil
}

// getLoginToken fetches the login token from Tuya API
func (c *Client) getLoginToken() (*tuya.LoginTokenResponse, error) {
	url := fmt.Sprintf("https://%s/api/login/token", c.baseURL)

	tokenReq := tuya.LoginTokenRequest{
		CountryCode: c.countryCode,
		Username:    c.email,
		IsUid:       false,
	}

	jsonData, err := json.Marshal(tokenReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.baseURL))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/login", c.baseURL))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp tuya.LoginTokenResponse
	if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	if !tokenResp.Success {
		return nil, errors.New("tuya: " + tokenResp.Msg)
	}

	return &tokenResp, nil
}

// performLogin sends the login request with encrypted password
func (c *Client) performLogin(token, encryptedPassword string) (*tuya.PasswordLoginResponse, error) {
	var loginURL string

	loginReq := tuya.PasswordLoginRequest{
		CountryCode: c.countryCode,
		Passwd:      encryptedPassword,
		Token:       token,
		IfEncrypt:   1,
		Options:     `{"group":1}`,
	}

	if tuya.IsEmailAddress(c.email) {
		loginURL = fmt.Sprintf("https://%s/api/private/email/login", c.baseURL)
		loginReq.Email = c.email
	} else {
		loginURL = fmt.Sprintf("https://%s/api/private/phone/login", c.baseURL)
		loginReq.Mobile = c.email
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return nil, err
	}
	logger.Info(string(jsonData))
	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.baseURL))
	req.Header.Set("Referer", fmt.Sprintf("https://%s/login", c.baseURL))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Read body content for logging and decoding
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	logger.Infof("resp.Body: %s", string(bodyBytes))
	var loginResp tuya.PasswordLoginResponse
	if err := json.Unmarshal(bodyBytes, &loginResp); err != nil {
		return nil, err
	}

	return &loginResp, nil
}

// SaveCredentials saves the credentials to the secret store
func (c *Client) SaveCredentials() error {
	if c.region == nil || c.email == "" || c.password == "" {
		return errors.New("credentials not set")
	}
	return c.secretStore.Save(c.region.Name, c.email, c.password)
}

// LoadCredentials loads stored credentials from the secret store
func (c *Client) LoadCredentials() error {
	region, email, password, err := c.secretStore.GetDecrypted()
	if err != nil {
		return err
	}

	r := GetRegionByName(region)
	if r == nil {
		return fmt.Errorf("invalid stored region: %s", region)
	}

	c.region = r
	c.baseURL = r.Host
	c.countryCode = r.Continent
	c.email = email
	c.password = password
	return nil
}

// HasStoredCredentials checks if there are stored credentials
func (c *Client) HasStoredCredentials() bool {
	return c.secretStore.Exists()
}

// DeleteCredentials removes stored credentials
func (c *Client) DeleteCredentials() error {
	return c.secretStore.Delete()
}

// GetStoredCredentials returns the stored credentials (without decrypting password)
func (c *Client) GetStoredCredentials() (*SecretData, error) {
	return c.secretStore.Get()
}

// GetLoginResult returns the last login result
func (c *Client) GetLoginResult() *tuya.LoginResult {
	return c.loginResult
}

// request makes an HTTP request to the Tuya API
func (c *Client) request(method, url string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.baseURL))

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	res, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d", response.StatusCode)
	}

	return res, nil
}

// Close closes the client and releases resources
func (c *Client) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
