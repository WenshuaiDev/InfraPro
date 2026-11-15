// Package access is an independent, local, read-only personnel identity domain.
// Neither business cookies nor observer credentials are accepted here.
package access

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

type UserView struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Permissions []string `json:"permissions"`
}
type SessionView struct {
	User          UserView  `json:"user"`
	CSRFToken     string    `json:"csrf_token"`
	ExpiresAt     time.Time `json:"expires_at"`
	IdleExpiresAt time.Time `json:"idle_expires_at"`
}
type Grant struct {
	Hash     string
	Revision string
	User     UserView
}
type session struct {
	User     UserView
	Created  time.Time
	Activity time.Time
	Revision string
}
type preauth struct {
	CSRFHash string
	Expires  time.Time
}
type window struct {
	At    time.Time
	Count int
}
type queryBucket struct {
	at     time.Time
	tokens float64
}
type Manager struct {
	mu         sync.RWMutex
	auth       settings.Auth
	digest     string
	valid      bool
	sessions   map[string]session
	preauth    map[string]preauth
	windows    map[string]window
	queries    map[string]queryBucket
	verifying  chan struct{}
	dummy      string
	csrfSecret string
	now        func() time.Time
}

func New() *Manager {
	dummy, _ := settings.PasswordHash("diagnostic-dummy-verification-only")
	return &Manager{sessions: map[string]session{}, preauth: map[string]preauth{}, windows: map[string]window{}, verifying: make(chan struct{}, 2), dummy: dummy, csrfSecret: settings.NewSecret(), now: time.Now}
}
func (m *Manager) Reload(auth settings.Auth, digest string) error {
	valid := auth.Validate() == nil
	m.mu.Lock()
	defer m.mu.Unlock()
	if !valid || m.auth.Revision != "" && (settings.Number(auth.Revision) < settings.Number(m.auth.Revision) || auth.Revision == m.auth.Revision && m.digest != "" && digest != m.digest) {
		m.valid = false
		m.sessions = map[string]session{}
		m.preauth = map[string]preauth{}
		return settings.Error("auth_configuration_unavailable")
	}
	if !m.valid || auth.Revision != m.auth.Revision || digest != m.digest {
		m.sessions = map[string]session{}
		m.preauth = map[string]preauth{}
	}
	m.auth = auth
	m.digest = digest
	m.valid = true
	return nil
}
func (m *Manager) Invalidate() {
	m.mu.Lock()
	m.valid = false
	m.sessions = map[string]session{}
	m.preauth = map[string]preauth{}
	m.mu.Unlock()
}
func (m *Manager) State() (initialized, allowed bool, revision string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.auth.Users) > 0, m.valid && len(m.auth.Users) > 0, m.auth.Revision
}
func (m *Manager) allow(key string, max int, now time.Time) bool {
	for k, v := range m.windows {
		if now.Sub(v.At) >= time.Minute {
			delete(m.windows, k)
		}
	}
	v, exists := m.windows[key]
	if !exists {
		if len(m.windows) >= 1024 {
			return false
		}
		v.At = now
	}
	if v.Count >= max {
		return false
	}
	v.Count++
	m.windows[key] = v
	return true
}
func (m *Manager) csrf(token string) string {
	h := hmac.New(sha256.New, []byte(m.csrfSecret))
	h.Write([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (m *Manager) Bootstrap(ip string) (nonce, csrf string, initialized, allowed bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if !m.allow("bootstrap:global", 120, now) || !m.allow("bootstrap:ip:"+settings.Digest([]byte(ip)), 30, now) {
		err = settings.Error("rate_limited")
		return
	}
	for key, v := range m.preauth {
		if !v.Expires.After(now) {
			delete(m.preauth, key)
		}
	}
	if len(m.preauth) >= 1024 {
		err = settings.Error("rate_limited")
		return
	}
	nonce = settings.NewSecret()
	csrf = m.csrf(nonce)
	m.preauth[settings.Digest([]byte(nonce))] = preauth{settings.Digest([]byte(csrf)), now.Add(10 * time.Minute)}
	initialized = len(m.auth.Users) > 0
	allowed = m.valid && initialized
	return
}
func (m *Manager) Login(ip, nonce, csrf, username, password string) (string, SessionView, error) {
	var empty SessionView
	m.mu.Lock()
	now := m.now()
	if !m.valid {
		m.mu.Unlock()
		return "", empty, settings.Error("auth_configuration_unavailable")
	}
	pre, ok := m.preauth[settings.Digest([]byte(nonce))]
	if !ok || !pre.Expires.After(now) || subtle.ConstantTimeCompare([]byte(pre.CSRFHash), []byte(settings.Digest([]byte(csrf)))) != 1 {
		m.mu.Unlock()
		return "", empty, settings.Error("csrf_invalid")
	}
	username = strings.ToLower(username)
	if !m.allow("login:global", 30, now) || !m.allow("login:ip:"+settings.Digest([]byte(ip)), 5, now) || !m.allow("login:user:"+settings.Digest([]byte(username)), 5, now) {
		m.mu.Unlock()
		return "", empty, settings.Error("rate_limited")
	}
	select {
	case m.verifying <- struct{}{}:
	default:
		m.mu.Unlock()
		return "", empty, settings.Error("rate_limited")
	}
	revision, digest := m.auth.Revision, m.digest
	user := settings.User{}
	hash := m.dummy
	for _, u := range m.auth.Users {
		if u.Username == username {
			user = u
			hash = u.PasswordHash
			break
		}
	}
	m.mu.Unlock()
	verified := settings.VerifyPassword(password, hash)
	<-m.verifying
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.valid || m.auth.Revision != revision || m.digest != digest {
		return "", empty, settings.Error("auth_configuration_unavailable")
	}
	if !verified || !user.Enabled || user.ID == "" {
		return "", empty, settings.Error("authentication_failed")
	}
	now = m.now()
	for key, s := range m.sessions {
		if !validSession(s, now) {
			delete(m.sessions, key)
		}
	}
	count := 0
	oldestKey := ""
	var oldest time.Time
	for key, s := range m.sessions {
		if s.User.ID == user.ID {
			count++
			if oldest.IsZero() || s.Created.Before(oldest) {
				oldest = s.Created
				oldestKey = key
			}
		}
	}
	if count >= 4 {
		delete(m.sessions, oldestKey)
	}
	if len(m.sessions) >= 64 {
		return "", empty, settings.Error("rate_limited")
	}
	delete(m.preauth, settings.Digest([]byte(nonce)))
	token := settings.NewSecret()
	s := session{User: UserView{user.ID, user.Username, []string{"diagnostics.read"}}, Created: now, Activity: now, Revision: revision}
	m.sessions[settings.Digest([]byte(token))] = s
	return token, m.view(s, token), nil
}
func validSession(s session, now time.Time) bool {
	return now.Sub(s.Created) < 8*time.Hour && now.Sub(s.Activity) < 30*time.Minute && !now.Before(s.Created)
}
func (m *Manager) view(s session, token string) SessionView {
	return SessionView{s.User, m.csrf(token), s.Created.Add(8 * time.Hour), minTime(s.Activity.Add(30*time.Minute), s.Created.Add(8*time.Hour))}
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (m *Manager) grant(token string) (session, Grant, error) {
	if !m.valid {
		return session{}, Grant{}, settings.Error("auth_configuration_unavailable")
	}
	hash := settings.Digest([]byte(token))
	s, ok := m.sessions[hash]
	if !ok || s.Revision != m.auth.Revision || !validSession(s, m.now()) {
		return s, Grant{}, settings.Error("authentication_required")
	}
	return s, Grant{hash, m.auth.Revision, s.User}, nil
}
func (m *Manager) Current(token string) (SessionView, Grant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, g, e := m.grant(token)
	if e != nil {
		return SessionView{}, g, e
	}
	return m.view(s, token), g, nil
}
func (m *Manager) Activity(token, csrf string) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, g, e := m.grant(token)
	if e != nil {
		return time.Time{}, e
	}
	if subtle.ConstantTimeCompare([]byte(m.csrf(token)), []byte(csrf)) != 1 {
		return time.Time{}, settings.Error("csrf_invalid")
	}
	if m.now().Sub(s.Activity) >= time.Minute {
		s.Activity = m.now()
		m.sessions[g.Hash] = s
	}
	return minTime(s.Activity.Add(30*time.Minute), s.Created.Add(8*time.Hour)), nil
}
func (m *Manager) Logout(token, csrf string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, _, err := m.grant(token); err != nil {
		delete(m.sessions, settings.Digest([]byte(token)))
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(m.csrf(token)), []byte(csrf)) != 1 {
		return settings.Error("csrf_invalid")
	}
	delete(m.sessions, settings.Digest([]byte(token)))
	return nil
}

// Guard rechecks authorization at the actual response boundary. Revocation
// cannot race with a response that has been computed but not begun writing.
func (m *Manager) Guard(g Grant, write func()) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[g.Hash]
	if !m.valid {
		return settings.Error("auth_configuration_unavailable")
	}
	if !ok || g.Revision != m.auth.Revision || !validSession(s, m.now()) {
		return settings.Error("authentication_required")
	}
	write()
	return nil
}
func (m *Manager) QueryAllowed(g Grant) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if m.queries == nil {
		m.queries = map[string]queryBucket{}
	}
	for id, b := range m.queries {
		if now.Sub(b.at) >= time.Minute {
			delete(m.queries, id)
		}
	}
	b, exists := m.queries[g.User.ID]
	if !exists {
		if len(m.queries) >= 32 {
			return false
		}
		b = queryBucket{now, 30}
	}
	b.tokens = min(30, b.tokens+max(0, now.Sub(b.at).Seconds())*2)
	b.at = now
	if b.tokens < 1 {
		m.queries[g.User.ID] = b
		return false
	}
	if !m.allow("query:"+g.User.ID, 120, now) {
		m.queries[g.User.ID] = b
		return false
	}
	b.tokens--
	m.queries[g.User.ID] = b
	return true
}
