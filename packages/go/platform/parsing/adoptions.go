package parsing

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

type device struct {
	ID               string `json:"id"`
	ProductID        string `json:"product_id"`
	ProductVersionID string `json:"product_version_id"`
	Status           string `json:"status"`
}

func deviceTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string) (device, error) {
	if auth.Entry == "platform" || !access.ValidID(id) {
		return device{}, k.NotFound("parsing")
	}
	if auth.Entry == "project" {
		if e := auth.Require(access.ProjectDeviceRead); e != nil {
			return device{}, e
		}
		var ok bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_device_grants WHERE tenant_id=$1 AND project_id=$2 AND kind='device' AND device_id=$3 AND ended_at IS NULL)`, auth.Tenant, auth.Project, id).Scan(&ok); e != nil {
			return device{}, e
		}
		if !ok {
			return device{}, k.NotFound("parsing")
		}
	} else if e := auth.Require(access.TenantDeviceRead); e != nil {
		return device{}, e
	}
	return readJSON[device](tx.QueryRow(ctx, `SELECT to_jsonb(d) FROM devices d WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, auth.Tenant, id))
}

const adoptionJSON = `to_jsonb(a)||jsonb_build_object('revision',a.revision::text,'source_default_revision',a.source_default_revision::text)`

func deviceStateTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string) (DeviceState, error) {
	out := DeviceState{Revision: "0", Mode: "unconfigured", Mapping: []Mapping{}}
	d, e := deviceTx(ctx, tx, auth, id)
	if e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT revision::text FROM parsing_device_heads WHERE device_id=$1`, id).Scan(&out.Revision)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT `+adoptionJSON+` FROM parsing_device_adoptions a WHERE device_id=$1 AND ended_at IS NULL`, id).Scan(&raw)
	if e == nil {
		var ad Adoption
		if e = decode(raw, &ad); e != nil {
			return out, e
		}
		out.Current = &ad
		out.Mode = ad.Mode
		if ad.Snapshot != nil {
			m, e := methodTx(ctx, tx, ad.Snapshot.Method.ID, ad.Snapshot.Method.Version)
			if e != nil {
				return out, e
			}
			out.Mapping, e = mappingTx(ctx, tx, m.Entry, d.ProductVersionID, ad.Snapshot.Bindings, ad.Snapshot.Ignored, id, false)
			if e != nil {
				out.Mapping = slices.Clone(ad.Snapshot.Mapping)
				for i := range out.Mapping {
					out.Mapping[i].Status = "semantic_conflict"
					out.Mapping[i].Reason = "当前输出与采用快照不相容，请核对输出代次。"
				}
			}
		}
	} else if e != pgx.ErrNoRows {
		return out, e
	}
	// Project device grants permit this product's configuration summary only.
	out.Default = Default{ProductVersionID: d.ProductVersionID, Revision: "0"}
	var revisionID *string
	e = tx.QueryRow(ctx, `SELECT revision::text,config_revision_id::text FROM parsing_product_defaults WHERE product_version_id=$1`, d.ProductVersionID).Scan(&out.Default.Revision, &revisionID)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	if revisionID != nil {
		cfg, e := readJSON[ConfigRevision](tx.QueryRow(ctx, `SELECT `+configJSON+` FROM parsing_config_revisions r JOIN parsing_configs c ON c.id=r.config_id WHERE r.id=$1 AND c.product_version_id=$2 AND c.owner_kind='product' AND (c.tenant_id IS NULL OR c.tenant_id=$3)`, *revisionID, d.ProductVersionID, auth.Tenant))
		if e != nil {
			return out, e
		}
		out.Default.Config = &cfg
	}
	out.DefaultChanged = out.Current != nil && out.Current.ConfigRevisionID != nil && (out.Default.Config == nil || *out.Current.ConfigRevisionID != out.Default.Config.ID)
	out.CanManage = auth.Entry == "tenant" && auth.Has(access.TenantDeviceParsingManage) && auth.Has(access.TenantParsingMethodRead)
	out.CanTest = auth.Entry == "tenant" && auth.Has(access.TenantParsingTestExecute) && auth.Has(access.TenantParsingMethodRead)
	return out, nil
}
func (s *Service) Device(ctx context.Context, a access.Actor, scope Scope, id string) (DeviceState, error) {
	var out DeviceState
	e := s.transact(ctx, a, scope, "device", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = deviceStateTx(ctx, tx, auth, id)
		if e == nil && s.ingress != nil && scope.Entry == "tenant" {
			var target *Snapshot
			if out.Current != nil {
				target = out.Current.Snapshot
			}
			impact, err := s.ingress.CheckAdoptionTx(ctx, tx, scope.Tenant, id, target)
			if err != nil {
				return err
			}
			out.Ingress = &impact
		}
		return e
	})
	return out, e
}
func (s *Service) History(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (k.Page[Adoption], error) {
	out := k.Page[Adoption]{Items: []Adoption{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "device", "history", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := deviceTx(ctx, tx, auth, id); e != nil {
			return e
		}
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, id))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+adoptionJSON+` FROM parsing_device_adoptions a WHERE device_id=$1 AND ($2::timestamptz IS NULL OR (started_at,id)<($2,$3)) ORDER BY started_at DESC,id DESC LIMIT $4`, id, cur.TimeValue(), k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := readJSON[Adoption](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				last := out.Items[len(out.Items)-1]
				out.NextCursor = k.Cursor{Scope: cur.Scope, At: last.StartedAt, ID: last.ID}.Encode()
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) previewTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, in Command) (Preview, string, error) {
	out := Preview{Mapping: []Mapping{}, Issues: []k.Issue{}}
	if e := auth.Require(access.TenantDeviceParsingManage, access.TenantParsingMethodRead); e != nil {
		return out, "", e
	}
	current, e := deviceStateTx(ctx, tx, auth, id)
	if e != nil {
		return out, "", e
	}
	out.Current = current
	if e = k.Expect(in.Revision, current.Revision); e != nil {
		return out, "", e
	}
	d, e := deviceTx(ctx, tx, auth, id)
	if e != nil {
		return out, "", e
	}
	mode := "inherit_snapshot"
	target := in.ConfigRevisionID
	switch in.Action {
	case "inherit_default", "upgrade_default", "restore_inheritance":
		if e = k.Expect(in.DefaultRevision, current.Default.Revision); e != nil {
			return out, "", e
		}
		if current.Default.Config == nil {
			return out, "", k.Problem("default_unconfigured", "产品版本没有默认解析配置。", 409)
		}
		if target != "" && target != current.Default.Config.ID {
			return out, "", k.Problem("revision_conflict", "确认的默认配置已变化。", 409)
		}
		target = current.Default.Config.ID
	case "override":
		mode = "override"
	case "rollback":
		if e = auth.Require(access.TenantDeviceHistoryRead); e != nil {
			return out, "", e
		}
		old, e := readJSON[Adoption](tx.QueryRow(ctx, `SELECT `+adoptionJSON+` FROM parsing_device_adoptions a WHERE device_id=$1 AND id=$2`, id, in.AdoptionID))
		if e != nil {
			return out, "", e
		}
		if old.ConfigRevisionID == nil {
			return out, "", k.Invalid("/adoption_id", "unconfigured", "未配置历史请使用清空采用。")
		}
		target = *old.ConfigRevisionID
		mode = old.Mode
	case "clear":
		out.CanSubmit = true
		if s.ingress != nil {
			impact, err := s.ingress.CheckAdoptionTx(ctx, tx, auth.Tenant, id, nil)
			if err != nil {
				return out, "", err
			}
			out.Current.Ingress = &impact
			for _, link := range impact.Entries {
				if !link.Compatible {
					out.CanSubmit = false
				}
			}
		}
		return out, "unconfigured", nil
	default:
		return out, "", k.Invalid("/action", "enum", "请选择明确的采用动作。")
	}
	if _, _, e = productTx(ctx, tx, auth, d.ProductID, d.ProductVersionID, false, true); e != nil {
		return out, "", e
	}
	cfg, e := configTx(ctx, tx, auth, target, true)
	if e != nil {
		return out, "", e
	}
	if cfg.ProductVersionID != d.ProductVersionID || cfg.Snapshot.OwnerKind == "device" && cfg.Snapshot.OwnerID != id || in.Action == "override" && cfg.Snapshot.OwnerKind != "device" {
		return out, "", k.NotFound("parsing")
	}
	m, e := methodTx(ctx, tx, cfg.Snapshot.Method.ID, cfg.Snapshot.Method.Version)
	if e != nil {
		return out, "", e
	}
	if e = allowedMethod(m, auth.Scope, true, false); e != nil {
		return out, "", e
	}
	mapping, e := mappingTx(ctx, tx, m.Entry, d.ProductVersionID, cfg.Snapshot.Bindings, cfg.Snapshot.Ignored, id, true)
	if e != nil {
		return out, "", e
	}
	out.Target = &cfg
	out.Mapping = mapping
	out.CanSubmit = true
	if s.ingress != nil {
		impact, err := s.ingress.CheckAdoptionTx(ctx, tx, auth.Tenant, id, &cfg.Snapshot)
		if err != nil {
			return out, "", err
		}
		out.Current.Ingress = &impact
		for _, link := range impact.Entries {
			if !link.Compatible {
				out.CanSubmit = false
				out.Issues = append(out.Issues, k.Issue{Path: "/config_revision_id", Code: "ingress_method_incompatible", Message: "有效入口关联与目标方法不相容：" + link.SourceID})
			}
		}
	}
	return out, mode, nil
}
func (s *Service) Preview(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (Preview, error) {
	var out Preview
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "device", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, _, e = s.previewTx(ctx, tx, auth, id, in)
		return e
	})
	return out, e
}
func adoptTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, in Command, p Preview, mode string) (Result, error) {
	out := Result{ID: id}
	if !p.CanSubmit {
		return out, k.Problem("ingress_method_incompatible", "请先停用冲突入口关联，或选择已验证的相容方法。", 409)
	}
	d, e := deviceTx(ctx, tx, auth, id)
	if e != nil {
		return out, e
	}
	var now time.Time
	e = tx.QueryRow(ctx, `SELECT GREATEST(clock_timestamp(),COALESCE((SELECT started_at+interval '1 microsecond' FROM parsing_device_adoptions WHERE device_id=$1 AND ended_at IS NULL),clock_timestamp()))`, id).Scan(&now)
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE parsing_device_adoptions SET ended_at=$2 WHERE device_id=$1 AND ended_at IS NULL`, id, now); e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO parsing_device_heads(tenant_id,device_id,revision) VALUES($1,$2,1) ON CONFLICT(device_id) DO UPDATE SET revision=parsing_device_heads.revision+1 RETURNING revision::text`, auth.Tenant, id).Scan(&out.Revision)
	if e != nil {
		return out, e
	}
	var snapshot any
	var sourceDefault any
	if p.Target != nil {
		snap := p.Target.Snapshot
		snap.Mapping = p.Mapping
		snapshot = snap
		out.ConfigRevisionID = p.Target.ID
		if mode == "inherit_snapshot" {
			sourceDefault = p.Current.Default.Revision
			if in.Action == "rollback" {
				var rev *string
				if e := tx.QueryRow(ctx, `SELECT source_default_revision::text FROM parsing_device_adoptions WHERE device_id=$1 AND id=$2`, id, in.AdoptionID).Scan(&rev); e != nil {
					return out, e
				}
				sourceDefault = rev
			}
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO parsing_device_adoptions(tenant_id,device_id,product_version_id,mode,config_revision_id,source_default_revision,snapshot,revision,started_at,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, auth.Tenant, id, d.ProductVersionID, mode, k.Nullable(out.ConfigRevisionID), sourceDefault, nullableJSON(snapshot), out.Revision, now, auth.Account, in.Reason)
	return out, e
}
func nullableJSON(v any) any {
	if v == nil {
		return nil
	}
	return k.JSON(v)
}
func (s *Service) Command(ctx context.Context, a access.Actor, scope Scope, id string, in Command) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "device", "commands", true, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := deviceTx(ctx, tx, auth, id); e != nil {
			return e
		}
		payload := []any{id, in}
		v, ok, e := replay(ctx, tx, auth, "device."+in.Action, in.Meta, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		p, mode, e := s.previewTx(ctx, tx, auth, id, in)
		if e != nil {
			return e
		}
		out, e = adoptTx(ctx, tx, auth, id, in, p, mode)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, "device."+in.Action, in.Meta, payload, out, map[string]string{"revision": p.Current.Revision, "mode": p.Current.Mode}, map[string]string{"revision": out.Revision, "revision_id": out.ConfigRevisionID, "mode": mode})
	})
	return out, e
}
func (s *Service) Batch(ctx context.Context, a access.Actor, scope Scope, in BatchCommand, preview bool) ([]Preview, Result, error) {
	out := []Preview{}
	var result Result
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, result, e
	}
	if len(in.Devices) < 1 || len(in.Devices) > 200 {
		return out, result, k.Invalid("/devices", "limit", "请选择 1–200 台同产品版本设备。")
	}
	in.Devices = slices.Clone(in.Devices)
	slices.SortFunc(in.Devices, func(a, b BatchDevice) int {
		if a.DeviceID < b.DeviceID {
			return -1
		}
		if a.DeviceID > b.DeviceID {
			return 1
		}
		return 0
	})
	action := "commands"
	if preview {
		action = "preview"
	}
	e := s.transact(ctx, a, scope, "batch", action, !preview, func(tx pgx.Tx, auth k.Authorization) error {
		productVersion := ""
		for i, item := range in.Devices {
			if i > 0 && item.DeviceID == in.Devices[i-1].DeviceID {
				return k.Invalid("/devices", "duplicate", "设备不可重复。")
			}
			d, e := deviceTx(ctx, tx, auth, item.DeviceID)
			if e != nil {
				return e
			}
			if productVersion != "" && productVersion != d.ProductVersionID {
				return k.Invalid("/devices", "product_mismatch", "批量采用须属于同一产品版本。")
			}
			productVersion = d.ProductVersionID
		}
		if !preview {
			v, ok, e := replay(ctx, tx, auth, "batch.commands", in.Meta, in)
			if e != nil {
				return e
			}
			if ok {
				result = v
				return nil
			}
		}
		modes := []string{}
		target := ""
		for _, item := range in.Devices {
			command := in.Command
			command.Revision = item.Revision
			p, mode, e := s.previewTx(ctx, tx, auth, item.DeviceID, command)
			if e != nil {
				return e
			}
			if p.Target != nil {
				if target != "" && target != p.Target.ID {
					return k.Invalid("/devices", "target_mismatch", "批量操作须使用同一目标配置。")
				}
				target = p.Target.ID
			}
			out = append(out, p)
			modes = append(modes, mode)
		}
		if preview {
			return nil
		}
		for i, item := range in.Devices {
			r, e := adoptTx(ctx, tx, auth, item.DeviceID, in.Command, out[i], modes[i])
			if e != nil {
				return e
			}
			if i == 0 {
				result = r
			}
			if _, e = tx.Exec(ctx, `INSERT INTO parsing_changes(scope_key,actor_id,operation,target_id,request_id,reason,before_snapshot,after_snapshot) VALUES($1,$2,'batch.device',$3,$4,$5,$6,$7)`, scopeKey(scope), a.ID, item.DeviceID, in.RequestID, in.Reason, k.JSON(map[string]string{"revision": item.Revision}), k.JSON(r)); e != nil {
				return e
			}
		}
		return complete(ctx, tx, auth, "batch.commands", in.Meta, in, result, nil, map[string]int{"count": len(in.Devices)})
	})
	return out, result, e
}

// AdoptOnCreateTx joins the already-authorized device transaction and never
// performs IO or acquires a second transaction. Omitted intent stays unconfigured.
func (s *Service) AdoptOnCreateTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope d.Scope, id, mode, expected, reason string) error {
	if mode == "" || mode == "unconfigured" {
		return nil
	}
	if mode != "inherit_default" {
		return k.Invalid("/parsing_adoption", "enum", "请选择继承默认或未配置。")
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, op(scope, "device", "commands"))
	if e != nil {
		return e
	}
	if e = auth.Require(access.TenantDeviceManage); e != nil {
		return e
	}
	if reason == "" {
		reason = "新建设备明确继承已确认的产品默认"
	}
	in := Command{Meta: Meta{Reason: reason}, Action: "inherit_default", Revision: "0", DefaultRevision: expected}
	p, adoptMode, e := s.previewTx(ctx, tx, auth, id, in)
	if e != nil {
		return e
	}
	_, e = adoptTx(ctx, tx, auth, id, in, p, adoptMode)
	return e
}
