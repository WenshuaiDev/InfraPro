package access

import (
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"testing"
	"time"
)

func manager(t *testing.T) (*Manager, *time.Time) {
	t.Helper()
	m := New()
	now := time.Now()
	m.now = func() time.Time { return now }
	hash, e := settings.PasswordHash("diagnostic-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Reload(settings.Auth{SchemaVersion: 1, Revision: "1", Users: []settings.User{{ID: "reader", Username: "reader", PasswordHash: hash, Enabled: true, Permissions: []string{"diagnostics.read"}}}}, "digest-one"); e != nil {
		t.Fatal(e)
	}
	return m, &now
}
func login(t *testing.T, m *Manager) (string, SessionView) {
	t.Helper()
	nonce, csrf, _, _, e := m.Bootstrap("127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	token, v, e := m.Login("127.0.0.1", nonce, csrf, "reader", "diagnostic-test-password")
	if e != nil {
		t.Fatal(e)
	}
	return token, v
}
func TestPollingDoesNotExtendIdleAndActivityDoesNotExtendAbsolute(t *testing.T) {
	m, now := manager(t)
	token, _ := login(t, m)
	for i := 0; i < 29; i++ {
		*now = now.Add(time.Minute)
		if _, _, e := m.Current(token); e != nil {
			t.Fatal(e)
		}
	}
	*now = now.Add(time.Minute)
	if _, _, e := m.Current(token); e == nil {
		t.Fatal("polling extended session")
	}
	token, v := login(t, m)
	for i := 0; i < 479; i++ {
		*now = now.Add(time.Minute)
		if _, e := m.Activity(token, v.CSRFToken); e != nil {
			t.Fatal(e)
		}
	}
	*now = now.Add(time.Minute)
	if _, e := m.Activity(token, v.CSRFToken); e == nil {
		t.Fatal("activity extended absolute lifetime")
	}
}
func TestRevocationBeforeFirstByteAndInvalidAuthFailClosed(t *testing.T) {
	m, _ := manager(t)
	token, _ := login(t, m)
	_, grant, e := m.Current(token)
	if e != nil {
		t.Fatal(e)
	}
	auth := m.auth
	auth.Revision = "2"
	auth.Users[0].Enabled = false
	if e = m.Reload(auth, "new-digest"); e != nil {
		t.Fatal(e)
	}
	written := false
	if e = m.Guard(grant, func() { written = true }); e == nil || written {
		t.Fatal("revoked query wrote data")
	}
	m.Invalidate()
	if _, _, e = m.Current(token); settings.Code(e) != "auth_configuration_unavailable" {
		t.Fatal(e)
	}
}
func TestPreauthCSRFAndBoundedTables(t *testing.T) {
	m, now := manager(t)
	nonce, _, _, _, _ := m.Bootstrap("ip")
	if _, _, e := m.Login("ip", nonce, "wrong", "reader", "diagnostic-test-password"); settings.Code(e) != "csrf_invalid" {
		t.Fatal(e)
	}
	m.mu.Lock()
	for i := 0; i < 2000; i++ {
		m.allow(settings.NewID("random"), 1, *now)
	}
	if len(m.windows) > 1024 {
		t.Fatal("unbounded limiter")
	}
	*now = now.Add(time.Minute)
	if !m.allow("after-expiry", 1, *now) {
		t.Fatal("limiter did not expire")
	}
	m.mu.Unlock()
}
