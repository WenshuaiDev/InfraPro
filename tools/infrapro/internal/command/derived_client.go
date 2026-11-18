package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

type derivedSession struct {
	Origin  string         `json:"origin"`
	Cookies []*http.Cookie `json:"cookies"`
}
type derivedClient struct {
	origin *url.URL
	client *http.Client
}

func newDerivedClient(origin string) (*derivedClient, error) {
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("--url 必须是控制台 origin，例如 https://console.example.com")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !(u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback())) {
		return nil, fmt.Errorf("非本机连接必须使用 HTTPS")
	}
	u.Path = ""
	jar, _ := cookiejar.New(nil)
	return &derivedClient{u, &http.Client{Jar: jar, Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func readPrivateFile(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("会话文件需要绝对路径")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, fmt.Errorf("会话文件不可读")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, fmt.Errorf("会话文件须为权限 0600 的普通小文件")
	}
	return io.ReadAll(io.LimitReader(f, 65537))
}
func loadDerivedClient(path, origin string) (*derivedClient, error) {
	body, e := readPrivateFile(path)
	if e != nil {
		return nil, e
	}
	var session derivedSession
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&session); e != nil {
		return nil, fmt.Errorf("会话文件格式无效")
	}
	c, e := newDerivedClient(session.Origin)
	if e != nil {
		return nil, e
	}
	if origin != "" && strings.TrimRight(origin, "/") != c.origin.String() {
		return nil, fmt.Errorf("会话 origin 与 --url 不一致")
	}
	found := false
	for _, cookie := range session.Cookies {
		if cookie != nil && cookie.Name == "infrapro_session" && cookie.Value != "" {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("会话文件没有登录凭据")
	}
	c.client.Jar.SetCookies(c.origin, session.Cookies)
	return c, nil
}
func (c *derivedClient) request(ctx context.Context, method, path string, body any, out any) error {
	if !strings.HasPrefix(path, "/api/v1/") {
		return fmt.Errorf("invalid API path")
	}
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, c.origin.String()+path, reader)
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/json")
	if method != "GET" {
		var csrf struct {
			Token string `json:"token"`
		}
		if e = c.request(ctx, "GET", "/api/v1/auth/csrf", nil, &csrf); e != nil {
			return e
		}
		if csrf.Token == "" {
			return fmt.Errorf("CSRF 会话不可用")
		}
		req.Header.Set("Origin", c.origin.String())
		req.Header.Set("X-CSRF-Token", csrf.Token)
		req.Header.Set("Content-Type", "application/json")
	}
	response, e := c.client.Do(req)
	if e != nil {
		return fmt.Errorf("API 请求未确认完成；使用同一 request_id 核对后重试")
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if e != nil || len(raw) > 8<<20 {
		return fmt.Errorf("API 响应不可完整读取")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		if failure.Code == "" {
			failure.Code = failure.Error.Code
		}
		if failure.Message == "" {
			failure.Message = failure.Error.Message
		}
		return fmt.Errorf("API %d %s：%s", response.StatusCode, failure.Code, failure.Message)
	}
	if out != nil && len(raw) > 0 {
		if e = json.Unmarshal(raw, out); e != nil {
			return fmt.Errorf("API 响应格式无效")
		}
	}
	return nil
}
func (a *App) derivedLogin(ctx context.Context, origin, username, path string) error {
	if username == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("login 需要 --username 和绝对路径 --session-file")
	}
	if a.in == nil || !term.IsTerminal(int(a.in.Fd())) {
		return fmt.Errorf("登录密码仅通过交互终端输入")
	}
	c, e := newDerivedClient(origin)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(path); !os.IsNotExist(e) {
		return fmt.Errorf("会话文件必须是尚不存在的新文件")
	}
	fmt.Fprint(a.errOut, "登录密码：")
	password, e := term.ReadPassword(int(a.in.Fd()))
	fmt.Fprintln(a.errOut)
	if e != nil {
		return e
	}
	var result struct {
		Status      string `json:"status"`
		ChallengeID string `json:"challenge_id"`
	}
	e = c.request(ctx, "POST", "/api/v1/auth/login", map[string]string{"username": username, "password": string(password)}, &result)
	for i := range password {
		password[i] = 0
	}
	if e != nil {
		return e
	}
	if result.Status == "mfa_required" {
		fmt.Fprint(a.errOut, "认证方式（1：动态码，2：恢复码）：")
		methodInput, e := term.ReadPassword(int(a.in.Fd()))
		fmt.Fprintln(a.errOut)
		if e != nil {
			return e
		}
		method := "totp"
		choice := strings.TrimSpace(string(methodInput))
		clear(methodInput)
		if choice == "2" {
			method = "recovery_code"
		} else if choice != "1" {
			return fmt.Errorf("请选择 1 或 2，未创建会话文件")
		}
		fmt.Fprint(a.errOut, "验证码（不回显）：")
		code, e := term.ReadPassword(int(a.in.Fd()))
		fmt.Fprintln(a.errOut)
		if e != nil {
			return e
		}
		e = c.request(ctx, "POST", "/api/v1/auth/mfa/verify", map[string]string{"challenge_id": result.ChallengeID, "method": method, "code": string(code)}, &result)
		clear(code)
		if e != nil {
			return e
		}
	}
	if result.Status != "authenticated" {
		_ = c.request(ctx, "POST", "/api/v1/auth/logout", map[string]string{}, nil)
		return fmt.Errorf("需要在控制台完成 MFA 绑定或恢复后重新登录；未创建会话文件")
	}
	session := derivedSession{Origin: c.origin.String()}
	found := false
	for _, cookie := range c.client.Jar.Cookies(c.origin) {
		if cookie.Name == "infrapro_session" && cookie.Value != "" {
			found = true
			session.Cookies = append(session.Cookies, cookie)
		} else if cookie.Name == "infrapro_csrf" {
			session.Cookies = append(session.Cookies, cookie)
		}
	}
	if !found {
		return fmt.Errorf("服务未交付正式会话；未创建会话文件")
	}

	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return fmt.Errorf("无法创建会话文件")
	}
	e = json.NewEncoder(f).Encode(session)
	if e == nil {
		e = f.Sync()
	}
	closeError := f.Close()
	if e == nil {
		e = closeError
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(a.out).Encode(map[string]string{"state": "authenticated", "session_file": path, "origin": session.Origin})
}
