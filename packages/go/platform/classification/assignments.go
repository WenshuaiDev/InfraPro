package classification

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// ReadDeviceTx interprets an already authorized device. It grants no device or dictionary access.
// A requested release fixes grouping while retaining the record's original selection release.
func ReadDeviceTx(ctx context.Context, tx pgx.Tx, tenant, id, release string) (Effective, error) {
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT jsonb_build_object('state',CASE WHEN c.category_id IS NULL THEN 'unclassified' WHEN n.category_id IS NULL THEN 'unavailable' ELSE 'classified' END,'origin',c.origin,'category_id',COALESCE(c.category_id::text,''),'selected_release_id',COALESCE(c.selected_release_id::text,''),'dictionary_release_id',COALESCE(COALESCE(NULLIF($3,'')::uuid,c.dictionary_release_id)::text,''),'override_revision',c.override_revision::text,'product_classification_revision',c.product_classification_revision::text,'record_id',COALESCE(c.record_id::text,''),'name',COALESCE(n.name,''),'path',COALESCE(to_jsonb(n.path),'[]'),'name_path',COALESCE(to_jsonb(n.name_path),'[]'),'sensor',n.sensor,'category_status',COALESCE(n.status,'')) FROM device_effective_classifications c LEFT JOIN device_category_release_nodes n ON n.release_id=COALESCE(NULLIF($3,'')::uuid,c.dictionary_release_id) AND n.category_id=c.category_id WHERE c.tenant_id=$1 AND c.device_id=$2`, tenant, id, release).Scan(&raw)
	if e == pgx.ErrNoRows {
		return Effective{}, k.NotFound("device")
	}
	if e != nil {
		return Effective{}, e
	}
	return decodeEffective(raw)
}
func ReadProductTx(ctx context.Context, tx pgx.Tx, id, release string) (Effective, error) {
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT jsonb_build_object('state',CASE WHEN p.category_id IS NULL THEN 'unclassified' WHEN n.category_id IS NULL THEN 'unavailable' ELSE 'classified' END,'origin','product_default','category_id',COALESCE(p.category_id::text,''),'selected_release_id',COALESCE(p.selected_release_id::text,''),'dictionary_release_id',COALESCE(COALESCE(NULLIF($2,'')::uuid,s.current_release_id)::text,''),'override_revision','0','product_classification_revision',COALESCE(p.revision,0)::text,'record_id',COALESCE(p.id::text,''),'name',COALESCE(n.name,''),'path',COALESCE(to_jsonb(n.path),'[]'),'name_path',COALESCE(to_jsonb(n.name_path),'[]'),'sensor',n.sensor,'category_status',COALESCE(n.status,'')) FROM device_classification_state s LEFT JOIN product_classification_records p ON p.product_version_id=$1 AND p.ended_at IS NULL LEFT JOIN device_category_release_nodes n ON n.release_id=COALESCE(NULLIF($2,'')::uuid,s.current_release_id) AND n.category_id=p.category_id`, id, release).Scan(&raw)
	if e != nil {
		return Effective{}, e
	}
	return decodeEffective(raw)
}

// InitializeProductTx records the deliberate publication choice independently of immutable content.
func InitializeProductTx(ctx context.Context, tx pgx.Tx, version, actor, request, reason string, selection Selection) error {
	if selection.Mode == "" {
		selection.Mode = "unclassified"
	}
	if selection.Mode != "classify" && selection.Mode != "unclassified" {
		return k.Invalid("/classification/mode", "classification_mode", "产品版本须明确分类或标记未分类。")
	}
	if e := validateSelectionTx(ctx, tx, selection); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO product_classification_records(product_version_id,revision,category_id,selected_release_id,created_by,reason,request_id) VALUES($1,1,$2,$3,$4,$5,$6)`, version, k.Nullable(selection.CategoryID), k.Nullable(selection.ReleaseID), actor, reason, request)
	return e
}
func validateSelectionTx(ctx context.Context, tx pgx.Tx, s Selection) error {
	if !slices.Contains([]string{"classify", "unclassified", "inherit"}, s.Mode) {
		return k.Invalid("/selection/mode", "selection_mode", "请选择明确归类、产品未分类或设备继承。")
	}
	if s.Mode != "classify" {
		if s.CategoryID != "" || s.ReleaseID != "" {
			return k.Invalid("/selection", "unused_category", "未分类和继承不能携带类别身份。")
		}
		return nil
	}
	if !access.ValidID(s.CategoryID) || !access.ValidID(s.ReleaseID) {
		return k.Invalid("/selection", "classification_reference", "请选择当前发布字典中的类别。")
	}
	var valid bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_category_release_nodes n JOIN device_classification_state s ON s.current_release_id=n.release_id WHERE n.release_id=$1 AND n.category_id=$2 AND n.status='active' AND n.selectable AND NOT EXISTS(SELECT 1 FROM device_category_release_nodes a WHERE a.release_id=n.release_id AND a.category_id=ANY(n.path) AND a.status='inactive'))`, s.ReleaseID, s.CategoryID).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return k.Problem("classification_selection_unavailable", "分类版本已更新、已停用或不可直接归类，请重新选择。", 409)
	}
	return nil
}
func targetTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, kind, id string, write bool) (TargetDetail, error) {
	out := TargetDetail{Target: Target{Kind: kind, ID: id}}
	if !access.ValidID(id) {
		return out, k.NotFound("classification")
	}
	switch kind {
	case "product_version":
		var owner *string
		e := tx.QueryRow(ctx, `SELECT p.owner_tenant_id::text,v.content->>'name',s.revision::text FROM device_product_versions v JOIN device_products p ON p.id=v.product_id JOIN device_product_version_states s ON s.version_id=v.id WHERE v.id=$1 AND p.deleted_at IS NULL`, id).Scan(&owner, &out.Name, &out.EntityRevision)
		if e == pgx.ErrNoRows {
			return out, k.NotFound("classification")
		}
		if e != nil {
			return out, e
		}
		if auth.Entry == "platform" {
			if owner != nil {
				return out, k.NotFound("classification")
			}
			if e = auth.Require(access.DeviceDefinitionRead); e != nil {
				return out, e
			}
			if write {
				if e = auth.Require(access.DeviceProductEdit); e != nil {
					return out, e
				}
			}
		} else if auth.Entry == "tenant" {
			if owner != nil && *owner != auth.Tenant {
				return out, k.NotFound("classification")
			}
			if owner != nil {
				if e = auth.Require(access.TenantDeviceDefinitionRead); e != nil {
					return out, e
				}
			}
			if write {
				if owner == nil {
					return out, access.ErrDenied
				}
				if e = auth.Require(access.TenantDeviceProductEdit); e != nil {
					return out, e
				}
			}
		} else {
			return out, access.ErrDenied
		}
		out.Effective, e = ReadProductTx(ctx, tx, id, "")
		out.Revision = out.Effective.ProductRevision
		return out, e
	case "device":
		if auth.Entry != "tenant" {
			return out, access.ErrDenied
		}
		if e := auth.Require(access.TenantDeviceRead); e != nil {
			return out, e
		}
		if write {
			if e := auth.Require(access.TenantDeviceManage); e != nil {
				return out, e
			}
		}
		e := tx.QueryRow(ctx, `SELECT name,revision::text FROM devices WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, auth.Tenant, id).Scan(&out.Name, &out.EntityRevision)
		if e == pgx.ErrNoRows {
			return out, k.NotFound("device")
		}
		if e != nil {
			return out, e
		}
		out.Effective, e = ReadDeviceTx(ctx, tx, auth.Tenant, id, "")
		out.Revision = out.Effective.OverrideRevision
		return out, e
	default:
		return out, k.Invalid("/kind", "classification_kind", "分类对象为固定产品版本或实际设备。")
	}
}
func (s *Service) Target(ctx context.Context, a access.Actor, scope Scope, kind, id string) (TargetDetail, error) {
	out := TargetDetail{}
	e := s.transact(ctx, a, scope, "target", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = targetTx(ctx, tx, auth, kind, id, false)
		return e
	})
	return out, e
}
func (s *Service) Targets(ctx context.Context, a access.Actor, scope Scope, kind, cursor, query string, limit int) (Page[TargetDetail], error) {
	out := Page[TargetDetail]{Items: []TargetDetail{}}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 || cursor != "" && !access.ValidID(cursor) || len([]rune(query)) > 200 {
		return out, k.Invalid("/cursor", "pagination", "分页或搜索参数无效。")
	}
	e := s.transact(ctx, a, scope, "targets", false, func(tx pgx.Tx, auth k.Authorization) error {
		var sql string
		switch kind {
		case "device":
			if auth.Entry != "tenant" {
				return access.ErrDenied
			}
			if e := auth.Require(access.TenantDeviceRead); e != nil {
				return e
			}
			sql = `SELECT id::text FROM devices WHERE tenant_id=$1 AND deleted_at IS NULL AND ($2='' OR id>NULLIF($2,'')::uuid) AND ($3='' OR name ILIKE '%'||$3||'%' OR code ILIKE '%'||$3||'%') ORDER BY id LIMIT $4`
		case "product_version":
			if auth.Entry == "platform" {
				if e := auth.Require(access.DeviceDefinitionRead); e != nil {
					return e
				}
			} else if e := auth.Require(access.TenantDeviceDefinitionRead); e != nil {
				return e
			}
			sql = `SELECT v.id::text FROM device_product_versions v JOIN device_products p ON p.id=v.product_id WHERE p.owner_tenant_id IS NOT DISTINCT FROM NULLIF($1,'')::uuid AND p.deleted_at IS NULL AND ($2='' OR v.id>NULLIF($2,'')::uuid) AND ($3='' OR v.content->>'name' ILIKE '%'||$3||'%' OR p.code ILIKE '%'||$3||'%') ORDER BY v.id LIMIT $4`
		default:
			return k.Invalid("/kind", "classification_kind", "请选择产品版本或设备。")
		}
		rows, e := tx.Query(ctx, sql, auth.Tenant, cursor, query, limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(ids) > limit {
			ids = ids[:limit]
			out.NextCursor = ids[limit-1]
		}
		for _, id := range ids {
			v, e := targetTx(ctx, tx, auth, kind, id, false)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, e
}
func normalizeBatch(in *BatchInput) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	if e := in.Meta.RequireReason(); e != nil {
		return e
	}
	if len(in.Targets) < 1 || len(in.Targets) > MaxBatchTargets {
		return k.Problem("classification_budget_exceeded", "一次须固定选择 1–5,000 个对象，不能在提交时重新扩大筛选范围。", 422)
	}
	seen := map[string]bool{}
	kind := in.Targets[0].Kind
	for _, t := range in.Targets {
		if t.Kind != kind || kind != "device" && kind != "product_version" || !access.ValidID(t.ID) || !k.Revision(t.Revision, true) || !k.Revision(t.EntityRevision, false) {
			return k.Invalid("/targets", "fixed_targets", "每批选择同一种对象，并提供分类修订和对象修订。")
		}
		if seen[t.ID] {
			return k.Invalid("/targets", "duplicate", "同一对象只能选择一次。")
		}
		seen[t.ID] = true
	}
	if kind == "device" && in.Selection.Mode == "unclassified" || kind == "product_version" && in.Selection.Mode == "inherit" {
		return k.Invalid("/selection/mode", "mode", "产品可以标记未分类；设备覆盖须明确归类，或清除覆盖恢复继承。")
	}
	slices.SortFunc(in.Targets, func(a, b Target) int { return strings.Compare(a.ID, b.ID) })
	return nil
}
func batchPayload(in BatchInput) BatchInput { in.PreviewHash = ""; in.PreviewID = ""; return in }
func calculateBatch(ctx context.Context, tx pgx.Tx, auth k.Authorization, in BatchInput) (BatchPreview, error) {
	out := BatchPreview{Changes: []AssignmentChange{}, PreservesOutputs: true}
	if e := validateSelectionTx(ctx, tx, in.Selection); e != nil {
		return out, e
	}
	var state string
	if e := tx.QueryRow(ctx, `SELECT revision::text FROM device_classification_state WHERE id=1`).Scan(&state); e != nil {
		return out, e
	}
	impactHashes := []string{}
	for _, t := range in.Targets {
		d, e := targetTx(ctx, tx, auth, t.Kind, t.ID, true)
		if e != nil {
			return out, e
		}
		if e = k.Expect(t.Revision, d.Revision); e != nil {
			return out, e
		}
		if e = k.Expect(t.EntityRevision, d.EntityRevision); e != nil {
			return out, e
		}
		next := d.Effective
		next.Path = []string{}
		next.NamePath = []string{}
		next.CategoryID = ""
		next.SelectedReleaseID = ""
		next.Name = ""
		next.Sensor = nil
		next.CategoryStatus = ""
		next.State = "unclassified"
		next.RecordID = ""
		if in.Selection.Mode == "classify" {
			next.CategoryID = in.Selection.CategoryID
			next.SelectedReleaseID = in.Selection.ReleaseID
			next.DictionaryReleaseID = in.Selection.ReleaseID
			next.State = "classified"
			if e = tx.QueryRow(ctx, `SELECT name,path,name_path,sensor,status FROM device_category_release_nodes WHERE release_id=$1 AND category_id=$2`, next.DictionaryReleaseID, next.CategoryID).Scan(&next.Name, &next.Path, &next.NamePath, &next.Sensor, &next.CategoryStatus); e != nil {
				return out, e
			}
		}
		n, _ := strconv.ParseInt(d.Revision, 10, 64)
		if t.Kind == "device" {
			next.Origin = "device_override"
			if in.Selection.Mode == "inherit" {
				var version string
				if e = tx.QueryRow(ctx, `SELECT product_version_id::text FROM devices WHERE tenant_id=$1 AND id=$2`, auth.Tenant, t.ID).Scan(&version); e != nil {
					return out, e
				}
				next, e = ReadProductTx(ctx, tx, version, "")
				if e != nil {
					return out, e
				}
			}
			next.OverrideRevision = strconv.FormatInt(n+1, 10)
		} else {
			next.Origin = "product_default"
			next.ProductRevision = strconv.FormatInt(n+1, 10)
		}
		ch := AssignmentChange{Target: t, Before: d.Effective, After: next}
		if t.Kind == "product_version" {
			ch.InheritedDevicesMayChange = true
			var hash string
			if e = tx.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(jsonb_build_array(c.tenant_id,c.device_id,d.revision,c.override_revision,c.product_classification_revision) ORDER BY c.tenant_id,c.device_id),'[]')::text),count(*) FILTER(WHERE d.tenant_id::text=$2) FROM device_effective_classifications c JOIN devices d ON d.tenant_id=c.tenant_id AND d.id=c.device_id WHERE c.product_version_id=$1 AND c.origin='product_default' AND d.deleted_at IS NULL`, t.ID, auth.Tenant).Scan(&hash, &ch.VisibleInheritedDevices); e != nil {
				return out, e
			}
			impactHashes = append(impactHashes, hash)
		}
		out.Changes = append(out.Changes, ch)
	}
	out.Hash = k.Hash([]any{batchPayload(in), auth.Entry, auth.Tenant, auth.Account, auth.TenantMembership, state, out.Changes, impactHashes})
	return out, nil
}
func (s *Service) PreviewBatch(ctx context.Context, a access.Actor, scope Scope, in BatchInput) (BatchPreview, error) {
	out := BatchPreview{}
	if e := normalizeBatch(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "preview", true, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = calculateBatch(ctx, tx, auth, in)
		if e != nil {
			return e
		}
		var stored BatchInput
		var prior BatchPreview
		err := tx.QueryRow(ctx, `SELECT input,evidence FROM device_classification_previews WHERE actor_id=$1 AND request_id=$2`, auth.Account, in.RequestID).Scan(&stored, &prior)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil {
			if k.Hash(stored) != k.Hash(batchPayload(in)) || prior.Hash != out.Hash {
				return k.Problem("preview_changed", "原分类预览已失效，请使用新的请求编号重新预览。", 409)
			}
			out = prior
			return nil
		}
		if e = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text,clock_timestamp()+interval '30 minutes'`).Scan(&out.ID, &out.ExpiresAt); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO device_classification_previews(id,actor_id,tenant_id,membership_id,entry_scope,input,preview_hash,evidence,expires_at,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, out.ID, auth.Account, k.Nullable(auth.Tenant), k.Nullable(auth.TenantMembership), auth.Entry, k.JSON(batchPayload(in)), out.Hash, k.JSON(out), out.ExpiresAt, in.RequestID)
		return e
	})
	return out, e
}
func (s *Service) ApplyBatch(ctx context.Context, a access.Actor, scope Scope, in BatchInput, meta access.Meta) (Result, error) {
	out := Result{}
	if e := normalizeBatch(&in); e != nil {
		return out, e
	}
	if !access.ValidID(in.PreviewID) || in.PreviewHash == "" {
		return out, k.Invalid("/preview_id", "preview_required", "请先预览并确认固定对象集合。")
	}
	e := s.transact(ctx, a, scope, "apply", true, func(tx pgx.Tx, auth k.Authorization) error {
		// Replays still need current target maintenance authority.
		for _, t := range in.Targets {
			if _, e := targetTx(ctx, tx, auth, t.Kind, t.ID, true); e != nil {
				return e
			}
		}
		var e error
		var ok bool
		out, ok, e = replay(ctx, tx, auth, in.Meta, "assign", in)
		if e != nil || ok {
			return e
		}
		var stored BatchInput
		var hash string
		var valid bool
		e = tx.QueryRow(ctx, `SELECT input,preview_hash,expires_at>clock_timestamp() FROM device_classification_previews WHERE id=$1 AND actor_id=$2 AND entry_scope=$3 AND tenant_id IS NOT DISTINCT FROM $4::uuid AND membership_id IS NOT DISTINCT FROM $5::uuid`, in.PreviewID, auth.Account, auth.Entry, k.Nullable(auth.Tenant), k.Nullable(auth.TenantMembership)).Scan(&stored, &hash, &valid)
		if e == pgx.ErrNoRows {
			return k.NotFound("classification")
		}
		if e != nil {
			return e
		}
		if !valid || hash != in.PreviewHash || k.Hash(stored) != k.Hash(batchPayload(in)) {
			return k.Problem("preview_changed", "预览已过期或提交对象与预览不一致。", 409)
		}
		p, e := calculateBatch(ctx, tx, auth, in)
		if e != nil {
			return e
		}
		if p.Hash != hash {
			return k.Problem("preview_changed", "分类、字典或影响设备已变化，请重新预览。", 409)
		}
		for _, ch := range p.Changes {
			t := ch.Target
			n, _ := strconv.ParseInt(t.Revision, 10, 64)
			if t.Kind == "product_version" {
				if _, e = tx.Exec(ctx, `UPDATE product_classification_records SET ended_at=clock_timestamp() WHERE product_version_id=$1 AND ended_at IS NULL`, t.ID); e != nil {
					return e
				}
				_, e = tx.Exec(ctx, `INSERT INTO product_classification_records(product_version_id,revision,category_id,selected_release_id,created_by,reason,request_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, t.ID, n+1, k.Nullable(in.Selection.CategoryID), k.Nullable(in.Selection.ReleaseID), auth.Account, in.Reason, in.RequestID)
			} else {
				if _, e = tx.Exec(ctx, `UPDATE device_classification_records SET ended_at=clock_timestamp() WHERE tenant_id=$1 AND device_id=$2 AND ended_at IS NULL`, auth.Tenant, t.ID); e != nil {
					return e
				}
				mode := "override"
				if in.Selection.Mode == "inherit" {
					mode = "inherit"
				}
				_, e = tx.Exec(ctx, `INSERT INTO device_classification_records(tenant_id,device_id,revision,mode,category_id,selected_release_id,created_by,reason,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, auth.Tenant, t.ID, n+1, mode, k.Nullable(in.Selection.CategoryID), k.Nullable(in.Selection.ReleaseID), auth.Account, in.Reason, in.RequestID)
			}
			if e != nil {
				return e
			}
			if e = change(ctx, tx, auth, in.Meta, "assign", t.Kind, t.ID, ch.Before, ch.After, meta); e != nil {
				return e
			}
		}
		if e = tx.QueryRow(ctx, `UPDATE device_classification_state SET revision=revision+1 WHERE id=1 RETURNING revision::text`).Scan(&out.Revision); e != nil {
			return e
		}
		out.ID = in.PreviewID
		out.Changed = true
		out.Applied = len(p.Changes)
		return receipt(ctx, tx, auth, in.Meta, "assign", in, out)
	})
	return out, e
}
func (s *Service) History(ctx context.Context, a access.Actor, scope Scope, kind, id, cursor string, limit int) (Page[json.RawMessage], error) {
	out := Page[json.RawMessage]{Items: []json.RawMessage{}}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 || cursor != "" && !access.ValidID(cursor) {
		return out, k.Invalid("/cursor", "pagination", "分页参数无效。")
	}
	e := s.transact(ctx, a, scope, "history", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := targetTx(ctx, tx, auth, kind, id, false); e != nil {
			return e
		}
		var query string
		if kind == "device" {
			if e := auth.Require(access.TenantDeviceHistoryRead); e != nil {
				return e
			}
			query = `SELECT to_jsonb(r)||jsonb_build_object('revision',r.revision::text,'selected_path',COALESCE(n.name_path,ARRAY[]::text[])),r.id::text FROM device_classification_records r LEFT JOIN device_category_release_nodes n ON n.release_id=r.selected_release_id AND n.category_id=r.category_id WHERE r.tenant_id=$1 AND r.device_id=$2 AND ($3='' OR r.id>NULLIF($3,'')::uuid) ORDER BY r.id LIMIT $4`
		} else {
			query = `SELECT to_jsonb(r)||jsonb_build_object('revision',r.revision::text,'selected_path',COALESCE(n.name_path,ARRAY[]::text[])),r.id::text FROM product_classification_records r LEFT JOIN device_category_release_nodes n ON n.release_id=r.selected_release_id AND n.category_id=r.category_id WHERE r.product_version_id=$2 AND ($1='' OR $1<>'') AND ($3='' OR r.id>NULLIF($3,'')::uuid) ORDER BY r.id LIMIT $4`
		}
		rows, e := tx.Query(ctx, query, auth.Tenant, id, cursor, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		ids := []string{}
		for rows.Next() {
			var raw json.RawMessage
			var id string
			if e = rows.Scan(&raw, &id); e != nil {
				return e
			}
			out.Items = append(out.Items, raw)
			ids = append(ids, id)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = ids[limit-1]
		}
		return nil
	})
	return out, e
}
