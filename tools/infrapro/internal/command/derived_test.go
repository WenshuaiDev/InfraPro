package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/google/uuid"
)

func TestDerivedCLIUsesAuthenticatedProjectAPIAndOriginalPreview(t *testing.T) {
	tenant, project, channel, preview, request := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	hash := strings.Repeat("a", 64)
	var posts int
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/csrf" {
			http.SetCookie(w, &http.Cookie{Name: "csrf", Value: "bound", Path: "/"})
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "csrf-token"})
			return
		}
		cookie, e := r.Cookie("infrapro_session")
		if e != nil || cookie.Value != "test-session" || r.Header.Get("Origin") != origin || r.Header.Get("X-CSRF-Token") != "csrf-token" {
			t.Error("CLI did not preserve authenticated CSRF boundary")
			w.WriteHeader(403)
			return
		}
		if c, e := r.Cookie("csrf"); e != nil || c.Value != "bound" {
			t.Error("missing CSRF cookie")
		}
		base := "/api/v1/tenants/" + tenant + "/projects/" + project + "/monitoring/derived/channels/" + channel
		posts++
		switch r.URL.Path {
		case base + "/control-preview":
			var in dm.ControlPreviewInput
			if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
				t.Error(e)
			}
			if in.Action != "pause" || in.Revision != "7" {
				t.Error(in)
			}
			_ = json.NewEncoder(w).Encode(dm.ControlPreview{ID: preview, Hash: hash, ControlRevision: "7", Proposal: dm.ControlProposal{Action: "pause"}})
		case base + "/control":
			var in dm.ControlInput
			if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
				t.Error(e)
			}
			if in.PreviewID != preview || in.PreviewHash != hash || in.Revision != "7" || in.RequestID != request {
				t.Error("reviewed preview or idempotency key replaced", in)
			}
			w.WriteHeader(403)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "permission_denied", "message": "项目输入权限已经撤销"})
		default:
			t.Error("wrong API scope", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	origin = server.URL
	path := filepath.Join(t.TempDir(), "session.json")
	body, _ := json.Marshal(derivedSession{Origin: origin, Cookies: []*http.Cookie{{Name: "infrapro_session", Value: "test-session", Path: "/"}}})
	if e := os.WriteFile(path, body, 0600); e != nil {
		t.Fatal(e)
	}
	var out, stderr bytes.Buffer
	a := New(nil, &out, &stderr)
	args := []string{"monitoring", "derived", "pause", "--tenant", tenant, "--project", project, "--channel", channel, "--expected-revision", "7", "--session-file", path}
	if e := a.Run(context.Background(), args); e != nil {
		t.Fatal(e)
	}
	if posts != 1 || !strings.Contains(out.String(), preview) {
		t.Fatal("preview did not remain reviewable", out.String())
	}
	args = append(args, "--apply", "--preview-id", preview, "--preview-hash", hash, "--request-id", request, "--reason", "按原预览暂停")
	e := a.Run(context.Background(), args)
	if e == nil || !strings.Contains(e.Error(), "permission_denied") || posts != 2 {
		t.Fatal("CLI bypassed current project denial", e, posts)
	}
	if strings.Contains(out.String()+stderr.String(), "test-session") || strings.Contains(out.String()+stderr.String(), "csrf-token") {
		t.Fatal("credential leaked to command output")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = loadDerivedClient(path, origin); e == nil {
		t.Fatal("publicly readable session accepted")
	}
}

func TestDerivedCLIRejectsCredentialForwardingAndTruncatedInput(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?x=y"} {
		if _, e := newDerivedClient(origin); e == nil {
			t.Fatal("unsafe origin", origin)
		}
	}
	forwarded := false
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded = true }))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c, e := newDerivedClient(redirect.URL)
	if e != nil {
		t.Fatal(e)
	}
	e = c.request(context.Background(), "GET", "/api/v1/test", nil, nil)
	if e == nil || forwarded {
		t.Fatal("redirect followed", e)
	}
	path := filepath.Join(t.TempDir(), "input.json")
	if e = os.WriteFile(path, []byte(`{"expected_revision":"1"} {"expected_revision":"2"}`), 0600); e != nil {
		t.Fatal(e)
	}
	var input dm.BackfillPreviewInput
	if e = decodeDerivedInput(path, &input); e == nil {
		t.Fatal("trailing request ignored")
	}
	var out bytes.Buffer
	if e = New(nil, &out, &out).Run(context.Background(), []string{"monitoring", "derived", "login", "--url", redirect.URL, "--username", "test", "--session-file", filepath.Join(t.TempDir(), "session.json")}); e == nil {
		t.Fatal("login accepted a nonterminal password path")
	}
}
