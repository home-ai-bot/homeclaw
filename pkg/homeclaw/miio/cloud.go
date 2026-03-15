// Package miio provides a client for the Xiaomi Mi Home cloud API.
// It supports account login (username/password) to obtain a service token
// and device list with per-device tokens for local miio communication.
//
// Login flow (3 steps, mirroring squachen/micloud & al-one/hass-xiaomi-miot):
//
//  1. GET  /pass/serviceLogin?sid=xiaomiio&_json=true
//     → extract _sign from JSON response (strip "&&&START&&&" prefix)
//
//  2. POST /pass/serviceLoginAuth2
//     fields: sid, hash(MD5-upper), callback, qs, user, _json, _sign
//     → response JSON: result=="ok", userId, ssecurity, location, passToken
//
//  3. GET  location (redirect URL)
//     → serviceToken cookie set by server
//
// API calls use RC4 encryption (ENCRYPT-RC4 algorithm) identical to hass-xiaomi-miot.
package miio

import (
	"crypto/md5"
	crypto_rand "crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

const (
	miAccountBase = "https://account.xiaomi.com"
	miAPIBase     = "https://api.io.mi.com/app"

	// stsCallback is the fixed callback used in the login POST – must match exactly.
	stsCallback = "https://sts.api.io.mi.com/sts"
	// loginQS is the URL-encoded query string forwarded to the STS callback.
	loginQS = "%3Fsid%3Dxiaomiio%26_json%3Dtrue"
)

// uaTemplate is the User-Agent format: <random-18-lower>-<random-16-upper> APP/xiaomi.smarthome APPV/62830
// This matches the UA used by hass-xiaomi-miot / squachen/micloud.
const uaTemplate = "%s-%s APP/xiaomi.smarthome APPV/62830"

// ─────────────────────────────────────────────────────────────────────────────
// CloudClient
// ─────────────────────────────────────────────────────────────────────────────

// CloudClient is a stateful client for the Mi Home cloud API.
// After Login() the UserID, SSECURITY and ServiceToken fields are populated
// and can be persisted for subsequent sessions via ExportSession / ImportSession.
type CloudClient struct {
	httpClient   *http.Client
	jar          *cookiejar.Jar
	agent        string // randomised User-Agent, generated once per client
	deviceID     string // random 6-char lowercase, seeded as cookie
	UserID       string
	ServiceToken string
	SSECURITY    string
	Country      string // "cn" | "de" | "us" | "sg" | "ru" | "tw" | "i2"
}

// NewCloudClient creates a CloudClient for the given country region.
// Pass "" to use "cn" (mainland China).
func NewCloudClient(country string) *CloudClient {
	if country == "" {
		country = "cn"
	}
	jar, _ := cookiejar.New(nil)
	c := &CloudClient{
		Country:  country,
		jar:      jar,
		agent:    generateAgent(),
		deviceID: generateDeviceID(),
	}
	// Allow redirects for step 3; we inspect cookies from the jar afterwards.
	c.httpClient = &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// Login
// ─────────────────────────────────────────────────────────────────────────────

// LoginResult holds the outcome of a LoginWithResult call.
type LoginResult struct {
	// OK is true when the login completed successfully.
	OK bool
	// NeedVerify is true when the account requires 2FA approval on the phone.
	// When NeedVerify is true, OK is false and NotifyURL contains the verification link.
	NeedVerify bool
	// NotifyURL is the Mi Account identity verification URL (only set when NeedVerify=true).
	NotifyURL string
}

// Login performs the three-step Mi Account SSO login.
// On success c.UserID, c.SSECURITY and c.ServiceToken are set.
func (c *CloudClient) Login(username, password string) error {
	res, err := c.LoginWithResult(username, password)
	if err != nil {
		return err
	}
	if res.NeedVerify {
		return fmt.Errorf(
			"login requires identity verification (2FA). "+
				"Please open the Mi Home app, approve the login request, then retry. "+
				"Verification URL: %s", res.NotifyURL)
	}
	return nil
}

// LoginWithResult performs the three-step Mi Account SSO login and returns a
// structured LoginResult instead of an opaque error for the 2FA case.
// On success (result.OK=true) c.UserID, c.SSECURITY and c.ServiceToken are set.
// When the account requires 2FA (result.NeedVerify=true) the caller should
// present result.NotifyURL to the user and retry after approval.
func (c *CloudClient) LoginWithResult(username, password string) (*LoginResult, error) {
	c.seedSessionCookies(username)

	// Step 1: obtain _sign.
	sign, err := c.loginStep1()
	if err != nil {
		return nil, fmt.Errorf("micloud login step1: %w", err)
	}

	// Step 2: submit credentials, obtain ssecurity + userId + redirect location.
	location, notifyURL, err := c.loginStep2(username, password, sign)
	if err != nil {
		return nil, fmt.Errorf("micloud login step2: %w", err)
	}
	if notifyURL != "" {
		// 2FA required – caller must show URL to user and retry.
		return &LoginResult{NeedVerify: true, NotifyURL: notifyURL}, nil
	}

	// Step 3: follow redirect to get serviceToken cookie.
	token, err := c.loginStep3(location)
	if err != nil {
		return nil, fmt.Errorf("micloud login step3: %w", err)
	}
	c.ServiceToken = token
	return &LoginResult{OK: true}, nil
}

// loginStep1 → GET /pass/serviceLogin?sid=xiaomiio&_json=true
// Returns the _sign value (may be empty string on first-ever call – that is fine).
func (c *CloudClient) loginStep1() (string, error) {
	reqURL := miAccountBase + "/pass/serviceLogin?sid=xiaomiio&_json=true"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	parsed, err := stripAndParse(body)
	if err != nil {
		return "", fmt.Errorf("step1 parse: %w (body=%s)", err, string(body))
	}

	sign, _ := parsed["_sign"].(string)
	return sign, nil
}

// step2Resp holds fields we care about from the serviceLoginAuth2 response.
type step2Resp struct {
	Result          string `json:"result"`
	UserID          any    `json:"userId"` // number or string depending on server
	SSECURITY       string `json:"ssecurity"`
	Location        string `json:"location"`
	PassToken       string `json:"passToken"`
	Code            int    `json:"code"`
	Desc            string `json:"desc"`
	NotificationURL string `json:"notificationUrl"` // set when 2FA / identity verification is required
}

// loginStep2 → POST /pass/serviceLoginAuth2
// Submits credentials and retrieves ssecurity, userId, and the redirect location.
// Returns (location, notifyURL, error): when notifyURL is non-empty the account
// requires 2FA and location will be empty.
func (c *CloudClient) loginStep2(username, password, sign string) (string, string, error) {
	// MD5 of password – uppercase hex, matching micloud's hashlib.md5(...).hexdigest().upper()
	pwdHash := strings.ToUpper(fmt.Sprintf("%x", md5.Sum([]byte(password))))

	form := url.Values{
		"sid":      {"xiaomiio"},
		"hash":     {pwdHash},
		"callback": {stsCallback},
		"qs":       {loginQS},
		"user":     {username},
		"_json":    {"true"},
	}
	if sign != "" {
		form.Set("_sign", sign)
	}

	reqURL := miAccountBase + "/pass/serviceLoginAuth2"
	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	c.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	parsed, err := stripAndParse(body)
	if err != nil {
		return "", "", fmt.Errorf("step2 parse: %w (body=%s)", err, string(body))
	}

	// Re-marshal into typed struct for convenience.
	tmp, _ := json.Marshal(parsed)
	var s2 step2Resp
	if err := json.Unmarshal(tmp, &s2); err != nil {
		return "", "", fmt.Errorf("step2 unmarshal: %w", err)
	}

	// 2FA / identity verification required – return notifyURL to caller.
	if s2.NotificationURL != "" && s2.SSECURITY == "" {
		return "", s2.NotificationURL, nil
	}

	if s2.Result != "ok" {
		return "", "", fmt.Errorf("login rejected (result=%s code=%d desc=%s) body=%.500s", s2.Result, s2.Code, s2.Desc, string(tmp))
	}
	if s2.SSECURITY == "" {
		return "", "", fmt.Errorf("empty ssecurity in step2 response body=%.500s", string(tmp))
	}
	if s2.Location == "" {
		return "", "", fmt.Errorf("empty location in step2 response")
	}

	c.SSECURITY = s2.SSECURITY
	switch v := s2.UserID.(type) {
	case string:
		c.UserID = v
	case float64:
		c.UserID = fmt.Sprintf("%.0f", v)
	}

	return s2.Location, "", nil
}

// loginStep3 → GET location
// Follows the redirect; the server sets serviceToken as a cookie.
func (c *CloudClient) loginStep3(location string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return "", err
	}
	c.setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check response cookies directly.
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "serviceToken" {
			return cookie.Value, nil
		}
	}

	// Also check cookie jar (redirect chains may store it there).
	u, _ := url.Parse(location)
	for _, cookie := range c.jar.Cookies(u) {
		if cookie.Name == "serviceToken" {
			return cookie.Value, nil
		}
	}
	// Try the STS domain explicitly.
	for _, rawDomain := range []string{"https://sts.api.io.mi.com", "https://api.io.mi.com"} {
		du, _ := url.Parse(rawDomain)
		for _, cookie := range c.jar.Cookies(du) {
			if cookie.Name == "serviceToken" {
				return cookie.Value, nil
			}
		}
	}

	body, _ := io.ReadAll(resp.Body)
	return "", fmt.Errorf("serviceToken not found in cookies; status=%d body=%.200s", resp.StatusCode, string(body))
}

// ─────────────────────────────────────────────────────────────────────────────
// Device list  (RC4-encrypted Mi Home API)
// ─────────────────────────────────────────────────────────────────────────────

// CloudDevice represents one device entry from the Mi Home cloud.
type CloudDevice struct {
	Did      string `json:"did"`   // miio device ID
	Token    string `json:"token"` // 32-hex local token
	Name     string `json:"name"`
	Model    string `json:"model"`
	IP       string `json:"localip"`
	MAC      string `json:"mac"`
	SSID     string `json:"ssid"`
	IsOnline bool   `json:"isOnline"`
}

// deviceListResult is the "result" object in the /home/device_list response.
type deviceListResult struct {
	List []CloudDevice `json:"list"`
}

// GetDevices fetches the complete device list from the Mi Home cloud.
// Login() must have been called (or ImportSession used) beforehand.
func (c *CloudClient) GetDevices() ([]CloudDevice, error) {
	if c.UserID == "" || c.ServiceToken == "" {
		return nil, fmt.Errorf("micloud: not logged in; call Login() first")
	}

	apiURL := c.apiHost() + "/home/device_list"
	params := map[string]string{
		"data": `{"getVirtualModel":true,"getHuamiDevices":1,"get_split_device":false,"support_smart_home":true}`,
	}

	raw, err := c.apiCallRC4(apiURL, params)
	if err != nil {
		return nil, fmt.Errorf("micloud get_devices: %w", err)
	}

	var result deviceListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("micloud device list parse: %w (raw=%s)", err, string(raw))
	}
	return result.List, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Extended API methods (from token_extractor.py)
// ─────────────────────────────────────────────────────────────────────────────

// Home represents a Mi Home household/family.
type Home struct {
	HomeID    int64 `json:"id"`
	HomeOwner int64 `json:"home_owner,omitempty"`
}

// HomeInfo represents detailed home information from gethome API.
type HomeInfo struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	OwnerID int64  `json:"owner_id"`
}

// ShareFamily represents a shared family from get_device_cnt API.
type ShareFamily struct {
	HomeID    int64 `json:"home_id"`
	HomeOwner int64 `json:"home_owner"`
}

// DeviceCountResult holds the result from get_device_cnt API.
type DeviceCountResult struct {
	Share struct {
		ShareFamilies []ShareFamily `json:"share_family"`
	} `json:"share"`
}

// GetHomes retrieves the list of homes for the current user.
func (c *CloudClient) GetHomes() ([]HomeInfo, error) {
	if c.UserID == "" || c.ServiceToken == "" {
		return nil, fmt.Errorf("micloud: not logged in; call Login() first")
	}

	apiURL := c.apiHost() + "/v2/homeroom/gethome"
	params := map[string]string{
		"data": `{"fg": true, "fetch_share": true, "fetch_share_dev": true, "limit": 300, "app_ver": 7}`,
	}

	raw, err := c.apiCallRC4(apiURL, params)
	if err != nil {
		return nil, fmt.Errorf("micloud get_homes: %w", err)
	}

	var envelope struct {
		Result struct {
			HomeList []HomeInfo `json:"homelist"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("micloud homes parse: %w (raw=%s)", err, string(raw))
	}
	return envelope.Result.HomeList, nil
}

// GetDeviceCount retrieves device count and shared families info.
func (c *CloudClient) GetDeviceCount() (*DeviceCountResult, error) {
	if c.UserID == "" || c.ServiceToken == "" {
		return nil, fmt.Errorf("micloud: not logged in; call Login() first")
	}

	apiURL := c.apiHost() + "/v2/user/get_device_cnt"
	params := map[string]string{
		"data": `{"fetch_own": true, "fetch_share": true}`,
	}

	raw, err := c.apiCallRC4(apiURL, params)
	if err != nil {
		return nil, fmt.Errorf("micloud get_device_cnt: %w", err)
	}

	var result DeviceCountResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("micloud device count parse: %w (raw=%s)", err, string(raw))
	}
	return &result, nil
}

// GetHomeDevices retrieves devices for a specific home.
func (c *CloudClient) GetHomeDevices(homeID, homeOwner int64) ([]CloudDevice, error) {
	if c.UserID == "" || c.ServiceToken == "" {
		return nil, fmt.Errorf("micloud: not logged in; call Login() first")
	}

	apiURL := c.apiHost() + "/v2/home/home_device_list"
	data := fmt.Sprintf(`{"home_owner": %d, "home_id": %d, "limit": 200, "get_split_device": true, "support_smart_home": true}`,
		homeOwner, homeID)
	params := map[string]string{"data": data}

	raw, err := c.apiCallRC4(apiURL, params)
	if err != nil {
		return nil, fmt.Errorf("micloud get_home_devices: %w", err)
	}

	var envelope struct {
		Result struct {
			DeviceInfo []CloudDevice `json:"device_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("micloud home devices parse: %w (raw=%s)", err, string(raw))
	}
	return envelope.Result.DeviceInfo, nil
}

// BeaconKeyResult holds the result from blt_get_beaconkey API.
type BeaconKeyResult struct {
	BeaconKey string `json:"beaconkey"`
	PDID      int    `json:"pdid"`
}

// GetBeaconKey retrieves the beacon key for a BLE device.
func (c *CloudClient) GetBeaconKey(did string) (*BeaconKeyResult, error) {
	if c.UserID == "" || c.ServiceToken == "" {
		return nil, fmt.Errorf("micloud: not logged in; call Login() first")
	}

	apiURL := c.apiHost() + "/v2/device/blt_get_beaconkey"
	data := fmt.Sprintf(`{"did":"%s","pdid":1}`, did)
	params := map[string]string{"data": data}

	raw, err := c.apiCallRC4(apiURL, params)
	if err != nil {
		return nil, fmt.Errorf("micloud get_beaconkey: %w", err)
	}

	var envelope struct {
		Result BeaconKeyResult `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("micloud beaconkey parse: %w (raw=%s)", err, string(raw))
	}
	return &envelope.Result, nil
}

// ServerRegion represents a Xiaomi cloud server region.
type ServerRegion string

const (
	ServerCN ServerRegion = "cn"
	ServerDE ServerRegion = "de"
	ServerUS ServerRegion = "us"
	ServerRU ServerRegion = "ru"
	ServerTW ServerRegion = "tw"
	ServerSG ServerRegion = "sg"
	ServerIN ServerRegion = "in"
	ServerI2 ServerRegion = "i2"
)

// AllServerRegions returns all available server regions.
func AllServerRegions() []ServerRegion {
	return []ServerRegion{ServerCN, ServerDE, ServerUS, ServerRU, ServerTW, ServerSG, ServerIN, ServerI2}
}

// String returns the string representation of the server region.
func (s ServerRegion) String() string {
	return string(s)
}

// ─────────────────────────────────────────────────────────────────────────────
// RC4-encrypted API call  (identical to hass-xiaomi-miot execute_api_call_encrypted)
// ─────────────────────────────────────────────────────────────────────────────

// apiCallRC4 performs a signed + RC4-encrypted POST to the Mi Home cloud API
// and returns the decrypted "result" JSON.
func (c *CloudClient) apiCallRC4(apiURL string, params map[string]string) (json.RawMessage, error) {
	millis := time.Now().UnixMilli()
	nonce := generateNonce(millis)
	signedNonce := signNonce(c.SSECURITY, nonce)

	encParams := buildEncParams(apiURL, "POST", signedNonce, nonce, params, c.SSECURITY)

	form := url.Values{}
	for k, v := range encParams {
		form.Set(k, v)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	c.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-xiaomi-protocal-flag-cli", "PROTOCAL-HTTP2")
	req.Header.Set("MIOT-ENCRYPT-ALGORITHM", "ENCRYPT-RC4")
	req.AddCookie(&http.Cookie{Name: "userId", Value: c.UserID})
	req.AddCookie(&http.Cookie{Name: "serviceToken", Value: c.ServiceToken})
	req.AddCookie(&http.Cookie{Name: "yetAnotherServiceToken", Value: c.ServiceToken})
	req.AddCookie(&http.Cookie{Name: "locale", Value: "en_CN"})
	req.AddCookie(&http.Cookie{Name: "timezone", Value: "GMT+08:00"})
	req.AddCookie(&http.Cookie{Name: "is_daylight", Value: "0"})
	req.AddCookie(&http.Cookie{Name: "dst_offset", Value: "0"})
	req.AddCookie(&http.Cookie{Name: "channel", Value: "MI_APP_STORE"})

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api post: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decrypt with RC4 using the response's signed nonce.
	respSignedNonce := signNonce(c.SSECURITY, encParams["_nonce"])
	decrypted, err := decryptRC4(respSignedNonce, string(body))
	if err != nil {
		return nil, fmt.Errorf("rc4 decrypt: %w (body=%.200s)", err, string(body))
	}

	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(decrypted, &envelope); err != nil {
		return nil, fmt.Errorf("envelope parse: %w (decrypted=%.200s)", err, string(decrypted))
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("api error (code=%d): %s", envelope.Code, envelope.Message)
	}
	return envelope.Result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// RC4 stream cipher  (no external dependency)
// ─────────────────────────────────────────────────────────────────────────────

type rc4Stream struct {
	s    [256]byte
	i, j byte
}

func newRC4(key []byte) *rc4Stream {
	r := &rc4Stream{}
	for i := range r.s {
		r.s[i] = byte(i)
	}
	var j byte
	for i := 0; i < 256; i++ {
		j += r.s[i] + key[i%len(key)]
		r.s[i], r.s[j] = r.s[j], r.s[i]
	}
	return r
}

func (r *rc4Stream) xorKeyStream(dst, src []byte) {
	for k := range src {
		r.i++
		r.j += r.s[r.i]
		r.s[r.i], r.s[r.j] = r.s[r.j], r.s[r.i]
		dst[k] = src[k] ^ r.s[r.i+r.s[r.j]]
	}
}

// skip1024 discards 1024 bytes – Xiaomi's required key-schedule warm-up.
func (r *rc4Stream) skip1024() {
	buf := make([]byte, 1024)
	r.xorKeyStream(buf, buf)
}

// encryptRC4 encrypts payload with RC4(base64-decoded signedNonce), returns base64.
func encryptRC4(signedNonce, payload string) string {
	key, _ := base64.StdEncoding.DecodeString(signedNonce)
	r := newRC4(key)
	r.skip1024()
	ct := make([]byte, len(payload))
	r.xorKeyStream(ct, []byte(payload))
	return base64.StdEncoding.EncodeToString(ct)
}

// decryptRC4 decrypts a base64-encoded RC4 payload.
func decryptRC4(signedNonce, payload string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(signedNonce)
	if err != nil {
		return nil, fmt.Errorf("signedNonce base64: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("payload base64: %w", err)
	}
	r := newRC4(key)
	r.skip1024()
	pt := make([]byte, len(ct))
	r.xorKeyStream(pt, ct)
	return pt, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Signature helpers  (mirroring hass-xiaomi-miot generate_enc_signature)
// ─────────────────────────────────────────────────────────────────────────────

// encSignature computes:
//
//	base64( SHA1( METHOD + "&" + path + "&" + k=v... + "&" + signedNonce ) )
//
// path strips the leading "/app" component as Xiaomi expects.
func encSignature(apiURL, method, signedNonce string, params map[string]string) string {
	parsed, _ := url.Parse(apiURL)
	path := strings.Replace(parsed.Path, "/app/", "/", 1)

	parts := []string{strings.ToUpper(method), path}
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	parts = append(parts, signedNonce)
	msg := strings.Join(parts, "&")
	h := sha1.Sum([]byte(msg))
	return base64.StdEncoding.EncodeToString(h[:])
}

// buildEncParams mirrors hass-xiaomi-miot generate_enc_params.
func buildEncParams(apiURL, method, signedNonce, nonce string, params map[string]string, ssecurity string) map[string]string {
	// 1. Compute plain-text rc4_hash__.
	plain := copyParams(params)
	plain["rc4_hash__"] = encSignature(apiURL, method, signedNonce, plain)

	// 2. Encrypt each param (including rc4_hash__).
	encrypted := make(map[string]string, len(plain)+4)
	for k, v := range plain {
		encrypted[k] = encryptRC4(signedNonce, v)
	}

	// 3. Compute signature over encrypted params.
	encrypted["signature"] = encSignature(apiURL, method, signedNonce, encrypted)
	encrypted["ssecurity"] = ssecurity
	encrypted["_nonce"] = nonce
	return encrypted
}

func copyParams(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Session persistence
// ─────────────────────────────────────────────────────────────────────────────

// Session is a serialisable login session for persistence between runs.
type Session struct {
	UserID       string `json:"user_id"`
	SSECURITY    string `json:"ssecurity"`
	ServiceToken string `json:"service_token"`
	Country      string `json:"country"`
}

// ExportSession returns the current session for persistence.
func (c *CloudClient) ExportSession() Session {
	return Session{
		UserID:       c.UserID,
		SSECURITY:    c.SSECURITY,
		ServiceToken: c.ServiceToken,
		Country:      c.Country,
	}
}

// ImportSession restores a previously saved session without re-logging-in.
func (c *CloudClient) ImportSession(s Session) {
	c.UserID = s.UserID
	c.SSECURITY = s.SSECURITY
	c.ServiceToken = s.ServiceToken
	if s.Country != "" {
		c.Country = s.Country
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func (c *CloudClient) apiHost() string {
	switch c.Country {
	case "cn":
		return "https://api.io.mi.com/app"
	case "de":
		return "https://de.api.io.mi.com/app"
	case "us":
		return "https://us.api.io.mi.com/app"
	case "tw":
		return "https://tw.api.io.mi.com/app"
	case "sg":
		return "https://sg.api.io.mi.com/app"
	case "ru":
		return "https://ru.api.io.mi.com/app"
	case "i2":
		return "https://i2.api.io.mi.com/app"
	default:
		return miAPIBase
	}
}

func (c *CloudClient) setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Accept-Encoding", "identity")
}

// seedSessionCookies sets the initial session cookies required by Xiaomi SSO
// (mirrors micloud._init_session cookie seeding).
func (c *CloudClient) seedSessionCookies(username string) {
	for _, rawDomain := range []string{"https://mi.com", "https://xiaomi.com"} {
		u, _ := url.Parse(rawDomain)
		c.jar.SetCookies(u, []*http.Cookie{
			{Name: "sdkVersion", Value: "accountsdk-18.8.15"},
			{Name: "deviceId", Value: c.deviceID},
			{Name: "userId", Value: username},
		})
	}
}

// stripAndParse strips Xiaomi's "&&&START&&&" prefix and JSON-parses the body.
func stripAndParse(body []byte) (map[string]any, error) {
	s := string(body)
	s = strings.Replace(s, "&&&START&&&", "", 1)
	s = strings.TrimSpace(s)
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// generateNonce returns a 12-byte base64 nonce:
// 8 random bytes ‖ 4-byte big-endian (millis/60000).
// This matches hass-xiaomi-miot generate_nonce(millis).
func generateNonce(millis int64) string {
	b := make([]byte, 12)
	_, _ = crypto_rand.Read(b[:8])
	binary.BigEndian.PutUint32(b[8:], uint32(millis/60000))
	return base64.StdEncoding.EncodeToString(b)
}

// signNonce computes: base64( SHA256( base64decode(ssecurity) ‖ base64decode(nonce) ) ).
func signNonce(ssecurity, nonce string) string {
	s, _ := base64.StdEncoding.DecodeString(ssecurity)
	n, _ := base64.StdEncoding.DecodeString(nonce)
	combined := append(s, n...)
	hash := sha256.Sum256(combined)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// generateAgent produces a randomised User-Agent compatible with Xiaomi's server.
// Format: "<18-lower>-<16-upper> APP/xiaomi.smarthome APPV/62830"
func generateAgent() string {
	const lower = "abcdefghijklmnopqrstuvwxyz"
	const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	rb1 := make([]byte, 18)
	_, _ = crypto_rand.Read(rb1)
	name := make([]byte, 18)
	for i, b := range rb1 {
		name[i] = lower[int(b)%len(lower)]
	}

	rb2 := make([]byte, 16)
	_, _ = crypto_rand.Read(rb2)
	agentID := make([]byte, 16)
	for i, b := range rb2 {
		agentID[i] = upper[int(b)%len(upper)]
	}

	return fmt.Sprintf(uaTemplate, string(name), string(agentID))
}

// generateDeviceID returns a random 6-character lowercase device ID.
func generateDeviceID() string {
	const lower = "abcdefghijklmnopqrstuvwxyz"
	rb := make([]byte, 6)
	_, _ = crypto_rand.Read(rb)
	id := make([]byte, 6)
	for i, b := range rb {
		id[i] = lower[int(b)%len(lower)]
	}
	return string(id)
}

// md5Sum is a convenience wrapper around crypto/md5.
func md5Sum(data []byte) [16]byte {
	return md5.Sum(data)
}
