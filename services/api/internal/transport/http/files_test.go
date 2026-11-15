package http_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFilesHTTPRealUploadStrictInputAndGuardedContent(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:files:http")
	}
	t.Setenv("INFRAPRO_FILES_E2E", "1")
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	_, e = env.Service.Bootstrap(ctx, "FilesOwner", "文件 HTTP 验收", password, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	cleanup := browserFiles(t, ctx, env)
	defer cleanup()
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	csrf, epoch := "", ""
	call := func(method, path string, body []byte, headers map[string]string) (int, http.Header, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(body))
		req.Header.Set("Origin", env.Config.TrustedOrigins[0])
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		if epoch != "" {
			req.Header.Set("X-Files-Epoch", epoch)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		r, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		data, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		return r.StatusCode, r.Header, data
	}
	jsonCall := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		data, _ := json.Marshal(body)
		status, _, raw := call(method, path, data, nil)
		if status != want {
			t.Fatalf("%s %s: %d %s", method, path, status, raw)
		}
		out := map[string]any{}
		if len(raw) > 0 {
			if e := json.Unmarshal(raw, &out); e != nil {
				t.Fatal(e)
			}
		}
		return out
	}
	csrf = jsonCall("GET", "/auth/csrf", nil, 200)["token"].(string)
	jsonCall("POST", "/auth/login", map[string]string{"username": "FilesOwner", "password": password}, 200)
	accounts := jsonCall("GET", "/access/accounts?q=FilesOwner", nil, 200)["items"].([]any)
	owner := accounts[0].(map[string]any)["id"].(string)
	tenant := jsonCall("POST", "/platform/tenants", map[string]string{"name": "文件传输验收", "code": "files-http", "administrator_account_id": owner, "client_request_id": uuid.NewString(), "reason": "HTTP 合成验收"}, 201)["tenant_id"].(string)
	base := "/tenants/" + tenant + "/files"
	caps := jsonCall("GET", base+"/capabilities", nil, 200)
	epoch = caps["restore_epoch"].(string)
	data := bytes.Repeat([]byte("文件内容\n"), 500000)
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	input := map[string]any{"request_id": uuid.NewString(), "reason": "真实 HTTP 文件链路", "host": caps["host"], "intent": "add", "filename": "中文资料.txt", "display_name": "HTTP 普通附件", "description": "", "size_bytes": strconv.Itoa(len(data)), "sha256": hash, "declared_media_type": "text/plain"}
	raw, _ := json.Marshal(input)
	for _, bad := range [][]byte{append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"unknown":"injected"}`)...), append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"request_id":"duplicate"}`)...), []byte(`{"host":{"kind":"managed","id":"foreign"}}`)} {
		status, _, _ := call("POST", base+"/uploads", bad, nil)
		if status != 400 {
			t.Fatal("strict body accepted", status)
		}
	}
	status, _, _ := call("POST", base+"/uploads", raw, map[string]string{"X-CSRF-Token": ""})
	if status != 403 {
		t.Fatal("CSRF bypass", status)
	}
	status, _, _ = call("POST", base+"/uploads", raw, map[string]string{"X-Files-Epoch": uuid.NewString()})
	if status != 409 {
		t.Fatal("epoch bypass", status)
	}
	upload := jsonCall("POST", base+"/uploads", input, 201)
	uid := upload["upload_id"].(string)
	replay := jsonCall("POST", base+"/uploads", input, 201)
	if replay["upload_id"] != uid || replay["replayed"] != true {
		t.Fatal("create receipt", replay)
	}
	partHeaders := map[string]string{"Content-Type": "application/octet-stream", "X-Content-SHA256": hash}
	status, _, raw = call("PUT", base+"/uploads/"+uid+"/parts/1", data, partHeaders)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	done := jsonCall("POST", base+"/uploads/"+uid+"/complete", map[string]any{"request_id": uuid.NewString(), "reason": "提交检查", "expected_revision": upload["revision"]}, 202)
	jobID := done["job_id"].(string)
	var job map[string]any
	for i := 0; i < 150; i++ {
		job = jsonCall("GET", base+"/jobs/"+jobID, nil, 200)
		if job["state"] == "succeeded" {
			break
		}
		if job["state"] == "failed" {
			t.Fatal(job)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if job["state"] != "succeeded" {
		t.Fatal(job)
	}
	aid, rid := job["result_attachment_id"].(string), job["result_revision_id"].(string)
	path := base + "/attachments/" + aid + "/revisions/" + rid + "/content"
	status, h, got := call("GET", path, nil, nil)
	if status != 200 || !bytes.Equal(data, got) {
		t.Fatal("guarded body mismatch", status)
	}
	if !strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") || !strings.Contains(h.Get("Cache-Control"), "no-store") || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("unsafe headers", h)
	}
	etag := h.Get("ETag")
	for _, tc := range []struct {
		method  string
		headers map[string]string
		status  int
		length  int
	}{{"HEAD", nil, 200, 0}, {"GET", map[string]string{"Range": "bytes=7-31"}, 206, 25}, {"GET", map[string]string{"Range": "bytes=-9"}, 206, 9}, {"GET", map[string]string{"If-None-Match": etag}, 304, 0}, {"GET", map[string]string{"If-Match": "\"wrong\""}, 412, 0}, {"GET", map[string]string{"Range": "bytes=1-3,7-9"}, 416, 0}, {"GET", map[string]string{"Range": "bytes=999999999999999999999-"}, 416, 0}, {"GET", map[string]string{"Range": "bytes=0-1", "If-Range": "\"old\""}, 200, len(data)}} {
		status, _, body := call(tc.method, path, nil, tc.headers)
		if status != tc.status || len(body) != tc.length {
			t.Fatalf("content %+v: status=%d bytes=%d", tc, status, len(body))
		}
	}
	for _, suffix := range []string{"?result_url=https://invalid", "?host_id=" + uuid.NewString(), "?limit=10000", "?cursor=forged"} {
		status, _, _ := call("GET", base+"/attachments"+suffix, nil, nil)
		if status < 400 || status >= 500 {
			t.Fatal("query bypass", suffix, status)
		}
	}
	// A client that stops reading must not retain authorization locks indefinitely.
	address, _ := url.Parse(server.URL)
	tcp, err := net.DialTCP("tcp", nil, mustTCPAddress(t, address.Host))
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	if err = tcp.SetReadBuffer(1024); err != nil {
		t.Fatal(err)
	}
	cookies := []string{}
	for _, cookie := range jar.Cookies(address) {
		cookies = append(cookies, cookie.Name+"="+cookie.Value)
	}
	if _, err = fmt.Fprintf(tcp, "GET /api/v1%s HTTP/1.1\r\nHost: %s\r\nCookie: %s\r\nConnection: close\r\n\r\n", path, address.Host, strings.Join(cookies, "; ")); err != nil {
		t.Fatal(err)
	}
	tcp.SetReadDeadline(time.Now().Add(15 * time.Second))
	reader := bufio.NewReader(tcp)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "200") {
		t.Fatal("slow read did not start", line, err)
	}
	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	began := time.Now()
	closed := false
	for time.Since(began) < 5*time.Second {
		var active int
		if err = env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_read_leases WHERE closed_at IS NULL`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active == 0 {
			closed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("Blocked downloader lease closed after %s", time.Since(began))
	if !closed {
		t.Fatal("blocked network write exceeded mandatory deadline")
	}
	tcp.Close()
	a := jsonCall("GET", base+"/attachments/"+aid, nil, 200)
	jsonCall("POST", base+"/attachments/"+aid+"/trash", map[string]any{"request_id": uuid.NewString(), "reason": "回收读取验证", "expected_revision": a["revision"]}, 200)
	for _, method := range []string{"GET", "HEAD"} {
		status, h, _ := call(method, path, nil, map[string]string{"If-None-Match": etag, "Range": "bytes=999999999-"})
		if status < 400 || status == 416 || h.Get("ETag") != "" || h.Get("Content-Range") != "" {
			t.Fatal("conditional leaked trashed body", status, h)
		}
	}
	// A session cookie is required even for a cached or unsatisfiable range request.
	client.Jar = nil
	for _, method := range []string{"GET", "HEAD"} {
		status, h, _ := call(method, path, nil, map[string]string{"If-None-Match": etag, "Range": "bytes=999999999-"})
		if status != 401 || h.Get("Content-Range") != "" || h.Get("ETag") != "" {
			t.Fatal("anonymous condition leaked", status, h)
		}
	}
}

func mustTCPAddress(t *testing.T, address string) *net.TCPAddr {
	t.Helper()
	a, e := net.ResolveTCPAddr("tcp", address)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
