package display

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Session struct {
	ID string `json:"id"`
	Scope
	InstanceID         string    `json:"instance_id"`
	SubjectID          string    `json:"-"`
	ReleaseID          string    `json:"release_id"`
	PackageID          string    `json:"package_id"`
	ConfigID           string    `json:"config_id"`
	Preview            bool      `json:"preview"`
	State              string    `json:"state"`
	Generation         string    `json:"generation"`
	Authorization      string    `json:"authorization_revision"`
	LeaseUntil         time.Time `json:"lease_until"`
	CreatedAt          time.Time `json:"created_at"`
	AccountID          string    `json:"-"`
	TerminalID         string    `json:"-"`
	PersonSession      string    `json:"-"`
	Environment        string    `json:"environment"`
	TerminalGeneration string    `json:"-"`
	TargetRelease      string    `json:"target_release_id"`
}

const sessionSelect = `SELECT s.id::text,s.tenant_id::text,s.project_id::text,s.instance_id::text,s.subject_id::text,COALESCE(s.release_id::text,''),s.package_id::text,s.config_id::text,s.preview,s.state,s.generation::text,s.authorization_revision,s.lease_until,s.created_at,COALESCE(r.account_id::text,''),COALESCE(r.terminal_id::text,''),COALESCE(r.person_session_id::text,''),r.environment::text,COALESCE(s.terminal_generation::text,''),COALESCE(i.current_release_id::text,'') FROM display_sessions s JOIN display_subjects r ON r.id=s.subject_id JOIN display_instances i ON i.id=s.instance_id`

func scanSession(row pgx.Row) (Session, error) {
	v := Session{}
	e := row.Scan(&v.ID, &v.Tenant, &v.Project, &v.InstanceID, &v.SubjectID, &v.ReleaseID, &v.PackageID, &v.ConfigID, &v.Preview, &v.State, &v.Generation, &v.Authorization, &v.LeaseUntil, &v.CreatedAt, &v.AccountID, &v.TerminalID, &v.PersonSession, &v.Environment, &v.TerminalGeneration, &v.TargetRelease)
	if e == pgx.ErrNoRows {
		e = problem("session_expired", "运行会话已过期，请重新进入。", 401)
	}
	return v, e
}
func (s *Service) Locate(ctx context.Context, a access.Actor, id string) (Scope, error) {
	var scope Scope
	if !access.ValidID(id) {
		return scope, hidden()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT tenant_id::text,project_id::text FROM display_instances WHERE id=$1`, id).Scan(&scope.Tenant, &scope.Project)
		if e != nil {
			return hidden()
		}
		return nil
	})
	if e != nil {
		return Scope{}, e
	}
	e = s.transact(ctx, a, scope, "list", func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		v, e := instanceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if !canView(auth, v) {
			return hidden()
		}
		return nil
	})
	return scope, e
}
func audienceValid(v string, o Options) bool {
	if v == o.RuntimeOrigin {
		return true
	}
	u, e := url.Parse(v)
	return e == nil && u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1") && u.Port() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}
func (s *Service) Launch(ctx context.Context, a access.Actor, scope Scope, id string, in Command, preview bool) (map[string]any, error) {
	out := map[string]any{}
	action := "launch"
	if preview {
		action = "preview"
	}
	if !digestPattern.MatchString(in.Challenge) || !audienceValid(in.Audience, s.options) || !access.ValidID(in.SessionID) {
		return out, invalid("/challenge", "需要一次性启动校验及受信目标。")
	}
	if !preview && in.Audience != s.options.RuntimeOrigin {
		return out, access.ErrDenied
	}
	e := s.transact(ctx, a, scope, action, func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		v, e := instanceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if !canView(auth, v) {
			return hidden()
		}
		config := in.ConfigID
		release := v.Current
		if !preview {
			if release == "" {
				return problem("not_published", "大屏尚未发布。", 409)
			}
			if e = tx.QueryRow(ctx, `SELECT config_id::text FROM display_releases WHERE id=$1 AND NOT withdrawn`, release).Scan(&config); e != nil {
				return hidden()
			}
		} else {
			release = ""
		}
		prep, e := s.prepareTx(ctx, tx, scope, v, config)
		if e != nil {
			return e
		}
		session, e := s.createSessionTx(ctx, tx, scope, v, prep, a.ID, in.SessionID, "", "", release, preview)
		if e != nil {
			return e
		}
		code := secret()
		_, e = tx.Exec(ctx, `INSERT INTO display_pairings(code_hash,challenge,audience,session_id,expires_at) VALUES($1,$2,$3,$4,clock_timestamp()+interval '2 minutes')`, hashBytes([]byte(code)), in.Challenge, in.Audience, session.ID)
		if e != nil {
			return e
		}
		out = map[string]any{"code": code, "expires_in": 120, "runtime_origin": s.options.RuntimeOrigin, "instance_id": id}
		return nil
	})
	return out, e
}
func (s *Service) createSessionTx(ctx context.Context, tx pgx.Tx, scope Scope, v Instance, prep Preparation, account, personSession, terminal, terminalGeneration, release string, preview bool) (Session, error) {
	// Serialize the count and insert across runtime replicas and terminal starts.
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('display-sessions:'||$1,0))`, scope.Tenant); e != nil {
		return Session{}, e
	}
	var count int
	e := tx.QueryRow(ctx, `SELECT count(*) FROM display_sessions WHERE tenant_id=$1 AND state IN ('starting','ready','degraded') AND lease_until>clock_timestamp()`, scope.Tenant).Scan(&count)
	if e != nil {
		return Session{}, e
	}
	if count >= s.options.MaxSessions {
		return Session{}, problem("budget_exceeded", "运行连接达到部署限制。", 429)
	}
	var subject string
	e = tx.QueryRow(ctx, `INSERT INTO display_subjects(tenant_id,project_id,instance_id,account_id,person_session_id,terminal_id,environment) VALUES($1,$2,$3,$4,$5,$6,(SELECT generation FROM display_environment WHERE id=1)) RETURNING id::text`, scope.Tenant, scope.Project, v.ID, nullable(account), nullable(personSession), nullable(terminal)).Scan(&subject)
	if e != nil {
		return Session{}, e
	}
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO display_sessions(tenant_id,project_id,instance_id,subject_id,release_id,package_id,config_id,preview,token_hash,asset_hash,terminal_generation,authorization_revision,lease_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,clock_timestamp()+$13::interval) RETURNING id::text`, scope.Tenant, scope.Project, v.ID, subject, nullable(release), prep.Package.ID, prep.Config.ID, preview, hashBytes([]byte(secret())), hashBytes([]byte(secret())), nullable(terminalGeneration), prep.GrantRevision, strconv.FormatFloat(s.options.Lease.Seconds(), 'f', 0, 64)+" seconds").Scan(&id)
	if e != nil {
		return Session{}, e
	}
	return scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, id))
}
func (s *Service) Exchange(ctx context.Context, code, verifier, audience string) (string, string, Session, error) {
	var out Session
	token, asset := secret(), secret()
	if len(code) > 200 || len(verifier) < 32 || len(verifier) > 200 || !audienceValid(audience, s.options) {
		return "", "", out, hidden()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var challenge, id string
		e := tx.QueryRow(ctx, `SELECT challenge,session_id::text FROM display_pairings WHERE code_hash=$1 AND audience=$2 AND used_at IS NULL AND expires_at>clock_timestamp() FOR UPDATE`, hashBytes([]byte(code)), audience).Scan(&challenge, &id)
		if e != nil {
			return hidden()
		}
		got := hashBytes([]byte(verifier))
		if subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) != 1 {
			return hidden()
		}
		out, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, id))
		if e != nil {
			return e
		}
		if _, _, e = s.checkSessionTx(ctx, tx, out, false); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_pairings SET used_at=clock_timestamp() WHERE code_hash=$1`, hashBytes([]byte(code)))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_sessions SET token_hash=$2,asset_hash=$3 WHERE id=$1`, id, hashBytes([]byte(token)), hashBytes([]byte(asset)))
		return e
	})
	return token, asset, out, e
}
func (s *Service) checkSessionTx(ctx context.Context, tx pgx.Tx, v Session, allowExpired bool) (Grant, Config, error) {
	current, loadErr := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
	if loadErr != nil {
		return Grant{}, Config{}, loadErr
	}
	v = current

	var active bool
	e := tx.QueryRow(ctx, `SELECT r.environment=e.generation AND t.status='active' AND st.enabled AND i.state='enabled' AND p.status IN ('active','archived') AND (s.preview OR NOT rel.withdrawn) FROM display_sessions s JOIN display_subjects r ON r.id=s.subject_id JOIN display_environment e ON e.id=1 JOIN tenants t ON t.id=s.tenant_id JOIN projects p ON p.id=s.project_id JOIN display_settings st ON st.tenant_id=s.tenant_id AND st.project_id=s.project_id JOIN display_instances i ON i.id=s.instance_id LEFT JOIN display_releases rel ON rel.id=s.release_id WHERE s.id=$1`, v.ID).Scan(&active)
	if e != nil {
		return Grant{}, Config{}, e
	}
	if !active || !slices.Contains([]string{"starting", "ready", "degraded"}, v.State) {
		return Grant{}, Config{}, problem("revoked", "大屏运行资格已失效。", 403)
	}
	if !allowExpired && v.LeaseUntil.Before(time.Now()) {
		return Grant{}, Config{}, problem("session_expired", "运行租约已到期。", 401)
	}
	g, e := grantTx(ctx, tx, v.InstanceID)
	if e != nil || g.Revoked || g.ValidUntil.Before(time.Now()) || g.Revision != v.Authorization {
		return g, Config{}, problem("revoked", "读取委托已变更或失效。", 403)
	}
	issuer, e := access.ProjectSnapshotTx(ctx, tx, g.IssuedBy, v.Tenant, v.Project, "")
	if e != nil {
		return g, Config{}, hidden()
	}
	if !slices.Contains(issuer.Permissions, access.ProjectDisplayGrant) {
		return g, Config{}, problem("revoked", "委托资格已失效。", 403)
	}
	for _, c := range s.permissionsFor(g.Capabilities) {
		if !slices.Contains(issuer.Permissions, c) {
			return g, Config{}, problem("revoked", "业务委托资格已失效。", 403)
		}
	}
	if v.AccountID != "" {
		if s.resolvePerson == nil {
			return g, Config{}, access.ErrDenied
		}
		if e = s.resolvePerson(ctx, tx, v.AccountID, v.PersonSession); e != nil {
			return g, Config{}, problem("session_expired", "人员登录已经失效。", 401)
		}
		op := access.DisplayOperation("launch")
		if v.Preview {
			op = access.DisplayOperation("preview")
		}
		auth, e := access.RequireProjectTx(ctx, tx, v.AccountID, op, v.Tenant, v.Project)
		if e != nil {
			return g, Config{}, problem("revoked", "人员项目资格已经失效。", 403)
		}
		var viewers []string
		if e = tx.QueryRow(ctx, `SELECT viewers::text[] FROM display_instances WHERE id=$1`, v.InstanceID).Scan(&viewers); e != nil {
			return g, Config{}, e
		}
		if len(viewers) > 0 && !slices.Contains(viewers, v.AccountID) && !manager(auth) {
			return g, Config{}, hidden()
		}
	} else {
		e = tx.QueryRow(ctx, `SELECT state='active' AND generation::text=$2 AND instance_id=$3 AND environment::text=$4 AND valid_until>clock_timestamp() FROM display_terminals WHERE id=$1`, v.TerminalID, v.TerminalGeneration, v.InstanceID, v.Environment).Scan(&active)
		if e != nil || !active {
			return g, Config{}, problem("revoked", "终端委托已经失效。", 403)
		}
	}
	c, e := configTx(ctx, tx, v.InstanceID, v.ConfigID)
	if e != nil {
		return g, c, e
	}
	p, e := packageTx(ctx, tx, v.InstanceID, v.PackageID)
	if e != nil {
		return g, c, e
	}
	if e = s.validateRequiredBindingsTx(ctx, tx, v.Scope, p.Manifest, c.Bindings, &g); e != nil {
		return g, c, problem("binding_invalid", "当前绑定或范围已经失效。", 409)
	}
	if !v.Preview && v.ReleaseID != v.TargetRelease {
		var changed time.Time
		e = tx.QueryRow(ctx, `SELECT created_at FROM display_releases WHERE id=$1`, v.TargetRelease).Scan(&changed)
		if e != nil || time.Since(changed) > s.options.SwitchGrace {
			return g, c, problem("release_changed", "发布已切换，请加载目标版本。", 409)
		}
	}
	return g, c, nil
}
func (s *Service) Session(ctx context.Context, token string) (Session, error) {
	var v Session
	if len(token) < 32 || len(token) > 200 {
		return v, problem("session_expired", "请从可信入口重新进入。", 401)
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var e error
		v, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.token_hash=$1`, hashBytes([]byte(token))))
		if e != nil {
			return e
		}
		_, _, e = s.checkSessionTx(ctx, tx, v, false)
		return e
	})
	return v, e
}
func (s *Service) ReadSubject(ctx context.Context, id string) (*access.ReadSubject, error) {
	var v Session
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.subject_id=$1 ORDER BY s.created_at DESC LIMIT 1`, id))
		return e
	})
	if e != nil {
		return nil, e
	}
	return s.subject(v), nil
}
func (s *Service) subject(v Session) *access.ReadSubject {
	sub := &access.ReadSubject{ID: v.SubjectID, Tenant: v.Tenant, Project: v.Project, Revision: v.Environment + ":" + v.Authorization + ":" + v.ConfigID}
	sub.Check = func(ctx context.Context, tx pgx.Tx) error {
		current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
		if e != nil {
			return e
		}
		_, _, e = s.checkSessionTx(ctx, tx, current, false)
		return e
	}
	sub.Authorize = func(ctx context.Context, tx pgx.Tx, op access.Operation) (access.ProjectAuthorization, error) {
		g, _, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return access.ProjectAuthorization{}, e
		}
		person := g.IssuedBy
		if v.AccountID != "" {
			person = v.AccountID
		}
		auth, e := access.RequireProjectTx(ctx, tx, person, op, v.Tenant, v.Project)
		if e != nil {
			return auth, e
		}
		allowed := s.permissionsFor(g.Capabilities)
		auth.Permissions = slices.DeleteFunc(slices.Clone(auth.Permissions), func(c access.Code) bool { return !slices.Contains(allowed, c) })
		auth.AllowedOperations = nil
		return auth, nil
	}
	sub.Windows = func(ctx context.Context, tx pgx.Tx, channels []string) (map[string][]access.ReadWindow, error) {
		g, _, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return nil, e
		}
		ids, e := grantIDsTx(ctx, tx, v.Scope, g, "channels")
		if e != nil {
			return nil, e
		}
		w, e := readWindow(g.Scope.From, g.Scope.To)
		if e != nil {
			return nil, e
		}
		out := map[string][]access.ReadWindow{}
		for _, id := range channels {
			if !slices.Contains(ids, id) {
				return nil, hidden()
			}
			out[id] = []access.ReadWindow{{From: w.From, To: w.To}}
		}
		return out, nil
	}
	return sub
}
func (s *Service) Bootstrap(ctx context.Context, v Session, asset string) (map[string]any, error) {
	out := map[string]any{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		g, c, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return e
		}
		p, e := packageTx(ctx, tx, v.InstanceID, v.PackageID)
		if e != nil {
			return e
		}
		caps := s.Capabilities()
		person := g.IssuedBy
		if v.AccountID != "" {
			person = v.AccountID
		}
		currentAuth, e := access.ProjectSnapshotTx(ctx, tx, person, v.Tenant, v.Project, "")
		if e != nil {
			return e
		}
		for i := range caps {
			if !slices.Contains(g.Capabilities, caps[i].Name) {
				caps[i].Availability = "unavailable"
				caps[i].Reason = "当前实例未获此能力委托。"
			}
			for _, permission := range caps[i].Permissions {
				if !slices.Contains(currentAuth.Permissions, permission) {
					caps[i].Availability = "unavailable"
					caps[i].Reason = "当前身份没有此业务读取资格。"
				}
			}
		}
		out = map[string]any{"session": v, "config": c.Parameters, "bindings": c.Bindings, "capabilities": caps, "manifest": p.Manifest, "protocol": ProtocolMajor, "server_time": time.Now().UTC().Format(time.RFC3339Nano), "timezone": "Asia/Shanghai", "mode": c.Mode}
		if asset != "" {
			out["entry_url"] = s.options.AssetOrigin + "/assets/" + asset + "/" + p.Manifest.Entry
		}
		return nil
	})
	return out, e
}
func (s *Service) Renew(ctx context.Context, v Session) (string, Session, error) {
	var out Session
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
		if e != nil {
			return e
		}
		if _, _, e = s.checkSessionTx(ctx, tx, current, false); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_sessions SET lease_until=clock_timestamp()+$2::interval,last_seen_at=clock_timestamp() WHERE id=$1`, v.ID, strconv.FormatFloat(s.options.Lease.Seconds(), 'f', 0, 64)+" seconds")
		if e != nil {
			return e
		}
		out, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
		return e
	})
	return "", out, e
}
func (s *Service) RuntimeControl(ctx context.Context, v Session, action, code string) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, _, e := s.checkSessionTx(ctx, tx, v, false); e != nil {
			return e
		}
		state := "ready"
		switch action {
		case "ready":
		case "close":
			state = "closed"
		case "error":
			state = "degraded"
			if len(code) > 80 || !keyPattern.MatchString(code) {
				code = "application_error"
			}
		default:
			return invalid("/action", "运行操作不支持。")
		}
		_, e := tx.Exec(ctx, `UPDATE display_sessions SET state=$2,last_error=$3,last_seen_at=clock_timestamp() WHERE id=$1`, v.ID, state, code)
		if e != nil {
			return e
		}
		if v.TerminalID != "" {
			_, e = tx.Exec(ctx, `UPDATE display_terminals SET actual_release_id=CASE WHEN $4 THEN $2::uuid ELSE actual_release_id END,last_error=$3,last_seen_at=clock_timestamp() WHERE id=$1`, v.TerminalID, nullable(v.ReleaseID), code, action == "ready")
		}
		if e != nil {
			return e
		}
		return eventTx(ctx, tx, v.Scope, v.InstanceID, "", "runtime."+action, map[string]string{"session_id": v.ID, "release_id": v.ReleaseID, "code": code})
	})
}

// resolveTargets never accepts tenant, project, subject or raw URL overrides.
type Target struct {
	Binding string   `json:"binding,omitempty"`
	IDs     []string `json:"ids,omitempty"`
}
type Query struct {
	From          *string           `json:"from,omitempty"`
	To            *string           `json:"to,omitempty"`
	Levels        []string          `json:"levels,omitempty"`
	Statuses      []string          `json:"statuses,omitempty"`
	Target        Target            `json:"target"`
	Range         *monitoring.Range `json:"range,omitempty"`
	ViewRange     *monitoring.Range `json:"view_range,omitempty"`
	AllHistory    bool              `json:"all_history,omitempty"`
	Qualities     []string          `json:"qualities,omitempty"`
	Timezone      string            `json:"timezone,omitempty"`
	Snapshot      string            `json:"snapshot_id,omitempty"`
	Selection     string            `json:"selection_id,omitempty"`
	Cursor        string            `json:"cursor,omitempty"`
	Limit         int               `json:"limit,omitempty"`
	Search        string            `json:"search,omitempty"`
	Kind          string            `json:"kind,omitempty"`
	ID            string            `json:"id,omitempty"`
	Version       string            `json:"version,omitempty"`
	FileID        string            `json:"file_id,omitempty"`
	Offset        int64             `json:"offset,omitempty"`
	Length        int64             `json:"length,omitempty"`
	PlotWidth     int               `json:"plot_width_px,omitempty"`
	PlotBudget    int               `json:"plot_point_budget,omitempty"`
	Period        string            `json:"period,omitempty"`
	Output        string            `json:"output,omitempty"`
	FollowSeconds int               `json:"follow_seconds,omitempty"`
}

func (s *Service) resolveTargetsTx(ctx context.Context, tx pgx.Tx, v Session, method string, q *Query) ([]string, error) {
	g, c, e := s.checkSessionTx(ctx, tx, v, false)
	if e != nil {
		return nil, e
	}
	kind := bindingKind(method)
	var ids []string
	var b Binding
	if q.Target.Binding != "" {
		var ok bool
		b, ok = c.Bindings[q.Target.Binding]
		if !ok || b.Kind != kind {
			return nil, problem("binding_invalid", "绑定不可用或类型不匹配。", 422)
		}
		p, pe := packageTx(ctx, tx, v.InstanceID, v.PackageID)
		if pe != nil {
			return nil, pe
		}
		bm := p.Manifest
		bm.Bindings = map[string]BindingSchema{q.Target.Binding: bm.Bindings[q.Target.Binding]}
		if e = s.validateBindingsTx(ctx, tx, v.Scope, bm, map[string]Binding{q.Target.Binding: b}, &g, true); e != nil {
			return nil, problem("binding_invalid", "所选绑定当前不可用。", 409)
		}
		ids, e = effectiveBindingIDsTx(ctx, tx, v.Scope, b, &g)
		if e != nil {
			return nil, e
		}
	} else {
		for _, binding := range c.Bindings {
			if binding.Kind == kind {
				more, e := effectiveBindingIDsTx(ctx, tx, v.Scope, binding, &g)
				if e != nil {
					return nil, e
				}
				if len(more) > 0 {
					if e = intersectBindingRange(&b, binding); e != nil {
						return nil, e
					}
				}
				ids = append(ids, more...)
			}
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(q.Target.IDs) > 0 {
		for _, id := range q.Target.IDs {
			if !slices.Contains(ids, id) {
				return nil, hidden()
			}
		}
		ids = slices.Clone(q.Target.IDs)
	}
	if q.ID != "" {
		if !slices.Contains(ids, q.ID) {
			return nil, hidden()
		}
		ids = []string{q.ID}
	}
	allowed, e := grantIDsTx(ctx, tx, v.Scope, g, kind)
	if e != nil {
		return nil, e
	}
	for _, id := range ids {
		if !slices.Contains(allowed, id) {
			return nil, hidden()
		}
	}
	if kind == "channels" && strings.HasPrefix(method, "monitoring.") {
		if q.FollowSeconds != 0 {
			if c.Mode != "follow" || q.FollowSeconds < 1 || q.FollowSeconds > 31*86400 || q.Range != nil || q.AllHistory || q.Snapshot != "" {
				return nil, invalid("/follow_seconds", "跟随窗口需要跟随配置且不能混用固定快照。")
			}
			to := time.Now().UTC().Format(time.RFC3339Nano)
			from := time.Now().UTC().Add(-time.Duration(q.FollowSeconds) * time.Second).Format(time.RFC3339Nano)
			q.Range = &monitoring.Range{From: &from, To: &to}
		}
		if method == "monitoring.latest" && q.Range == nil && !q.AllHistory {
			q.AllHistory = true
		}
		if !q.AllHistory && q.Range == nil {
			return nil, invalid("/range", "需要明确时段，全部历史须显式声明。")
		}
		ceiling, e := readWindow(g.Scope.From, g.Scope.To)
		if e != nil {
			return nil, e
		}
		bound, e := readWindow(b.From, b.To)
		if e != nil {
			return nil, e
		}
		ceiling, ok := ceiling.Intersect(bound)
		if !ok {
			return nil, hidden()
		}
		if q.Range != nil {
			requested, e := readWindow(q.Range.From, q.Range.To)
			if e != nil {
				return nil, e
			}
			if !ceiling.Covers(requested) {
				return nil, invalid("/range", "查询超出绑定或读取授权时段。")
			}
		} else if ceiling.From != nil || ceiling.To != nil {
			q.AllHistory = false
			q.Range = &monitoring.Range{}
			if ceiling.From != nil {
				v := time.Unix(0, *ceiling.From).UTC().Format(time.RFC3339Nano)
				q.Range.From = &v
			}
			if ceiling.To != nil {
				v := time.Unix(0, *ceiling.To).UTC().Format(time.RFC3339Nano)
				q.Range.To = &v
			}
		}
	}
	q.Target = Target{IDs: ids}
	return ids, nil
}
func decodeQuery(raw json.RawMessage) (Query, error) {
	q := Query{}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	e := Decode(raw, &q)
	if e != nil {
		return q, e
	}
	if q.Limit < 0 || q.Limit > 1000 || len(q.Search) > 120 || len(q.Cursor) > 4096 || q.Length < 0 || q.Offset < 0 {
		return q, invalid("/params", "查询参数超出限制。")
	}
	if e = normalizeIDs(q.Target.IDs); e != nil {
		return q, e
	}
	return q, nil
}
