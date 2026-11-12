package devicecatalog

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func checkDraftReferences(issues []Issue) error {
	for _, i := range issues {
		if i.Code == "definition_not_found" || i.Code == "not_found" {
			return &Error{Code: "validation_failed", Message: "所选定义不可访问。", Status: 422, Issues: []Issue{i}}
		}
	}
	return nil
}
func createProductTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, in ProductCreate, content Content, source string, snapshot any) (Result, error) {
	var out Result
	parent, e := manufacturerTx(ctx, tx, auth.Scope, in.ManufacturerID, auth.Entry == "tenant" && auth.Has(access.TenantDeviceCatalogRead))
	if e != nil {
		return out, e
	}
	if parent.Status != "active" {
		return out, k.Problem("definition_unavailable", "厂家已归档，不能新建或复制产品。", 409)
	}
	e = tx.QueryRow(ctx, `INSERT INTO device_products(owner_tenant_id,manufacturer_id,code,created_by,updated_by) VALUES($1,$2,$3,$4,$4) RETURNING id::text,revision::text`, owner(auth.Scope), in.ManufacturerID, in.Code, auth.Account).Scan(&out.ID, &out.Revision)
	if e != nil {
		return out, e
	}
	var previous *Content
	if source != "" {
		previous = &content
	}
	issues, _, e := validateTx(ctx, tx, auth, out.ID, content, previous, false)
	if e != nil {
		return out, e
	}
	if e = checkDraftReferences(issues); e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO device_product_drafts(product_id,content,copied_from_version_id,source_snapshot,updated_by) VALUES($1,$2,$3,$4,$5)`, out.ID, k.JSON(content), k.Nullable(source), k.JSON(snapshot), auth.Account)
	out.Changed = true
	return out, e
}
func (s *Service) CreateProduct(ctx context.Context, a access.Actor, scope Scope, in ProductCreate, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	in.Code = k.Text(in.Code)
	if !k.Code(in.Code) {
		return out, k.Invalid("/code", "code", "产品编号格式无效。")
	}
	content := Content{SchemaVersion: 1, Name: in.Name, Model: in.Model, Description: in.Description, Purpose: in.Purpose, OutputSlots: in.OutputSlots, ReleaseNotes: in.ReleaseNotes}
	if e := Normalize(&content); e != nil {
		return out, e
	}
	in.Name = content.Name
	in.Model = content.Model
	in.Description = content.Description
	in.Purpose = content.Purpose
	in.OutputSlots = content.OutputSlots
	in.ReleaseNotes = content.ReleaseNotes
	e := s.transact(ctx, a, scope, "product", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", "create")
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, in)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		out, e = createProductTx(ctx, tx, auth, in, content, "", nil)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, in, "product", out.ID, in.ManufacturerID, out.ID, "", nil, content, out, meta)
	})
	return out, e
}
func (s *Service) CopyProduct(ctx context.Context, a access.Actor, scope Scope, in ProductCopy, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	in.Code = k.Text(in.Code)
	if !k.Code(in.Code) {
		return out, k.Invalid("/code", "code", "产品编号格式无效。")
	}
	e := s.transact(ctx, a, scope, "product", "copy", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", "copy")
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, in)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		version, product, e := ResolveVersionTx(ctx, tx, scope.Tenant, in.SourceVersionID, true)
		if e != nil {
			return e
		}
		if scope.Entry == "tenant" && product.OwnerTenantID == nil && !auth.Has(access.TenantDeviceCatalogRead) {
			return k.NotFound("definition")
		}
		content := version.Content
		if in.Name != nil {
			content.Name = *in.Name
		}
		if in.Model != nil {
			content.Model = *in.Model
		}
		if e = Normalize(&content); e != nil {
			return e
		}
		out, e = createProductTx(ctx, tx, auth, ProductCreate{Meta: in.Meta, ManufacturerID: in.ManufacturerID, Code: in.Code}, content, version.ID, map[string]any{"product_id": product.ID, "version_id": version.ID, "number": version.Number, "content": version.Content})
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, in, "product", out.ID, in.ManufacturerID, out.ID, "", nil, content, out, meta)
	})
	return out, e
}
func (s *Service) SaveDraft(ctx context.Context, a access.Actor, scope Scope, id string, in DraftSave, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := Normalize(&in.Content); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "product", "draft-save", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", "draft-save")
		input := []any{id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		p, e := productTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if d == nil {
			return k.Problem("revision_conflict", "当前没有草稿，请先从已有版本新建草稿。", 409)
		}
		if e = k.Expect(in.Revision, d.Revision); e != nil {
			return e
		}
		issues, _, e := validateTx(ctx, tx, auth, id, in.Content, &d.Content, false)
		if e != nil {
			return e
		}
		if e = checkDraftReferences(issues); e != nil {
			return e
		}
		out = Result{ID: id, Changed: true, Issues: issues}
		e = tx.QueryRow(ctx, `UPDATE device_product_drafts SET content=$2,revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE product_id=$1 RETURNING revision::text`, id, k.JSON(in.Content), a.ID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, input, "product", id, p.ManufacturerID, id, "", d.Content, in.Content, out, meta)
	})
	return out, e
}
func (s *Service) NewDraft(ctx context.Context, a access.Actor, scope Scope, id string, in DraftCreate, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "product", "draft-create", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", "draft-create")
		input := []any{id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		p, e := productTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, p.Revision); e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if d != nil {
			return k.Problem("revision_conflict", "已有草稿，请继续编辑。", 409)
		}
		version, _, e := ResolveVersionTx(ctx, tx, scope.Tenant, in.VersionID, false)
		if e != nil {
			return e
		}
		if version.ProductID != id {
			return k.NotFound("definition")
		}
		_, e = tx.Exec(ctx, `INSERT INTO device_product_drafts(product_id,base_version_id,content,updated_by) VALUES($1,$2,$3,$4)`, id, version.ID, k.JSON(version.Content), a.ID)
		if e != nil {
			return e
		}
		out = Result{ID: id, Revision: "1", Changed: true}
		_, e = tx.Exec(ctx, `UPDATE device_products SET revision=revision+1,updated_at=clock_timestamp(),updated_by=$2 WHERE id=$1`, id, a.ID)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, input, "product", id, p.ManufacturerID, id, version.ID, nil, version.Content, out, meta)
	})
	return out, e
}
func (s *Service) DeleteDraft(ctx context.Context, a access.Actor, scope Scope, id string, in DraftRevision, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := in.Meta.RequireReason(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "product", "draft-delete", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", "draft-delete")
		input := []any{id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		p, e := productTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if d == nil {
			return k.NotFound("definition")
		}
		if e = k.Expect(in.Revision, d.Revision); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM device_product_drafts WHERE product_id=$1`, id); e != nil {
			return e
		}
		out = Result{ID: id, Changed: true}
		e = tx.QueryRow(ctx, `UPDATE device_products SET revision=revision+1,updated_at=clock_timestamp(),updated_by=$2 WHERE id=$1 RETURNING revision::text`, id, a.ID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, input, "product", id, p.ManufacturerID, id, "", d, nil, out, meta)
	})
	return out, e
}
func (s *Service) Validate(ctx context.Context, a access.Actor, scope Scope, id string) (Validation, error) {
	out := Validation{Issues: []Issue{}}
	e := s.transact(ctx, a, scope, "product", "validate", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := productTx(ctx, tx, scope, id); e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if d == nil {
			return k.NotFound("definition")
		}
		out.Issues, _, e = validateTx(ctx, tx, auth, id, d.Content, &d.Content, true)
		out.Valid = len(out.Issues) == 0
		return e
	})
	return out, e
}
func (s *Service) Publish(ctx context.Context, a access.Actor, scope Scope, id string, in DraftRevision, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "product", "publish", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", "publish")
		input := []any{id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		p, e := productTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT id FROM device_products WHERE id=$1 FOR UPDATE`, id); e != nil {
			return e
		}
		d, e := draftTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if d == nil {
			return k.Problem("revision_conflict", "草稿已关闭或发布。", 409)
		}
		if e = k.Expect(in.Revision, d.Revision); e != nil {
			return e
		}
		for _, slot := range d.Content.OutputSlots {
			if slot.Mode == "fixed" {
				permission := access.EngineeringMetricRead
				if scope.Entry == "tenant" {
					permission = access.TenantEngineeringCatalogRead
				}
				if e := auth.Require(permission); e != nil {
					return e
				}
				break
			}
		}
		issues, _, e := validateTx(ctx, tx, auth, id, d.Content, &d.Content, true)
		if e != nil {
			return e
		}
		if len(issues) > 0 {
			return &Error{Code: "validation_failed", Message: "产品未通过发布校验。", Status: 422, Issues: issues}
		}
		out = Result{ID: id, Changed: true}
		e = tx.QueryRow(ctx, `INSERT INTO device_product_versions(product_id,number,schema_version,content,content_hash,source_version_id,published_by) SELECT $1,COALESCE(MAX(number),0)+1,1,$2,$3,$4,$5 FROM device_product_versions WHERE product_id=$1 RETURNING id::text,number`, id, k.JSON(d.Content), k.Hash(d.Content), d.CopiedFromVersionID, a.ID).Scan(&out.VersionID, &out.Number)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO device_product_version_states(version_id,updated_by) VALUES($1,$2)`, out.VersionID, a.ID); e != nil {
			return e
		}
		for i, slot := range d.Content.OutputSlots {
			if _, e = tx.Exec(ctx, `INSERT INTO device_product_slot_keys(product_id,key,mode,core_signature) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, slot.Key, slot.Mode, slot.Core()); e != nil {
				return e
			}
			var shapes any
			if slot.Mode == "configurable" {
				shapes = slot.AllowedShapes
			}
			if _, e = tx.Exec(ctx, `INSERT INTO device_product_version_slots(version_id,product_id,slot_key,mode,slot_order,content,metric_version_id,unit_version_id,allowed_shapes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, out.VersionID, id, slot.Key, slot.Mode, slot.Order, k.JSON(slot), k.Nullable(slot.MetricVersionID), slot.UnitVersionID, shapes); e != nil {
				return e
			}
			if slot.Mode == "fixed" {
				for _, dep := range []struct{ id, path string }{{slot.MetricVersionID, "/metric_version_id"}, {value(slot.UnitVersionID), "/unit_version_id"}} {
					if dep.id != "" {
						if _, e = tx.Exec(ctx, `INSERT INTO device_product_dependencies(product_version_id,engineering_version_id,path) VALUES($1,$2,$3)`, out.VersionID, dep.id, fmtPath(i)+dep.path); e != nil {
							return e
						}
					}
				}
			}
		}
		if _, e = tx.Exec(ctx, `DELETE FROM device_product_drafts WHERE product_id=$1`, id); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `UPDATE device_products SET revision=revision+1,updated_at=clock_timestamp(),updated_by=$2 WHERE id=$1 RETURNING revision::text`, id, a.ID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, input, "product", id, p.ManufacturerID, id, out.VersionID, d, d.Content, out, meta)
	})
	return out, e
}
func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Receipt revalidates the original scope and object read before exposing a result.
func (s *Service) Receipt(ctx context.Context, a access.Actor, scope Scope, id string) (Result, error) {
	var out Result
	if !access.ValidID(id) {
		return out, k.NotFound("definition")
	}
	e := s.transact(ctx, a, scope, "request", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		result, _, _, target, ok, e := k.ReceiptTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		if !ok {
			return k.NotFound("definition")
		}
		var kind, targetID string
		for i, c := range target {
			if c == ':' {
				kind = target[:i]
				targetID = target[i+1:]
				break
			}
		}
		read := access.DeviceDefinitionRead
		if scope.Entry == "tenant" {
			read = access.TenantDeviceDefinitionRead
		}
		if !auth.Has(read) {
			return k.NotFound("definition")
		}
		table := ""
		if kind == "manufacturer" {
			table = "device_manufacturers"
		}
		if kind == "product" {
			table = "device_products"
		}
		if table == "" {
			return k.NotFound("definition")
		}
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id=$1 AND owner_tenant_id IS NOT DISTINCT FROM $2::uuid)`, targetID, owner(scope)).Scan(&exists)
		if e != nil {
			return e
		}
		if !exists {
			return k.NotFound("definition")
		}
		out = result
		return nil
	})
	return out, e
}

// JSON fields that explicitly accept null are decoded by their own DTO; other
// optional string fields must not silently interpret null as "unchanged".
func RejectNull(raw []byte, fields ...string) error {
	var m map[string]json.RawMessage
	if e := json.Unmarshal(raw, &m); e != nil {
		return e
	}
	for _, field := range fields {
		if v, ok := m[field]; ok && string(v) == "null" {
			return k.Problem("invalid_request", "此字段不接受 null。", 400)
		}
	}
	return nil
}
