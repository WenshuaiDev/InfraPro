package display

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Command struct {
	ConfigFormat int                               `json:"config_format,omitempty"`
	ReadViews    map[string]displayread.Definition `json:"read_views,omitempty"`
	taskCommand  *tasks.Command
	Meta
	Name        string             `json:"name,omitempty"`
	State       string             `json:"state,omitempty"`
	Enabled     bool               `json:"enabled,omitempty"`
	Viewers     []string           `json:"viewers,omitempty"`
	PackageID   string             `json:"package_id,omitempty"`
	ConfigID    string             `json:"config_id,omitempty"`
	ReleaseID   string             `json:"release_id,omitempty"`
	Preparation string             `json:"preparation,omitempty"`
	Parameters  map[string]any     `json:"parameters,omitempty"`
	Bindings    map[string]Binding `json:"bindings,omitempty"`
	Mode        string             `json:"mode,omitempty"`
	Grant       *Grant             `json:"grant,omitempty"`
	Size        int64              `json:"size,omitempty"`
	SHA256      string             `json:"sha256,omitempty"`
	Challenge   string             `json:"challenge,omitempty"`
	Audience    string             `json:"audience,omitempty"`
	SessionID   string             `json:"-"`
	ValidUntil  time.Time          `json:"valid_until,omitempty"`
	InstanceID  string             `json:"instance_id,omitempty"`
}

func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, action string, fn func(pgx.Tx, access.ProjectAuthorization) error) error {
	if !access.ValidID(scope.Tenant) || !access.ValidID(scope.Project) || a.Check == nil || a.ReadSubject() != nil {
		return hidden()
	}
	op := access.DisplayOperation(action)
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, action != "list" && action != "detail" && action != "upload.get"); e != nil {
			return e
		}
		if e := a.Check(ctx, tx, false); e != nil {
			return e
		}
		auth, e := access.RequireProjectTx(ctx, tx, a.ID, op, scope.Tenant, scope.Project)
		if e != nil {
			return e
		}
		return fn(tx, auth)
	})
}
func settingsTx(ctx context.Context, tx pgx.Tx, scope Scope) (Settings, error) {
	out := Settings{Revision: "0"}
	e := tx.QueryRow(ctx, `SELECT enabled,revision::text FROM display_settings WHERE tenant_id=$1 AND project_id=$2`, scope.Tenant, scope.Project).Scan(&out.Enabled, &out.Revision)
	if e == pgx.ErrNoRows {
		e = nil
	}
	return out, e
}

const instanceSelect = `SELECT id::text,tenant_id::text,project_id::text,name,state,revision::text,COALESCE(current_release_id::text,''),COALESCE(draft_config_id::text,''),viewers::text[],created_at FROM display_instances`

func scanInstance(row pgx.Row) (Instance, error) {
	v := Instance{}
	e := row.Scan(&v.ID, &v.Tenant, &v.Project, &v.Name, &v.State, &v.Revision, &v.Current, &v.Draft, &v.Viewers, &v.CreatedAt)
	if e == pgx.ErrNoRows {
		e = hidden()
	}
	return v, e
}
func instanceTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (Instance, error) {
	if !access.ValidID(id) {
		return Instance{}, hidden()
	}
	return scanInstance(tx.QueryRow(ctx, instanceSelect+` WHERE tenant_id=$1 AND project_id=$2 AND id=$3 FOR UPDATE`, scope.Tenant, scope.Project, id))
}
func manager(auth access.ProjectAuthorization) bool {
	for _, c := range displayManagerCodes() {
		if slices.Contains(auth.Permissions, c) {
			return true
		}
	}
	return false
}
func displayManagerCodes() []access.Code {
	return []access.Code{access.ProjectDisplayManage, access.ProjectDisplaySettings, access.ProjectDisplayUpload, access.ProjectDisplayConfigure, access.ProjectDisplayGrant, access.ProjectDisplayPreview, access.ProjectDisplayPublish, access.ProjectDisplayTerminals}
}
func canView(auth access.ProjectAuthorization, v Instance) bool {
	return manager(auth) || len(v.Viewers) == 0 || slices.Contains(v.Viewers, auth.AccountID)
}
func (s *Service) List(ctx context.Context, a access.Actor, scope Scope) (map[string]any, error) {
	out := map[string]any{}
	e := s.transact(ctx, a, scope, "list", func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		st, e := settingsTx(ctx, tx, scope)
		if e != nil {
			return e
		}
		out["settings"] = st
		out["capabilities"] = s.Capabilities()
		out["runtime_origin"] = s.options.RuntimeOrigin
		rows, e := tx.Query(ctx, instanceSelect+` WHERE tenant_id=$1 AND project_id=$2 AND ($3 OR cardinality(viewers)=0 OR $4::uuid=ANY(viewers)) ORDER BY created_at,id LIMIT 101`, scope.Tenant, scope.Project, manager(auth), a.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []Instance{}
		for rows.Next() {
			v, e := scanInstance(rows)
			if e != nil {
				return e
			}
			items = append(items, v)
		}
		out["items"] = items
		return rows.Err()
	})
	return out, e
}
func jsonRows(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]json.RawMessage, error) {
	rows, e := tx.Query(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
func (s *Service) Detail(ctx context.Context, a access.Actor, scope Scope, id string) (map[string]any, error) {
	out := map[string]any{}
	e := s.transact(ctx, a, scope, "detail", func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		v, e := instanceTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if !canView(auth, v) {
			return hidden()
		}
		out["instance"] = v
		out["access_url"] = s.options.RuntimeOrigin + "/display/" + v.ID
		if !manager(auth) {
			return nil
		}
		for name, q := range map[string]string{
			"packages":  `SELECT jsonb_build_object('id',id,'sha256',sha256,'state',state,'manifest',manifest,'created_at',created_at) FROM display_packages WHERE instance_id=$1 ORDER BY created_at DESC LIMIT 100`,
			"configs":   `SELECT jsonb_build_object('id',id,'package_id',package_id,'parameters',parameters,'bindings',bindings,'config_format',config_format,'read_views',read_views,'mode',mode,'created_at',created_at) FROM display_configs WHERE instance_id=$1 ORDER BY created_at DESC LIMIT 100`,
			"releases":  `SELECT jsonb_build_object('id',id,'package_id',package_id,'config_id',config_id,'requirements',requirements,'withdrawn',withdrawn,'created_at',created_at) FROM display_releases WHERE instance_id=$1 ORDER BY created_at DESC LIMIT 100`,
			"uploads":   `SELECT jsonb_build_object('id',u.id,'size',u.size_bytes::text,'state',u.state,'job',jsonb_build_object('id',j.id,'state',j.state,'issues',j.issues,'error_code',j.error_code)) FROM display_uploads u LEFT JOIN display_package_jobs j ON j.upload_id=u.id WHERE u.instance_id=$1 ORDER BY u.created_at DESC LIMIT 100`,
			"sessions":  `SELECT jsonb_build_object('id',id,'release_id',release_id,'preview',preview,'state',CASE WHEN lease_until<=clock_timestamp() AND state IN ('starting','ready','degraded') THEN 'expired' ELSE state END,'last_seen_at',last_seen_at,'lease_until',lease_until,'last_error',last_error,'last_data_at',last_data_at,'last_query_state',last_query_state) FROM display_sessions WHERE instance_id=$1 ORDER BY created_at DESC LIMIT 100`,
			"terminals": `SELECT jsonb_build_object('id',id,'name',name,'state',state,'generation',generation::text,'instance_id',instance_id,'actual_release_id',actual_release_id,'last_seen_at',last_seen_at,'last_error',last_error,'valid_until',valid_until) FROM display_terminals WHERE instance_id=$1 ORDER BY name,id LIMIT 100`,
			"events":    `SELECT jsonb_build_object('id',id::text,'kind',kind,'details',details,'created_at',created_at) FROM display_events WHERE instance_id=$1 ORDER BY id DESC LIMIT 100`,
		} {
			items, e := jsonRows(ctx, tx, q, id)
			if e != nil {
				return e
			}
			out[name] = items
		}
		g, e := grantTx(ctx, tx, id)
		if e == nil {
			out["grant"] = g
		} else if e != pgx.ErrNoRows {
			return e
		}
		return nil
	})
	return out, e
}
func grantTx(ctx context.Context, tx pgx.Tx, id string) (Grant, error) {
	v := Grant{}
	e := tx.QueryRow(ctx, `SELECT revision::text,capabilities,scope,fields,valid_until,revoked,issued_by::text,scope_format,read_scopes FROM display_grants WHERE instance_id=$1`, id).Scan(&v.Revision, &v.Capabilities, &v.Scope, &v.Fields, &v.ValidUntil, &v.Revoked, &v.IssuedBy, &v.Format, &v.ReadScopes)
	if e == nil {
		e = displayread.ValidateFormat(v.Format, len(v.ReadScopes) > 0, "/scope_format")
	}
	return v, e
}
func configTx(ctx context.Context, tx pgx.Tx, instance, id string) (Config, error) {
	v := Config{}
	if !access.ValidID(id) {
		return v, hidden()
	}
	e := tx.QueryRow(ctx, `SELECT id::text,package_id::text,parameters,bindings,mode,created_at,config_format,read_views FROM display_configs WHERE instance_id=$1 AND id=$2`, instance, id).Scan(&v.ID, &v.PackageID, &v.Parameters, &v.Bindings, &v.Mode, &v.CreatedAt, &v.Format, &v.ReadViews)
	if e == pgx.ErrNoRows {
		e = hidden()
	}
	if e == nil {
		e = displayread.ValidateFormat(v.Format, len(v.ReadViews) > 0, "/config_format")
	}
	return v, e
}
func packageTx(ctx context.Context, tx pgx.Tx, instance, id string) (Package, error) {
	v := Package{}
	if !access.ValidID(id) {
		return v, hidden()
	}
	e := tx.QueryRow(ctx, `SELECT id::text,sha256,state,manifest,created_at FROM display_packages WHERE instance_id=$1 AND id=$2`, instance, id).Scan(&v.ID, &v.Hash, &v.State, &v.Manifest, &v.CreatedAt)
	if e == pgx.ErrNoRows {
		e = hidden()
	}
	if e == nil && v.State != "ready" {
		e = problem("package_unavailable", "包内容未就绪或缺失。", 409)
	}
	return v, e
}
func eventTx(ctx context.Context, tx pgx.Tx, scope Scope, instance, actor, kind string, details any) error {
	var id int64
	e := tx.QueryRow(ctx, `INSERT INTO display_events(tenant_id,project_id,instance_id,actor_id,kind,details) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, scope.Tenant, scope.Project, nullable(instance), nullable(actor), kind, jsonBytes(details)).Scan(&id)
	if e != nil {
		return e
	}
	return appendNotificationTx(ctx, tx, id, scope, instance, kind, details)
}
func (s *Service) Command(ctx context.Context, a access.Actor, scope Scope, id, action, target string, in Command) (map[string]any, error) {
	out := map[string]any{}
	notifyInstance := true
	if !access.ValidID(in.RequestID) || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 2000 {
		return out, invalid("/request_id", "需要请求编号及操作原因。")
	}
	e := s.transact(ctx, a, scope, action, func(tx pgx.Tx, auth access.ProjectAuthorization) error {
		// Creating the disabled settings row never creates an application.
		if _, e := tx.Exec(ctx, `INSERT INTO display_settings(tenant_id,project_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, scope.Tenant, scope.Project); e != nil {
			return e
		}
		var op, oldHash string
		e := tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM display_receipts WHERE tenant_id=$1 AND project_id=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.Project, a.ID, in.RequestID).Scan(&op, &oldHash, &out)
		payload := hash([]any{id, target, in})
		if in.taskCommand != nil {
			payload = hash([]any{id, target, in, *in.taskCommand})
		}
		if e == nil {
			if op != action || oldHash != payload {
				return conflict()
			}
			out["replayed"] = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		st, e := settingsTx(ctx, tx, scope)
		if e != nil {
			return e
		}
		before, e := auditSnapshotTx(ctx, tx, scope, id)
		if action == "create" {
			before = nil
		}
		if e != nil {
			return e
		}
		if action == "settings" {
			if in.Revision != st.Revision && !(st.Revision == "1" && in.Revision == "0") {
				return conflict()
			}
			if auth.ProjectStatus == "archived" && in.Enabled {
				return access.ErrDenied
			}
			var rev string
			e = tx.QueryRow(ctx, `UPDATE display_settings SET enabled=$3,revision=revision+1 WHERE tenant_id=$1 AND project_id=$2 RETURNING revision::text`, scope.Tenant, scope.Project, in.Enabled).Scan(&rev)
			if e != nil {
				return e
			}
			out = map[string]any{"revision": rev, "enabled": in.Enabled}
			if !in.Enabled {
				if e = s.revokeTx(ctx, tx, scope, "", "project_disabled"); e != nil {
					return e
				}
			}
		} else if action == "create" {
			if !st.Enabled {
				return problem("display_disabled", "请先启用项目大屏。", 409)
			}
			if e = validName(in.Name); e != nil {
				return e
			}
			var count int
			e = tx.QueryRow(ctx, `SELECT count(*) FROM display_instances WHERE tenant_id=$1 AND project_id=$2`, scope.Tenant, scope.Project).Scan(&count)
			if e != nil {
				return e
			}
			if count >= s.options.MaxInstances {
				return problem("budget_exceeded", "项目实例数达到部署限制。", 429)
			}
			var newID string
			e = tx.QueryRow(ctx, `INSERT INTO display_instances(tenant_id,project_id,name) VALUES($1,$2,$3) RETURNING id::text`, scope.Tenant, scope.Project, strings.TrimSpace(in.Name)).Scan(&newID)
			if e != nil {
				return e
			}
			id = newID
			out = map[string]any{"id": id, "revision": "1"}
		} else {
			v, e := instanceTx(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			if !canView(auth, v) {
				return hidden()
			}
			if in.taskCommand == nil && in.Revision != v.Revision {
				return conflict()
			}
			if v.State == "archived" && !slices.Contains([]string{"grant", "withdraw", "upload.cancel", "terminal.update", "instance"}, action) {
				return access.ErrDenied
			}
			switch action {
			case "instance":
				notifyInstance = in.State != v.State || hash(in.Viewers) != hash(v.Viewers)
				if !slices.Contains([]string{"enabled", "disabled", "archived"}, in.State) {
					return invalid("/state", "实例状态无效。")
				}
				if auth.ProjectStatus == "archived" && (in.State == "enabled" || in.Name != v.Name || hash(in.Viewers) != hash(v.Viewers)) {
					return access.ErrDenied
				}
				if v.State == "archived" && (in.State != "archived" || in.Name != v.Name || hash(in.Viewers) != hash(v.Viewers)) {
					return invalid("/state", "已归档实例不可重新启用。")
				}
				if e = validName(in.Name); e != nil {
					return e
				}
				if e = validateViewers(ctx, tx, scope, in.Viewers); e != nil {
					return e
				}
				_, e = tx.Exec(ctx, `UPDATE display_instances SET name=$2,state=$3,viewers=$4 WHERE id=$1`, id, strings.TrimSpace(in.Name), in.State, in.Viewers)
				if e != nil {
					return e
				}
				if in.State != "enabled" || hash(in.Viewers) != hash(v.Viewers) {
					if e = s.revokeTx(ctx, tx, scope, id, "instance_changed"); e != nil {
						return e
					}
				}
			case "config":
				p, e := packageTx(ctx, tx, id, in.PackageID)
				if e != nil {
					return e
				}
				if in.Parameters == nil {
					in.Parameters = map[string]any{}
				}
				if e = ValidateConfig(p.Manifest, in.Parameters); e != nil {
					return e
				}
				if in.Mode != "fixed" && in.Mode != "follow" {
					return invalid("/mode", "请选择固定或跟随模式。")
				}
				if e = s.validateBindingsTx(ctx, tx, scope, p.Manifest, in.Bindings, nil, false); e != nil {
					return e
				}
				if in.ReadViews == nil {
					in.ReadViews = map[string]displayread.Definition{}
				}
				var configID string
				if e = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&configID); e != nil {
					return e
				}
				if e = s.freezeReadViewsTx(ctx, tx, a, scope, v, p, &in, configID); e != nil {
					return e
				}
				e = tx.QueryRow(ctx, `INSERT INTO display_configs(id,tenant_id,project_id,instance_id,package_id,parameters,bindings,mode,created_by,config_format,read_views) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, configID, scope.Tenant, scope.Project, id, in.PackageID, jsonBytes(in.Parameters), jsonBytes(in.Bindings), in.Mode, a.ID, in.ConfigFormat, jsonBytes(in.ReadViews)).Scan(&configID)
				if e != nil {
					return e
				}
				_, e = tx.Exec(ctx, `UPDATE display_instances SET draft_config_id=$2 WHERE id=$1`, id, configID)
				if e != nil {
					return e
				}
				out["config_id"] = configID
			case "grant":
				if in.Grant == nil {
					return invalid("/grant", "需要读取授权。")
				}
				if e = s.saveGrantTx(ctx, tx, a, scope, v, auth, *in.Grant); e != nil {
					return e
				}
			case "publish", "rollback":
				if e = s.publishTx(ctx, tx, a, scope, v, action, in, out); e != nil {
					return e
				}
			case "withdraw":
				if !access.ValidID(in.ReleaseID) {
					return hidden()
				}
				tag, e := tx.Exec(ctx, `UPDATE display_releases SET withdrawn=true WHERE instance_id=$1 AND id=$2`, id, in.ReleaseID)
				if e != nil {
					return e
				}
				if tag.RowsAffected() != 1 {
					return hidden()
				}
				_, e = tx.Exec(ctx, `UPDATE display_sessions SET state='revoked',lease_until=clock_timestamp() WHERE instance_id=$1 AND release_id=$2`, id, in.ReleaseID)
				if e != nil {
					return e
				}
			case "terminal.create", "terminal.update":
				if e = s.terminalCommandTx(ctx, tx, scope, v, auth, action, target, in, out); e != nil {
					return e
				}
			case "upload":
				if e = s.createUploadTx(ctx, tx, a, scope, v, in, out); e != nil {
					return e
				}
			case "upload.complete", "upload.cancel", "package.retry":
				if e = tasks.SetCommandTx(ctx, tx, a, tasks.Command{Reason: in.Reason}); e != nil {
					return e
				}
				if in.taskCommand != nil {
					var job string
					if e = tx.QueryRow(ctx, `SELECT id::text FROM display_package_jobs WHERE upload_id=$1 FOR UPDATE`, target).Scan(&job); e != nil {
						return e
					}
					views, err := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{{Module: "display", Kind: "job", RootID: job}}, "detail")
					if err != nil {
						return err
					}
					if len(views) != 1 {
						return tasks.Hidden()
					}
					if err = tasks.ValidateCommand(*in.taskCommand, views[0], a); err != nil {
						return err
					}
					if err = tasks.SetCommandTx(ctx, tx, a, *in.taskCommand, views[0]); err != nil {
						return err
					}
				} else if action == "upload.cancel" || action == "package.retry" {
					var fact taskrecord.Change
					err := tx.QueryRow(ctx, `SELECT m.snapshot FROM display_package_jobs j JOIN display_task_metadata m USING(id) WHERE j.upload_id=$1`, target).Scan(&fact)
					if err != nil && err != pgx.ErrNoRows {
						return err
					}
					if err == nil {
						kind := "cancel"
						if action == "package.retry" {
							kind = "retry"
						}
						if e = tasks.SetCommandTx(ctx, tx, a, tasks.Command{RequestID: in.RequestID, Action: kind, Reason: in.Reason}, tasks.Present(fact, "")); e != nil {
							return e
						}
					}
				}
				if e = s.uploadCommandTx(ctx, tx, scope, id, target, action, out); e != nil {
					return e
				}
			case "package.delete":
				if e = s.deletePackageTx(ctx, tx, id, target); e != nil {
					return e
				}
			default:
				return invalid("/action", "操作不支持。")
			}
			var rev string
			if e = tx.QueryRow(ctx, `UPDATE display_instances SET revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&rev); e != nil {
				return e
			}
			out["revision"] = rev
			out["id"] = id
		}
		safe := map[string]any{}
		for k, v := range out {
			if k != "pair_code" {
				safe[k] = v
			}
		}
		if e = eventTx(ctx, tx, scope, id, a.ID, action, map[string]any{"result": safe, "reason": in.Reason, "notify_instance": notifyInstance}); e != nil {
			return e
		}
		after, e := auditSnapshotTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		fact := ar.LegacyInput{Event: "display." + action, ObjectType: "display", ObjectID: id, Before: before, After: after, ActorID: a.ID, ActorType: "account", Entry: "project", Tenant: scope.Tenant, Project: scope.Project, Reason: in.Reason, RequestID: in.RequestID}
		pack := in.PackageID
		if action == "package.delete" {
			pack = target
		}
		if ar.ID(pack) {
			fact.Links = append(fact.Links, ar.Link{Kind: "business", Module: "display", TargetKind: "display_package", TargetID: pack, Relation: "subject", Scope: ar.Scope{Kind: "project", Tenant: scope.Tenant, Project: scope.Project}})
		}
		command, e := ar.CommandTx(ctx, tx)
		if e != nil {
			return e
		}
		if command.Module == "display" && command.Kind == "job" {
			var change taskrecord.Change
			if e = tx.QueryRow(ctx, `SELECT snapshot FROM display_task_metadata WHERE id=$1`, command.RootID).Scan(&change); e != nil {
				return e
			}
			if e = taskrecord.SetAuditCommandOutcomeTx(ctx, tx, change); e != nil {
				return e
			}
		}
		if _, e = ar.AppendLegacyTx(ctx, tx, fact); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO display_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, scope.Project, a.ID, in.RequestID, action, payload, jsonBytes(safe))
		return e
	})
	return out, e
}
func validName(name string) error {
	if len(strings.TrimSpace(name)) == 0 || len([]rune(name)) > 120 {
		return invalid("/name", "名称为 1–120 字符。")
	}
	return nil
}
func validateViewers(ctx context.Context, tx pgx.Tx, scope Scope, ids []string) error {
	if ids == nil {
		return nil
	}
	if len(ids) > 500 {
		return invalid("/viewers", "最多 500 个成员。")
	}
	for _, id := range ids {
		if !access.ValidID(id) {
			return invalid("/viewers", "成员标识无效。")
		}
	}
	var count int
	e := tx.QueryRow(ctx, `SELECT count(DISTINCT account_id) FROM project_memberships WHERE tenant_id=$1 AND project_id=$2 AND account_id=ANY($3::uuid[]) AND status='active'`, scope.Tenant, scope.Project, ids).Scan(&count)
	if e != nil {
		return e
	}
	if count != len(ids) {
		return hidden()
	}
	return nil
}
func (s *Service) revokeTx(ctx context.Context, tx pgx.Tx, scope Scope, id, reason string) error {
	_, e := tx.Exec(ctx, `UPDATE display_sessions SET state='revoked',lease_until=clock_timestamp(),last_error=$4 WHERE tenant_id=$1 AND project_id=$2 AND ($3='' OR instance_id::text=$3) AND state IN ('starting','ready','degraded')`, scope.Tenant, scope.Project, id, reason)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET cancel_requested=true WHERE display_subject_id IN (SELECT subject_id FROM display_sessions WHERE tenant_id=$1 AND project_id=$2 AND ($3='' OR instance_id::text=$3) AND state='revoked') AND state IN ('queued','running','retry_wait')`, scope.Tenant, scope.Project, id)
	return e
}
func revision(v string) bool {
	n, e := strconv.ParseInt(v, 10, 64)
	return e == nil && n >= 0 && strconv.FormatInt(n, 10) == v
}

// Audit snapshots contain only persisted management metadata. Config parameters,
// session credentials and package contents never enter the record.
func auditSnapshotTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (map[string]any, error) {
	var value map[string]any
	if id == "" {
		e := tx.QueryRow(ctx, `SELECT jsonb_build_object('enabled',enabled,'revision',revision::text) FROM display_settings WHERE tenant_id=$1 AND project_id=$2`, scope.Tenant, scope.Project).Scan(&value)
		return value, e
	}
	e := tx.QueryRow(ctx, `SELECT jsonb_build_object('name',i.name,'state',i.state,'revision',i.revision::text,'viewers',i.viewers,'current_release_id',i.current_release_id,'draft_config_id',i.draft_config_id,'package_id',c.package_id,'mode',c.mode,'grant',CASE WHEN g.instance_id IS NULL THEN NULL ELSE jsonb_build_object('revision',g.revision::text,'capabilities',g.capabilities,'fields',g.fields,'valid_until',g.valid_until,'revoked',g.revoked) END) FROM display_instances i LEFT JOIN display_configs c ON c.id=i.draft_config_id LEFT JOIN display_grants g ON g.instance_id=i.id WHERE i.id=$1 AND i.tenant_id=$2 AND i.project_id=$3`, id, scope.Tenant, scope.Project).Scan(&value)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return value, e
}
