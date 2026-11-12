package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
)

func (s *Service) List(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Device], error) {
	out := k.Page[Device]{Items: []Device{}}
	e := s.transact(ctx, a, scope, "device", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		if !slices.Contains([]string{"", "all", "public", "private"}, f.Source) || !slices.Contains([]string{"", "enabled", "disabled"}, f.Status) || !slices.Contains([]string{"", "recorded", "unrecorded"}, f.Installation) {
			return k.Problem("invalid_request", "设备筛选条件无效。", 400)
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "devices"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+deviceJSON+deviceFrom+` WHERE d.tenant_id=$1 AND d.deleted_at IS NULL
   AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=d.tenant_id AND g.project_id=$2 AND g.device_id=d.id AND g.kind='device' AND g.ended_at IS NULL))
   AND ($3='' OR d.name ILIKE '%'||$3||'%' OR d.code ILIKE '%'||$3||'%' OR d.serial_number ILIKE '%'||$3||'%') AND ($4='' OR d.status=$4)
   AND ($5::uuid IS NULL OR p.manufacturer_id=$5) AND ($6::uuid IS NULL OR d.product_id=$6) AND ($7::uuid IS NULL OR d.product_version_id=$7)
   AND ($8='' OR ($8='recorded')=EXISTS(SELECT 1 FROM device_installations i WHERE i.device_id=d.id AND i.uninstalled_at IS NULL))
   AND ($12 IN ('','all') OR ($12='public')=(p.owner_tenant_id IS NULL)) AND ($9::timestamptz IS NULL OR (d.created_at,d.id)>($9,$10::uuid)) ORDER BY d.created_at,d.id LIMIT $11`, auth.Tenant, k.Nullable(auth.Project), f.Query, f.Status, k.Nullable(f.ManufacturerID), k.Nullable(f.ProductID), k.Nullable(f.ProductVersionID), f.Installation, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1, f.Source)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var d Device
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &d); e != nil {
				return e
			}
			d.Visibility = "full"
			d.Actions = deviceActions(auth, d)
			out.Items = append(out.Items, d)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.At = last.CreatedAt
			cursor.ID = last.ID
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
func (s *Service) Detail(ctx context.Context, a access.Actor, scope Scope, id string) (Device, error) {
	var out Device
	e := s.transact(ctx, a, scope, "device", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := projectReadable(ctx, tx, auth, "device", id, false); e != nil {
			return e
		}
		var e error
		out, e = loadDevice(ctx, tx, scope.Tenant, id, false)
		if e != nil {
			return e
		}
		out.Actions = deviceActions(auth, out)
		var raw []byte
		e = tx.QueryRow(ctx, `SELECT to_jsonb(i)||jsonb_build_object('revision',revision::text) FROM device_installations i WHERE tenant_id=$1 AND device_id=$2 AND uninstalled_at IS NULL`, auth.Tenant, id).Scan(&raw)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		var i Installation
		if e = json.Unmarshal(raw, &i); e != nil {
			return e
		}
		i, e = s.projectInstallation(ctx, tx, a, auth, i)
		if e != nil {
			return e
		}
		out.CurrentInstallation = &i
		return nil
	})
	return out, e
}
func (s *Service) Output(ctx context.Context, a access.Actor, scope Scope, device, id string) (Output, error) {
	var out Output
	e := s.transact(ctx, a, scope, "output", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := projectReadable(ctx, tx, auth, "output", id, false); e != nil {
			return e
		}
		var e error
		out, e = loadOutput(ctx, tx, auth.Tenant, device, id)
		if e != nil {
			return e
		}
		use := auth.Entry == "tenant" || projectReadable(ctx, tx, auth, "output", id, true) == nil
		out.Actions = outputActions(auth, out, use)
		return nil
	})
	return out, e
}
func (s *Service) Outputs(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Output], error) {
	out := k.Page[Output]{Items: []Output{}}
	e := s.transact(ctx, a, scope, "output", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		if !slices.Contains([]string{"", "all", "public", "private"}, f.Source) || !slices.Contains([]string{"", "enabled", "disabled"}, f.Status) {
			return k.Problem("invalid_request", "设备状态筛选无效。", 400)
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "outputs"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+outputJSON+`,g.can_use_source`+outputFrom+` JOIN device_products p ON p.id=d.product_id JOIN project_device_grants g ON g.tenant_id=o.tenant_id AND g.output_id=o.id AND g.kind='output' AND g.project_id=$2 AND g.ended_at IS NULL WHERE o.tenant_id=$1 AND d.deleted_at IS NULL AND ($3='' OR o.name ILIKE '%'||$3||'%' OR d.code ILIKE '%'||$3||'%' OR d.name ILIKE '%'||$3||'%') AND ($4::uuid IS NULL OR o.device_id=$4) AND ($5::timestamptz IS NULL OR (o.started_at,o.id)>($5,$6::uuid)) AND ($8::uuid IS NULL OR p.manufacturer_id=$8) AND ($9::uuid IS NULL OR d.product_id=$9) AND ($10='' OR d.status=$10) AND ($11 IN ('','all') OR ($11='public')=(p.owner_tenant_id IS NULL)) ORDER BY o.started_at,o.id LIMIT $7`, auth.Tenant, auth.Project, f.Query, k.Nullable(f.DeviceID), cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1, k.Nullable(f.ManufacturerID), k.Nullable(f.ProductID), f.Status, f.Source)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var o Output
			var use bool
			if e = rows.Scan(&raw, &use); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &o); e != nil {
				return e
			}
			o.Visibility = "full"
			o.Actions = outputActions(auth, o, use)
			out.Items = append(out.Items, o)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.At = last.StartedAt
			cursor.ID = last.ID
			out.NextCursor = cursor.Encode()
		}
		rows.Close()
		return outputPageDefinitions(ctx, tx, out.Items)
	})
	return out, e
}
func (s *Service) Slots(ctx context.Context, a access.Actor, scope Scope, device string, f Filter) (k.Page[OutputSlot], error) {
	out := k.Page[OutputSlot]{Items: []OutputSlot{}}
	e := s.transact(ctx, a, scope, "output", "slots", false, func(tx pgx.Tx, auth k.Authorization) error {
		d, e := loadDevice(ctx, tx, auth.Tenant, device, false)
		if e != nil {
			return e
		}
		if e = f.Validate(); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"slots", device, d.OutputRevision}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT s.content,COALESCE(o.id::text,'') FROM device_product_version_slots s LEFT JOIN device_outputs o ON o.product_version_id=s.version_id AND o.slot_key=s.slot_key AND o.device_id=$2 AND o.ended_at IS NULL WHERE s.version_id=$1 AND ($3='' OR s.content->>'name' ILIKE '%'||$3||'%' OR s.slot_key ILIKE '%'||$3||'%') AND ($4='' OR (s.slot_order,s.slot_key)>($5,$4)) ORDER BY s.slot_order,s.slot_key LIMIT $6`, d.ProductVersionID, device, f.Query, cursor.Key, cursor.Number, f.Limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var slot OutputSlot
			var id string
			if e = rows.Scan(&slot.Template, &id); e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, slot)
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			ids = ids[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = device
			cursor.Key = last.Template.Key
			cursor.Number = last.Template.Order
			out.NextCursor = cursor.Encode()
		}

		nonempty := []string{}
		for _, id := range ids {
			if id != "" {
				nonempty = append(nonempty, id)
			}
		}
		outputs, e := s.OutputsByIDTx(ctx, tx, a, scope, nonempty, false)
		if e != nil {
			return e
		}
		for i, id := range ids {
			if o, ok := outputs[id]; ok {
				out.Items[i].Current = &o
			}
		}

		roots := []string{}
		for _, slot := range out.Items {
			if slot.Template.Mode == "fixed" {
				roots = append(roots, slot.Template.MetricVersionID)
				if slot.Template.UnitVersionID != nil {
					roots = append(roots, *slot.Template.UnitVersionID)
				}
			}
		}
		definitions, e := engineering.ResolvePublicBatchTx(ctx, tx, roots, false)
		if e != nil {
			return e
		}
		for i := range out.Items {
			slot := out.Items[i].Template
			refs := []string{}
			if slot.Mode == "fixed" {
				refs = append(refs, slot.MetricVersionID)
				if slot.UnitVersionID != nil {
					refs = append(refs, *slot.UnitVersionID)
				}
			}
			out.Items[i].Definitions = engineering.PublicDependencySet(definitions, refs)
		}
		return nil
	})
	return out, e
}
func (s *Service) Generations(ctx context.Context, a access.Actor, scope Scope, device string, f Filter) (k.Page[Output], error) {
	out := k.Page[Output]{Items: []Output{}}
	e := s.transact(ctx, a, scope, "output", "generations", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := loadDevice(ctx, tx, auth.Tenant, device, false); e != nil {
			return e
		}
		if e := f.Validate(); e != nil {
			return e
		}
		if !k.Key(f.SlotKey) {
			return k.Problem("invalid_request", "请选择有效槽位键。", 400)
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"generations", device}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+outputJSON+outputFrom+` WHERE o.tenant_id=$1 AND o.device_id=$2 AND o.slot_key=$3 AND ($4=0 OR o.generation<$4) ORDER BY o.generation DESC LIMIT $5`, auth.Tenant, device, f.SlotKey, cursor.Number, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var o Output
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &o); e != nil {
				return e
			}
			o.Actions = outputActions(auth, o, true)
			o.Visibility = "full"
			out.Items = append(out.Items, o)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.Number = last.Generation
			out.NextCursor = cursor.Encode()
		}
		rows.Close()
		return outputPageDefinitions(ctx, tx, out.Items)
	})
	return out, e
}

const adoptableProducts = `WITH RECURSIVE blocked(id) AS (SELECT version_id FROM engineering_version_states WHERE NOT enabled UNION SELECT d.version_id FROM engineering_dependencies d JOIN blocked b ON b.id=d.target_id) `

func (s *Service) ProductCandidates(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[c.Version], error) {
	out := k.Page[c.Version]{Items: []c.Version{}}
	e := s.transact(ctx, a, scope, "device", "product-candidates", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		pub, private := auth.Has(access.TenantDeviceCatalogRead), auth.Has(access.TenantDeviceDefinitionRead)
		if !pub && !private {
			return access.ErrDenied
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "product-candidates"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, adoptableProducts+`SELECT v.id::text,v.published_at FROM device_product_versions v JOIN device_product_version_states s ON s.version_id=v.id JOIN device_products p ON p.id=v.product_id JOIN device_manufacturers m ON m.id=p.manufacturer_id WHERE m.deleted_at IS NULL AND p.deleted_at IS NULL AND s.enabled AND (($2 AND p.owner_tenant_id IS NULL) OR ($3 AND p.owner_tenant_id=$1)) AND NOT EXISTS(SELECT 1 FROM device_product_dependencies d JOIN blocked b ON b.id=d.engineering_version_id WHERE d.product_version_id=v.id) AND ($4='' OR p.code ILIKE '%'||$4||'%' OR v.content->>'name' ILIKE '%'||$4||'%' OR v.content->>'model' ILIKE '%'||$4||'%') AND ($5::uuid IS NULL OR p.id=$5) AND ($6::timestamptz IS NULL OR (v.published_at,v.id)>($6,$7::uuid)) ORDER BY v.published_at,v.id LIMIT $8`, auth.Tenant, pub, private, f.Query, k.Nullable(f.ProductID), cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			var at any
			if e = rows.Scan(&id, &at); e != nil {
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
		more := len(ids) > f.Limit
		if more {
			ids = ids[:f.Limit]
		}

		versions, _, e := c.ResolveVersionsTx(ctx, tx, auth.Tenant, ids, true)
		if e != nil {
			return e
		}
		for _, id := range ids {
			out.Items = append(out.Items, versions[id])
		}

		if more {
			last := out.Items[len(out.Items)-1]
			cursor.ID = last.ID
			cursor.At = last.PublishedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}

// OutputsByIDTx batches an already SQL-paginated set while independently enforcing
// current scope and grants. It is not a bypass for callers passing guessed IDs.
func (s *Service) OutputsByIDTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, ids []string, use bool) (map[string]Output, error) {
	out := map[string]Output{}
	op := scope.Operation("output", "detail")
	if use {
		op = scope.Operation("source", "candidates")
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, op)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT `+outputJSON+`,COALESCE(g.can_use_source,false)`+outputFrom+` LEFT JOIN project_device_grants g ON g.tenant_id=o.tenant_id AND g.project_id=$2 AND g.kind='output' AND g.output_id=o.id AND g.ended_at IS NULL WHERE o.tenant_id=$1 AND o.id=ANY($3::uuid[]) AND d.deleted_at IS NULL AND ($2::uuid IS NULL OR (g.id IS NOT NULL AND (NOT $4 OR g.can_use_source)))`, scope.Tenant, k.Nullable(scope.Project), ids, use)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var o Output
		var canUse bool
		if e = rows.Scan(&raw, &canUse); e != nil {
			return out, e
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			return out, e
		}
		o.Visibility = "full"
		o.Actions = outputActions(auth, o, scope.Entry == "tenant" || canUse)
		out[o.ID] = o
	}

	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	outputs := make([]*Output, 0, len(out))
	for id := range out {
		o := out[id]
		outputs = append(outputs, &o)
	}
	if e = outputDefinitions(ctx, tx, outputs); e != nil {
		return out, e
	}
	for _, o := range outputs {
		out[o.ID] = *o
	}
	return out, nil
}
