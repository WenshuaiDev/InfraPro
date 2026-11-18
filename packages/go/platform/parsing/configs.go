package parsing

import (
	"context"
	"fmt"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
)

const configJSON = `to_jsonb(r)||jsonb_build_object('number',r.number::text,'name',c.name,'enabled',c.enabled,'revision',c.revision::text)`

func configTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, newUse bool) (ConfigRevision, error) {
	if !access.ValidID(id) {
		return ConfigRevision{}, k.NotFound("parsing")
	}
	v, e := readJSON[ConfigRevision](tx.QueryRow(ctx, `SELECT `+configJSON+` FROM parsing_config_revisions r JOIN parsing_configs c ON c.id=r.config_id WHERE r.id=$1 AND (c.tenant_id IS NULL OR c.tenant_id=$2)`, id, k.Nullable(auth.Tenant)))
	if e != nil {
		return v, e
	}
	if v.Snapshot.OwnerKind == "device" {
		if _, e = deviceTx(ctx, tx, auth, v.Snapshot.OwnerID); e != nil {
			return v, e
		}
	} else {
		_, _, e = productTx(ctx, tx, auth, "", v.ProductVersionID, false, false)
		if e != nil {
			return v, e
		}
	}
	if newUse && !v.Enabled {
		return v, k.Problem("config_deprecated", "该配置已停止新采用。", 409)
	}
	return v, nil
}
func productTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, product, version string, write, newUse bool) (d.Version, d.Product, error) {
	v, p, e := d.ResolveVersionTx(ctx, tx, auth.Tenant, version, newUse)
	if e != nil {
		return v, p, e
	}
	if product != "" && p.ID != product {
		return v, p, k.NotFound("parsing")
	}
	if auth.Entry == "platform" {
		if p.OwnerTenantID != nil {
			return v, p, k.NotFound("parsing")
		}
		e = auth.Require(access.DeviceDefinitionRead)
		if write && e == nil {
			e = auth.Require(access.DeviceProductEdit, access.DeviceProductParsingManage, access.ParsingMethodRead)
		}
	} else if p.OwnerTenantID == nil {
		e = auth.Require(access.TenantDeviceCatalogRead)
		if write {
			e = access.ErrDenied
		}
	} else {
		if *p.OwnerTenantID != auth.Tenant {
			return v, p, k.NotFound("parsing")
		}
		e = auth.Require(access.TenantDeviceDefinitionRead)
		if write && e == nil {
			e = auth.Require(access.TenantDeviceProductEdit, access.TenantDeviceProductParsingManage, access.TenantParsingMethodRead)
		}
	}
	return v, p, e
}
func mappingTx(ctx context.Context, tx pgx.Tx, entry c.Entry, version string, bindings []Binding, ignored []Ignored, device string, newUse bool) ([]Mapping, error) {
	out := []Mapping{}
	outputs := map[string]c.Output{}
	for _, v := range entry.Outputs {
		outputs[v.Key] = v
	}
	seen := map[string]bool{}
	slots := map[string]bool{}
	for i, b := range bindings {
		path := fmt.Sprintf("/output_bindings/%d", i)
		o, ok := outputs[b.OutputKey]
		if !ok || seen[b.OutputKey] || slots[b.SlotKey] || !b.Confirmed {
			return nil, k.Invalid(path, "mapping_conflict", "输出键与槽位须唯一且逐项确认双方测量含义。")
		}
		seen[b.OutputKey] = true
		slots[b.SlotKey] = true
		var slot d.Slot
		if e := tx.QueryRow(ctx, `SELECT content FROM device_product_version_slots WHERE version_id=$1 AND slot_key=$2`, version, b.SlotKey).Scan(&slot); e == pgx.ErrNoRows {
			return nil, k.Invalid(path, "slot_missing", "产品版本没有该输出槽位。")
		} else if e != nil {
			return nil, e
		}
		m := Mapping{OutputKey: b.OutputKey, SlotKey: b.SlotKey, Status: "semantic_unknown", MethodSemantics: o.Semantics}
		if slot.Mode == "configurable" && !slices.Contains(slot.AllowedShapes, o.ValueType) {
			return nil, k.Invalid(path, "incompatible_shape", "方法值类型与槽位允许形态不相容。")
		}
		if slot.Mode == "fixed" {
			sem := d.Semantics{MetricVersionID: slot.MetricVersionID, UnitVersionID: slot.UnitVersionID, TimestampMeaning: slot.TimestampMeaning, AggregationDescription: slot.AggregationDescription}
			if e := checkSemanticsTx(ctx, tx, o.Semantics, &sem, newUse); e != nil {
				return nil, e
			}
			m.SlotSemantics = &sem
		}
		if device != "" {
			var sem d.Semantics
			var raw []byte
			e := tx.QueryRow(ctx, `SELECT to_jsonb(o),id::text,generation,semantic_hash FROM device_outputs o WHERE device_id=$1 AND slot_key=$2 AND ended_at IS NULL`, device, b.SlotKey).Scan(&raw, &m.OutputID, &m.OutputGeneration, &m.OutputSemanticHash)
			if e == pgx.ErrNoRows {
				m.Status = "output_unconfigured"
			} else if e != nil {
				return nil, e
			} else {
				if e = decode(raw, &sem); e != nil {
					return nil, e
				}
				if e = checkSemanticsTx(ctx, tx, o.Semantics, &sem, newUse); e != nil {
					return nil, e
				}
				m.SlotSemantics = &sem
				m.Status = "mapped"
				if sem.ObservationDirection == "" || sem.ReferenceBasis == "" {
					m.Status = "semantic_unknown"
				}
			}
		}
		out = append(out, m)
	}
	for i, b := range ignored {
		_, ok := outputs[b.OutputKey]
		if !ok || seen[b.OutputKey] || k.Text(b.Reason) == "" || len(b.Reason) > 2000 {
			return nil, k.Invalid(fmt.Sprintf("/ignored_outputs/%d", i), "invalid_ignore", "忽略项必须唯一、属于输出契约并填写理由。")
		}
		seen[b.OutputKey] = true
		out = append(out, Mapping{OutputKey: b.OutputKey, Status: "ignored", Reason: b.Reason, MethodSemantics: outputs[b.OutputKey].Semantics})
	}
	if len(seen) != len(outputs) {
		return nil, k.Invalid("/output_bindings", "unmapped_output", "所有测量输出必须映射或明确忽略。")
	}
	return out, nil
}
func checkSemanticsTx(ctx context.Context, tx pgx.Tx, m c.Semantics, sem *d.Semantics, newUse bool) error {
	v, defs, e := d.ResolveSemanticsTx(ctx, tx, *sem, newUse)
	if e != nil {
		return e
	}
	*sem = v
	unit := ""
	for _, def := range defs {
		if v.UnitVersionID != nil && def.ID == *v.UnitVersionID && def.Content.Unit != nil {
			unit = def.Content.Unit.Symbol
			break
		}
	}
	if m.ValueType != v.ValueShape || m.Unit != unit || m.TimestampMeaning != "" && m.TimestampMeaning != v.TimestampMeaning || m.Aggregation != "" && v.AggregationDescription != "" && m.Aggregation != v.AggregationDescription || m.Direction != "" && v.ObservationDirection != "" && m.Direction != v.ObservationDirection || m.Reference != "" && v.ReferenceBasis != "" && m.Reference != v.ReferenceBasis {
		return k.Invalid("/output_bindings", "incompatible_semantics", "单位、值形态、时间、统计含义、方向或基准存在已知矛盾，不能采用。")
	}
	return nil
}
func (s *Service) Configs(ctx context.Context, a access.Actor, scope Scope, product, version string, public bool, f Filter) (k.Page[ConfigRevision], error) {
	out := k.Page[ConfigRevision]{Items: []ConfigRevision{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	kind := "configs"
	if public {
		kind = "public-configs"
	}
	e := s.transact(ctx, a, scope, kind, "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, _, e := productTx(ctx, tx, auth, product, version, false, false); e != nil {
			return e
		}
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, version))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+configJSON+` FROM parsing_configs c JOIN parsing_config_revisions r ON r.config_id=c.id WHERE c.product_version_id=$1 AND c.owner_kind='product' AND ($2::uuid IS NULL OR r.id>$2) AND ($3='' OR c.name ILIKE '%'||$3||'%') ORDER BY r.id LIMIT $4`, version, k.Nullable(cur.ID), f.Query, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := readJSON[ConfigRevision](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) DeviceConfigs(ctx context.Context, a access.Actor, scope Scope, device string, f Filter) (k.Page[ConfigRevision], error) {
	out := k.Page[ConfigRevision]{Items: []ConfigRevision{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	kind := "device-configs"
	e := s.transact(ctx, a, scope, kind, "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := deviceTx(ctx, tx, auth, device); e != nil {
			return e
		}
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, device))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+configJSON+` FROM parsing_configs c JOIN parsing_config_revisions r ON r.config_id=c.id WHERE c.device_id=$1 AND c.owner_kind='device' AND ($2::uuid IS NULL OR r.id>$2) AND ($3='' OR c.name ILIKE '%'||$3||'%') ORDER BY r.id LIMIT $4`, device, k.Nullable(cur.ID), f.Query, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := readJSON[ConfigRevision](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) SaveConfig(ctx context.Context, a access.Actor, scope Scope, product, version, device, config string, in ConfigInput) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	if e := k.String(in.Name, "/name", 200, true); e != nil {
		return out, e
	}
	kind, action := "configs", "create"
	if device != "" {
		kind = "device-configs"
	}
	if config != "" {
		action = "revision"
	}
	e := s.transact(ctx, a, scope, kind, action, true, func(tx pgx.Tx, auth k.Authorization) error {
		ownerKind, ownerID := "product", product
		if device != "" {
			v, e := deviceTx(ctx, tx, auth, device)
			if e != nil {
				return e
			}
			version = v.ProductVersionID
			product = v.ProductID
			ownerKind = "device"
			ownerID = device
			if e = auth.Require(access.TenantDeviceParsingManage, access.TenantParsingMethodRead); e != nil {
				return e
			}
			if _, _, e = productTx(ctx, tx, auth, product, version, false, true); e != nil {
				return e
			}
		} else {
			if _, _, e := productTx(ctx, tx, auth, product, version, true, true); e != nil {
				return e
			}
		}
		payload := []any{product, version, device, config, in}
		operation := kind + "." + action
		v, ok, e := replay(ctx, tx, auth, operation, in.Meta, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		if in.SourceRevisionID != "" {
			source, e := configTx(ctx, tx, auth, in.SourceRevisionID, false)
			if e != nil {
				return e
			}
			if in.MethodID == "" && in.MethodVersion == "" {
				in.MethodID = source.Snapshot.Method.ID
				in.MethodVersion = source.Snapshot.Method.Version
			}
			if in.Bindings == nil && in.Ignored == nil {
				in.Bindings = source.Snapshot.Bindings
				in.Ignored = source.Snapshot.Ignored
			}
		}
		m, e := methodTx(ctx, tx, in.MethodID, in.MethodVersion)
		if e != nil {
			return e
		}
		if e = allowedMethod(m, scope, true, false); e != nil {
			return e
		}
		mapping, e := mappingTx(ctx, tx, m.Entry, version, in.Bindings, in.Ignored, device, true)
		if e != nil {
			return e
		}
		out.ID = config
		if config == "" {
			if in.Revision != "0" {
				return k.Expect(in.Revision, "0")
			}
			e = tx.QueryRow(ctx, `INSERT INTO parsing_configs(tenant_id,owner_kind,product_id,product_version_id,device_id,name,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,revision::text`, k.Nullable(scope.Tenant), ownerKind, product, version, k.Nullable(device), in.Name, a.ID).Scan(&out.ID, &out.Revision)
		} else {
			var rev string
			e = tx.QueryRow(ctx, `SELECT revision::text FROM parsing_configs WHERE id=$1 AND owner_kind=$2 AND product_version_id=$3 AND tenant_id IS NOT DISTINCT FROM $4::uuid AND device_id IS NOT DISTINCT FROM $5::uuid FOR UPDATE`, config, ownerKind, version, k.Nullable(scope.Tenant), k.Nullable(device)).Scan(&rev)
			if e == pgx.ErrNoRows {
				return k.NotFound("parsing")
			}
			if e != nil {
				return e
			}
			if e = k.Expect(in.Revision, rev); e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `UPDATE parsing_configs SET name=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, config, in.Name).Scan(&out.Revision)
		}
		if e != nil {
			return e
		}
		snapshot := Snapshot{OwnerKind: ownerKind, OwnerID: ownerID, TenantID: scope.Tenant, ProductVersionID: version, Method: m.Entry.Method(), OutputContractDigest: m.Entry.OutputContractDigest, Bindings: in.Bindings, Ignored: in.Ignored, Mapping: mapping, Example: m.Entry.Example, SourceRevisionID: in.SourceRevisionID}
		e = tx.QueryRow(ctx, `INSERT INTO parsing_config_revisions(config_id,product_version_id,number,method_id,method_version,snapshot,content_digest,source_revision_id,reason,created_by) VALUES($1,$2,(SELECT COALESCE(max(number),0)+1 FROM parsing_config_revisions WHERE config_id=$1),$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, out.ID, version, in.MethodID, in.MethodVersion, k.JSON(snapshot), k.Hash(snapshot), k.Nullable(in.SourceRevisionID), in.Reason, a.ID).Scan(&out.ConfigRevisionID)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, operation, in.Meta, payload, out, nil, map[string]string{"config_id": out.ID, "revision_id": out.ConfigRevisionID, "revision": out.Revision, "method_id": in.MethodID, "method_version": in.MethodVersion})
	})
	return out, e
}
func defaultTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, version string) (Default, error) {
	out := Default{ProductVersionID: version, Revision: "0"}
	var id *string
	e := tx.QueryRow(ctx, `SELECT revision::text,config_revision_id::text FROM parsing_product_defaults WHERE product_version_id=$1`, version).Scan(&out.Revision, &id)
	if e == pgx.ErrNoRows {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	if id != nil {
		v, e := configTx(ctx, tx, auth, *id, false)
		if e != nil {
			return out, e
		}
		out.Config = &v
	}
	return out, nil
}
func (s *Service) Default(ctx context.Context, a access.Actor, scope Scope, product, version string, public bool) (Default, error) {
	var out Default
	kind := "defaults"
	if public {
		kind = "public-defaults"
	}
	e := s.transact(ctx, a, scope, kind, "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, _, e := productTx(ctx, tx, auth, product, version, false, false); e != nil {
			return e
		}
		var e error
		out, e = defaultTx(ctx, tx, auth, version)
		return e
	})
	return out, e
}
func (s *Service) SetDefault(ctx context.Context, a access.Actor, scope Scope, product, version string, in DefaultInput) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "defaults", "set", true, func(tx pgx.Tx, auth k.Authorization) error {
		if _, _, e := productTx(ctx, tx, auth, product, version, true, true); e != nil {
			return e
		}
		payload := []any{product, version, in}
		v, ok, e := replay(ctx, tx, auth, "defaults.set", in.Meta, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		before, e := defaultTx(ctx, tx, auth, version)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, before.Revision); e != nil {
			return e
		}
		if in.ConfigRevisionID != "" {
			cfg, e := configTx(ctx, tx, auth, in.ConfigRevisionID, true)
			if e != nil {
				return e
			}
			if cfg.ProductVersionID != version || cfg.Snapshot.OwnerKind != "product" {
				return k.NotFound("parsing")
			}
			m, e := methodTx(ctx, tx, cfg.Snapshot.Method.ID, cfg.Snapshot.Method.Version)
			if e != nil {
				return e
			}
			if e = allowedMethod(m, scope, true, false); e != nil {
				return e
			}
			if _, e = mappingTx(ctx, tx, m.Entry, version, cfg.Snapshot.Bindings, cfg.Snapshot.Ignored, "", true); e != nil {
				return e
			}
		}
		out.ID = version
		out.ConfigRevisionID = in.ConfigRevisionID
		e = tx.QueryRow(ctx, `INSERT INTO parsing_product_defaults(product_version_id,config_revision_id,revision,updated_by) VALUES($1,$2,1,$3) ON CONFLICT(product_version_id) DO UPDATE SET config_revision_id=$2,revision=parsing_product_defaults.revision+1,updated_by=$3,updated_at=clock_timestamp() RETURNING revision::text`, version, k.Nullable(in.ConfigRevisionID), a.ID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, "defaults.set", in.Meta, payload, out, map[string]string{"revision": before.Revision}, map[string]string{"revision": out.Revision, "revision_id": in.ConfigRevisionID})
	})
	return out, e
}
func (s *Service) SetConfigState(ctx context.Context, a access.Actor, scope Scope, product, version, config string, in StateInput) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	if in.State != "enabled" && in.State != "deprecated" {
		return out, k.Invalid("/state", "enum", "请选择允许或停止新采用。")
	}
	e := s.transact(ctx, a, scope, "configs", "state", true, func(tx pgx.Tx, auth k.Authorization) error {
		if _, _, e := productTx(ctx, tx, auth, product, version, true, false); e != nil {
			return e
		}
		var rev string
		if e := tx.QueryRow(ctx, `SELECT revision::text FROM parsing_configs WHERE id=$1 AND product_version_id=$2 AND owner_kind='product'`, config, version).Scan(&rev); e != nil {
			return k.NotFound("parsing")
		}
		payload := []any{product, version, config, in}
		v, ok, e := replay(ctx, tx, auth, "configs.state", in.Meta, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		if e = k.Expect(in.Revision, rev); e != nil {
			return e
		}
		out.ID = config
		e = tx.QueryRow(ctx, `UPDATE parsing_configs SET enabled=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, config, in.State == "enabled").Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, "configs.state", in.Meta, payload, out, nil, map[string]string{"state": in.State, "revision": out.Revision})
	})
	return out, e
}
