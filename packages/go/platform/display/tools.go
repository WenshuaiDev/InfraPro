package display

import (
	"context"
	"crypto/subtle"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"strconv"
)

// Upload pairing is separate from the data session: an empty instance can receive
// its first package. It never grants business reads or configuration/publication.
func (s *Service) AuthorizeTool(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (map[string]any, error) {
	out := map[string]any{}
	if !digestPattern.MatchString(in.Challenge) || !audienceValid(in.Audience, s.options) || in.Audience == s.options.RuntimeOrigin || !access.ValidID(in.SessionID) {
		return out, invalid("/challenge", "工具必须使用本机一次性配对。")
	}
	code := secret()
	err := s.transact(ctx, a, scope, "tool.authorize", func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		v, e := instanceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if v.State == "archived" {
			return access.ErrDenied
		}
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM display_tool_sessions WHERE account_id=$1 AND expires_at>clock_timestamp()`, a.ID).Scan(&count); e != nil {
			return e
		}
		if count >= 8 {
			return problem("budget_exceeded", "本机工具会话数量达到限制。", 429)
		}
		_, e = tx.Exec(ctx, `INSERT INTO display_tool_sessions(tenant_id,project_id,instance_id,account_id,person_session_id,environment,challenge,audience,code_hash) VALUES($1,$2,$3,$4,$5,(SELECT generation FROM display_environment WHERE id=1),$6,$7,$8)`, scope.Tenant, scope.Project, id, a.ID, in.SessionID, in.Challenge, in.Audience, hashBytes([]byte(code)))
		if e != nil {
			return e
		}
		return eventTx(ctx, tx, scope, id, a.ID, "tool.authorize", map[string]string{"purpose": "upload"})
	})
	if err == nil {
		out = map[string]any{"code": code, "runtime_origin": s.options.RuntimeOrigin, "expires_in": 120}
	}
	return out, err
}
func (s *Service) ExchangeTool(ctx context.Context, code, verifier, audience string) (map[string]string, error) {
	out := map[string]string{}
	if len(code) > 200 || len(verifier) < 32 || len(verifier) > 200 {
		return out, hidden()
	}
	token := secret()
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var id, challenge string
		e := tx.QueryRow(ctx, `SELECT id::text,challenge FROM display_tool_sessions WHERE code_hash=$1 AND audience=$2 AND token_hash IS NULL AND pair_until>clock_timestamp() AND environment=(SELECT generation FROM display_environment WHERE id=1) FOR UPDATE`, hashBytes([]byte(code)), audience).Scan(&id, &challenge)
		if e != nil {
			return hidden()
		}
		got := hashBytes([]byte(verifier))
		if subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) != 1 {
			return hidden()
		}
		_, e = tx.Exec(ctx, `UPDATE display_tool_sessions SET token_hash=$2 WHERE id=$1`, id, hashBytes([]byte(token)))
		out = map[string]string{"id": id, "token": token}
		return e
	})
	return out, e
}
func (s *Service) toolIdentity(ctx context.Context, id, token string) (access.Actor, Scope, string, error) {
	var a access.Actor
	var scope Scope
	var instance, person string
	if !access.ValidID(id) || len(token) > 200 {
		return a, scope, instance, hidden()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT account_id::text,person_session_id::text,tenant_id::text,project_id::text,instance_id::text FROM display_tool_sessions WHERE id=$1 AND token_hash=$2 AND expires_at>clock_timestamp() AND environment=(SELECT generation FROM display_environment WHERE id=1)`, id, hashBytes([]byte(token))).Scan(&a.ID, &person, &scope.Tenant, &scope.Project, &instance)
	})
	if e != nil {
		return a, scope, instance, hidden()
	}
	a.Check = func(ctx context.Context, tx pgx.Tx, _ bool) error {
		if s.resolvePerson == nil {
			return hidden()
		}
		var valid bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_tool_sessions WHERE id=$1 AND token_hash=$2 AND expires_at>clock_timestamp() AND environment=(SELECT generation FROM display_environment WHERE id=1))`, id, hashBytes([]byte(token))).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return hidden()
		}
		return s.resolvePerson(ctx, tx, a.ID, person)
	}
	return a, scope, instance, nil
}
func (s *Service) ToolCommand(ctx context.Context, id, token, action, target string, in Command) (map[string]any, error) {
	a, scope, instance, e := s.toolIdentity(ctx, id, token)
	if e != nil {
		return nil, e
	}
	switch action {
	case "inspect":
		var v Instance
		e = s.transact(ctx, a, scope, "tool.authorize", func(tx pgx.Tx, _ access.ProjectAuthorization) error {
			var e error
			v, e = instanceTx(ctx, tx, scope, instance)
			return e
		})
		return map[string]any{"instance": v, "max_package_bytes": strconv.FormatInt(s.options.MaxPackageBytes, 10)}, e
	case "upload.get":
		return s.Upload(ctx, a, scope, instance, target)
	case "upload", "upload.complete", "upload.cancel", "package.retry":
		return s.Command(ctx, a, scope, instance, action, target, in)
	default:
		return nil, hidden()
	}
}
func (s *Service) ToolUploadPart(ctx context.Context, id, token, upload string, part int, sha string, data []byte) error {
	a, scope, instance, e := s.toolIdentity(ctx, id, token)
	if e != nil {
		return e
	}
	return s.UploadPart(ctx, a, scope, instance, upload, part, sha, data)
}
