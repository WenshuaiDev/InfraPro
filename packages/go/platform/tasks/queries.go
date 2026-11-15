package tasks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Filter struct {
	ScopeKind string `json:"scope_kind,omitempty"`
	Source    string `json:"source,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Tenant    string `json:"tenant_id,omitempty"`
	Project   string `json:"project_id,omitempty"`
	Type      string `json:"type,omitempty"`
	Group     string `json:"group,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Q         string `json:"q,omitempty"`
	Sort      string `json:"sort,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

func (f *Filter) validate(o Options) error {
	if f.Mode == "" {
		f.Mode = "mine"
	}
	if f.Limit == 0 {
		f.Limit = o.PageSize
	}
	if !slices.Contains([]string{"mine", "managed"}, f.Mode) || !slices.Contains([]string{"", "active", "attention", "ended"}, f.Group) || !slices.Contains([]string{"", "newest", "oldest"}, f.Sort) || f.Limit < 1 || f.Limit > o.MaxPageSize || utf8.RuneCountInString(f.Q) > 100 || len(f.Cursor) > 4096 {
		return access.Validation("filter", "任务筛选无效。")
	}
	if !slices.Contains([]string{"", "platform", "tenant", "project"}, f.ScopeKind) || f.ScopeKind == "platform" && (f.Tenant != "" || f.Project != "") {
		return access.Validation("scope", "请选择有效任务范围。")
	}
	if f.Tenant != "" && !access.ValidID(f.Tenant) || f.Project != "" && (!access.ValidID(f.Project) || f.Tenant == "") {
		return access.Validation("scope", "请选择有效任务范围。")
	}
	if f.Source != "" {
		for _, m := range strings.Split(f.Source, ",") {
			if !slices.Contains([]string{"monitoring", "derived", "spatial", "display", "files", "audit"}, m) {
				return access.Validation("source", "任务来源无效。")
			}
		}
	}
	if f.Type != "" {
		types := strings.Split(f.Type, ",")
		if len(types) > len(taskrecord.Catalog()) {
			return access.Validation("type", "任务类型筛选过多。")
		}
		for _, kind := range types {
			if _, ok := taskrecord.Lookup(kind); !ok {
				return access.Validation("type", "任务类型无效。")
			}
		}
		slices.Sort(types)
		f.Type = strings.Join(slices.Compact(types), ",")
	}
	for _, v := range []string{f.From, f.To} {
		if v != "" {
			if _, e := time.Parse(time.RFC3339Nano, v); e != nil {
				return access.Validation("time", "任务时间筛选无效。")
			}
		}
	}
	if f.From != "" && f.To != "" {
		a, _ := time.Parse(time.RFC3339Nano, f.From)
		b, _ := time.Parse(time.RFC3339Nano, f.To)
		if a.After(b) {
			return access.Validation("time", "开始时间不能晚于结束时间。")
		}
	}
	f.Q = strings.TrimSpace(f.Q)
	return nil
}

type Page struct {
	Items                 []View    `json:"items"`
	NextCursor            string    `json:"next_cursor,omitempty"`
	Coverage              string    `json:"coverage"`
	IndexState            string    `json:"index_state"`
	ObservedAt            time.Time `json:"observed_at"`
	AuthorizationRevision string    `json:"authorization_revision"`
}
type Summary struct {
	CountsState           string    `json:"counts_state"`
	Active                *int      `json:"active"`
	Attention             *int      `json:"attention"`
	Ended                 *int      `json:"ended"`
	Total                 *int      `json:"total"`
	ObservedAt            time.Time `json:"observed_at"`
	AuthorizationRevision string    `json:"authorization_revision"`
}
type cursor struct {
	Account, Filter, Authorization, Epoch string
	Upper, At                             time.Time
	ID                                    string
}

func sign(c cursor, secret string) string {
	b := taskrecord.JSON(c)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func decode(s, secret string) (cursor, error) {
	var c cursor
	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return c, stale()
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return c, stale()
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return c, stale()
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(b)
	if !hmac.Equal(h.Sum(nil), sig) || json.Unmarshal(b, &c) != nil {
		return c, stale()
	}
	return c, nil
}
func stale() error {
	return Problem("task_cursor_stale", "权限或任务环境已变化，请刷新首页。", 409)
}
func unavailable() error {
	return Problem("task_source_unavailable", "任务来源暂时无法核实，请稍后重试。", 503)
}
func budget() error {
	return Problem("task_query_budget_exceeded", "任务筛选范围过大，请缩小时段或工作空间后重试。", 503)
}
func authorizationVector(ctx context.Context, tx pgx.Tx, account, rev string) (string, error) {
	var resource string
	e := tx.QueryRow(ctx, `SELECT COALESCE(string_agg(p.project_id::text||':'||p.revision::text||':'||COALESCE(m.revision,0)::text||':'||COALESCE(s.revision,0)::text,',' ORDER BY p.project_id),'') FROM project_resource_policies p JOIN project_memberships member ON member.tenant_id=p.tenant_id AND member.project_id=p.project_id AND member.account_id=$1 AND member.status='active' LEFT JOIN monitoring_access_states m ON m.tenant_id=p.tenant_id AND m.project_id=p.project_id LEFT JOIN project_spatial_states s ON s.tenant_id=p.tenant_id AND s.project_id=p.project_id`, account).Scan(&resource)
	return taskrecord.Hash([]string{rev, resource}), e
}

type scopeAccess struct {
	Scope taskrecord.Scope
	Codes []access.Code
	Name  string
}

func scopesTx(ctx context.Context, tx pgx.Tx, a access.Actor, f Filter) ([]scopeAccess, error) {
	rows, e := tx.Query(ctx, `SELECT 'tenant',tenant_id::text,'' FROM tenant_memberships WHERE account_id=$1 AND status='active' AND ($2::uuid IS NULL OR tenant_id=$2) AND $3::uuid IS NULL UNION ALL SELECT 'project',tenant_id::text,project_id::text FROM project_memberships WHERE account_id=$1 AND status='active' AND ($2::uuid IS NULL OR tenant_id=$2) AND ($3::uuid IS NULL OR project_id=$3) ORDER BY 1,2,3`, a.ID, taskrecord.Null(f.Tenant), taskrecord.Null(f.Project))
	if e != nil {
		return nil, e
	}
	raw := []taskrecord.Scope{}
	for rows.Next() {
		var v taskrecord.Scope
		if e = rows.Scan(&v.Kind, &v.Tenant, &v.Project); e != nil {
			rows.Close()
			return nil, e
		}
		raw = append(raw, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if f.Tenant == "" && f.Project == "" {
		raw = append(raw, taskrecord.Scope{Kind: "platform"})
	}
	out := []scopeAccess{}
	for _, scope := range raw {
		if f.ScopeKind != "" && f.ScopeKind != scope.Kind {
			continue
		}
		codes, name, _, e := ScopeTx(ctx, tx, a, scope)
		if Denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		out = append(out, scopeAccess{scope, codes, name})
	}
	return out, nil
}
func (s *Service) catalogTx(ctx context.Context, tx pgx.Tx, a access.Actor, f Filter) ([]taskrecord.Definition, []string, error) {
	scopes, e := scopesTx(ctx, tx, a, f)
	if e != nil {
		return nil, nil, e
	}
	types := []taskrecord.Definition{}
	keys := []string{}
	for _, scope := range scopes {
		for _, d := range taskrecord.Catalog() {
			if f.Source != "" && !slices.Contains(strings.Split(f.Source, ","), d.Module) || f.Type != "" && !slices.Contains(strings.Split(f.Type, ","), d.Type) || s.sources[d.Module] == nil || !CanUse(scope.Codes, d, scope.Scope.Kind, f.Mode) {
				continue
			}
			if !slices.ContainsFunc(types, func(v taskrecord.Definition) bool { return v.Type == d.Type }) {
				types = append(types, d)
			}
			key := scope.Scope.Kind + ":" + scope.Scope.Tenant + ":" + scope.Scope.Project + ":" + d.Type
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	return types, keys, nil
}
func matches(v View, f Filter) bool {
	if f.Group == "active" && !taskrecord.Active(v.State) || f.Group == "attention" && v.State != "failed" && v.State != "needs_action" || f.Group == "ended" && !slices.Contains([]string{"succeeded", "failed", "cancelled"}, v.State) {
		return false
	}
	q := strings.ToLower(f.Q)
	return q == "" || strings.Contains(strings.ToLower(v.Title), q) || strings.HasPrefix(v.ID, q) || strings.HasPrefix(v.Source.RootID, q)
}
func (s *Service) listTx(ctx context.Context, tx pgx.Tx, a access.Actor, f Filter, rev string, all bool) (Page, error) {
	out := Page{Items: []View{}, Coverage: "complete", ObservedAt: time.Now().UTC()}
	auth, e := authorizationVector(ctx, tx, a.ID, rev)
	if e != nil {
		return out, e
	}
	out.AuthorizationRevision = auth
	var epoch, secret string
	if e = tx.QueryRow(ctx, `SELECT restore_epoch::text,cursor_secret,mode FROM task_protocol WHERE id=1 AND schema_version=1`).Scan(&epoch, &secret, &out.IndexState); e != nil {
		return out, e
	}
	var pending bool
	if e = tx.QueryRow(ctx, `SELECT pending OR EXISTS(SELECT 1 FROM task_source_states WHERE last_error_code<>'') FROM task_index_backlog`).Scan(&pending); e != nil {
		return out, e
	}
	if pending && out.IndexState == "ready" {
		out.IndexState = "syncing"
	}
	if out.IndexState != "ready" {
		out.Coverage = "partial"
	}
	filters := f
	filters.Cursor = ""
	key := taskrecord.Hash(filters)
	c := cursor{Account: a.ID, Filter: key, Authorization: auth, Epoch: epoch, Upper: out.ObservedAt}
	if f.Cursor != "" {
		c, e = decode(f.Cursor, secret)
		if e != nil {
			return out, e
		}
		if c.Account != a.ID || c.Filter != key || c.Authorization != auth || c.Epoch != epoch {
			return out, stale()
		}
	}
	_, keys, e := s.catalogTx(ctx, tx, a, f)
	if e != nil {
		return out, e
	}
	if len(keys) == 0 {
		return out, nil
	}
	// Titles are catalog-defined by Present, not mutable source metadata. Narrow
	// candidates by that catalog or literal ID prefixes before the expensive live
	// source check; every surviving candidate still needs current authorization.
	search := strings.ToLower(f.Q)
	searchTypes := []string{}
	for _, definition := range taskrecord.Catalog() {
		if strings.Contains(strings.ToLower(definition.Title), search) {
			searchTypes = append(searchTypes, definition.Type)
		}
	}
	scanned := 0
	// Only these constants reach SQL. The signed cursor binds the sort and the
	// canonical type set, so changing either cannot reuse a different page order.
	direction, comparison := "DESC", "<"
	if f.Sort == "oldest" {
		direction, comparison = "ASC", ">"
	}
	boundaryAt, boundaryID := c.At, c.ID
	needed := f.Limit + 1
	if all {
		needed = s.options.CandidateBudget + 1
	}
	for scanned < s.options.CandidateBudget {
		batch := min(s.options.CandidateBatch, s.options.CandidateBudget-scanned)
		if !all {
			// Authorize only the candidates needed to fill this page plus its
			// lookahead. Hidden rows continue through subsequent bounded batches.
			batch = min(batch, needed-len(out.Items))
		}
		var at any
		if !boundaryAt.IsZero() {
			at = boundaryAt
		}
		rows, e := tx.Query(ctx, fmt.Sprintf(`SELECT id::text,snapshot FROM task_records r WHERE origin_kind IN ('person','legacy_unknown') AND ($1<>'mine' OR originator_account_id=$2) AND scope_kind||':'||COALESCE(tenant_id::text,'')||':'||COALESCE(project_id::text,'')||':'||task_type=ANY($3::text[]) AND created_at<=$4 AND ($5::timestamptz IS NULL OR (created_at,id)%s($5::timestamptz,$6::uuid)) AND ($7::timestamptz IS NULL OR created_at>=$7) AND ($8::timestamptz IS NULL OR created_at<=$8) AND ($10='' OR task_type=ANY($11::text[]) OR left(r.id::text,length($10))=$10 OR left(r.source_root_id::text,length($10))=$10) AND NOT EXISTS(SELECT 1 FROM task_relations rel WHERE rel.child_task_id=r.id AND relation_kind='member') ORDER BY created_at %s,id %s LIMIT $9`, comparison, direction, direction), f.Mode, a.ID, keys, c.Upper, at, taskrecord.Null(boundaryID), taskrecord.Null(f.From), taskrecord.Null(f.To), batch, search, searchTypes)
		if e != nil {
			return out, e
		}
		type candidate struct {
			id string
			c  taskrecord.Change
		}
		candidates := []candidate{}
		groups := map[string][]taskrecord.SourceRef{}
		for rows.Next() {
			var v candidate
			if e = rows.Scan(&v.id, &v.c); e != nil {
				rows.Close()
				return out, e
			}
			candidates = append(candidates, v)
			groups[v.c.Module] = append(groups[v.c.Module], v.c.SourceRef)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
		views := map[string]View{}
		for module, refs := range groups {
			source := s.sources[module]
			if source == nil {
				return out, Problem("task_type_unavailable", "任务来源版本不兼容。", 503)
			}
			resolved, e := source.ResolveTx(ctx, tx, a, refs, f.Mode)
			if e != nil {
				return out, unavailable()
			}
			for _, v := range resolved {
				views[v.ID] = v
			}
		}
		for _, v := range candidates {
			scanned++
			boundaryAt, boundaryID = v.c.CreatedAt, v.id
			view, ok := views[v.id]
			if !ok {
				var err error
				view, err = s.retainedTx(ctx, tx, a, v.c, f.Mode)
				if err != nil && !Denied(err) {
					return out, err
				}
				ok = err == nil
			}
			if ok && matches(view, f) {
				out.Items = append(out.Items, view)
			}
			if len(out.Items) >= needed {
				last := out.Items[f.Limit-1]
				c.At, c.ID = last.CreatedAt, last.ID
				out.NextCursor = sign(c, secret)
				out.Items = out.Items[:f.Limit]
				return out, nil
			}
		}
		if len(candidates) < batch {
			return out, nil
		}
	}
	var more bool
	if e = tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM task_records r WHERE origin_kind IN ('person','legacy_unknown') AND ($1<>'mine' OR originator_account_id=$2) AND scope_kind||':'||COALESCE(tenant_id::text,'')||':'||COALESCE(project_id::text,'')||':'||task_type=ANY($3::text[]) AND created_at<=$4 AND (created_at,id)%s($5::timestamptz,$6::uuid) AND ($7::timestamptz IS NULL OR created_at>=$7) AND ($8::timestamptz IS NULL OR created_at<=$8) AND ($9='' OR task_type=ANY($10::text[]) OR left(r.id::text,length($9))=$9 OR left(r.source_root_id::text,length($9))=$9) AND NOT EXISTS(SELECT 1 FROM task_relations rel WHERE rel.child_task_id=r.id AND relation_kind='member'))`, comparison), f.Mode, a.ID, keys, c.Upper, boundaryAt, boundaryID, taskrecord.Null(f.From), taskrecord.Null(f.To), search, searchTypes).Scan(&more); e != nil {
		return out, e
	}
	if !more {
		return out, nil
	}
	return out, budget()
}
func (s *Service) List(ctx context.Context, a access.Actor, f Filter) (Page, error) {
	out := Page{}
	if e := f.validate(s.options); e != nil {
		return out, e
	}
	limited, cancel := context.WithTimeout(ctx, s.options.MetadataTimeout)
	defer cancel()
	e := s.transaction(limited, a, func(tx pgx.Tx, rev string) error {
		var e error
		out, e = s.listTx(limited, tx, a, f, rev, false)
		return e
	})
	if errors.Is(e, context.DeadlineExceeded) || limited.Err() != nil {
		e = budget()
	}
	return out, e
}
func (s *Service) Summary(ctx context.Context, a access.Actor, f Filter) (Summary, error) {
	out := Summary{CountsState: "unavailable", ObservedAt: time.Now().UTC()}
	if e := f.validate(s.options); e != nil {
		return out, e
	}
	f.Cursor = ""
	limited, cancel := context.WithTimeout(ctx, s.options.MetadataTimeout)
	defer cancel()
	e := s.transaction(limited, a, func(tx pgx.Tx, rev string) error {
		page, e := s.listTx(limited, tx, a, f, rev, true)
		out.AuthorizationRevision = page.AuthorizationRevision
		if e != nil {
			return e
		}
		if page.Coverage != "complete" {
			return nil
		}
		active, attention, ended, total := 0, 0, 0, len(page.Items)
		for _, v := range page.Items {
			if taskrecord.Active(v.State) {
				active++
			}
			if v.State == "failed" || v.State == "needs_action" {
				attention++
			}
			if slices.Contains([]string{"succeeded", "failed", "cancelled"}, v.State) {
				ended++
			}
		}
		out.CountsState = "exact"
		out.Active = &active
		out.Attention = &attention
		out.Ended = &ended
		out.Total = &total
		return nil
	})
	var ae *access.Error
	if errors.As(e, &ae) && ae.Code == "task_query_budget_exceeded" || limited.Err() != nil {
		return out, nil
	}
	return out, e
}
func (s *Service) Catalog(ctx context.Context, a access.Actor, f Filter) ([]taskrecord.Definition, error) {
	out := []taskrecord.Definition{}
	if e := f.validate(s.options); e != nil {
		return out, e
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error { var e error; out, _, e = s.catalogTx(ctx, tx, a, f); return e })
	return out, e
}
func (s *Service) Operations(ctx context.Context, a access.Actor, f Filter) ([]Operation, error) {
	out := []Operation{}
	if e := f.validate(s.options); e != nil {
		return out, e
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		for _, key := range []string{"monitoring", "derived", "spatial", "display", "files", "audit"} {
			src := s.sources[key]
			if src == nil {
				continue
			}
			v, e := src.OperationsTx(ctx, tx, a, f)
			if e != nil {
				return e
			}
			out = append(out, v...)
		}
		return nil
	})
	return out, e
}
func pageError(e error) error {
	if e == pgx.ErrNoRows {
		return Hidden()
	}
	if e != nil {
		return fmt.Errorf("task metadata: %w", e)
	}
	return nil
}
