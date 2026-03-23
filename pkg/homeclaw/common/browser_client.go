// Package common provides common utility functions.
package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// BrowserClientOption is a functional option for configuring a BrowserClient.
type BrowserClientOption func(*BrowserClient)

// WithUserAgent sets the default User-Agent header for all requests.
func WithUserAgent(ua string) BrowserClientOption {
	return func(c *BrowserClient) {
		c.defaultHeaders["User-Agent"] = ua
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) BrowserClientOption {
	return func(c *BrowserClient) {
		c.timeout = d
	}
}

// WithDefaultHeader sets a default header applied to every request.
func WithDefaultHeader(key, value string) BrowserClientOption {
	return func(c *BrowserClient) {
		c.defaultHeaders[key] = value
	}
}

// WithProxy sets an HTTP proxy URL.
func WithProxy(proxyURL string) BrowserClientOption {
	return func(c *BrowserClient) {
		c.proxyURL = proxyURL
	}
}

// WithFollowRedirects controls whether the client follows redirects (default: true).
func WithFollowRedirects(follow bool) BrowserClientOption {
	return func(c *BrowserClient) {
		c.followRedirects = follow
	}
}

// BrowserResponse wraps http.Response and provides convenience helpers.
type BrowserResponse struct {
	*http.Response
	Body []byte // pre-read response body
}

// Text returns the response body as a string.
func (r *BrowserResponse) Text() string {
	return string(r.Body)
}

// LoginState represents the state of a multi-step login flow.
type LoginState int

const (
	// LoginStateIdle means no login is in progress.
	LoginStateIdle LoginState = iota
	// LoginStateAwaitingCaptcha means the server returned a captcha challenge.
	LoginStateAwaitingCaptcha
	// LoginStateAwaitingSMSCode means the server sent an SMS verification code.
	LoginStateAwaitingSMSCode
	// LoginStateAwaitingEmailCode means the server sent an email verification code.
	LoginStateAwaitingEmailCode
	// LoginStateAwaitingQRScan means the server is waiting for a QR code scan.
	LoginStateAwaitingQRScan
	// LoginStateSuccess means login completed successfully.
	LoginStateSuccess
	// LoginStateFailed means login failed.
	LoginStateFailed
)

// String returns a human-readable name for the login state.
func (s LoginState) String() string {
	switch s {
	case LoginStateIdle:
		return "idle"
	case LoginStateAwaitingCaptcha:
		return "awaiting_captcha"
	case LoginStateAwaitingSMSCode:
		return "awaiting_sms_code"
	case LoginStateAwaitingEmailCode:
		return "awaiting_email_code"
	case LoginStateAwaitingQRScan:
		return "awaiting_qr_scan"
	case LoginStateSuccess:
		return "success"
	case LoginStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// SessionData holds the authentication result after a successful login.
type SessionData struct {
	// Cookies contains all cookies from the jar after login, keyed by domain.
	Cookies map[string][]*http.Cookie
	// Headers contains any extra headers that should be sent with authenticated requests
	// (e.g. Authorization, X-Auth-Token).
	Headers map[string]string
	// Extra is a free-form map for storing provider-specific data
	// (e.g. serviceToken, userID, ssecurity, access_token, etc.).
	Extra map[string]interface{}
	// ExpiresAt is the optional expiry time of the session (zero = never expires).
	ExpiresAt time.Time
}

// IsExpired returns true if the session has a non-zero expiry and it has passed.
func (s *SessionData) IsExpired() bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// BrowserClient is a stateful HTTP client that mimics browser behaviour:
//   - Persists cookies across requests via a shared cookie jar
//   - Applies default headers (User-Agent, Referer, etc.) to every request
//   - Tracks a multi-step login flow state (captcha / SMS / email / QR)
//   - Stores a SessionData after a successful login for use as SSO credentials
type BrowserClient struct {
	mu sync.RWMutex

	httpClient      *http.Client
	jar             *cookiejar.Jar
	timeout         time.Duration
	proxyURL        string
	followRedirects bool

	defaultHeaders map[string]string

	// login flow state
	loginState LoginState
	session    *SessionData

	// captcha challenge data set by the caller
	captchaURL   string
	captchaToken string // opaque token returned alongside captcha URL

	// SMS / email challenge data
	smsPhone  string
	emailAddr string
}

// NewBrowserClient creates a new BrowserClient with the given options.
func NewBrowserClient(opts ...BrowserClientOption) (*BrowserClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("browserClient: create cookie jar: %w", err)
	}

	c := &BrowserClient{
		jar:             jar,
		timeout:         30 * time.Second,
		followRedirects: true,
		defaultHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
				"AppleWebKit/537.36 (KHTML, like Gecko) " +
				"Chrome/124.0.0.0 Safari/537.36",
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "en-US,en;q=0.5",
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	c.httpClient = c.buildHTTPClient(c.followRedirects)
	return c, nil
}

// buildHTTPClient constructs an http.Client with the current settings.
func (c *BrowserClient) buildHTTPClient(followRedirects bool) *http.Client {
	transport := http.DefaultTransport
	if c.proxyURL != "" {
		if pu, err := url.Parse(c.proxyURL); err == nil {
			transport = &http.Transport{Proxy: http.ProxyURL(pu)}
		}
	}

	client := &http.Client{
		Timeout:   c.timeout,
		Jar:       c.jar,
		Transport: transport,
	}
	if !followRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}

// ---- Cookie helpers --------------------------------------------------------

// SetCookie manually sets a cookie in the jar for the given URL.
func (c *BrowserClient) SetCookie(rawURL, name, value string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("browserClient SetCookie: parse url %q: %w", rawURL, err)
	}
	c.jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value}})
	return nil
}

// GetCookie returns the value of a named cookie for the given URL, or "".
func (c *BrowserClient) GetCookie(rawURL, name string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, ck := range c.jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

// GetCookieAny searches multiple domains for a named cookie (jar-wide search).
// Useful when you do not know which exact domain a cookie is scoped to.
func (c *BrowserClient) GetCookieAny(name string, domains ...string) string {
	for _, d := range domains {
		if v := c.GetCookie(d, name); v != "" {
			return v
		}
	}
	return ""
}

// Cookies returns all cookies stored for a given URL.
func (c *BrowserClient) Cookies(rawURL string) []*http.Cookie {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	return c.jar.Cookies(u)
}

// ClearCookies removes all cookies from the jar by replacing it with a new one.
func (c *BrowserClient) ClearCookies() error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jar = jar
	c.httpClient.Jar = jar
	return nil
}

// ---- Default header helpers ------------------------------------------------

// SetDefaultHeader adds or replaces a default header applied to every request.
func (c *BrowserClient) SetDefaultHeader(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.defaultHeaders[key] = value
}

// RemoveDefaultHeader removes a default header.
func (c *BrowserClient) RemoveDefaultHeader(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.defaultHeaders, key)
}

// ---- Core request methods --------------------------------------------------

// Do executes an *http.Request, applying default headers and returning a
// BrowserResponse with the body pre-read.
// Pass allowRedirect=false to capture 3xx responses without following them.
func (c *BrowserClient) Do(req *http.Request, allowRedirect bool) (*BrowserResponse, error) {
	c.mu.RLock()
	for k, v := range c.defaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	c.mu.RUnlock()

	client := c.httpClient
	if !allowRedirect {
		client = c.buildHTTPClient(false)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Replace body so callers can still call resp.Body if needed.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return &BrowserResponse{Response: resp, Body: body}, nil
}

// Get performs a GET request.
func (c *BrowserClient) Get(rawURL string, params url.Values, extraHeaders map[string]string) (*BrowserResponse, error) {
	return c.GetCtx(context.Background(), rawURL, params, extraHeaders, true)
}

// GetNoRedirect performs a GET request without following redirects.
func (c *BrowserClient) GetNoRedirect(rawURL string, params url.Values, extraHeaders map[string]string) (*BrowserResponse, error) {
	return c.GetCtx(context.Background(), rawURL, params, extraHeaders, false)
}

// GetCtx performs a GET request with a custom context.
func (c *BrowserClient) GetCtx(ctx context.Context, rawURL string, params url.Values, extraHeaders map[string]string, allowRedirect bool) (*BrowserResponse, error) {
	if len(params) > 0 {
		rawURL = rawURL + "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return c.Do(req, allowRedirect)
}

// PostForm performs a POST request with form-encoded body.
func (c *BrowserClient) PostForm(rawURL string, params url.Values, formData url.Values, extraHeaders map[string]string) (*BrowserResponse, error) {
	return c.PostFormCtx(context.Background(), rawURL, params, formData, extraHeaders)
}

// PostFormCtx performs a POST request with a custom context.
func (c *BrowserClient) PostFormCtx(ctx context.Context, rawURL string, params url.Values, formData url.Values, extraHeaders map[string]string) (*BrowserResponse, error) {
	if len(params) > 0 {
		rawURL = rawURL + "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return c.Do(req, true)
}

// PostJSON performs a POST request with a JSON body.
func (c *BrowserClient) PostJSON(rawURL string, jsonBody []byte, extraHeaders map[string]string) (*BrowserResponse, error) {
	return c.PostJSONCtx(context.Background(), rawURL, jsonBody, extraHeaders)
}

// PostJSONCtx performs a POST request with a JSON body and custom context.
func (c *BrowserClient) PostJSONCtx(ctx context.Context, rawURL string, jsonBody []byte, extraHeaders map[string]string) (*BrowserResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return c.Do(req, true)
}

// ---- Login flow state machine ----------------------------------------------

// LoginState returns the current login flow state (thread-safe).
func (c *BrowserClient) LoginState() LoginState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loginState
}

// SetLoginState updates the login state (intended for use by login flow
// implementations built on top of BrowserClient).
func (c *BrowserClient) SetLoginState(s LoginState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loginState = s
}

// SetCaptchaChallenge records a captcha challenge returned by the server.
// The captchaToken is the opaque value that must be submitted together with
// the user-solved captcha answer.
func (c *BrowserClient) SetCaptchaChallenge(captchaURL, captchaToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.captchaURL = captchaURL
	c.captchaToken = captchaToken
	c.loginState = LoginStateAwaitingCaptcha
}

// CaptchaChallenge returns the current captcha URL and token, if any.
func (c *BrowserClient) CaptchaChallenge() (captchaURL, captchaToken string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.captchaURL, c.captchaToken
}

// SetSMSChallenge records that an SMS code has been sent to phoneNumber.
func (c *BrowserClient) SetSMSChallenge(phoneNumber string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smsPhone = phoneNumber
	c.loginState = LoginStateAwaitingSMSCode
}

// SMSPhone returns the phone number that the SMS code was sent to.
func (c *BrowserClient) SMSPhone() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.smsPhone
}

// SetEmailChallenge records that an email verification code has been sent.
func (c *BrowserClient) SetEmailChallenge(emailAddr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emailAddr = emailAddr
	c.loginState = LoginStateAwaitingEmailCode
}

// EmailAddr returns the email address the verification code was sent to.
func (c *BrowserClient) EmailAddr() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.emailAddr
}

// SetQRChallenge records that QR-code scanning is required.
func (c *BrowserClient) SetQRChallenge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loginState = LoginStateAwaitingQRScan
}

// ---- Session / SSO ---------------------------------------------------------

// SaveSession records a successful login result.
// After calling this, Session() and IsLoggedIn() reflect the new state.
func (c *BrowserClient) SaveSession(data *SessionData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = data
	c.loginState = LoginStateSuccess

	// Apply any session headers as defaults so future requests are authenticated.
	for k, v := range data.Headers {
		c.defaultHeaders[k] = v
	}

	// Restore cookies from session data into the jar.
	for domain, cookies := range data.Cookies {
		if u, err := url.Parse(domain); err == nil {
			c.jar.SetCookies(u, cookies)
		}
	}
}

// Session returns the stored session data, or nil if not logged in.
func (c *BrowserClient) Session() *SessionData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// IsLoggedIn returns true when a non-expired session is stored.
func (c *BrowserClient) IsLoggedIn() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.session == nil {
		return false
	}
	return !c.session.IsExpired()
}

// ExportSession builds a SessionData snapshot of the current cookie jar
// across the provided domain list plus any previously-stored session headers/extra.
// Call this after a successful login to persist the state externally.
func (c *BrowserClient) ExportSession(domains []string) *SessionData {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data := &SessionData{
		Cookies: make(map[string][]*http.Cookie),
		Headers: make(map[string]string),
		Extra:   make(map[string]interface{}),
	}

	// Copy cookies from jar for all requested domains.
	for _, d := range domains {
		u, err := url.Parse(d)
		if err != nil {
			continue
		}
		cookies := c.jar.Cookies(u)
		if len(cookies) > 0 {
			data.Cookies[d] = cookies
		}
	}

	// Carry over existing session headers / extra if present.
	if c.session != nil {
		for k, v := range c.session.Headers {
			data.Headers[k] = v
		}
		for k, v := range c.session.Extra {
			data.Extra[k] = v
		}
		data.ExpiresAt = c.session.ExpiresAt
	}

	return data
}

// ClearSession removes the stored session and resets login state to idle.
func (c *BrowserClient) ClearSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = nil
	c.loginState = LoginStateIdle
}

// Reset resets the client to a clean state: clears cookies, session, login
// state, captcha/SMS/email challenge data, and removes auth-related default
// headers (Authorization, X-Auth-Token).
func (c *BrowserClient) Reset() error {
	if err := c.ClearCookies(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = nil
	c.loginState = LoginStateIdle
	c.captchaURL = ""
	c.captchaToken = ""
	c.smsPhone = ""
	c.emailAddr = ""
	delete(c.defaultHeaders, "Authorization")
	delete(c.defaultHeaders, "X-Auth-Token")
	return nil
}
