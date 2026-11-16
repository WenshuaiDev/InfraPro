package parsing

import (
	"context"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

const runJSON = `to_jsonb(r)||jsonb_build_object('method_name',r.input_snapshot#>>'{entry,name}','current_generation',r.current_generation::text,'revision',r.revision::text,'control_revision',r.control_revision::text,'state',e.state,'error_code',e.error_code)`

func runTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (Run, error) {
	v, e := readJSON[Run](tx.QueryRow(ctx, `SELECT `+runJSON+` FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.scope_key=$1 AND r.id=$2`, scopeKey(scope), id))
	v.ScopeKey = scopeKey(scope)
	v.TaskRef = taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "parsing", Kind: "run", RootID: v.ID}, Generation: v.Generation}
	v.TaskRef.TaskID = v.TaskRef.SourceRef.ID()
	return v, e
}
func runScope(r Run) Scope {
	if r.TenantID == nil {
		return platform()
	}
	return tenant(*r.TenantID)
}
func (s *Service) authorizeRunTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, r Run, execute bool) error {
	if auth.Entry == "project" || scopeKey(auth.Scope) != r.ScopeKey {
		return k.NotFound("parsing")
	}
	if execute {
		if e := require(auth, access.ParsingTestExecute, access.TenantParsingTestExecute); e != nil {
			return e
		}
		if e := require(auth, access.ParsingMethodRead, access.TenantParsingMethodRead); e != nil {
			return e
		}
	} else if e := require(auth, access.ParsingRunRead, access.TenantParsingRunRead); e != nil {
		return e
	}
	switch r.Kind {
	case "device_test":
		if r.DeviceID == nil {
			return k.NotFound("parsing")
		}
		_, e := deviceTx(ctx, tx, auth, *r.DeviceID)
		return e
	case "product_test":
		if r.ProductVersionID == nil {
			return k.NotFound("parsing")
		}
		_, _, e := productTx(ctx, tx, auth, "", *r.ProductVersionID, false, false)
		return e
	case "method_test":
		return require(auth, access.ParsingMethodRead, access.TenantParsingMethodRead)
	}
	return k.NotFound("parsing")
}
func (s *Service) freezeTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, in RunInput) (FrozenInput, string, string, string, error) {
	var frozen FrozenInput
	kind, deviceID, productVersion := in.Kind, in.DeviceID, ""
	if in.ReplayRunID != "" {
		if in.SampleID != "" || in.MethodID != "" || in.MethodVersion != "" || in.ConfigRevisionID != "" || in.Transport != "" || in.DeviceID != "" {
			return frozen, "", "", "", k.Invalid("/replay_run_id", "frozen_input", "重放只接受原运行引用，不能替换固定输入。")
		}
		old, e := runTx(ctx, tx, auth.Scope, in.ReplayRunID)
		if e != nil {
			return frozen, "", "", "", e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, old, true); e != nil {
			return frozen, "", "", "", e
		}
		frozen = old.Input
		kind = old.Kind
		if old.DeviceID != nil {
			deviceID = *old.DeviceID
		}
		if old.ProductVersionID != nil {
			productVersion = *old.ProductVersionID
		}
		m, e := methodTx(ctx, tx, frozen.Entry.MethodID, frozen.Entry.MethodVersion)
		if e == nil {
			e = allowedMethod(m, auth.Scope, false, kind == "method_test")
		}
		return frozen, kind, deviceID, productVersion, e
	}
	// Serialize a new reference against retention cleanup before reading ready state.
	if _, e := tx.Exec(ctx, `SELECT id FROM parsing_samples WHERE id=$1 FOR UPDATE`, in.SampleID); e != nil {
		return frozen, "", "", "", e
	}
	sample, e := sampleTx(ctx, tx, auth.Scope, in.SampleID)
	if e != nil {
		return frozen, "", "", "", e
	}
	if sample.State != "ready" {
		return frozen, "", "", "", k.Problem("sample_unavailable", "请先完成样本上传确认。", 409)
	}
	frozen = FrozenInput{Sample: sample, Context: c.Context{ReceivedAt: sample.ReceivedAt, Transport: in.Transport}, Mapping: []Mapping{}, TraceID: uuid.NewString()}
	methodID, methodVersion := in.MethodID, in.MethodVersion
	newUse := false
	switch in.Kind {
	case "method_test":
		if in.ConfigRevisionID != "" || in.DeviceID != "" {
			return frozen, "", "", "", k.Invalid("/kind", "context", "方法调试不接受配置或设备引用。")
		}
		newUse = auth.Entry == "tenant"
	case "product_test":
		if in.MethodID != "" || in.MethodVersion != "" || in.DeviceID != "" {
			return frozen, "", "", "", k.Invalid("/kind", "context", "产品试解析只能引用已保存配置。")
		}
		cfg, e := configTx(ctx, tx, auth, in.ConfigRevisionID, false)
		if e != nil {
			return frozen, "", "", "", e
		}
		if cfg.Snapshot.OwnerKind != "product" {
			return frozen, "", "", "", k.NotFound("parsing")
		}
		frozen.Config = &cfg
		productVersion = cfg.ProductVersionID
		methodID = cfg.Snapshot.Method.ID
		methodVersion = cfg.Snapshot.Method.Version
	case "device_test":
		if in.MethodID != "" || in.MethodVersion != "" {
			return frozen, "", "", "", k.Invalid("/kind", "context", "设备试解析只能引用采用或已保存候选配置。")
		}
		d, e := deviceTx(ctx, tx, auth, in.DeviceID)
		if e != nil {
			return frozen, "", "", "", e
		}
		productVersion = d.ProductVersionID
		frozen.DeviceStatus = d.Status
		configID := in.ConfigRevisionID
		if configID == "" {
			state, e := deviceStateTx(ctx, tx, auth, in.DeviceID)
			if e != nil {
				return frozen, "", "", "", e
			}
			if state.Current == nil || state.Current.ConfigRevisionID == nil {
				return frozen, "", "", "", k.Problem("device_unconfigured", "设备尚未采用解析配置。", 409)
			}
			configID = *state.Current.ConfigRevisionID
			frozen.AdoptionID = state.Current.ID
		}
		cfg, e := configTx(ctx, tx, auth, configID, false)
		if e != nil {
			return frozen, "", "", "", e
		}
		if cfg.ProductVersionID != d.ProductVersionID || cfg.Snapshot.OwnerKind == "device" && cfg.Snapshot.OwnerID != in.DeviceID {
			return frozen, "", "", "", k.NotFound("parsing")
		}
		frozen.Config = &cfg
		methodID = cfg.Snapshot.Method.ID
		methodVersion = cfg.Snapshot.Method.Version
	default:
		return frozen, "", "", "", k.Invalid("/kind", "enum", "请选择方法、产品或设备试解析。")
	}
	m, e := methodTx(ctx, tx, methodID, methodVersion)
	if e != nil {
		return frozen, "", "", "", e
	}
	if e = allowedMethod(m, auth.Scope, newUse, in.Kind == "method_test"); e != nil {
		return frozen, "", "", "", e
	}
	if !slices.Contains(m.Entry.Transports, in.Transport) {
		return frozen, "", "", "", k.Invalid("/transport", "unsupported_transport", "该方法不支持所选传输上下文。")
	}
	if sample.ByteLength > m.Entry.MaxLength {
		return frozen, "", "", "", k.Problem("limit_exceeded", "样本超过该方法的完整输入限制。", 413)
	}
	frozen.Entry = m.Entry
	if frozen.Config != nil {
		cfg := frozen.Config
		frozen.Mapping, e = mappingTx(ctx, tx, m.Entry, productVersion, cfg.Snapshot.Bindings, cfg.Snapshot.Ignored, deviceID, false)
		if e != nil {
			frozen.Mapping = slices.Clone(cfg.Snapshot.Mapping)
			for i := range frozen.Mapping {
				frozen.Mapping[i].Status = "semantic_conflict"
				frozen.Mapping[i].Reason = "当前设备输出语义已变化，试解析结果不会正式采用。"
			}
		}
	}
	return frozen, kind, deviceID, productVersion, nil
}
func (s *Service) createRunTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, in RunInput) (Result, error) {
	var out Result
	if !s.options.Enabled {
		return out, k.Problem("execution_disabled", "解析执行尚未启用；目录及历史仍可读取。", 503)
	}
	if s.objects == nil {
		return out, k.Problem("storage_unconfigured", "解析证据存储尚未配置。", 503)
	}
	frozen, kind, deviceID, productVersion, e := s.freezeTx(ctx, tx, auth, in)
	if e != nil {
		return out, e
	}
	var queued int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.scope_key=$1 AND e.state='queued'`, scopeKey(auth.Scope)).Scan(&queued); e != nil {
		return out, e
	}
	if queued >= s.options.Queue {
		return out, k.Problem("queue_budget_exceeded", "当前范围试解析队列已满，请等待已有任务完成。", 429)
	}
	compare := in.ComparedToRunID
	if in.ReplayRunID != "" {
		compare = in.ReplayRunID
	}
	if compare != "" {
		old, e := runTx(ctx, tx, auth.Scope, compare)
		if e != nil {
			return out, e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, old, false); e != nil {
			return out, e
		}
	}
	configID := ""
	if frozen.Config != nil {
		configID = frozen.Config.ID
	}
	e = tx.QueryRow(ctx, `INSERT INTO parsing_runs(scope_key,tenant_id,kind,sample_id,method_id,method_version,config_revision_id,product_version_id,device_id,adoption_id,input_snapshot,compared_to_run_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text`, scopeKey(auth.Scope), k.Nullable(auth.Tenant), kind, frozen.Sample.ID, frozen.Entry.MethodID, frozen.Entry.MethodVersion, k.Nullable(configID), k.Nullable(productVersion), k.Nullable(deviceID), k.Nullable(frozen.AdoptionID), k.JSON(frozen), k.Nullable(compare), auth.Account).Scan(&out.ID)
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO parsing_run_executions(run_id,generation,actor_id,state) VALUES($1,1,$2,'queued')`, out.ID, auth.Account); e != nil {
		return out, e
	}
	out.Generation = "1"
	out.Revision = "1"
	out.State = "queued"
	ref, e := publishTaskTx(ctx, tx, out.ID, "submitted", auth.Account)
	if e != nil {
		return out, e
	}
	out.TaskRef = &ref
	return out, nil
}
func (s *Service) CreateRun(ctx context.Context, a access.Actor, scope Scope, in RunInput) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, false); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "runs", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		v, ok, e := replay(ctx, tx, auth, "runs.create", in.Meta, in)
		if e != nil {
			return e
		}
		if ok {
			r, e := runTx(ctx, tx, scope, v.ID)
			if e != nil {
				return e
			}
			if e = s.authorizeRunTx(ctx, tx, auth, r, true); e != nil {
				return e
			}
			out = v
			return nil
		}
		out, e = s.createRunTx(ctx, tx, auth, in)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, "runs.create", in.Meta, in, out, nil, map[string]string{"run_id": out.ID, "generation": out.Generation})
	})
	return out, e
}
func (s *Service) Compare(ctx context.Context, a access.Actor, scope Scope, in CompareInput) (CompareResult, error) {
	var out CompareResult
	if e := validateMeta(&in.Meta, false); e != nil {
		return out, e
	}
	if in.Left.SampleID == "" || in.Left.SampleID != in.Right.SampleID || in.Left.Transport != in.Right.Transport || in.Left.ReplayRunID != "" || in.Right.ReplayRunID != "" {
		return out, k.Invalid("/right", "comparison_input", "版本对比必须使用同一样本与传输上下文。")
	}
	e := s.transact(ctx, a, scope, "runs", "compare", true, func(tx pgx.Tx, auth k.Authorization) error {
		v, ok, e := replay(ctx, tx, auth, "runs.compare", in.Meta, in)
		if e != nil {
			return e
		}
		if ok {
			left, e := runTx(ctx, tx, scope, v.ID)
			if e != nil {
				return e
			}
			right, e := runTx(ctx, tx, scope, v.OtherID)
			if e != nil {
				return e
			}
			if e = s.authorizeRunTx(ctx, tx, auth, left, false); e != nil {
				return e
			}
			if e = s.authorizeRunTx(ctx, tx, auth, right, false); e != nil {
				return e
			}
			out.Left = Result{ID: left.ID, Generation: left.Generation, TaskRef: &left.TaskRef, Replayed: true}
			out.Right = Result{ID: right.ID, Generation: right.Generation, TaskRef: &right.TaskRef, Replayed: true}
			return nil
		}
		out.Left, e = s.createRunTx(ctx, tx, auth, in.Left)
		if e != nil {
			return e
		}
		right := in.Right
		right.ComparedToRunID = out.Left.ID
		out.Right, e = s.createRunTx(ctx, tx, auth, right)
		if e != nil {
			return e
		}
		receipt := out.Left
		receipt.OtherID = out.Right.ID
		return complete(ctx, tx, auth, "runs.compare", in.Meta, in, receipt, nil, map[string]string{"run_id": out.Left.ID, "result_id": out.Right.ID})
	})
	return out, e
}

const executionJSON = `to_jsonb(e)||jsonb_build_object('generation',e.generation::text,'fence',e.fence::text)`

func executionTx(ctx context.Context, tx pgx.Tx, id, generation string) (Execution, error) {
	v, e := readJSON[Execution](tx.QueryRow(ctx, `SELECT `+executionJSON+` FROM parsing_run_executions e WHERE run_id=$1 AND generation=$2`, id, generation))
	v.Attempts = []Attempt{}
	if e != nil {
		return v, e
	}
	rows, e := tx.Query(ctx, `SELECT to_jsonb(a)||jsonb_build_object('fence',a.fence::text) FROM parsing_run_attempts a WHERE run_id=$1 AND generation=$2 ORDER BY started_at,execution_id`, id, generation)
	if e != nil {
		return v, e
	}
	defer rows.Close()
	for rows.Next() {
		a, e := readJSON[Attempt](rows)
		if e != nil {
			return v, e
		}
		v.Attempts = append(v.Attempts, a)
	}
	return v, rows.Err()
}
func (s *Service) Run(ctx context.Context, a access.Actor, scope Scope, id string) (RunDetail, error) {
	out := RunDetail{Executions: []Execution{}}
	e := s.transact(ctx, a, scope, "runs", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		r, e := runTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, r, false); e != nil {
			return e
		}
		out.Run = r
		out.CanReadSample = require(auth, access.ParsingSampleRead, access.TenantParsingSampleRead) == nil
		rows, e := tx.Query(ctx, `SELECT generation::text FROM parsing_run_executions WHERE run_id=$1 ORDER BY generation DESC LIMIT 100`, id)
		if e != nil {
			return e
		}
		gens := []string{}
		for rows.Next() {
			var g string
			if e = rows.Scan(&g); e != nil {
				rows.Close()
				return e
			}
			gens = append(gens, g)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, g := range gens {
			x, e := executionTx(ctx, tx, id, g)
			if e != nil {
				return e
			}
			out.Executions = append(out.Executions, x)
		}
		if r.DeviceID != nil {
			current, e := deviceStateTx(ctx, tx, auth, *r.DeviceID)
			if e != nil {
				return e
			}
			out.CurrentChanged = current.Current == nil || current.Current.ID != r.Input.AdoptionID || k.Hash(current.Mapping) != k.Hash(r.Input.Mapping)
		}
		return nil
	})
	return out, e
}
func (s *Service) Execution(ctx context.Context, a access.Actor, scope Scope, id, generation string) (Execution, error) {
	var out Execution
	if !k.Revision(generation, false) {
		return out, k.NotFound("parsing")
	}
	e := s.transact(ctx, a, scope, "runs", "execution", false, func(tx pgx.Tx, auth k.Authorization) error {
		r, e := runTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, r, false); e != nil {
			return e
		}
		out, e = executionTx(ctx, tx, id, generation)
		return e
	})
	return out, e
}
func (s *Service) Runs(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Run], error) {
	out := k.Page[Run]{Items: []Run{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "runs", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "runs"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+runJSON+` FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.scope_key=$1 AND ($2::uuid IS NULL OR r.id>$2) AND ($3='' OR e.state=$3) AND ($4::uuid IS NULL OR r.device_id=$4) AND ($5='' OR r.method_id ILIKE '%'||$5||'%') ORDER BY r.id LIMIT 2000`, scopeKey(scope), k.Nullable(cur.ID), f.State, k.Nullable(f.DeviceID), f.Query)
		if e != nil {
			return e
		}
		candidates := []Run{}
		for rows.Next() {
			v, e := readJSON[Run](rows)
			if e != nil {
				rows.Close()
				return e
			}
			v.ScopeKey = scopeKey(scope)
			candidates = append(candidates, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		last := ""
		for _, v := range candidates {
			last = v.ID
			if e = s.authorizeRunTx(ctx, tx, auth, v, false); e != nil {
				if denied(e) {
					continue
				}
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			v.Input = FrozenInput{}
			v.TaskRef = taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "parsing", Kind: "run", RootID: v.ID}, Generation: v.Generation}
			v.TaskRef.TaskID = v.TaskRef.SourceRef.ID()
			out.Items = append(out.Items, v)
		}
		if out.NextCursor == "" && len(candidates) == 2000 {
			out.NextCursor = k.Cursor{Scope: cur.Scope, ID: last}.Encode()
		}
		return nil
	})
	return out, e
}
func denied(e error) bool {
	if e == access.ErrDenied {
		return true
	}
	if v, ok := e.(interface{ StatusCode() int }); ok {
		return v.StatusCode() == 403 || v.StatusCode() == 404
	}
	return false
}
func (s *Service) Result(ctx context.Context, a access.Actor, scope Scope, id, generation string, raw bool, offset, limit int) (ResultView, []byte, error) {
	out := ResultView{State: "none", Mapping: []Mapping{}, Quality: "not_evaluated", Adopted: false}
	if !k.Revision(generation, false) || offset < 0 || limit < 1 || limit > 200 {
		return out, nil, k.Problem("invalid_request", "结果分页范围无效。", 400)
	}
	var run Run
	var execution Execution
	var o object
	canRaw := false
	action := "result"
	if raw {
		action = "response"
	}
	authorize := func() error {
		return s.transact(ctx, a, scope, "runs", action, false, func(tx pgx.Tx, auth k.Authorization) error {
			var e error
			run, e = runTx(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			if e = s.authorizeRunTx(ctx, tx, auth, run, false); e != nil {
				return e
			}
			canRaw = require(auth, access.ParsingSampleRead, access.TenantParsingSampleRead) == nil
			if raw && !canRaw {
				return access.ErrDenied
			}
			execution, e = executionTx(ctx, tx, id, generation)
			if e != nil {
				return e
			}
			if execution.ResultObjectID != nil {
				o, e = objectTx(ctx, tx, *execution.ResultObjectID)
			}
			return e
		})
	}
	if e := authorize(); e != nil {
		return out, nil, e
	}
	out.Mapping = execution.Mapping
	if execution.ResultObjectID == nil {
		out.State = execution.State
		return out, nil, nil
	}
	out.ResultDigest = o.Hash
	b, e := s.readObject(ctx, o, c.MaxResponse)
	if e != nil {
		if codeOf(e) == "result_missing" {
			out.State = "missing"
			return out, nil, nil
		}
		return out, nil, e
	}
	if e = authorize(); e != nil {
		return out, nil, e
	}
	if raw {
		e = s.transact(ctx, a, scope, "runs", "response", true, func(tx pgx.Tx, auth k.Authorization) error {
			return access.Audit(ctx, tx, "parsing.runs.response", "parsing_runs", id, "", nil, map[string]string{"run_id": id, "generation": generation}, access.Meta{ActorID: a.ID, ActorScope: auth.Entry, TenantID: auth.Tenant})
		})
		return out, b, e
	}
	var response c.Response
	if c.StrictJSON(b, &response) != nil || execution.ErrorCode == "invalid_response" {
		out.State = "quarantined"
		return out, nil, nil
	}
	if !canRaw {
		response = c.RedactRaw(response)
	}
	out.State = "available"
	total := max(len(response.Steps), len(response.Records), len(response.Fields))
	if offset+limit < total {
		out.NextCursor = fmtNumber(offset + limit)
	}
	response.Steps = response.Steps[min(offset, len(response.Steps)):min(offset+limit, len(response.Steps))]
	response.Records = response.Records[min(offset, len(response.Records)):min(offset+limit, len(response.Records))]
	response.Fields = response.Fields[min(offset, len(response.Fields)):min(offset+limit, len(response.Fields))]
	out.Response = &response
	return out, nil, nil
}
func fmtNumber(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return strings.TrimSpace(digits)
}
