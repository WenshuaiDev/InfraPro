package display

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"net/url"
	"slices"
	"strings"
	"time"
)

func (s *Service) sourceVectorTx(ctx context.Context, tx pgx.Tx, v Session, ids []string) (map[string]string, error) {
	out := map[string]string{"environment": v.Environment, "config": v.ConfigID, "grant": v.Authorization}
	for name, q := range map[string]string{
		"raw":             `SELECT COALESCE((SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1),'0')`,
		"derived":         `SELECT COALESCE(md5(string_agg(channel_id::text||':'||head_revision::text,',' ORDER BY channel_id)),'0') FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=ANY($2::uuid[])`,
		"resources":       `SELECT COALESCE(md5(string_agg(id::text||':'||revision::text||':'||COALESCE(deleted_at::text,''),',' ORDER BY id)),'0') FROM engineering_resource_nodes WHERE tenant_id=$1 AND id=ANY($2::uuid[])`,
		"control":         `SELECT COALESCE(max(id),0)::text FROM display_events WHERE tenant_id=$1 AND project_id=$3 AND (instance_id=$4 OR instance_id IS NULL) AND kind NOT IN ('query_failed','budget_rejected','resynchronized','session_ended')`,
		"data_grants":     `SELECT COALESCE((SELECT revision::text FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$3),'0')`,
		"resource_grants": `SELECT revision::text FROM project_resource_policies WHERE tenant_id=$1 AND project_id=$3`,
	} {
		args := []any{v.Tenant, ids, v.Project, v.InstanceID}
		q = "WITH display_input AS (SELECT $1::uuid tenant,$2::uuid[] ids,$3::uuid project,$4::uuid instance) " + q
		var value string
		if e := tx.QueryRow(ctx, q, args...).Scan(&value); e != nil {
			return nil, e
		}
		out[name] = value
	}
	rev, e := access.Lock(ctx, tx, false)
	out["authorization"] = rev
	return out, e
}
func (s *Service) Query(ctx context.Context, v Session, call Call) (result Envelope, queryErr error) {
	parent := ctx
	started := time.Now()
	defer func() {
		s.Measure("queries", started, queryErr != nil)
		if queryErr != nil && parent.Err() == nil {
			code, kind := "dependency_unavailable", "query_failed"
			if de, ok := queryErr.(*Error); ok {
				code = de.Code
			}
			if code == "budget_exceeded" {
				kind = "budget_rejected"
			}
			s.RecordDiagnostic(parent, v, kind, code)
		}
	}()
	if newReadCall(call) {
		return s.readQuery(ctx, v, call)
	}
	out := Envelope{Generation: call.Generation, ReadAt: time.Now().UTC().Format(time.RFC3339Nano)}
	capName, ok := methodCapabilities[call.Method]
	if !ok {
		return out, invalid("/method", "SDK 读取方法未注册。")
	}
	if s.descriptors[capName].Availability == "unavailable" || s.providers[capName] == nil {
		return out, problem("capability_unavailable", "该业务能力目前不可用。", 409)
	}
	q, e := decodeQuery(call.Params)
	if e != nil {
		return out, e
	}
	if q.Length > s.options.MaxFileBytes {
		return out, problem("budget_exceeded", "单次文件读取超过字节预算。", 429)
	}
	if len(call.Generation) > 80 {
		return out, invalid("/generation", "查询代次过长。")
	}
	done, e := s.budget.Take(map[string]int{"tenant:" + v.Tenant: s.options.MaxQueries * 8, "project:" + v.Project: s.options.MaxQueries * 4, "instance:" + v.InstanceID: s.options.MaxQueries * 2, "session:" + v.ID: s.options.MaxQueries, "method:" + capName: s.options.MaxQueries * 8})
	if e != nil {
		return out, e
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var ids []string
	var grant Grant
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
		if e != nil {
			return e
		}
		v = current
		g, _, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return e
		}
		grant = g
		if !slices.Contains(g.Capabilities, capName) {
			return hidden()
		}
		for _, field := range fieldsFor(call.Method) {
			if !slices.Contains(g.Fields, field) {
				return hidden()
			}
		}
		ids, e = s.resolveTargetsTx(ctx, tx, v, call.Method, &q)
		if e != nil {
			return e
		}
		out.SourceVector, e = s.sourceVectorTx(ctx, tx, v, ids)
		return e
	})
	if e != nil {
		return out, e
	}
	out.Authorization = hash([]any{out.SourceVector["authorization"], out.SourceVector["data_grants"], out.SourceVector["resource_grants"], scopeFingerprint(grant)})
	call.Params = jsonBytes(q)
	// Query results, including cursors, belong to a durable subject. Sharing does
	// not make another subject's snapshot reusable.
	cacheKey := hash([]any{v.SubjectID, v.Environment, v.ConfigID, out.Authorization, out.SourceVector, call.Method, q})
	data, e := s.budget.Share(ctx, cacheKey, func(shared context.Context) (any, error) {
		return s.providers[capName].Read(shared, s.subject(v), call)
	})
	if e != nil {
		return out, e
	}
	if int64(len(jsonBytes(data))) > s.options.MaxResultBytes {
		return out, problem("budget_exceeded", "返回结果超过预算，请缩小范围或分页。", 429)
	}
	// Slow object/ClickHouse reads must recheck the current lease and every
	// authorization ceiling before their result can leave the runtime.
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
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
		after, e := s.sourceVectorTx(ctx, tx, current, ids)
		if e != nil {
			return e
		}
		if after["authorization"] != out.SourceVector["authorization"] || after["data_grants"] != out.SourceVector["data_grants"] || after["resource_grants"] != out.SourceVector["resource_grants"] {
			return problem("reset_required", "读取期间授权范围发生变化，请重新建立查询。", 409)
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	out.Data = data
	out.State = resultState(data)
	if slices.Contains([]string{"failed", "needs_action", "canceled"}, out.State) {
		return out, problem("query_failed", "业务查询未完成，请检查数据准备状态。", 409)
	}
	_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE display_sessions SET last_data_at=CASE WHEN $2<>'pending' THEN clock_timestamp() ELSE last_data_at END,last_query_state=$2 WHERE id=$1 AND state IN ('starting','ready','degraded')`, v.ID, out.State)
		return e
	})
	out.ReadAt = time.Now().UTC().Format(time.RFC3339Nano)
	return out, nil
}
func resultState(data any) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(jsonBytes(data), &obj) != nil {
		return "ready"
	}
	var state string
	_ = json.Unmarshal(obj["state"], &state)
	if slices.Contains([]string{"pending", "empty", "partial", "ready", "failed", "needs_action", "canceled"}, state) {
		return state
	}
	if raw, ok := obj["items"]; ok && string(raw) == "[]" {
		return "empty"
	}
	return "ready"
}
func (s *Service) Bindings(ctx context.Context, v Session, name string) (map[string]any, error) {
	out := map[string]any{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		g, config, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return e
		}
		pack, e := packageTx(ctx, tx, v.InstanceID, v.PackageID)
		if e != nil {
			return e
		}
		person := v.AccountID
		if person == "" {
			person = g.IssuedBy
		}
		auth, e := access.ProjectSnapshotTx(ctx, tx, person, v.Tenant, v.Project, "")
		if e != nil {
			return e
		}
		for key, b := range config.Bindings {
			if name != "" && key != name {
				continue
			}
			compatible := false
			for _, capName := range g.Capabilities {
				if bindingKind(capName) != b.Kind {
					continue
				}
				permitted := true
				for _, p := range s.permissionsFor([]string{capName}) {
					if !slices.Contains(auth.Permissions, p) {
						permitted = false
					}
				}
				if permitted && s.providers[capName] != nil {
					compatible = true
					break
				}
			}
			m := pack.Manifest
			m.Bindings = map[string]BindingSchema{key: m.Bindings[key]}
			if !compatible || s.validateBindingsTx(ctx, tx, v.Scope, m, map[string]Binding{key: b}, &g, true) != nil {
				if name != "" {
					return problem("binding_invalid", "绑定当前不可用。", 409)
				}
				out[key] = map[string]any{"kind": b.Kind, "ids": []string{}, "revision": "", "state": "unavailable", "code": "binding_invalid"}
				continue
			}
			if s.qualifyBindings != nil {
				if e = s.qualifyBindings(ctx, tx, s.subject(v), map[string]Binding{key: b}); e != nil {
					if name != "" {
						return problem("binding_invalid", "绑定当前不可用。", 409)
					}
					out[key] = map[string]any{"kind": b.Kind, "ids": []string{}, "revision": "", "state": "unavailable", "code": "binding_invalid"}
					continue
				}
			}
			ids, e := effectiveBindingIDsTx(ctx, tx, v.Scope, b, &g)
			if e != nil {
				return e
			}
			out[key] = map[string]any{"kind": b.Kind, "ids": ids, "revision": hash(ids), "state": "ready", "from": b.From, "to": b.To}
			if len(ids) == 0 {
				out[key] = map[string]any{"kind": b.Kind, "ids": ids, "revision": hash(ids), "state": "empty"}
			}
		}
		if name != "" && out[name] == nil {
			return problem("binding_invalid", "命名绑定不存在。", 404)
		}
		return nil
	})
	return out, e
}
func (s *Service) Navigation(ctx context.Context, v Session, kind, id string) (string, error) {
	if v.TerminalID != "" && kind != "display" {
		return "", problem("person_login_required", "此页面需要人员登录。", 403)
	}
	if !access.ValidID(id) {
		return "", hidden()
	}
	var target string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		_, _, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return e
		}
		switch kind {
		case "display":
			other, e := instanceTx(ctx, tx, v.Scope, id)
			if e != nil {
				return e
			}
			if v.AccountID != "" {
				auth, e := access.RequireProjectTx(ctx, tx, v.AccountID, access.DisplayOperation("launch"), v.Tenant, v.Project)
				if e != nil || !canView(auth, other) {
					return hidden()
				}
			} else if id != v.InstanceID {
				return hidden()
			}
			target = s.options.RuntimeOrigin + "/display/" + id
		case "resource", "monitoring":
			q := Query{ID: id}
			method := "resources.get"
			if kind == "monitoring" {
				method = "channels.get"
			}
			if _, e = s.resolveTargetsTx(ctx, tx, v, method, &q); e != nil {
				return e
			}
			if kind == "monitoring" {
				target = strings.TrimSuffix(s.options.ConsoleOrigin, "/") + "/t/" + v.Tenant + "/p/" + v.Project + "/monitoring?channel=" + id
			} else {
				var resourceKind, facility, parentKind, parent string
				if e = tx.QueryRow(ctx, `SELECT kind,facility_id::text,COALESCE(parent_kind,''),COALESCE(parent_id::text,'') FROM engineering_resource_nodes WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, v.Tenant, id).Scan(&resourceKind, &facility, &parentKind, &parent); e != nil {
					return hidden()
				}
				target = strings.TrimSuffix(s.options.ConsoleOrigin, "/") + "/t/" + v.Tenant + "/p/" + v.Project + "/resources/facilities/" + facility
				search := url.Values{}
				if resourceKind != "facility" {
					search.Set(resourceKind, id)
				}
				if resourceKind == "channel" && parentKind == "point" {
					search.Set("point", parent)
				}
				if len(search) > 0 {
					target += "?" + search.Encode()
				}
			}
		default:
			return invalid("/target", "不支持的跳转目标。")
		}
		return nil
	})
	return target, e
}

func fieldsFor(method string) []string {
	out := []string{"metadata"}
	if strings.HasPrefix(method, "monitoring.") || strings.HasPrefix(method, "artifacts.") {
		out = append(out, "values", "quality")
	}
	if strings.HasPrefix(method, "sources.") {
		out = append(out, "source")
	}
	if strings.HasPrefix(method, "spatial.") {
		out = append(out, "properties")
	}
	return out
}
