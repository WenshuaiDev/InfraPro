package parsing

import (
	"bytes"
	"context"
	"errors"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type serviceError struct {
	code  string
	retry bool
}

func (e *serviceError) Error() string { return e.code }
func (s *Service) call(ctx context.Context, path string, in any) ([]byte, error) {
	if !s.options.Enabled {
		return nil, &serviceError{"execution_disabled", false}
	}
	u, e := url.Parse(s.options.URL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, &serviceError{"service_configuration_invalid", false}
	}
	ip := net.ParseIP(u.Hostname())
	// Plain HTTP is restricted to loopback or the same-host container service.
	// Private address space alone does not establish a same-host trust boundary.
	if u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost" && u.Hostname() != "parser" && (ip == nil || !ip.IsLoopback())) {
		return nil, &serviceError{"service_tls_required", false}
	}
	token, e := securefile.Read(s.options.TokenFile, 4096, securefile.Private())
	if e != nil || len(bytes.TrimSpace(token)) < 32 {
		return nil, &serviceError{"service_auth_failed", false}
	}
	method := "GET"
	var body io.Reader
	if in != nil {
		method = "POST"
		body = bytes.NewReader(k.JSON(in))
	}
	timeout := s.options.Timeout
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(s.options.URL, "/")+path, body)
	if e != nil {
		return nil, &serviceError{"service_configuration_invalid", false}
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/json")
	if ctx.Value(productionKey{}) == true {
		req.Header.Set("X-Infrapro-Workload", "ingestion")
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, &serviceError{"service_unavailable", true}
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, c.MaxResponse+1))
	if e != nil {
		return nil, &serviceError{"service_unavailable", true}
	}
	if len(b) > c.MaxResponse {
		return nil, &serviceError{"limit_exceeded", false}
	}
	if resp.StatusCode != 200 {
		code := "parser_error"
		switch resp.StatusCode {
		case 400:
			code = "invalid_request"
		case 401, 403:
			code = "service_auth_failed"
		case 404:
			code = "method_unavailable"
		case 409:
			code = "version_conflict"
		case 413, 422:
			code = "limit_exceeded"
		case 429, 503, 504:
			code = "service_unavailable"
		}
		return b, &serviceError{code, resp.StatusCode == 429 || resp.StatusCode == 503 || resp.StatusCode == 504}
	}
	return b, nil
}
func codeOf(e error) string {
	if e == nil {
		return ""
	}
	var remote *serviceError
	if errors.As(e, &remote) {
		return remote.code
	}
	var problem *k.Error
	if errors.As(e, &problem) {
		return problem.Code
	}
	return "invalid_response"
}
