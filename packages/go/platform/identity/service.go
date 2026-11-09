package identity

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Service struct {
	access         *access.Service
	repo           *Repository
	cache          *Cache
	cfg            config.Config
	logger         *slog.Logger
	dummy          string
	passwordSlots  chan struct{}
	activityFailed atomic.Bool
}

func NewService(pool *pgxpool.Pool, client *redis.Client, cfg config.Config, logger *slog.Logger) (*Service, error) {
	dummy, e := HashPassword("InfraPro internal timing placeholder 492")
	if e != nil {
		return nil, e
	}
	return &Service{access: access.New(pool, logger), repo: NewRepository(pool), cache: NewCache(client, cfg.RedisKeyPrefix, logger), cfg: cfg, logger: logger, dummy: dummy, passwordSlots: make(chan struct{}, 4)}, nil
}
func (s *Service) Ready(ctx context.Context) error {
	if e := s.repo.pool.Ping(ctx); e != nil {
		return e
	}
	if e := migrations.Check(ctx, s.repo.pool); e != nil {
		return e
	}
	if e := s.access.Check(ctx); e != nil {
		return e
	}
	return s.cache.Ping(ctx)
}
func (s *Service) passwordWork(ctx context.Context, fn func() error) error {
	select {
	case s.passwordSlots <- struct{}{}:
		defer func() { <-s.passwordSlots }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) hash(ctx context.Context, password string) (string, error) {
	var hash string
	e := s.passwordWork(ctx, func() error { var err error; hash, err = HashPassword(password); return err })
	return hash, e
}
func (s *Service) Bootstrap(ctx context.Context, username, display, password string, m Meta) (Account, error) {
	name, e := validName(username, display)
	if e != nil {
		return Account{}, e
	}
	hash, e := s.hash(ctx, password)
	if e != nil {
		return Account{}, e
	}
	return s.repo.Bootstrap(ctx, username, name, hash, m)
}
func (s *Service) Initialized(ctx context.Context) (bool, error) { return s.repo.Initialized(ctx) }
func (s *Service) SecurityEvents(ctx context.Context, p Principal, cursor string) (Page[SecurityEvent], error) {
	var v Page[SecurityEvent]
	e := s.readAuthorized(ctx, p, access.OpSecurityEventRead, func(tx pgx.Tx) error { var e error; v, e = s.repo.Events(ctx, tx, p.Account.ID, cursor); return e })
	return v, e
}
func (s *Service) Invitations(ctx context.Context, p Principal, cursor string) (Page[Invitation], error) {
	var v Page[Invitation]
	e := s.readAuthorized(ctx, p, access.OpInvitationRead, func(tx pgx.Tx) error { var e error; v, e = s.repo.Invitations(ctx, tx, p.Account.ID, cursor); return e })
	return v, e
}
func (s *Service) Rate(ctx context.Context, scope, ip, key string) error {
	if e := s.cache.Rate(ctx, scope+":ip", ip, s.cfg.IPRequestLimit, time.Minute); e != nil {
		return e
	}
	if key != "" {
		return s.cache.Rate(ctx, scope+":credential", key, s.cfg.CredentialRequestLimit, time.Minute)
	}
	return nil
}
func (s *Service) Login(ctx context.Context, username, password string, m Meta) (Account, string, time.Time, error) {
	if e := s.Rate(ctx, "login", m.IP, strings.ToLower(username)); e != nil {
		return Account{}, "", time.Time{}, e
	}
	if len(username) > 128 {
		return Account{}, "", time.Time{}, ErrCredentials
	}
	token, e := newToken()
	if e != nil {
		return Account{}, "", time.Time{}, e
	}
	var p Principal
	e = s.passwordWork(ctx, func() error {
		var err error
		p, err = s.repo.Login(ctx, username, password, s.dummy, digest(token), s.cfg.SessionAbsolute, s.cfg.PasswordFailures, m)
		return err
	})
	if e != nil {
		return Account{}, "", time.Time{}, e
	}
	if e = s.cache.Put(ctx, p, s.cacheTTL(p)); e != nil {
		// Independent bounded context persists revocation even if the client disconnected during cache write.
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.repo.RevokeToken(clean, p.TokenHash, m); err != nil {
			s.logger.Error("failed to revoke undispatched session")
		}
		return Account{}, "", time.Time{}, e
	}
	return p.Account, token, p.Session.ExpiresAt, nil
}
func (s *Service) cacheTTL(p Principal) time.Duration {
	return min(s.cfg.SessionCacheTTL, time.Until(p.Session.LastActivityAt.Add(s.cfg.SessionIdle)), time.Until(p.Session.ExpiresAt))
}
func (s *Service) effectiveActivity(ctx context.Context, p *Principal) error {
	a, e := s.cache.Activity(ctx, p.Session.ID)
	if e != nil {
		return e
	}
	if a.Version == p.Version && a.At.After(p.Session.LastActivityAt) && !a.At.After(time.Now()) {
		p.Session.LastActivityAt = a.At
	}
	return nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if !validToken(token) {
		return Principal{}, ErrUnauthorized
	}
	hash := digest(token)
	cached, e := s.cache.Get(ctx, hash)
	if e != nil {
		return Principal{}, e
	}
	var p Principal
	if cached == nil {
		p, e = s.repo.LoadSession(ctx, hash)
	} else {
		p = *cached
		e = s.repo.CheckSession(ctx, &p)
	}
	if e != nil {
		if errors.Is(e, ErrUnauthorized) && cached != nil {
			if err := s.cache.Delete(ctx, p.Session.ID, hash); err != nil {
				return Principal{}, err
			}
		}
		return Principal{}, e
	}
	if e = s.effectiveActivity(ctx, &p); e != nil {
		return Principal{}, e
	}
	if !p.Session.LastActivityAt.Add(s.cfg.SessionIdle).After(time.Now()) || !p.Session.ExpiresAt.After(time.Now()) {
		return Principal{}, ErrUnauthorized
	}
	if cached == nil {
		if e = s.cache.Put(ctx, p, s.cacheTTL(p)); e != nil {
			return Principal{}, e
		}
	}
	return p, nil
}
func (s *Service) Me(ctx context.Context, p Principal) (Account, error) {
	return s.repo.Account(ctx, p.Account.ID)
}
func (s *Service) Logout(ctx context.Context, token string, m Meta) error {
	if !validToken(token) {
		return nil
	}
	if e := s.repo.RevokeToken(ctx, digest(token), m); e != nil {
		return e
	}
	return s.DrainInvalidations(ctx)
}
func (s *Service) Revoke(ctx context.Context, p Principal, id string, m Meta) error {
	if id != "" && !uuidPattern.MatchString(id) {
		return ErrNotFound
	}
	if e := s.repo.Revoke(ctx, p, id, s.cfg.SessionIdle, m); e != nil {
		return e
	}
	return s.DrainInvalidations(ctx)
}
func (s *Service) Reauthenticate(ctx context.Context, p Principal, password string, m Meta) error {
	if e := s.Rate(ctx, "password", m.IP, p.Account.ID); e != nil {
		return e
	}
	return s.passwordWork(ctx, func() error {
		return s.repo.ConfirmPassword(ctx, p, password, "", s.cfg.SessionIdle, s.cfg.PasswordFailures, m)
	})
}
func (s *Service) ChangePassword(ctx context.Context, p Principal, current, newPassword string, m Meta) error {
	if current == newPassword {
		return FieldError("new_password", "新密码不能与当前密码相同。")
	}
	if e := s.Rate(ctx, "password", m.IP, p.Account.ID); e != nil {
		return e
	}
	hash, e := s.hash(ctx, newPassword)
	if e != nil {
		return e
	}
	e = s.passwordWork(ctx, func() error {
		return s.repo.ConfirmPassword(ctx, p, current, hash, s.cfg.SessionIdle, s.cfg.PasswordFailures, m)
	})
	if e != nil {
		return e
	}
	return s.DrainInvalidations(ctx)
}
func (s *Service) Profile(ctx context.Context, p Principal, display string, m Meta) (Account, error) {
	name, e := validateDisplay(display)
	if e != nil {
		return Account{}, e
	}
	return s.repo.Profile(ctx, p, name, s.cfg.SessionIdle, m)
}
func (s *Service) Sessions(ctx context.Context, p Principal) (Page[Session], error) {
	page := Page[Session]{Items: []Session{}}
	var sessions []Principal
	e := s.readAuthorized(ctx, p, access.OpSessionRead, func(tx pgx.Tx) error { var e error; sessions, e = s.repo.Sessions(ctx, tx, p.Account.ID); return e })
	if e != nil {
		return page, e
	}
	for _, v := range sessions {
		if e = s.effectiveActivity(ctx, &v); e != nil {
			return page, e
		}
		if v.Session.LastActivityAt.Add(s.cfg.SessionIdle).After(time.Now()) {
			v.Session.Current = v.Session.ID == p.Session.ID
			page.Items = append(page.Items, v.Session)
		}
	}
	return page, nil
}
func (s *Service) Activity(ctx context.Context, p Principal) error {
	if s.activityFailed.Load() {
		return ErrUnavailable
	}
	if e := s.cache.Rate(ctx, "activity", p.Session.ID, 6, time.Minute); e != nil {
		return e
	}
	now := time.Now()
	return s.cache.RecordActivity(ctx, p, now, min(s.cfg.SessionIdle, time.Until(p.Session.ExpiresAt)))
}
func (s *Service) CreateInvitation(ctx context.Context, p Principal, oldID, note string, m Meta) (Link, error) {
	if oldID != "" && !uuidPattern.MatchString(oldID) {
		return Link{}, ErrNotFound
	}
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > 200 {
		return Link{}, FieldError("note", "备注最多 200 个字符。")
	}
	if e := s.cache.Rate(ctx, "invitation-create", p.Account.ID, s.cfg.InvitationRate, time.Minute); e != nil {
		return Link{}, e
	}
	token, e := newToken()
	if e != nil {
		return Link{}, e
	}
	expiry, e := s.repo.CreateInvitation(ctx, p, oldID, note, digest(token), s.cfg.InvitationTTL, s.cfg.SessionIdle, s.cfg.InvitationLimit, m)
	return Link{URL: strings.TrimRight(s.cfg.PublicConsoleURL, "/") + "/invite#token=" + token, ExpiresAt: expiry}, e
}
func (s *Service) RevokeInvitation(ctx context.Context, p Principal, id string, m Meta) error {
	if !uuidPattern.MatchString(id) {
		return ErrNotFound
	}
	return s.repo.RevokeInvitation(ctx, p, id, s.cfg.SessionIdle, m)
}
func (s *Service) Inspect(ctx context.Context, token string, reset bool, m Meta) (Inspection, error) {
	scope := "invitation"
	if reset {
		scope = "reset"
	}
	if e := s.Rate(ctx, scope, m.IP, token); e != nil {
		return Inspection{}, e
	}
	if !validToken(token) {
		return Inspection{}, ErrLink
	}
	if reset {
		return s.repo.InspectReset(ctx, digest(token))
	}
	return s.repo.InspectInvitation(ctx, digest(token))
}
func (s *Service) AcceptInvitation(ctx context.Context, token, username, display, password string, m Meta) error {
	if _, e := s.Inspect(ctx, token, false, m); e != nil {
		return e
	}
	name, e := validName(username, display)
	if e != nil {
		return e
	}
	hash, e := s.hash(ctx, password)
	if e != nil {
		return e
	}
	return s.repo.AcceptInvitation(ctx, digest(token), username, name, hash, m)
}
func (s *Service) CompleteReset(ctx context.Context, token, password string, m Meta) error {
	if _, e := s.Inspect(ctx, token, true, m); e != nil {
		return e
	}
	hash, e := s.hash(ctx, password)
	if e != nil {
		return e
	}
	if e = s.repo.CompleteReset(ctx, digest(token), hash, m); e != nil {
		return e
	}
	return s.DrainInvalidations(ctx)
}
func reason(m Meta) error {
	if strings.TrimSpace(m.Reason) == "" || utf8.RuneCountInString(m.Reason) > 500 {
		return FieldError("reason", "维护操作必须填写 1–500 字符的原因。")
	}
	return nil
}
func (s *Service) ResetLink(ctx context.Context, username string, m Meta) (Link, error) {
	if e := reason(m); e != nil {
		return Link{}, e
	}
	token, e := newToken()
	if e != nil {
		return Link{}, e
	}
	expiry, e := s.repo.ResetLink(ctx, username, digest(token), s.cfg.ResetTTL, m)
	return Link{URL: strings.TrimRight(s.cfg.PublicConsoleURL, "/") + "/reset-password#token=" + token, ExpiresAt: expiry}, e
}
func (s *Service) Find(ctx context.Context, username string) (Account, error) {
	return s.repo.Find(ctx, username)
}
func (s *Service) SetStatus(ctx context.Context, username string, enabled, emergency bool, m Meta) error {
	if e := reason(m); e != nil {
		return e
	}
	return s.repo.SetStatus(ctx, username, enabled, emergency, m)
}
func (s *Service) Audit(ctx context.Context, cursor string) (Page[AuditEvent], error) {
	return s.repo.Audit(ctx, cursor)
}
func (s *Service) RestoreRevoke(ctx context.Context, m Meta) error {
	if e := reason(m); e != nil {
		return e
	}
	return s.repo.RevokeAfterRestore(ctx, m)
}
func (s *Service) ClearSessionCache(ctx context.Context) error { return s.cache.ClearSessions(ctx) }
func (s *Service) DrainInvalidations(ctx context.Context) error {
	batch, e := s.repo.Invalidations(ctx)
	if e != nil {
		return e
	}
	for _, v := range batch {
		if e = s.cache.Delete(ctx, v.ID, v.Hash); e != nil {
			s.logger.Warn("session cache invalidation pending; database revocation is effective")
			return e
		}
		if e = s.repo.InvalidationDone(ctx, v.ID); e != nil {
			return e
		}
	}
	return nil
}

// RunJobs is canceled with the service. Activity is flushed every 30 seconds to stay within the 60 second loss bound.
func (s *Service) RunJobs(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, cancel := context.WithTimeout(ctx, 20*time.Second)
			e := s.cache.FlushActivities(job, s.repo, s.cfg.SessionIdle)
			s.activityFailed.Store(e != nil)
			if e != nil {
				s.logger.Warn("activity persistence unavailable; activity renewal suspended")
			}
			if e = s.DrainInvalidations(job); e != nil {
				s.logger.Warn("cache invalidation retry pending")
			}
			if n%20 == 0 {
				if e = s.repo.Cleanup(job, s.cfg.AuditRetention); e != nil {
					s.logger.Warn("identity cleanup pending")
				}
				s.cache.LogStats()
			}
			n++
			cancel()
		}
	}
}
func (s *Service) Flush(ctx context.Context) error {
	return s.cache.FlushActivities(ctx, s.repo, s.cfg.SessionIdle)
}
