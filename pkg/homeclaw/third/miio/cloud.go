package miio

import (
	"bytes"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Cloud struct {
	client *http.Client

	sid       string
	cookies   string // for auth
	ssecurity []byte // for encryption

	userID    string
	passToken string

	auth map[string]string
}

func NewCloud(sid string) *Cloud {
	return &Cloud{
		client: &http.Client{Timeout: 15 * time.Second},
		sid:    sid,
	}
}

func (c *Cloud) finishAuth(location string) error {
	res, err := c.client.Get(location)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// LoginWithVerify
	//   - userId, cUserId, serviceToken from cookies
	//   - passToken from redirect cookies
	//   - ssecurity from extra header
	// LoginWithToken
	//   - userId, cUserId, serviceToken from cookies
	var cUserId, serviceToken string

	for res != nil {
		for _, cookie := range res.Cookies() {
			switch cookie.Name {
			case "userId":
				c.userID = cookie.Value
			case "cUserId":
				cUserId = cookie.Value
			case "serviceToken":
				serviceToken = cookie.Value
			case "passToken":
				c.passToken = cookie.Value
			}
		}

		if s := res.Header.Get("Extension-Pragma"); s != "" {
			var v1 struct {
				Ssecurity []byte `json:"ssecurity"`
			}
			if err = json.Unmarshal([]byte(s), &v1); err != nil {
				return err
			}
			c.ssecurity = v1.Ssecurity
		}

		res = res.Request.Response
	}

	c.cookies = fmt.Sprintf("userId=%s; cUserId=%s; serviceToken=%s", c.userID, cUserId, serviceToken)

	return nil
}

func (c *Cloud) LoginWithToken(userID, passToken string) error {
	req, err := http.NewRequest("GET", "https://account.xiaomi.com/pass/serviceLogin?_json=true&sid="+c.sid, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; passToken=%s", userID, passToken))

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}

	var v1 struct {
		Ssecurity []byte `json:"ssecurity"`
		PassToken string `json:"passToken"`
		Location  string `json:"location"`
	}
	if _, err = readLoginResponse(res.Body, &v1); err != nil {
		return err
	}

	c.ssecurity = v1.Ssecurity
	c.passToken = v1.PassToken

	return c.finishAuth(v1.Location)
}

func (c *Cloud) UserToken() (string, string) {
	return c.userID, c.passToken
}

func (c *Cloud) Request(baseURL, apiURL, params string, headers map[string]string) ([]byte, error) {
	form := url.Values{"data": {params}}

	nonce := genNonce()
	signedNonce := genSignedNonce(c.ssecurity, nonce)

	// 1. gen hash for data param
	form.Set("rc4_hash__", genSignature64("POST", apiURL, form, signedNonce))

	// 2. encrypt data and hash params
	for _, v := range form {
		ciphertext, err := crypt(signedNonce, []byte(v[0]))
		if err != nil {
			return nil, err
		}
		v[0] = base64.StdEncoding.EncodeToString(ciphertext)
	}

	// 3. add signature for encrypted data and hash params
	form.Set("signature", genSignature64("POST", apiURL, form, signedNonce))

	// 4. add nonce
	form.Set("_nonce", base64.StdEncoding.EncodeToString(nonce))

	req, err := http.NewRequest("POST", baseURL+apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, err
	}

	plaintext, err := crypt(signedNonce, ciphertext)
	if err != nil {
		return nil, err
	}

	var res1 struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err = json.Unmarshal(plaintext, &res1); err != nil {
		return nil, err
	}

	if res1.Code != 0 {
		return nil, errors.New("xiaomi: " + res1.Message)
	}

	return res1.Result, nil
}

func readLoginResponse(rc io.ReadCloser, v any) ([]byte, error) {
	defer rc.Close()

	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	body, ok := bytes.CutPrefix(body, []byte("&&&START&&&"))
	if !ok {
		return nil, fmt.Errorf("xiaomi: %s", body)
	}

	return body, json.Unmarshal(body, &v)
}

func genNonce() []byte {
	ts := time.Now().Unix() / 60

	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce[:8])
	binary.BigEndian.PutUint32(nonce[8:], uint32(ts))
	return nonce
}

func genSignedNonce(ssecurity, nonce []byte) []byte {
	hasher := sha256.New()
	hasher.Write(ssecurity)
	hasher.Write(nonce)
	return hasher.Sum(nil)
}

func crypt(key, plaintext []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	tmp := make([]byte, 1024)
	cipher.XORKeyStream(tmp, tmp)

	ciphertext := make([]byte, len(plaintext))
	cipher.XORKeyStream(ciphertext, plaintext)

	return ciphertext, nil
}

func genSignature64(method, path string, values url.Values, signedNonce []byte) string {
	s := method + "&" + path + "&data=" + values.Get("data")
	if values.Has("rc4_hash__") {
		s += "&rc4_hash__=" + values.Get("rc4_hash__")
	}
	s += "&" + base64.StdEncoding.EncodeToString(signedNonce)

	hasher := sha1.New()
	hasher.Write([]byte(s))
	signature := hasher.Sum(nil)

	return base64.StdEncoding.EncodeToString(signature)
}

type Request struct {
	Method     string
	URL        string
	RawParams  string
	Body       url.Values
	Headers    url.Values
	RawCookies string
}

func (r Request) Encode() *http.Request {
	if r.RawParams != "" {
		r.URL += "?" + r.RawParams
	}

	var body io.Reader
	if r.Body != nil {
		body = strings.NewReader(r.Body.Encode())
	}

	req, err := http.NewRequest(r.Method, r.URL, body)
	if err != nil {
		return nil
	}

	if r.Headers != nil {
		req.Header = http.Header(r.Headers)
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if r.RawCookies != "" {
		req.Header.Set("Cookie", r.RawCookies)
	}

	return req
}
