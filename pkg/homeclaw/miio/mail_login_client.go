// Package miio 提供小米账号 App 登录客户端实现
//
// 严格对应 Python 版 token_extractor.py 中的
// PasswordXiaomiCloudConnector / QrCodeXiaomiCloudConnector。
package miio

import (
	"crypto/md5" //nolint:gosec
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---------- 共享数据结构 ----------

// AppLoginResult 登录成功后返回的凭据
type AppLoginResult struct {
	UserID       string // 用户 ID（数字字符串）
	CUserID      string // 加密用户 ID
	SSecurity    string // base64 编码的安全密钥
	PassToken    string // passToken
	ServiceToken string // serviceToken（用于后续 API 调用）
	Location     string // 最终回调地址
}

// ---------- 共享基类（对应 XiaomiCloudConnector.__init__） ----------

type xiaomiCloudBase struct {
	agent        string
	deviceID     string
	session      *http.Client
	jar          *cookiejar.Jar
	ssecurity    string
	userID       string // 对应 Python self.userId
	serviceToken string // 对应 Python self._serviceToken
}

func newXiaomiCloudBase() (*xiaomiCloudBase, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	b := &xiaomiCloudBase{
		agent:    appGenerateAgent(),
		deviceID: appGenerateDeviceID(),
		jar:      jar,
	}
	b.session = &http.Client{
		Timeout: MIHomeHTTPAPITimeout,
		Jar:     jar,
	}
	return b, nil
}

// appToJSON 去除 "&&&START&&&" 前缀后解析 JSON
// 对应 Python XiaomiCloudConnector.to_json
func appToJSON(text string) (map[string]interface{}, error) {
	cleaned := strings.Replace(text, "&&&START&&&", "", 1)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// appSignedNonce 计算 base64( sha256( decode(ssecurity) + decode(nonce) ) )
// 对应 Python XiaomiCloudConnector.signed_nonce
func appSignedNonce(ssecurity, nonce string) (string, error) {
	secBytes, err := base64.StdEncoding.DecodeString(ssecurity)
	if err != nil {
		return "", err
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(secBytes)
	h.Write(nonceBytes)
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// appGenerateAgent 生成随机 User-Agent
// 对应 Python XiaomiCloudConnector.generate_agent
func appGenerateAgent() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	agentID := make([]byte, 13)
	for i := range agentID {
		agentID[i] = byte('A' + rng.Intn(5)) // A-E
	}
	randText := make([]byte, 18)
	for i := range randText {
		randText[i] = byte('a' + rng.Intn(26)) // a-z
	}
	return fmt.Sprintf("%s-%s APP/com.xiaomi.mihome APPV/10.5.201",
		string(randText), string(agentID))
}

// appGenerateDeviceID 生成 6 位随机小写字母设备 ID
// 对应 Python XiaomiCloudConnector.generate_device_id
func appGenerateDeviceID() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	id := make([]byte, 6)
	for i := range id {
		id[i] = byte('a' + rng.Intn(26))
	}
	return string(id)
}

// ---------- 密码登录（对应 PasswordXiaomiCloudConnector） ----------

// ErrTwoFactorRequired 表示登录需要邮件二次验证。
//
// Login() 检测到 2FA 时返回此错误，调用方收到后提示用户查收邮件，
// 输入验证码后在**同一个 PasswordConnector 实例**上调用 CompleteTwoFactor。
type ErrTwoFactorRequired struct {
	// TwoFactorContext 是继续 2FA 流程所需的不透明上下文字符串（仅供展示或日志）。
	TwoFactorContext string
	// notificationURL 是小米返回的原始 notificationUrl，CompleteTwoFactor 内部使用。
	notificationURL string
}

func (e *ErrTwoFactorRequired) Error() string {
	return "2FA email verification required"
}

// PasswordConnector 使用账号密码登录小米账号
type PasswordConnector struct {
	xiaomiCloudBase

	// 对应 Python PasswordXiaomiCloudConnector 私有字段
	username  string
	password  string
	sign      string // _sign
	cUserID   string // _cUserId
	passToken string // _passToken
	location  string // _location
	code      interface{}

	// tfa2Context 暂存 start2FAEmailFlow 返回的 context，供 complete2FAEmailFlow 使用
	tfa2Context string
	// tfa2IckCookie 暂存 sendEmailTicket 时的 ick cookie
	tfa2IckCookie string
	// tfa2NotifURL 暂存触发 2FA 的 notificationUrl，CompleteTwoFactor 用于重建会话
	tfa2NotifURL string
}

// NewPasswordConnector 创建密码登录客户端
func NewPasswordConnector(username, password string) *PasswordConnector {
	base, err := newXiaomiCloudBase()
	if err != nil {
		// cookiejar.New 基本不会失败
		jar, _ := cookiejar.New(nil)
		base = &xiaomiCloudBase{
			agent:    appGenerateAgent(),
			deviceID: appGenerateDeviceID(),
			jar:      jar,
			session:  &http.Client{Timeout: MIHomeHTTPAPITimeout, Jar: jar},
		}
	}
	return &PasswordConnector{
		xiaomiCloudBase: *base,
		username:        username,
		password:        password,
	}
}

// Login 执行密码登录，成功返回 AppLoginResult。
//
// 严格对应 Python PasswordXiaomiCloudConnector.login。
//
// 若账号启用了邮件二次验证，返回 *ErrTwoFactorRequired 错误，
// 调用方读取其 TwoFactorContext 后提示用户输入邮件验证码，
// 再调用 CompleteTwoFactor(ctx, code) 完成登录。
//
//	result, err := connector.Login()
//	var tfa *ErrTwoFactorRequired
//	if errors.As(err, &tfa) {
//	    code := askUserForEmailCode()
//	    result, err = connector.CompleteTwoFactor(tfa.TwoFactorContext, code)
//	}
func (c *PasswordConnector) Login() (*AppLoginResult, error) {
	// 设置 session cookies（对应 Python self._session.cookies.set(...)）
	miURL, _ := url.Parse("https://mi.com")
	xiaomiURL, _ := url.Parse("https://xiaomi.com")
	sdkCookie := &http.Cookie{Name: "sdkVersion", Value: "accountsdk-18.8.15"}
	devCookie := &http.Cookie{Name: "deviceId", Value: c.deviceID}
	c.jar.SetCookies(miURL, []*http.Cookie{sdkCookie, devCookie})
	c.jar.SetCookies(xiaomiURL, []*http.Cookie{sdkCookie, devCookie})

	// login_step_1
	if ok, err := c.loginStep1(); !ok {
		if err != nil {
			return nil, fmt.Errorf("login step1: %w", err)
		}
		return nil, fmt.Errorf("invalid username")
	}

	// login_step_2（可能返回 *ErrTwoFactorRequired，直接透传给调用方）
	if ok, err := c.loginStep2(); !ok {
		if err != nil {
			return nil, err // 包含 *ErrTwoFactorRequired 或其他错误
		}
		return nil, fmt.Errorf("invalid login or password")
	}

	// login_step_3（仅当 location 不为空且 serviceToken 还未设置时）
	if c.location != "" && c.serviceToken == "" {
		if ok, err := c.loginStep3(); !ok {
			if err != nil {
				return nil, fmt.Errorf("unable to get service token: %w", err)
			}
			return nil, fmt.Errorf("unable to get service token")
		}
	}

	return c.result(), nil
}

// CompleteTwoFactor 在用户提供邮件验证码后完成登录。
//
// 必须在**同一个 PasswordConnector 实例**上调用（Login() 返回 *ErrTwoFactorRequired 之后）。
// 内部会重新执行步骤 1-3（刷新会话 + ick cookie）再提交验证码，
// 因为步骤 1-3 建立的 cookie 会随时间过期或在进程重启后丢失。
//
// code — 用户收到的邮件验证码
//
// 对应 Python do_2fa_email_flow 的完整流程。
func (c *PasswordConnector) CompleteTwoFactor(code string) (*AppLoginResult, error) {
	if c.tfa2NotifURL == "" {
		return nil, fmt.Errorf("CompleteTwoFactor must be called on the same PasswordConnector instance that returned ErrTwoFactorRequired from Login()")
	}
	// 重新执行步骤 1-3，刷新会话状态和 ick cookie
	ctx, err := c.start2FAEmailFlow(c.tfa2NotifURL)
	if err != nil {
		return nil, fmt.Errorf("re-establish 2FA session failed: %w", err)
	}
	if err := c.complete2FAEmailFlow(ctx, code); err != nil {
		return nil, err
	}
	// do_2fa_email_flow 成功后 serviceToken 已设置，login_step_3 条件为假，无需再调。
	return c.result(), nil
}

// result 构造 AppLoginResult
func (c *PasswordConnector) result() *AppLoginResult {
	return &AppLoginResult{
		UserID:       c.userID,
		CUserID:      c.cUserID,
		SSecurity:    c.ssecurity,
		PassToken:    c.passToken,
		ServiceToken: c.serviceToken,
		Location:     c.location,
	}
}

// loginStep1 对应 Python PasswordXiaomiCloudConnector.login_step_1
func (c *PasswordConnector) loginStep1() (bool, error) {
	reqURL := "https://account.xiaomi.com/pass/serviceLogin?sid=xiaomiio&_json=true"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// cookies: {"userId": self._username}
	acctURL, _ := url.Parse("https://account.xiaomi.com")
	c.jar.SetCookies(acctURL, []*http.Cookie{
		{Name: "userId", Value: c.username},
	})

	resp, err := c.session.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	jsonResp, err := appToJSON(string(raw))
	if err != nil {
		return false, err
	}

	if sign, ok := jsonResp["_sign"].(string); ok && sign != "" {
		c.sign = sign
		return true, nil
	}
	if ssec, ok := jsonResp["ssecurity"].(string); ok && ssec != "" {
		c.ssecurity = ssec
		c.userID = intValStr(jsonResp["userId"])
		c.cUserID, _ = jsonResp["cUserId"].(string)
		c.passToken, _ = jsonResp["passToken"].(string)
		c.location, _ = jsonResp["location"].(string)
		c.code = jsonResp["code"]
		return true, nil
	}

	return false, nil
}

// loginStep2 对应 Python PasswordXiaomiCloudConnector.login_step_2
//
// 当响应包含 notificationUrl 时，直接调用 do2FAEmailFlow 并返回其结果，
// 这与 Python `return self.do_2fa_email_flow(verify_url)` 完全一致：
// do_2fa_email_flow 内部会设置 ssecurity 和 serviceToken，
// login() 随后的 step3 判断 `c.location != "" && c.serviceToken == ""`
// 为假，故跳过 step3。
func (c *PasswordConnector) loginStep2() (bool, error) {
	reqURL := "https://account.xiaomi.com/pass/serviceLoginAuth2"

	// hash = MD5(password).hexdigest().upper()
	h := md5.New() //nolint:gosec
	h.Write([]byte(c.password))
	pwdHash := strings.ToUpper(fmt.Sprintf("%x", h.Sum(nil)))

	fields := url.Values{}
	fields.Set("sid", "xiaomiio")
	fields.Set("hash", pwdHash)
	fields.Set("callback", "https://sts.api.io.mi.com/sts")
	fields.Set("qs", "%3Fsid%3Dxiaomiio%26_json%3Dtrue")
	fields.Set("user", c.username)
	fields.Set("_sign", c.sign)
	fields.Set("_json", "true")

	// allow_redirects=False
	noRedirect := &http.Client{
		Timeout: MIHomeHTTPAPITimeout,
		Jar:     c.jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(http.MethodPost, reqURL+"?"+fields.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := noRedirect.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	// valid = response is not None and response.status_code == 200
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("http status %d: %s", resp.StatusCode, string(raw[:minInt(200, len(raw))]))
	}

	jsonResp, err := appToJSON(string(raw))
	if err != nil {
		return false, err
	}

	// captchaUrl 处理（本实现不支持交互式验证码，直接返回错误）
	if captchaURL, ok := jsonResp["captchaUrl"].(string); ok && captchaURL != "" {
		return false, fmt.Errorf("captcha required: %s", captchaURL)
	}

	// valid = "ssecurity" in json_resp and len(str(json_resp["ssecurity"])) > 4
	ssec, _ := jsonResp["ssecurity"].(string)
	valid := len(ssec) > 4

	if valid {
		c.ssecurity = ssec
		c.userID = intValStr(jsonResp["userId"])
		c.cUserID, _ = jsonResp["cUserId"].(string)
		c.passToken, _ = jsonResp["passToken"].(string)
		c.location, _ = jsonResp["location"].(string)
		c.code = jsonResp["code"]
		return true, nil
	}

	// else 分支：检查 notificationUrl（2FA）
	if notifURL, ok := jsonResp["notificationUrl"].(string); ok && notifURL != "" {
		// 对应 Python: return self.do_2fa_email_flow(verify_url)
		// 这里拆分为两步：先发送邮件验证码（start），暂存 context，
		// 再由调用方提供验证码后调用 CompleteTwoFactor 完成后半段。
		tfaCtx, err := c.start2FAEmailFlow(notifURL)
		if err != nil {
			return false, err
		}
		// 暂存 notifURL 以便 CompleteTwoFactor 重新建立会话
		c.tfa2NotifURL = notifURL
		return false, &ErrTwoFactorRequired{
			TwoFactorContext: tfaCtx,
			notificationURL:  notifURL,
		}
	}

	return false, nil
}

// loginStep3 对应 Python PasswordXiaomiCloudConnector.login_step_3
func (c *PasswordConnector) loginStep3() (bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.location, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.session.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// response.cookies.get("serviceToken")
		for _, ck := range resp.Cookies() {
			if ck.Name == "serviceToken" {
				c.serviceToken = ck.Value
				break
			}
		}
	}
	return resp.StatusCode == http.StatusOK, nil
}

// start2FAEmailFlow 对应 Python do_2fa_email_flow 的前半段（步骤 1-3）：
// 打开 notificationUrl、获取 identity list、发送邮件验证码。
//
// 返回值 context 是后续 complete2FAEmailFlow 所需的不透明字符串。
func (c *PasswordConnector) start2FAEmailFlow(notificationURL string) (tfaContext string, err error) {
	headers := map[string]string{
		"User-Agent":   c.agent,
		"Content-Type": "application/x-www-form-urlencoded",
	}
	doGet := c.makeDoGet(headers)
	doPost := c.makeDoPost(headers)

	// 1) Open notificationUrl (authStart)
	r, err := doGet(notificationURL, nil, true)
	if err != nil {
		return "", err
	}
	r.Body.Close()

	// 2) Fetch identity options (list)
	parsed, err := url.Parse(notificationURL)
	if err != nil {
		return "", err
	}
	context := parsed.Query().Get("context")

	listParams := url.Values{}
	listParams.Set("sid", "xiaomiio")
	listParams.Set("context", context)
	listParams.Set("_locale", "en_US")

	r, err = doGet("https://account.xiaomi.com/identity/list", listParams, true)
	if err != nil {
		return "", err
	}
	r.Body.Close()

	// 3) Request email ticket（发送验证码到邮箱）
	sendParams := url.Values{}
	sendParams.Set("_dc", fmt.Sprintf("%d", time.Now().UnixMilli()))
	sendParams.Set("sid", "xiaomiio")
	sendParams.Set("context", context)
	sendParams.Set("mask", "0")
	sendParams.Set("_locale", "en_US")

	ickCookie := ""
	acctURL, _ := url.Parse("https://account.xiaomi.com")
	for _, ck := range c.jar.Cookies(acctURL) {
		if ck.Name == "ick" {
			ickCookie = ck.Value
			break
		}
	}
	// 暂存 ick，complete2FAEmailFlow 中 verifyEmail 需要
	c.tfa2IckCookie = ickCookie

	sendData := url.Values{}
	sendData.Set("retry", "0")
	sendData.Set("icode", "")
	sendData.Set("_json", "true")
	sendData.Set("ick", ickCookie)

	r, err = doPost("https://account.xiaomi.com/identity/auth/sendEmailTicket", sendParams, sendData)
	if err != nil {
		return "", err
	}
	r.Body.Close()

	return context, nil
}

// complete2FAEmailFlow 对应 Python do_2fa_email_flow 的后半段（步骤 4-7）：
// 提交用户输入的邮件验证码、跟随重定向链、提取 ssecurity + serviceToken。
//
// context — start2FAEmailFlow 返回的值（即 ErrTwoFactorRequired.TwoFactorContext）
// code    — 用户从邮件中获取的验证码
func (c *PasswordConnector) complete2FAEmailFlow(context, code string) error {
	headers := map[string]string{
		"User-Agent":   c.agent,
		"Content-Type": "application/x-www-form-urlencoded",
	}
	doGet := c.makeDoGet(headers)
	doPost := c.makeDoPost(headers)
	ickCookie := c.tfa2IckCookie

	// 4) 提交验证码
	verifyParams := url.Values{}
	verifyParams.Set("_flag", "8")
	verifyParams.Set("_json", "true")
	verifyParams.Set("sid", "xiaomiio")
	verifyParams.Set("context", context)
	verifyParams.Set("mask", "0")
	verifyParams.Set("_locale", "en_US")

	verifyData := url.Values{}
	verifyData.Set("_flag", "8")
	verifyData.Set("ticket", code)
	verifyData.Set("trust", "false")
	verifyData.Set("_json", "true")
	verifyData.Set("ick", ickCookie)

	r, err := doPost("https://account.xiaomi.com/identity/auth/verifyEmail", verifyParams, verifyData)
	if err != nil {
		return err
	}
	verifyRaw, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return err
	}

	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("verifyEmail failed: status=%d body=%s", r.StatusCode, string(verifyRaw[:minInt(500, len(verifyRaw))]))
	}

	// 检查 JSON 中的错误码（如 70014 = 验证码错误）
	var jrCheck map[string]interface{}
	if json.Unmarshal(verifyRaw, &jrCheck) == nil {
		if code, _ := jrCheck["code"].(float64); int(code) != 0 {
			tips, _ := jrCheck["tips"].(string)
			desc, _ := jrCheck["desc"].(string)
			return fmt.Errorf("verifyEmail error (code=%d): %s %s", int(code), tips, desc)
		}
	}

	// 解析 finish_loc（对应 Python try/except 逻辑）
	var finishLoc string
	var jrVerify map[string]interface{}
	if err := json.Unmarshal(verifyRaw, &jrVerify); err == nil {
		finishLoc, _ = jrVerify["location"].(string)
	}
	// fallback: Location header
	if finishLoc == "" {
		finishLoc = r.Header.Get("Location")
	}
	// fallback: regex search in body
	if finishLoc == "" && len(verifyRaw) > 0 {
		re := regexp.MustCompile(`https://account\.xiaomi\.com/identity/result/check\?[^"']+`)
		if m := re.Find(verifyRaw); m != nil {
			finishLoc = string(m)
		}
	}

	// Fallback: directly hit result/check
	if finishLoc == "" {
		checkParams := url.Values{}
		checkParams.Set("sid", "xiaomiio")
		checkParams.Set("context", context)
		checkParams.Set("_locale", "en_US")
		r0, err := doGet("https://account.xiaomi.com/identity/result/check", checkParams, false)
		if err != nil {
			return err
		}
		r0.Body.Close()
		if r0.StatusCode == http.StatusMovedPermanently || r0.StatusCode == http.StatusFound {
			loc := r0.Header.Get("Location")
			if strings.Contains(r0.Request.URL.String(), "serviceLoginAuth2/end") {
				finishLoc = r0.Request.URL.String()
			} else if loc != "" {
				finishLoc = loc
			}
		}
	}

	if finishLoc == "" {
		return fmt.Errorf("unable to determine finish location after verifyEmail: status=%d headers=%v body=%s",
			r.StatusCode, r.Header, string(verifyRaw[:minInt(800, len(verifyRaw))]))
	}

	// First hop: GET identity/result/check (allow_redirects=False)
	var endURL string
	if strings.Contains(finishLoc, "identity/result/check") {
		r, err = doGet(finishLoc, nil, false)
		if err != nil {
			return err
		}
		r.Body.Close()
		endURL = r.Header.Get("Location")
	} else {
		endURL = finishLoc
	}

	if endURL == "" {
		return fmt.Errorf("could not find Auth2/end URL in finish chain")
	}

	// 6) Call Auth2/end WITHOUT redirects to capture 'extension-pragma' header
	r, err = doGet(endURL, nil, false)
	if err != nil {
		return err
	}
	auth2Body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return err
	}

	// Some servers return 200 first (HTML 'Tips' page), then 302 on next call.
	if r.StatusCode == http.StatusOK && strings.Contains(string(auth2Body), "Xiaomi Account - Tips") {
		r, err = doGet(endURL, nil, false)
		if err != nil {
			return err
		}
		auth2Body, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return err
		}
	}

	// 解析 extension-pragma header 获取 ssecurity
	if extPrag := r.Header.Get("extension-pragma"); extPrag != "" {
		var epJSON map[string]interface{}
		if err := json.Unmarshal([]byte(extPrag), &epJSON); err == nil {
			if ssec, ok := epJSON["ssecurity"].(string); ok && ssec != "" {
				c.ssecurity = ssec
			}
		}
	}

	if c.ssecurity == "" {
		return fmt.Errorf("extension-pragma header missing ssecurity; cannot continue")
	}

	// 7) Find STS redirect and visit it (to set serviceToken cookie)
	stsURL := r.Header.Get("Location")
	if stsURL == "" && len(auth2Body) > 0 {
		idx := strings.Index(string(auth2Body), "https://sts.api.io.mi.com/sts")
		if idx != -1 {
			body := string(auth2Body)
			end := strings.Index(body[idx:], `"`)
			if end == -1 {
				end = minInt(idx+300, len(body))
			} else {
				end = idx + end
			}
			stsURL = body[idx:end]
		}
	}
	if stsURL == "" {
		return fmt.Errorf("Auth2/end did not provide STS redirect")
	}

	r, err = c.session.Get(stsURL)
	if err != nil {
		return err
	}
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("STS did not complete: status=%d", r.StatusCode)
	}

	// Extract serviceToken from cookie jar
	stsJarURL, _ := url.Parse("https://sts.api.io.mi.com")
	for _, ck := range c.jar.Cookies(stsJarURL) {
		if ck.Name == "serviceToken" {
			c.serviceToken = ck.Value
			break
		}
	}
	if c.serviceToken == "" {
		return fmt.Errorf("could not parse serviceToken from STS cookies")
	}

	// install_service_token_cookies
	c.installServiceTokenCookies(c.serviceToken)

	// Update ids from cookies if available
	xiaomiJarURL, _ := url.Parse("https://xiaomi.com")
	if c.userID == "" {
		for _, ck := range c.jar.Cookies(xiaomiJarURL) {
			if ck.Name == "userId" {
				c.userID = ck.Value
				break
			}
		}
	}
	if c.cUserID == "" {
		for _, ck := range c.jar.Cookies(xiaomiJarURL) {
			if ck.Name == "cUserId" {
				c.cUserID = ck.Value
				break
			}
		}
	}

	return nil
}

// makeDoGet 创建带 headers 的 GET 辅助函数（allowRedirect=false 时不跟随跳转）
func (c *PasswordConnector) makeDoGet(headers map[string]string) func(rawURL string, params url.Values, allowRedirect bool) (*http.Response, error) {
	return func(rawURL string, params url.Values, allowRedirect bool) (*http.Response, error) {
		if params != nil {
			rawURL = rawURL + "?" + params.Encode()
		}
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		client := c.session
		if !allowRedirect {
			client = &http.Client{
				Timeout: MIHomeHTTPAPITimeout,
				Jar:     c.jar,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
		}
		return client.Do(req)
	}
}

// makeDoPost 创建带 headers 的 POST 辅助函数
func (c *PasswordConnector) makeDoPost(headers map[string]string) func(rawURL string, params url.Values, data url.Values) (*http.Response, error) {
	return func(rawURL string, params url.Values, data url.Values) (*http.Response, error) {
		encoded := rawURL
		if params != nil {
			encoded = rawURL + "?" + params.Encode()
		}
		req, err := http.NewRequest(http.MethodPost, encoded, strings.NewReader(data.Encode()))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return c.session.Do(req)
	}
}

// installServiceTokenCookies 对应 Python install_service_token_cookies
func (c *PasswordConnector) installServiceTokenCookies(token string) {
	for _, domain := range []string{".api.io.mi.com", ".io.mi.com", ".mi.com"} {
		u, _ := url.Parse("https://" + strings.TrimPrefix(domain, "."))
		c.jar.SetCookies(u, []*http.Cookie{
			{Name: "serviceToken", Value: token, Domain: domain},
			{Name: "yetAnotherServiceToken", Value: token, Domain: domain},
		})
	}
}

// ---------- 二维码登录（对应 QrCodeXiaomiCloudConnector） ----------

// QrCodeConnector 使用二维码登录小米账号
type QrCodeConnector struct {
	xiaomiCloudBase

	cUserID        string // _cUserId
	passToken      string // _pass_token（注意 Python 用 _pass_token 非 _passToken）
	location       string // _location
	qrImageURL     string // _qr_image_url
	loginURL       string // _login_url
	longPollingURL string // _long_polling_url
	timeout        int    // _timeout
}

// NewQrCodeConnector 创建二维码登录客户端
func NewQrCodeConnector() (*QrCodeConnector, error) {
	base, err := newXiaomiCloudBase()
	if err != nil {
		return nil, err
	}
	return &QrCodeConnector{xiaomiCloudBase: *base}, nil
}

// Login 执行二维码登录
// 严格对应 Python QrCodeXiaomiCloudConnector.login
//
// onQR 回调在获取到二维码图片后调用，参数为图片字节和可选的登录 URL。
// 对应 Python 中 login_step_2 调用 present_image_image 展示二维码。
func (c *QrCodeConnector) Login(onQR func(imgBytes []byte, loginURL string)) (*AppLoginResult, error) {
	// login_step_1
	if ok, err := c.loginStep1(); !ok {
		if err != nil {
			return nil, fmt.Errorf("unable to get login message: %w", err)
		}
		return nil, fmt.Errorf("unable to get login message")
	}

	// login_step_2（获取并展示二维码图片）
	if ok, err := c.loginStep2(onQR); !ok {
		if err != nil {
			return nil, fmt.Errorf("unable to get login QR image: %w", err)
		}
		return nil, fmt.Errorf("unable to get login QR image")
	}

	// login_step_3（长轮询等待扫码）
	if ok, err := c.loginStep3(); !ok {
		if err != nil {
			return nil, fmt.Errorf("unable to login: %w", err)
		}
		return nil, fmt.Errorf("unable to login")
	}

	// login_step_4（获取 serviceToken）
	if ok, err := c.loginStep4(); !ok {
		if err != nil {
			return nil, fmt.Errorf("unable to get service token: %w", err)
		}
		return nil, fmt.Errorf("unable to get service token")
	}

	return &AppLoginResult{
		UserID:       c.userID,
		CUserID:      c.cUserID,
		SSecurity:    c.ssecurity,
		PassToken:    c.passToken,
		ServiceToken: c.serviceToken,
		Location:     c.location,
	}, nil
}

// loginStep1 对应 Python QrCodeXiaomiCloudConnector.login_step_1
func (c *QrCodeConnector) loginStep1() (bool, error) {
	params := url.Values{}
	params.Set("_qrsize", "480")
	params.Set("qs", "%3Fsid%3Dxiaomiio%26_json%3Dtrue")
	params.Set("callback", "https://sts.api.io.mi.com/sts")
	params.Set("_hasLogo", "false")
	params.Set("sid", "xiaomiio")
	params.Set("serviceParam", "")
	params.Set("_locale", "en_GB")
	params.Set("_dc", fmt.Sprintf("%d", time.Now().UnixMilli()))

	resp, err := c.session.Get("https://account.xiaomi.com/longPolling/loginUrl?" + params.Encode())
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	jsonResp, err := appToJSON(string(raw))
	if err != nil {
		return false, err
	}

	if _, ok := jsonResp["qr"]; !ok {
		return false, nil
	}

	c.qrImageURL, _ = jsonResp["qr"].(string)
	c.loginURL, _ = jsonResp["loginUrl"].(string)
	c.longPollingURL, _ = jsonResp["lp"].(string)
	if t, ok := jsonResp["timeout"].(float64); ok {
		c.timeout = int(t)
	}
	return true, nil
}

// loginStep2 对应 Python QrCodeXiaomiCloudConnector.login_step_2
func (c *QrCodeConnector) loginStep2(onQR func(imgBytes []byte, loginURL string)) (bool, error) {
	resp, err := c.session.Get(c.qrImageURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("http status %d", resp.StatusCode)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	// 对应 Python: print + present_image_image + print loginUrl
	if onQR != nil {
		onQR(imgBytes, c.loginURL)
	}
	return true, nil
}

// loginStep3 对应 Python QrCodeXiaomiCloudConnector.login_step_3（长轮询）
func (c *QrCodeConnector) loginStep3() (bool, error) {
	startTime := time.Now()
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 180
	}

	pollClient := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     c.jar,
	}

	var resp *http.Response
	for {
		var err error
		resp, err = pollClient.Get(c.longPollingURL)
		if err != nil {
			// requests.exceptions.Timeout → retry
			if time.Since(startTime) > time.Duration(timeout)*time.Second {
				return false, fmt.Errorf("long polling timed out after %d seconds", timeout)
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			break
		}

		resp.Body.Close()
		if time.Since(startTime) > time.Duration(timeout)*time.Second {
			return false, fmt.Errorf("long polling timed out, last status: %d", resp.StatusCode)
		}
		// 对应 Python: _LOGGER.error("Long polling failed, retrying...")
		continue
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return false, nil
	}

	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return false, err
	}

	jsonResp, err := appToJSON(string(raw))
	if err != nil {
		return false, err
	}

	c.userID = intValStr(jsonResp["userId"])
	c.ssecurity, _ = jsonResp["ssecurity"].(string)
	c.cUserID, _ = jsonResp["cUserId"].(string)
	c.passToken, _ = jsonResp["passToken"].(string)
	c.location, _ = jsonResp["location"].(string)
	return true, nil
}

// loginStep4 对应 Python QrCodeXiaomiCloudConnector.login_step_4
func (c *QrCodeConnector) loginStep4() (bool, error) {
	if c.location == "" {
		return false, fmt.Errorf("no location found")
	}

	req, err := http.NewRequest(http.MethodGet, c.location, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := c.session.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	// response.cookies["serviceToken"]
	for _, ck := range resp.Cookies() {
		if ck.Name == "serviceToken" {
			c.serviceToken = ck.Value
			return true, nil
		}
	}
	return false, fmt.Errorf("serviceToken not found in response cookies")
}

// ---------- 工具函数 ----------

// minInt 返回两整数中的较小值
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
