package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{0,160}$`)

type cursor struct {
	Actor   string    `json:"a"`
	Scope   string    `json:"s"`
	Vector  string    `json:"v"`
	Filter  string    `json:"f"`
	At      time.Time `json:"t"`
	ID      string    `json:"i"`
	Upper   time.Time `json:"u"`
	Purpose string    `json:"p"`
}

func signCursor(c cursor, secret string) string {
	b := ar.JSON(c)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func parseCursor(v, secret string) (cursor, error) {
	var c cursor
	if len(v) > 2048 {
		return c, Problem("audit_cursor_stale", 409)
	}
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return c, Problem("audit_cursor_stale", 409)
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || e2 != nil {
		return c, Problem("audit_cursor_stale", 409)
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(b)
	if !hmac.Equal(sig, h.Sum(nil)) || json.Unmarshal(b, &c) != nil || (c.Purpose == "exports" && !ar.ID(c.ID)) || (c.Purpose != "exports" && !ar.Number(c.ID, 1)) {
		return c, Problem("audit_cursor_stale", 409)
	}
	return c, nil
}
func filterHash(f Filter) string { f.Cursor = ""; return ar.Hash(f) }
func (s *Service) normalizeFilter(f Filter, auth authorization, explicit bool) (Filter, error) {
	if f.Limit == 0 {
		f.Limit = s.options.PageSize
	}
	if f.Limit < 1 || f.Limit > s.options.MaxPageSize || utf8.RuneCountInString(f.Query) > 100 || len(f.Cursor) > 2048 {
		return f, Problem("audit_filter_invalid", 400)
	}
	if explicit && (f.From == "" || f.To == "") {
		return f, Problem("audit_filter_invalid", 400)
	}
	if f.From == "" {
		f.From = s.now().Add(-s.options.DefaultWindow).Format(time.RFC3339Nano)
	}
	if f.To == "" {
		f.To = s.now().Format(time.RFC3339Nano)
	}
	from, e := time.Parse(time.RFC3339Nano, f.From)
	to, e2 := time.Parse(time.RFC3339Nano, f.To)
	if e != nil || e2 != nil || !from.Before(to) || to.Sub(from) > s.options.MaxWindow {
		return f, Problem("audit_filter_invalid", 400)
	}
	f.From = from.UTC().Format(time.RFC3339Nano)
	f.To = to.UTC().Format(time.RFC3339Nano)
	for _, id := range []string{f.ReferenceID, f.ActorID, f.OperationID, f.TaskID} {
		if id != "" && !ar.ID(id) {
			return f, Problem("audit_filter_invalid", 400)
		}
	}
	for _, id := range []string{f.ReferenceID, f.ObjectID, f.ObjectType, f.EventType, f.RequestID, f.TaskModule, f.TaskKind} {
		if !identifier.MatchString(id) {
			return f, Problem("audit_filter_invalid", 400)
		}
	}
	if !slices.Contains([]string{"", "success", "failure", "denied", "conflict", "cancelled", "rate_limited", "unknown"}, f.Outcome) || !slices.Contains([]string{"", "security", "management", "business", "task", "file", "maintenance"}, f.Category) {
		return f, Problem("audit_filter_invalid", 400)
	}
	if (f.OperationID != "" || f.RequestID != "") && !auth.Has("technical.read") {
		return f, access.ErrDenied
	}
	if f.TaskID != "" && (f.TaskModule == "" || f.TaskKind == "" || !ar.Number(f.Generation, 1)) {
		return f, Problem("audit_filter_invalid", 400)
	}
	return f, nil
}
func (s *Service) Events(ctx context.Context, a access.Actor, scope Scope, f Filter) (EventPage, error) {
	out := EventPage{Items: []Event{}, MissingFields: []string{}}
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = s.eventsTx(ctx, tx, a, auth, f)
		return e
	})
	return out, e
}
func (s *Service) eventsTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, input Filter) (EventPage, error) {
	out := EventPage{Items: []Event{}, MissingFields: []string{}, ObservedAt: s.now(), Coverage: "complete", AuthorizationRevision: auth.Vector}
	var pending bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_audit_handoffs WHERE translated_at IS NULL AND tenant_id::text=$1 AND project_id::text=$2)`, auth.Scope.Tenant, auth.Scope.Project).Scan(&pending); e != nil {
		return out, e
	}
	if pending {
		out.Coverage = "partial"
		out.MissingFields = append(out.MissingFields, "display_handoffs")
	}
	// Cursor default time bounds must be stable across calls; bind the original
	// caller filter and the first observed upper bound separately.
	filterDigest := filterHash(input)
	position := cursor{Actor: a.ID, Scope: auth.Scope.Key(), Vector: auth.Vector, Filter: filterDigest, Upper: s.now(), Purpose: "events"}
	if input.Cursor != "" {
		v, e := parseCursor(input.Cursor, auth.Secret)
		if e != nil {
			return out, e
		}
		if v.Actor != a.ID || v.Scope != auth.Scope.Key() || v.Vector != auth.Vector || v.Filter != filterDigest || v.Purpose != "events" {
			return out, Problem("audit_cursor_stale", 409)
		}
		position = v
	}
	if input.From == "" {
		input.From = position.Upper.Add(-s.options.DefaultWindow).Format(time.RFC3339Nano)
	}
	if input.To == "" {
		input.To = position.Upper.Format(time.RFC3339Nano)
	}
	f, e := s.normalizeFilter(input, auth, false)
	if e != nil {
		return out, e
	}
	inspected := 0
	for {
		batch, e := s.candidatesTx(ctx, tx, auth, f, position, 100, "")
		if e != nil {
			return out, e
		}
		if len(batch) == 0 {
			break
		}
		if inspected+len(batch) > s.options.CandidateBudget {
			return EventPage{}, Problem("audit_query_budget_exceeded", 503)
		}
		inspected += len(batch)
		projected, e := s.projectTx(ctx, tx, a, auth, batch)
		if e != nil {
			return EventPage{}, e
		}
		for i, event := range projected {
			if event == nil {
				continue
			}
			match, e := matchesTx(ctx, tx, f, *event)
			if e != nil {
				return EventPage{}, e
			}
			if !match {
				continue
			}
			if len(out.Items) == f.Limit {
				last := out.Items[len(out.Items)-1]
				position.At = last.RecordedAt
				position.ID = last.ID
				out.NextCursor = signCursor(position, auth.Secret)
				return out, nil
			}
			out.Items = append(out.Items, *event)
			if batch[i].Schema == 0 {
				out.Coverage = "partial"
				if !slices.Contains(out.MissingFields, "legacy_history") {
					out.MissingFields = append(out.MissingFields, "legacy_history")
				}
			}
		}
		last := batch[len(batch)-1]
		position.At = last.Recorded
		position.ID = last.ID
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}
func matchesTx(ctx context.Context, tx pgx.Tx, f Filter, e Event) (bool, error) {
	if f.ActorID != "" && e.Actor.ID != f.ActorID || f.ObjectType != "" && e.Object.Type != f.ObjectType || f.ObjectID != "" && e.Object.ID != f.ObjectID || f.EventType != "" && e.EventType != f.EventType || f.Outcome != "" && e.Outcome != f.Outcome || f.Category != "" && e.Category != f.Category || f.OperationID != "" && e.OperationID != f.OperationID {
		return false, nil
	}
	if f.RequestID != "" && (e.Technical == nil || e.Technical.RequestID != f.RequestID) {
		return false, nil
	}
	if f.Query != "" && !strings.Contains(strings.ToLower(strings.Join([]string{e.ActionLabel, e.EventType, e.Reason, e.Actor.Name, e.Actor.Username, e.Object.Label, string(ar.JSON(e.Changes))}, " ")), strings.ToLower(f.Query)) {
		return false, nil
	}
	if f.ReferenceID != "" {
		if e.Object.LabelState == ar.Redacted {
			return false, nil
		}
		if !ar.ID(f.ReferenceID) {
			return false, Problem("audit_filter_invalid", 400)
		}
		var match bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_event_links WHERE event_id=$1 AND (target_id=$2 OR revision=$2))`, e.ID, f.ReferenceID).Scan(&match); err != nil || !match {
			return false, err
		}
	}
	if f.TaskID != "" {
		var match bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_event_links WHERE event_id=$1 AND link_kind='task_execution' AND source_module=$2 AND target_kind=$3 AND target_id=$4 AND generation=$5)`, e.ID, f.TaskModule, f.TaskKind, f.TaskID, f.Generation).Scan(&match)
		return match, err
	}

	return true, nil
}

type rawEvent struct {
	ID, EventType, ActorType, ActorID, AccountID, ObjectType, ObjectID, Reason, RequestID                                                                                                                     string
	Recorded, Occurred                                                                                                                                                                                        time.Time
	Scope                                                                                                                                                                                                     Scope
	Visibility, Category, Phase, Outcome, OperationID, Initiator, RequestedBy, ExecutorKind, ExecutorRef, ObjectRevision, ErrorCode, Quality, TimeSource, SourceModule, SourceKey, RequestKind, IP, UserAgent string
	Changes                                                                                                                                                                                                   []ar.Change
	Context, Details                                                                                                                                                                                          map[string]any
	Missing                                                                                                                                                                                                   []string
	Schema                                                                                                                                                                                                    int
	ActorCurrent, InitiatorCurrent, RequestedByCurrent                                                                                                                                                        string
}

const rawColumns = `e.id::text,e.event_type,e.actor_type,COALESCE(e.actor_id::text,''),COALESCE(e.account_id::text,''),e.object_type,e.object_id,e.reason,e.request_id,e.created_at,COALESCE(e.occurred_at,e.created_at),COALESCE(e.tenant_id::text,''),COALESCE(e.project_id::text,''),e.visibility_class,e.category,e.phase,e.outcome,COALESCE(e.operation_id::text,''),COALESCE(e.initiator_id::text,''),COALESCE(e.requested_by::text,''),e.executor_kind,e.executor_ref,e.object_revision,e.error_code,e.evidence_quality,e.time_source,COALESCE(e.source_module,''),COALESCE(e.source_key,''),e.request_id_kind,e.source_ip,e.user_agent,e.changes,e.safe_context,e.details,e.missing_fields,e.schema_version,COALESCE(a.display_name,''),COALESCE(i.display_name,''),COALESCE(r.display_name,'')`
const rawFrom = ` FROM account_security_events e LEFT JOIN accounts a ON a.id=e.actor_id LEFT JOIN accounts i ON i.id=e.initiator_id LEFT JOIN accounts r ON r.id=e.requested_by `

func scanRaw(row pgx.Row) (rawEvent, error) {
	var v rawEvent
	e := row.Scan(&v.ID, &v.EventType, &v.ActorType, &v.ActorID, &v.AccountID, &v.ObjectType, &v.ObjectID, &v.Reason, &v.RequestID, &v.Recorded, &v.Occurred, &v.Scope.Tenant, &v.Scope.Project, &v.Visibility, &v.Category, &v.Phase, &v.Outcome, &v.OperationID, &v.Initiator, &v.RequestedBy, &v.ExecutorKind, &v.ExecutorRef, &v.ObjectRevision, &v.ErrorCode, &v.Quality, &v.TimeSource, &v.SourceModule, &v.SourceKey, &v.RequestKind, &v.IP, &v.UserAgent, &v.Changes, &v.Context, &v.Details, &v.Missing, &v.Schema, &v.ActorCurrent, &v.InitiatorCurrent, &v.RequestedByCurrent)
	v.Scope.Kind = "platform"
	if v.Scope.Tenant != "" {
		v.Scope.Kind = "tenant"
	}
	if v.Scope.Project != "" {
		v.Scope.Kind = "project"
	}
	return v, e
}
func (s *Service) candidatesTx(ctx context.Context, tx pgx.Tx, auth authorization, f Filter, c cursor, limit int, id string) ([]rawEvent, error) {
	args := []any{visibleClasses(auth), f.From, f.To, c.Upper}
	where := `e.visibility_class=ANY($1::text[]) AND e.created_at>=$2::timestamptz AND e.created_at<$3::timestamptz AND e.created_at<=$4`
	bind := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if auth.Scope.Kind == "platform" {
		where += ` AND e.project_id IS NULL`
		if !slices.Contains(auth.Permissions, access.TenantRead) {
			where += ` AND e.tenant_id IS NULL`
		}
	} else {
		where += ` AND e.tenant_id=` + bind(auth.Scope.Tenant) + `::uuid`
		if auth.Scope.Kind == "project" {
			where += ` AND e.project_id=` + bind(auth.Scope.Project) + `::uuid`
		} else {
			where += ` AND e.project_id IS NULL`
		}
	}
	if !c.At.IsZero() {
		where += ` AND (e.created_at,e.id)<(` + bind(c.At) + `,` + bind(c.ID) + `::bigint)`
	}
	if id != "" {
		where += ` AND e.id=` + bind(id) + `::bigint`
	}
	rows, e := tx.Query(ctx, `SELECT `+rawColumns+rawFrom+` WHERE `+where+` ORDER BY e.created_at DESC,e.id DESC LIMIT `+bind(limit), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []rawEvent{}
	for rows.Next() {
		v, e := scanRaw(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
