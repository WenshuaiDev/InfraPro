package displayread

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// Materialized contains only authorized public projections. Rows are stable
// ordered directory items or groups; Metadata contains full-scope totals.
type Materialized struct {
	Rows         []json.RawMessage
	Metadata     map[string]any
	Single       any
	State        string
	Completeness Completeness
	// Work and Proof are private owning-domain state, never response metadata.
	Work     json.RawMessage
	Proof    json.RawMessage
	Sections map[string][]json.RawMessage
}
type Handler func(context.Context, pgx.Tx, Plan) (Materialized, error)
type Executor func(context.Context, Plan, json.RawMessage) (Materialized, error)
type Validator func(context.Context, pgx.Tx, Plan, json.RawMessage) error
type Service struct {
	access           *access.Service
	handlers         map[string]Handler
	executors        map[string]Executor
	validators       map[string]Validator
	authority        func(context.Context, Owner) (Authority, error)
	drilldown        DrilldownHandler
	selectionPrepare SelectionHandler
	releaseWork      func(context.Context, pgx.Tx, Plan, json.RawMessage) error
	bindWork         func(context.Context, pgx.Tx, string, json.RawMessage) error
}

func New(a *access.Service) *Service {
	return &Service{access: a, handlers: map[string]Handler{}, executors: map[string]Executor{}, validators: map[string]Validator{}}
}
func (s *Service) WithAuthority(fn func(context.Context, Owner) (Authority, error)) { s.authority = fn }
func (s *Service) RegisterWork(method string, prepare Handler, run Executor, check Validator) {
	s.Register(method, prepare)
	s.executors[method] = run
	s.validators[method] = check
}
func (s *Service) Register(method string, h Handler) {
	if h == nil || s.handlers[method] != nil {
		panic("invalid display read handler")
	}
	if _, e := NewRequest(method); e != nil {
		panic(e)
	}
	s.handlers[method] = h
}
func (s *Service) RegisterValidator(method string, check Validator) { s.validators[method] = check }
func (s *Service) Methods() []string {
	out := []string{}
	for method := range s.handlers {
		out = append(out, method)
	}
	return out
}
func (s *Service) Read(ctx context.Context, a Authority, method string, raw json.RawMessage, generation string) (out Result, err error) {
	if len(generation) > 80 {
		return out, Invalid("/generation", "查询代次过长。")
	}
	request, e := DecodeRequest(method, raw)
	if e != nil {
		return out, e
	}
	ctx, cancel := context.WithTimeout(ctx, SyncDeadline)
	defer cancel()
	if q, ok := request.(*QueryRequest); ok {
		return s.get(ctx, a, q.QueryID, q.Cursor, q.Limit, "", "", generation, q.Section)
	}
	if q, ok := request.(*QueryControlRequest); ok {
		return s.control(ctx, a, method, q.QueryID, generation)
	}
	if q, ok := request.(*DrilldownRequest); ok {
		return s.createSelection(ctx, a, q, generation)
	}
	if q := paging(request); q != nil && q.QueryID != "" {
		return s.get(ctx, a, q.QueryID, q.Cursor, q.Limit, method, requestHash(request), generation)
	}
	var savedID string
	defer func() {
		if err != nil && savedID != "" {
			cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer done()
			_ = s.access.Transact(cleanup, func(tx pgx.Tx) error { return s.cancelOwnedTx(cleanup, tx, a, savedID) })
		}
	}()
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		// One metadata snapshot is materialized; no transaction survives an HTTP
		// request or is held open for a client's next page.
		if _, e := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); e != nil {
			return e
		}
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		p, prepared, e := s.preparePlanTx(ctx, tx, a, method, request)
		if e != nil {
			return e
		}
		if p.ParentQuery == "" {
			cached, err := s.copyCacheTx(ctx, tx, p)
			if err != nil {
				return err
			}
			if cached != "" {
				savedID = cached
				return nil
			}
		}
		if s.executors[method] != nil {
			if e = s.checkQueueBudgetTx(ctx, tx, a.Owner); e != nil {
				return e
			}
		}
		var data Materialized
		if prepared != nil {
			data = *prepared
		} else if method == "views.list" || method == "views.describe" {
			name := ""
			if q, ok := request.(*ViewRequest); ok {
				name = q.Name
			}
			data.Single, e = a.Views(ctx, tx, name)
		} else if h := s.handlers[method]; h != nil {
			data, e = h(ctx, tx, p)
		} else {
			return Problem("capability_unavailable", "读取实现未安装。", 409)
		}
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		expires := now.Add(QueryLifetime)
		if !p.Query.ExpiresAt.IsZero() && p.Query.ExpiresAt.Before(expires) {
			expires = p.Query.ExpiresAt
		}
		metadataAsOf := p.Query.MetadataAsOf
		p.Query = QueryInfo{Method: method, ScopeHash: Hash(p.Scopes), FilterHash: requestHash(request), MetadataAsOf: now, CreatedAt: now, ExpiresAt: expires, Completeness: data.Completeness}
		if p.Query.Completeness.Scope == "" {
			p.Query.Completeness = Completeness{Scope: "complete", Data: "not_applicable", Reasons: []string{}}
		}
		state := data.State
		if len(data.Work) > 0 {
			state = "queued"
			data.Completeness = Completeness{Scope: "complete", Data: "pending", Reasons: []string{}}
			p.Query.Completeness = data.Completeness
		}
		if state == "" {
			state = "ready"
			if data.Single == nil && len(data.Rows) == 0 && !isStatistics(method) {
				state = "empty"
			}
		}
		meta := data.Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		if data.Single != nil {
			meta["single"] = data.Single
		}
		fields := planFields(p)
		e = tx.QueryRow(ctx, `INSERT INTO display_read_queries(tenant_id,project_id,instance_id,subject_id,session_id,config_id,environment,method,request,scope_hash,filter_hash,authority_hash,source_vector,scopes,included_field_groups,state,metadata,completeness,row_count,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id::text,read_snapshot_id::text,metadata_as_of,created_at`, a.Owner.Tenant, a.Owner.Project, a.Owner.Instance, a.Owner.Subject, a.Owner.Session, a.Owner.Config, a.Owner.Environment, method, Bytes(request), p.Query.ScopeHash, p.Query.FilterHash, Hash([]string{p.Evidence.Authorization, p.Evidence.Safety}), Bytes(p.Evidence), Bytes(p.Scopes), fields, state, Bytes(meta), Bytes(p.Query.Completeness), len(data.Rows), expires).Scan(&p.Query.ID, &p.Query.SnapshotID, &p.Query.MetadataAsOf, &p.Query.CreatedAt)
		if e != nil {
			return e
		}
		savedID = p.Query.ID
		if p.ParentQuery != "" {
			if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET parent_query_id=$2,metadata_as_of=$3 WHERE id=$1`, savedID, p.ParentQuery, metadataAsOf); e != nil {
				return e
			}
		}
		if len(data.Work) > 0 {
			if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET work=$2,proof=$3 WHERE id=$1`, savedID, data.Work, data.Proof); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO display_read_jobs(query_id) VALUES($1)`, savedID); e != nil {
				return e
			}
			if s.bindWork != nil {
				if e = s.bindWork(ctx, tx, savedID, data.Proof); e != nil {
					return e
				}
			}
		} else if len(data.Proof) > 0 {
			if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET proof=$2 WHERE id=$1`, savedID, data.Proof); e != nil {
				return e
			}
		}
		if len(data.Work) == 0 {
			if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET completed_at=clock_timestamp() WHERE id=$1`, savedID); e != nil {
				return e
			}
		}
		_, e = tx.CopyFrom(ctx, pgx.Identifier{"display_read_query_rows"}, []string{"query_id", "ordinal", "sort_key", "data"}, pgx.CopyFromSlice(len(data.Rows), func(i int) ([]any, error) { return []any{p.Query.ID, i, strconv.Itoa(i), data.Rows[i]}, nil }))
		if e != nil {
			return e
		}
		return s.saveSectionsTx(ctx, tx, savedID, data.Sections)
	})
	if e != nil {
		return out, e
	}
	limit := PageSize
	if p := paging(request); p != nil {
		limit = p.Limit
	}
	// A second transaction checks revocation after slow storage/domain reads.
	return s.get(ctx, a, savedID, "", limit, method, requestHash(request), generation)
}
func requestHash(request any) string {
	var value map[string]json.RawMessage
	_ = json.Unmarshal(Bytes(request), &value)
	for _, key := range []string{"query_id", "cursor", "limit"} {
		delete(value, key)
	}
	return Hash(value)
}
func (s *Service) get(ctx context.Context, a Authority, id, cursor string, limit int, method, filter, generation string, sections ...string) (out Result, err error) {
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var info QueryInfo
		var original, state string
		var evidence Evidence
		var metadata map[string]json.RawMessage
		var fields []string
		var count int64
		var proof, requestRaw json.RawMessage
		var scopes map[string]Scope
		var failure string
		e := tx.QueryRow(ctx, `SELECT id::text,method,scope_hash,filter_hash,read_snapshot_id::text,metadata_as_of,created_at,expires_at,completeness,source_vector,metadata,included_field_groups,state,row_count,proof,request,scopes,error_code FROM display_read_queries WHERE id=$1 AND subject_id=$2 AND session_id=$3 AND config_id=$4 AND environment=$5`, id, a.Owner.Subject, a.Owner.Session, a.Owner.Config, a.Owner.Environment).Scan(&info.ID, &original, &info.ScopeHash, &info.FilterHash, &info.SnapshotID, &info.MetadataAsOf, &info.CreatedAt, &info.ExpiresAt, &info.Completeness, &evidence, &metadata, &fields, &state, &count, &proof, &requestRaw, &scopes, &failure)
		if e != nil {
			if e == pgx.ErrNoRows {
				return Hidden()
			}
			return e
		}
		info.Method = original
		if e = s.checkParentTx(ctx, tx, id); e != nil {
			return e
		}
		if method != "" && method != original || filter != "" && filter != info.FilterHash {
			return Problem("invalid_cursor", "查询或筛选与原查询不一致。", 409)
		}
		if method == "" {
			if e = a.Require(ctx, tx, "queries.get"); e != nil {
				return e
			}
		}
		current, e := a.Validate(ctx, tx, original, &evidence)
		if e != nil {
			return e
		}
		if !info.ExpiresAt.After(time.Now()) || state == "expired" {
			return Problem("query_expired", "读取快照已过期。", 409)
		}
		if state == "canceled" {
			return Problem("aborted", "查询已取消或释放。", 409)
		}
		if state == "failed" {
			if failure == "" {
				failure = "query_failed"
			}
			status := 409
			if failure == "dependency_unavailable" {
				status = 503
			}
			return Problem(failure, "后台读取未完成，请检查读取状态后重试。", status)
		}
		if validate := s.validators[original]; validate != nil && len(proof) > 0 {
			request, e := DecodeRequest(original, requestRaw)
			if e != nil {
				return e
			}
			if e = validate(ctx, tx, Plan{Authority: a, Method: original, Request: request, Scopes: scopes, Evidence: evidence, Query: info}, proof); e != nil {
				return e
			}
		}
		if current.LeaseUntil.Before(info.ExpiresAt) {
			info.ExpiresAt = current.LeaseUntil
		}
		out = Result{State: state, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Authorization: current.Authorization, Generation: generation, SourceVector: evidence.Vector, Query: &info, IncludedFields: fields}
		if state == "queued" || state == "running" {
			out.State = "pending"
			out.RetryAfter = 500
			return nil
		}
		if limit == 0 {
			limit = PageSize
			request, e := DecodeRequest(original, requestRaw)
			if e != nil {
				return e
			}
			if page := paging(request); page != nil {
				limit = page.Limit
			}
			// A continuation inherits its cursor's page size, even when the
			// caller explicitly changed the first page size through queries.get.
			if cursor != "" {
				section := ""
				if len(sections) > 0 {
					section = sections[0]
				}
				if section == "" {
					e = tx.QueryRow(ctx, `SELECT page_size FROM display_read_cursors WHERE token=$1 AND query_id=$2`, cursor, id).Scan(&limit)
				} else {
					e = tx.QueryRow(ctx, `SELECT page_size FROM display_read_section_cursors WHERE token=$1 AND query_id=$2 AND section=$3`, cursor, id, section).Scan(&limit)
				}
				if e == pgx.ErrNoRows {
					return Problem("invalid_cursor", "分页游标不匹配。", 409)
				}
				if e != nil {
					return e
				}
			}
		}
		if limit < 1 || limit > pageLimit(original) {
			return Invalid("/limit", "分页大小超过方法预算。")
		}
		section := ""
		if len(sections) > 0 {
			section = sections[0]
		}
		if section != "" {
			page, e := s.sectionPageTx(ctx, tx, id, section, cursor, limit)
			if e != nil {
				return e
			}
			out.Data = page
			return nil
		}
		ordinal := int64(0)
		if cursor != "" {
			var signedSize int
			if e = tx.QueryRow(ctx, `SELECT ordinal,page_size FROM display_read_cursors WHERE token=$1 AND query_id=$2`, cursor, id).Scan(&ordinal, &signedSize); e != nil {
				if e == pgx.ErrNoRows {
					return Problem("invalid_cursor", "分页游标不匹配。", 409)
				}
				return e
			}
			if limit != signedSize {
				return Problem("invalid_cursor", "分页大小与游标不匹配。", 409)
			}
		}
		if single, ok := metadata["single"]; ok {
			out.Data = single
		} else {
			rows, e := tx.Query(ctx, `SELECT data FROM display_read_query_rows WHERE query_id=$1 AND ordinal>=$2 ORDER BY ordinal LIMIT $3`, id, ordinal, limit)
			if e != nil {
				return e
			}
			items := []json.RawMessage{}
			for rows.Next() {
				var item json.RawMessage
				if e = rows.Scan(&item); e != nil {
					rows.Close()
					return e
				}
				items = append(items, item)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			next := ordinal + int64(len(items))
			hasMore := next < count
			token := ""
			if hasMore {
				token = rand.Text() + rand.Text()
				e = tx.QueryRow(ctx, `INSERT INTO display_read_cursors(token,query_id,ordinal,page_size) VALUES($1,$2,$3,$4) ON CONFLICT(query_id,ordinal,page_size) DO UPDATE SET query_id=EXCLUDED.query_id RETURNING token`, token, id, next, limit).Scan(&token)
				if e != nil {
					return e
				}
			}
			data := map[string]any{}
			for key, value := range metadata {
				data[key] = value
			}
			data["query_id"], data["next_cursor"], data["has_more"] = id, token, hasMore
			for _, name := range []string{"effective_windows", "coverage"} {
				var exists bool
				if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_read_query_sections WHERE query_id=$1 AND section=$2)`, id, name).Scan(&exists); e != nil {
					return e
				}
				if exists || original == "statistics.monitoring" {
					page, e := s.sectionPageTx(ctx, tx, id, name, "", limit)
					if e != nil {
						return e
					}
					data[name] = page
				}
			}
			if isStatistics(original) {
				data["groups"] = items
				data["group_count"] = fmt.Sprint(count)
			} else {
				data["items"] = items
				data["total"] = fmt.Sprint(count)
			}
			out.Data = data
		}
		if len(Bytes(out)) > MaxResultBytes {
			return Problem("budget_exceeded", "结果超过 8 MiB，请裁剪字段或减少每页数量。", 429)
		}
		return nil
	})
	return out, err
}
func pageLimit(method string) int {
	if method == "monitoring.details" {
		return MaxDetails
	}
	if method == "map.facilities" || method == "map.features" {
		return MaxMapPageSize
	}
	return MaxPageSize
}
func (s *Service) control(ctx context.Context, a Authority, method, id, generation string) (out Result, err error) {
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if e := a.Require(ctx, tx, method); e != nil {
			return e
		}
		current, e := a.Validate(ctx, tx, method, nil)
		if e != nil {
			return e
		}
		if e = s.cancelOwnedTx(ctx, tx, a, id); e != nil {
			return e
		}
		out = Result{State: "ready", Data: map[string]any{"query_id": id, "released": true}, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Generation: generation, Authorization: current.Authorization, SourceVector: current.Vector, IncludedFields: []string{}}
		return nil
	})
	return out, err
}

// Only a query created for this verified runtime owner can be released. Internal
// request cleanup does not re-authorize data: it removes work after a response
// could no longer be delivered (including a caller disconnect after commit).
func (s *Service) cancelOwnedTx(ctx context.Context, tx pgx.Tx, a Authority, id string) error {
	if _, e := access.Lock(ctx, tx, false); e != nil {
		return e
	}
	var original string
	var proof json.RawMessage
	e := tx.QueryRow(ctx, `SELECT method,proof FROM display_read_queries WHERE id=$1 AND subject_id=$2 AND session_id=$3 AND config_id=$4 AND environment=$5`, id, a.Owner.Subject, a.Owner.Session, a.Owner.Config, a.Owner.Environment).Scan(&original, &proof)
	if e == pgx.ErrNoRows {
		return Hidden()
	}
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `WITH RECURSIVE family AS (SELECT id FROM display_read_queries WHERE id=$1 UNION ALL SELECT q.id FROM display_read_queries q JOIN family f ON q.parent_query_id=f.id) UPDATE display_read_queries SET state='canceled',consumers=0,canceled_at=COALESCE(canceled_at,clock_timestamp()) WHERE id IN(SELECT id FROM family)`, id)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE display_read_jobs j SET state='canceled',fence=fence+1,lease_until=NULL FROM display_read_queries q WHERE q.id=j.query_id AND q.subject_id=$1 AND q.session_id=$2 AND q.state='canceled' AND j.state IN ('queued','running')`, a.Owner.Subject, a.Owner.Session)
	if e != nil {
		return e
	}
	if s.releaseWork != nil && s.executors[original] != nil {
		return s.releaseWork(ctx, tx, Plan{Authority: a, Method: original}, proof)
	}
	return nil
}
