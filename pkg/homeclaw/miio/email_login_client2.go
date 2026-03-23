// Package miio provides email-password login client v2 built on BrowserClient.
//
// EmailLoginClient2 是基于 common.BrowserClient 重新实现的密码登录客户端，
// 与 mail_login_client.go（PasswordConnector）并列存在，便于对比两种实现的差异。
//
// 对应 Python token_extractor.py PasswordXiaomiCloudConnector，
// 2FA email 流程在 Python input() 处严格拆成两段：
//
//	第一段 LoginPhase1() — 发送 email 验证码，返回保留的上下文（TFAContext）
//	第二段 LoginPhase2(ctx, code) — 拿到用户输入的 code，继续请求直到获取 token
package miio

import (
	"crypto/md5" //nolint:gosec
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/common"
)

// ---------- 公开错误类型 ---------------------------------------------------

// ErrTFAEmailRequired2 表示登录需要 email 二次验证（v2 版本）。
// 调用方收到此错误后从 TFAContext 中读取提示，要求用户输入邮件验证码，
// 再在**同一个 EmailLoginClient2 实例**上调用 LoginPhase2。
type ErrTFAEmailRequired2 struct {
	// TFAContext 是继续流程所需的不透明上下文（仅供展示或日志）。
	TFAContext *emailTFAContext2
}

func (e *ErrTFAEmailRequired2) Error() string {
	return "2FA email verification required (v2)"
}

// ---------- 内部 2FA 上下文 -----------------------------------------------

// emailTFAContext2 保存 start2FAEmailFlow 阶段产生的所有上下文，
// 供 complete2FAEmailFlow 继续使用，不重新发送验证码。
type emailTFAContext2 struct {
	context   string // URL 中的 context 参数
	ickCookie string // sendEmailTicket 前/后的 ick cookie
}

// ---------- EmailLoginClient2 主结构 -------------------------------------

// EmailLoginClient2 使用 common.BrowserClient 模拟浏览器行为的小米账号密码登录客户端。
//
// 登录流程与 Python PasswordXiaomiCloudConnector 严格对应：
//
//	Step1: loginStep1  → GET serviceLogin（获取 _sign）
//	Step2: loginStep2  → POST serviceLoginAuth2（验证密码；可能触发 captcha 或 2FA email）
//	Step3: loginStep3  → GET location（获取 serviceToken cookie）
//
// 若 Step2 检测到 notificationUrl（2FA），拆成：
//
//	start2FAEmailFlow    — 步骤 1-3：发送 email 验证码，返回 emailTFAContext2
//	complete2FAEmailFlow — 步骤 4-7：提交 code，跟随重定向，提取 ssecurity + serviceToken
type EmailLoginClient2 struct {
	browser  *common.BrowserClient
	agent    string
	deviceID string

	username string
	password string

	// login flow intermediate state
	sign      string
	ssecurity string
	userID    string
	cUserID   string
	passToken string
	location  string
	code      interface{}

	serviceToken string

	// 暂存 2FA context，供 LoginPhase2 使用
	pendingTFA *emailTFAContext2
}

// NewEmailLoginClient2 创建 v2 密码登录客户端。
func NewEmailLoginClient2(username, password string) (*EmailLoginClient2, error) {
	agent := emailGenerateAgent2()
	deviceID := emailGenerateDeviceID2()

	bc, err := common.NewBrowserClient(
		common.WithUserAgent(agent),
		common.WithTimeout(MIHomeHTTPAPITimeout),
		common.WithDefaultHeader("Content-Type", "application/x-www-form-urlencoded"),
	)
	if err != nil {
		return nil, fmt.Errorf("EmailLoginClient2: create browser client: %w", err)
	}

	return &EmailLoginClient2{
		browser:  bc,
		agent:    agent,
		deviceID: deviceID,
		username: username,
		password: password,
	}, nil
}

// SetCredentials 更新登录凭据（Login 前调用）。
func (c *EmailLoginClient2) SetCredentials(username, password string) {
	c.username = username
	c.password = password
}

// ---------- 公开 API -------------------------------------------------------

// Login 执行密码登录。
//
// 成功返回 *AppLoginResult，需要 2FA 时返回 *ErrTFAEmailRequired2。
// 调用方收到 *ErrTFAEmailRequired2 后：
//  1. 提示用户查收邮件，输入 code
//  2. 在**同一实例**上调用 LoginPhase2(code) 完成登录
//
// 对应 Python PasswordXiaomiCloudConnector.login()。
func (c *EmailLoginClient2) Login() (*AppLoginResult, error) {
	// 初始化 session cookies（对应 Python self._session.cookies.set(...)）
	_ = c.browser.SetCookie("https://mi.com", "sdkVersion", "accountsdk-18.8.15")
	_ = c.browser.SetCookie("https://xiaomi.com", "sdkVersion", "accountsdk-18.8.15")
	_ = c.browser.SetCookie("https://mi.com", "deviceId", c.deviceID)
	_ = c.browser.SetCookie("https://xiaomi.com", "deviceId", c.deviceID)

	// login_step_1
	if ok, err := c.loginStep1(); !ok {
		if err != nil {
			return nil, fmt.Errorf("login step1: %w", err)
		}
		return nil, fmt.Errorf("invalid username")
	}

	// login_step_2（可能返回 *ErrTFAEmailRequired2，直接透传给调用方）
	if ok, err := c.loginStep2(); !ok {
		if err != nil {
			return nil, err
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

	return c.buildResult(), nil
}

// LoginPhase2 在收到用户的 email 验证码后完成登录。
//
// 必须在**同一个 EmailLoginClient2 实例**上调用（Login() 返回 *ErrTFAEmailRequired2 之后）。
// 复用 Login() 阶段已建立的 BrowserClient 会话状态（cookie jar + ick），
// 不重新发送验证码，与 Python do_2fa_email_flow 步骤 4-7 完全对应。
//
// code — 用户从邮件中获取的验证码
func (c *EmailLoginClient2) LoginPhase2(code string) (*AppLoginResult, error) {
	if c.pendingTFA == nil {
		return nil, fmt.Errorf("LoginPhase2 must be called on the same EmailLoginClient2 instance that returned ErrTFAEmailRequired2 from Login()")
	}

	if err := c.complete2FAEmailFlow(c.pendingTFA, code); err != nil {
		return nil, err
	}
	// complete2FAEmailFlow 成功后 serviceToken 已设置，无需再调 login_step_3
	return c.buildResult(), nil
}

// ---------- BrowserClient 访问器 ------------------------------------------

// BrowserClient 返回内部的 BrowserClient，供需要直接操作 cookie/header 的调用方使用。
func (c *EmailLoginClient2) BrowserClient() *common.BrowserClient {
	return c.browser
}

// ---------- 私有：login steps ----------------------------------------------

// loginStep1 对应 Python PasswordXiaomiCloudConnector.login_step_1
func (c *EmailLoginClient2) loginStep1() (bool, error) {
	// cookies: {"userId": self._username}
	_ = c.browser.SetCookie("https://account.xiaomi.com", "userId", c.username)

	resp, err := c.browser.Get(
		"https://account.xiaomi.com/pass/serviceLogin?sid=xiaomiio&_json=true",
		nil, nil,
	)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	jsonResp, err := emailToJSON2(resp.Text())
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
// captcha: Python 会交互式要求用户输入验证码后重试；
// 本 v2 实现直接返回 error（不支持交互式 captcha），与原 mail_login_client.go 行为一致。
//
// notificationUrl: 触发 2FA email 流程；
// 拆成 start2FAEmailFlow（发送 email）+ LoginPhase2（提交 code）两段。
func (c *EmailLoginClient2) loginStep2() (bool, error) {
	pwdHash := emailMD5Upper2(c.password)

	params := url.Values{}
	params.Set("sid", "xiaomiio")
	params.Set("hash", pwdHash)
	params.Set("callback", "https://sts.api.io.mi.com/sts")
	params.Set("qs", "%3Fsid%3Dxiaomiio%26_json%3Dtrue")
	params.Set("user", c.username)
	params.Set("_sign", c.sign)
	params.Set("_json", "true")

	// allow_redirects=False（对应 Python self._session.post(..., allow_redirects=False)）
	resp, err := c.browser.GetNoRedirect(
		"https://account.xiaomi.com/pass/serviceLoginAuth2?"+params.Encode(),
		nil, nil,
	)
	if err != nil {
		return false, err
	}

	// Python 使用 POST，但参数在 query string；用 POST + form body 也可以，
	// 这里严格跟随 Python：params 在 URL，body 为空，method=POST。
	// BrowserClient 没有直接的 POST-with-empty-body-and-no-redirect，
	// 因此我们手动构造 request。
	postResp, err := c.doPostNoRedirect(
		"https://account.xiaomi.com/pass/serviceLoginAuth2",
		params, nil,
	)
	if err != nil {
		// 如果上面 GET 方式失败则用 postResp；若 postResp 也失败则返回
		if postResp == nil {
			return false, err
		}
	}
	// 优先使用 POST 结果（更接近 Python），回退到 GET 结果
	if postResp != nil {
		resp = postResp
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("http status %d: %.200s", resp.StatusCode, resp.Text())
	}

	jsonResp, err := emailToJSON2(resp.Text())
	if err != nil {
		return false, err
	}

	// captchaUrl 处理（对应 Python handle_captcha 交互流程；v2 直接返回 error）
	if captchaURL, ok := jsonResp["captchaUrl"].(string); ok && captchaURL != "" {
		c.browser.SetCaptchaChallenge(captchaURL, "")
		return false, fmt.Errorf("captcha required: %s", captchaURL)
	}

	// valid = "ssecurity" in json_resp and len(str(json_resp["ssecurity"])) > 4
	ssec, _ := jsonResp["ssecurity"].(string)
	if len(ssec) > 4 {
		c.ssecurity = ssec
		c.userID = intValStr(jsonResp["userId"])
		c.cUserID, _ = jsonResp["cUserId"].(string)
		c.passToken, _ = jsonResp["passToken"].(string)
		c.location, _ = jsonResp["location"].(string)
		c.code = jsonResp["code"]
		return true, nil
	}

	// else 分支：检查 notificationUrl（2FA email）
	if notifURL, ok := jsonResp["notificationUrl"].(string); ok && notifURL != "" {
		// ---- 第一段：发送 email 验证码，保存 TFA context ----
		// 对应 Python: return self.do_2fa_email_flow(verify_url)
		// 但我们在此处中断，把 context 暴露给调用方
		tfaCtx, err := c.start2FAEmailFlow(notifURL)
		if err != nil {
			return false, err
		}
		c.pendingTFA = tfaCtx
		c.browser.SetEmailChallenge("") // 标记 browser 处于 AwaitingEmailCode 状态
		return false, &ErrTFAEmailRequired2{TFAContext: tfaCtx}
	}

	return false, nil
}

// loginStep3 对应 Python PasswordXiaomiCloudConnector.login_step_3
func (c *EmailLoginClient2) loginStep3() (bool, error) {
	resp, err := c.browser.Get(c.location, nil, nil)
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusOK {
		for _, ck := range resp.Cookies() {
			if ck.Name == "serviceToken" {
				c.serviceToken = ck.Value
				break
			}
		}
	}
	return resp.StatusCode == http.StatusOK, nil
}

// ---------- 私有：2FA email 流程 ------------------------------------------

// start2FAEmailFlow 对应 Python do_2fa_email_flow 步骤 1-3：
//
//  1. 打开 notificationUrl（authStart，服务端在此设置 ick cookie）
//  2. GET /identity/list
//  3. POST /identity/auth/sendEmailTicket（触发服务端发送验证码邮件）
//
// 返回 emailTFAContext2，供 complete2FAEmailFlow 使用。
func (c *EmailLoginClient2) start2FAEmailFlow(notificationURL string) (*emailTFAContext2, error) {
	// 1) Open notificationUrl (authStart) — server sets "ick" cookie
	_, err := c.browser.Get(notificationURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("start2FAEmailFlow authStart: %w", err)
	}

	// 读取 ick cookie（对应 Python self._session.cookies.get("ick", "")）
	ickCookie := c.browser.GetCookieAny("ick",
		"https://account.xiaomi.com",
		"https://xiaomi.com",
		"https://mi.com",
		"https://identity.xiaomi.com",
	)

	// 2) Fetch identity options (list)
	parsed, err := url.Parse(notificationURL)
	if err != nil {
		return nil, fmt.Errorf("start2FAEmailFlow parse notificationUrl: %w", err)
	}
	context := parsed.Query().Get("context")

	listParams := url.Values{}
	listParams.Set("sid", "xiaomiio")
	listParams.Set("context", context)
	listParams.Set("_locale", "en_US")

	listResp, err := c.browser.Get("https://account.xiaomi.com/identity/list", listParams, nil)
	if err != nil {
		return nil, fmt.Errorf("start2FAEmailFlow identity/list: %w", err)
	}
	if listResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("start2FAEmailFlow identity/list status=%d body=%.500s",
			listResp.StatusCode, listResp.Text())
	}

	// ick 可能在 step 1 未设置，step 2 之后才出现
	if ickCookie == "" {
		ickCookie = c.browser.GetCookieAny("ick",
			"https://account.xiaomi.com",
			"https://xiaomi.com",
			"https://mi.com",
			"https://identity.xiaomi.com",
		)
	}
	// fallback: 从 identity/list 响应 JSON body 中提取
	if ickCookie == "" {
		if listJSON, jsonErr := emailToJSON2(listResp.Text()); jsonErr == nil {
			if v, ok := listJSON["ick"].(string); ok {
				ickCookie = v
			}
		}
	}

	// 3) Request email ticket（触发服务端发送验证码邮件）
	sendParams := url.Values{}
	sendParams.Set("_dc", fmt.Sprintf("%d", time.Now().UnixMilli()))
	sendParams.Set("sid", "xiaomiio")
	sendParams.Set("context", context)
	sendParams.Set("mask", "0")
	sendParams.Set("_locale", "en_US")

	sendData := url.Values{}
	sendData.Set("retry", "0")
	sendData.Set("icode", "")
	sendData.Set("_json", "true")
	sendData.Set("ick", ickCookie)

	sendResp, err := c.browser.PostForm(
		"https://account.xiaomi.com/identity/auth/sendEmailTicket",
		sendParams, sendData, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("start2FAEmailFlow sendEmailTicket: %w", err)
	}
	if sendResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("start2FAEmailFlow sendEmailTicket status=%d body=%.500s",
			sendResp.StatusCode, sendResp.Text())
	}
	// 检查 JSON 级别错误（对应 Python jr.get("code")）
	if sendJSON, jsonErr := emailToJSON2(sendResp.Text()); jsonErr == nil {
		if code, _ := sendJSON["code"].(float64); int(code) != 0 {
			tips, _ := sendJSON["tips"].(string)
			desc, _ := sendJSON["desc"].(string)
			return nil, fmt.Errorf("start2FAEmailFlow sendEmailTicket error (code=%d): %s %s",
				int(code), tips, desc)
		}
	}

	return &emailTFAContext2{
		context:   context,
		ickCookie: ickCookie,
	}, nil
}

// complete2FAEmailFlow 对应 Python do_2fa_email_flow 步骤 4-7：
//
//  4. POST /identity/auth/verifyEmail（提交用户输入的验证码）
//  5. 解析 finish location
//  6. GET identity/result/check（不跟随跳转，捕获 Location）
//  7. GET Auth2/end（不跟随跳转，读取 extension-pragma → ssecurity）
//  8. GET STS URL（设置 serviceToken cookie）
//
// 复用 start2FAEmailFlow 建立的 BrowserClient 会话（cookie jar 已含 ick 等 cookie）。
func (c *EmailLoginClient2) complete2FAEmailFlow(tfaCtx *emailTFAContext2, code string) error {
	// 重新从 jar 读取 ick（Python: self._session.cookies.get("ick", "")，始终读取活跃 jar）
	ickCookie := c.browser.GetCookieAny("ick",
		"https://account.xiaomi.com",
		"https://xiaomi.com",
		"https://mi.com",
		"https://identity.xiaomi.com",
	)
	if ickCookie == "" {
		ickCookie = tfaCtx.ickCookie // 回退到 start 阶段保存的值
	}

	// 4) 提交验证码（对应 Python step 4）
	verifyParams := url.Values{}
	verifyParams.Set("_flag", "8")
	verifyParams.Set("_json", "true")
	verifyParams.Set("sid", "xiaomiio")
	verifyParams.Set("context", tfaCtx.context)
	verifyParams.Set("mask", "0")
	verifyParams.Set("_locale", "en_US")

	verifyData := url.Values{}
	verifyData.Set("_flag", "8")
	verifyData.Set("ticket", code)
	verifyData.Set("trust", "false")
	verifyData.Set("_json", "true")
	verifyData.Set("ick", ickCookie)

	verifyResp, err := c.browser.PostForm(
		"https://account.xiaomi.com/identity/auth/verifyEmail",
		verifyParams, verifyData, nil,
	)
	if err != nil {
		return fmt.Errorf("complete2FAEmailFlow verifyEmail: %w", err)
	}
	if verifyResp.StatusCode != http.StatusOK {
		return fmt.Errorf("complete2FAEmailFlow verifyEmail status=%d body=%.500s",
			verifyResp.StatusCode, verifyResp.Text())
	}

	// 检查 JSON 级别错误（如 70014 = 验证码错误）
	var finishLoc string
	if jr, jsonErr := emailToJSON2(verifyResp.Text()); jsonErr == nil {
		if errCode, _ := jr["code"].(float64); int(errCode) != 0 {
			tips, _ := jr["tips"].(string)
			desc, _ := jr["desc"].(string)
			return fmt.Errorf("complete2FAEmailFlow verifyEmail error (code=%d): %s %s",
				int(errCode), tips, desc)
		}
		finishLoc, _ = jr["location"].(string)
	}
	// fallback: Location header
	if finishLoc == "" {
		finishLoc = verifyResp.Header.Get("Location")
	}
	// fallback: regex 搜索 body
	if finishLoc == "" && len(verifyResp.Body) > 0 {
		re := regexp.MustCompile(`https://account\.xiaomi\.com/identity/result/check\?[^"']+`)
		if m := re.Find(verifyResp.Body); m != nil {
			finishLoc = string(m)
		}
	}

	// Fallback: 直接访问 result/check（对应 Python fallback 逻辑）
	if finishLoc == "" {
		checkParams := url.Values{}
		checkParams.Set("sid", "xiaomiio")
		checkParams.Set("context", tfaCtx.context)
		checkParams.Set("_locale", "en_US")
		r0, err := c.browser.GetNoRedirect("https://account.xiaomi.com/identity/result/check", checkParams, nil)
		if err != nil {
			return fmt.Errorf("complete2FAEmailFlow result/check fallback: %w", err)
		}
		if r0.StatusCode == http.StatusMovedPermanently || r0.StatusCode == http.StatusFound {
			loc := r0.Header.Get("Location")
			reqURL := ""
			if r0.Request != nil {
				reqURL = r0.Request.URL.String()
			}
			if strings.Contains(reqURL, "serviceLoginAuth2/end") {
				finishLoc = reqURL
			} else if loc != "" {
				finishLoc = loc
			}
		}
	}

	if finishLoc == "" {
		return fmt.Errorf("complete2FAEmailFlow: unable to determine finish location after verifyEmail (status=%d headers=%v body=%.800s)",
			verifyResp.StatusCode, verifyResp.Header, verifyResp.Text())
	}

	// 5) First hop: GET identity/result/check，不跟随跳转（对应 Python allow_redirects=False）
	var endURL string
	if strings.Contains(finishLoc, "identity/result/check") {
		checkResp, err := c.browser.GetNoRedirect(finishLoc, nil, nil)
		if err != nil {
			return fmt.Errorf("complete2FAEmailFlow result/check: %w", err)
		}
		endURL = checkResp.Header.Get("Location")
	} else {
		endURL = finishLoc
	}
	if endURL == "" {
		return fmt.Errorf("complete2FAEmailFlow: could not find Auth2/end URL in finish chain")
	}

	// 6) GET Auth2/end，不跟随跳转，读取 extension-pragma（含 ssecurity）
	auth2Resp, err := c.browser.GetNoRedirect(endURL, nil, nil)
	if err != nil {
		return fmt.Errorf("complete2FAEmailFlow Auth2/end: %w", err)
	}

	// 部分服务器先返回 200 Tips 页，再 302；对应 Python 的二次 GET
	if auth2Resp.StatusCode == http.StatusOK && strings.Contains(auth2Resp.Text(), "Xiaomi Account - Tips") {
		auth2Resp, err = c.browser.GetNoRedirect(endURL, nil, nil)
		if err != nil {
			return fmt.Errorf("complete2FAEmailFlow Auth2/end(second): %w", err)
		}
	}

	// 解析 extension-pragma header 获取 ssecurity（对应 Python ep_json["ssecurity"]）
	if extPrag := auth2Resp.Header.Get("extension-pragma"); extPrag != "" {
		var epJSON map[string]interface{}
		if err := json.Unmarshal([]byte(extPrag), &epJSON); err == nil {
			if ssec, ok := epJSON["ssecurity"].(string); ok && ssec != "" {
				c.ssecurity = ssec
			}
		}
	}
	if c.ssecurity == "" {
		return fmt.Errorf("complete2FAEmailFlow: extension-pragma header missing ssecurity; cannot continue")
	}

	// 7) Find STS redirect and visit it（对应 Python step 7）
	stsURL := auth2Resp.Header.Get("Location")
	if stsURL == "" && len(auth2Resp.Body) > 0 {
		idx := strings.Index(auth2Resp.Text(), "https://sts.api.io.mi.com/sts")
		if idx != -1 {
			body := auth2Resp.Text()
			end := strings.Index(body[idx:], `"`)
			if end == -1 {
				end = emailMin2(idx+300, len(body))
			} else {
				end = idx + end
			}
			stsURL = body[idx:end]
		}
	}
	if stsURL == "" {
		return fmt.Errorf("complete2FAEmailFlow: Auth2/end did not provide STS redirect")
	}

	// GET STS URL，跟随跳转（对应 Python allow_redirects=True）
	stsResp, err := c.browser.Get(stsURL, nil, nil)
	if err != nil {
		return fmt.Errorf("complete2FAEmailFlow STS: %w", err)
	}
	if stsResp.StatusCode != http.StatusOK {
		return fmt.Errorf("complete2FAEmailFlow STS: status=%d", stsResp.StatusCode)
	}

	// 从 cookie jar 中提取 serviceToken（对应 Python self._session.cookies.get("serviceToken", ...)）
	c.serviceToken = c.browser.GetCookieAny("serviceToken",
		"https://sts.api.io.mi.com",
		"https://api.io.mi.com",
	)
	if c.serviceToken == "" {
		return fmt.Errorf("complete2FAEmailFlow: could not parse serviceToken from STS cookies")
	}

	// install_service_token_cookies（对应 Python self.install_service_token_cookies(...)）
	c.installServiceTokenCookies2(c.serviceToken)

	// 更新 userId / cUserId（从 cookie jar 回填）
	if c.userID == "" {
		c.userID = c.browser.GetCookieAny("userId",
			"https://xiaomi.com", "https://sts.api.io.mi.com")
	}
	if c.cUserID == "" {
		c.cUserID = c.browser.GetCookieAny("cUserId",
			"https://xiaomi.com", "https://sts.api.io.mi.com")
	}

	// 登录成功，保存 session 到 BrowserClient（SSO 状态）
	c.browser.SaveSession(&common.SessionData{
		Cookies: map[string][]*http.Cookie{
			"https://api.io.mi.com": c.browser.Cookies("https://api.io.mi.com"),
			"https://xiaomi.com":    c.browser.Cookies("https://xiaomi.com"),
		},
		Extra: map[string]interface{}{
			"serviceToken": c.serviceToken,
			"ssecurity":    c.ssecurity,
			"userId":       c.userID,
			"cUserId":      c.cUserID,
			"passToken":    c.passToken,
		},
	})

	return nil
}

// installServiceTokenCookies2 对应 Python install_service_token_cookies
func (c *EmailLoginClient2) installServiceTokenCookies2(token string) {
	for _, domain := range []string{".api.io.mi.com", ".io.mi.com", ".mi.com"} {
		rawURL := "https://" + strings.TrimPrefix(domain, ".")
		_ = c.browser.SetCookie(rawURL, "serviceToken", token)
		_ = c.browser.SetCookie(rawURL, "yetAnotherServiceToken", token)
	}
}

// buildResult 构造 AppLoginResult（与 mail_login_client.go 相同结构）
func (c *EmailLoginClient2) buildResult() *AppLoginResult {
	return &AppLoginResult{
		UserID:       c.userID,
		CUserID:      c.cUserID,
		SSecurity:    c.ssecurity,
		PassToken:    c.passToken,
		ServiceToken: c.serviceToken,
		Location:     c.location,
	}
}

// ---------- 私有：HTTP helpers --------------------------------------------

// doPostNoRedirect 使用 BrowserClient 底层 Do 方法执行一个不跟随跳转的 POST。
// params 附加在 URL query string，data 为 form body（可为 nil）。
// 对应 Python self._session.post(url, params=fields, allow_redirects=False)。
func (c *EmailLoginClient2) doPostNoRedirect(rawURL string, params url.Values, data url.Values) (*common.BrowserResponse, error) {
	fullURL := rawURL
	if len(params) > 0 {
		fullURL = rawURL + "?" + params.Encode()
	}
	body := ""
	if len(data) > 0 {
		body = data.Encode()
	}
	req, err := http.NewRequest(http.MethodPost, fullURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.browser.Do(req, false)
}

// ---------- 私有：工具函数 -------------------------------------------------

// emailToJSON2 去除 "&&&START&&&" 前缀后解析 JSON
func emailToJSON2(text string) (map[string]interface{}, error) {
	cleaned := strings.Replace(text, "&&&START&&&", "", 1)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// emailMD5Upper2 计算 MD5 并返回大写十六进制（对应 Python hashlib.md5(...).hexdigest().upper()）
func emailMD5Upper2(s string) string {
	h := md5.New() //nolint:gosec
	h.Write([]byte(s))
	return strings.ToUpper(fmt.Sprintf("%x", h.Sum(nil)))
}

// emailGenerateAgent2 对应 Python XiaomiCloudConnector.generate_agent
func emailGenerateAgent2() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	agentID := make([]byte, 13)
	for i := range agentID {
		agentID[i] = byte('A' + rng.Intn(5))
	}
	randText := make([]byte, 18)
	for i := range randText {
		randText[i] = byte('a' + rng.Intn(26))
	}
	return fmt.Sprintf("%s-%s APP/com.xiaomi.mihome APPV/10.5.201",
		string(randText), string(agentID))
}

// emailGenerateDeviceID2 对应 Python XiaomiCloudConnector.generate_device_id
func emailGenerateDeviceID2() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	id := make([]byte, 6)
	for i := range id {
		id[i] = byte('a' + rng.Intn(26))
	}
	return string(id)
}

// emailMin2 返回两整数中的较小值
func emailMin2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
