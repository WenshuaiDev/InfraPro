package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

func (s *Service) Changes(ctx context.Context, a access.Actor, scope Scope, device, output string, f Filter) (k.Page[Change], error) {
	out := k.Page[Change]{Items: []Change{}}
	kind, id := "device", device
	if output != "" {
		kind = "output"
		id = output
	}
	e := s.transact(ctx, a, scope, kind, "changes", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := projectReadable(ctx, tx, auth, kind, id, false); e != nil {
			return e
		}
		if output != "" {
			o, e := loadOutput(ctx, tx, auth.Tenant, "", output)
			if e != nil {
				return e
			}
			device = o.DeviceID
		} else {
			if _, e := loadDevice(ctx, tx, auth.Tenant, device, true); e != nil {
				return e
			}
		}
		if e := f.Validate(); e != nil {
			return e
		}
		if !slices.Contains([]string{"", "device", "output", "installation", "source"}, f.Kind) {
			return k.Problem("invalid_request", "履历类型无效。", 400)
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"device-changes", device, output}))
		if e != nil {
			return e
		}
		channelHistory := auth.Has(access.ProjectEngineeringResourceRead) && auth.Has(access.ProjectEngineeringResourceHistoryRead)
		if auth.Entry == "tenant" {
			channelHistory = auth.Has(access.TenantEngineeringResourceRead)
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(c) FROM device_resource_changes c WHERE c.tenant_id=$1 AND c.device_id=$2 AND ($3::uuid IS NULL OR c.output_id=$3 OR (split_part(c.operation,':',3)='output' AND c.after->>'id'=$3::text))
   AND (c.binding_id IS NULL OR $5)
   AND ($4::uuid IS NULL OR ((c.output_id IS NULL OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=c.tenant_id AND g.project_id=$4 AND g.kind='output' AND (g.output_id=c.output_id OR (split_part(c.operation,':',3)='output' AND g.output_id::text=c.after->>'id')) AND g.ended_at IS NULL))
    AND (c.binding_id IS NULL OR ($5 AND EXISTS(SELECT 1 FROM monitoring_source_bindings b JOIN project_resource_grants g ON g.channel_id=b.channel_id JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE b.id=c.binding_id AND g.project_id=$4 AND g.ended_at IS NULL)))))
   AND ($9='' OR split_part(c.operation,':',3)=$9) AND ($10='' OR c.operation ILIKE '%'||$10||'%' OR ($4::uuid IS NULL AND (c.reason ILIKE '%'||$10||'%' OR c.actor_name ILIKE '%'||$10||'%'))) AND ($6::timestamptz IS NULL OR (c.changed_at,c.id)<($6,$7::uuid)) ORDER BY c.changed_at DESC,c.id DESC LIMIT $8`, auth.Tenant, device, k.Nullable(output), k.Nullable(auth.Project), channelHistory, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1, f.Kind, f.Query)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw []byte
			var change Change
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(raw, &change); e != nil {
				rows.Close()
				return e
			}
			change.Visibility = "full"
			out.Items = append(out.Items, change)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.ChangedAt
			out.NextCursor = cursor.Encode()
		}
		return s.projectChanges(ctx, tx, a, auth, out.Items)
	})
	return out, e
}

// History snapshots are never assumed safe merely because the containing device
// is visible. Related outputs, targets, actors and free text have separate rights.
func projectHistoryFields(raw json.RawMessage, operation string) json.RawMessage {
	if strings.Contains(operation, ":source:") || strings.Contains(operation, ":installation:") {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	if nested, ok := object["device"]; ok && strings.Contains(operation, ":device:") {
		if json.Unmarshal(nested, &object) != nil {
			return nil
		}
	}
	allowed := []string{"id", "code", "name", "description", "status", "serial_number", "revision", "product", "product_id", "product_version_id"}
	if strings.Contains(operation, ":output:") {
		allowed = []string{"id", "slot_key", "generation", "name", "description", "revision", "metric_version_id", "unit_version_id", "value_shape", "timestamp_meaning", "aggregation_description", "observation_direction", "reference_basis", "started_at", "ended_at"}
	}
	safe := map[string]json.RawMessage{}
	for _, key := range allowed {
		if v, ok := object[key]; ok {
			safe[key] = v
		}
	}
	return k.JSON(safe)
}
func (s *Service) Receipt(ctx context.Context, a access.Actor, scope Scope, request string) (Result, error) {
	var out Result
	if !access.ValidID(request) {
		return out, k.NotFound("device")
	}
	e := k.Transact(ctx, s.access, a, scope, scope.Operation("request", "detail"), k.Locks{}, func(tx pgx.Tx, auth k.Authorization) error {
		result, _, _, target, found, e := k.ReceiptTx(ctx, tx, auth, request)
		if e != nil {
			return e
		}
		if !found {
			return k.NotFound("device")
		}
		parts := strings.SplitN(target, ":", 2)
		if len(parts) != 2 {
			return k.NotFound("device")
		}
		kind, id := parts[0], parts[1]
		require := func(codes ...access.Code) error {
			if auth.Require(codes...) != nil {
				return k.NotFound("device")
			}
			return nil
		}
		switch kind {
		case "manufacturer", "product":
			permission := access.DeviceDefinitionRead
			if scope.Entry == "tenant" {
				permission = access.TenantDeviceDefinitionRead
			}
			if scope.Entry == "project" {
				return k.NotFound("device")
			}
			if e = require(permission); e != nil {
				return e
			}
			table := "device_products"
			if kind == "manufacturer" {
				table = "device_manufacturers"
			}
			var visible bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id=$1 AND owner_tenant_id IS NOT DISTINCT FROM $2::uuid)`, id, k.Nullable(scope.Tenant)).Scan(&visible)
			if e != nil {
				return e
			}
			if !visible {
				return k.NotFound("device")
			}
		case "device", "device_output", "device_installation":
			permission := access.TenantDeviceRead
			if scope.Entry == "project" {
				permission = access.ProjectDeviceRead
			}
			if e = require(permission); e != nil {
				return e
			}
			device := id
			objectKind := "device"
			if kind == "device_output" {
				o, e := loadOutput(ctx, tx, auth.Tenant, "", id)
				if e != nil {
					return e
				}
				device = o.DeviceID
				objectKind = "output"
			}
			if kind == "device_installation" {
				e = tx.QueryRow(ctx, `SELECT device_id::text FROM device_installations WHERE tenant_id=$1 AND id=$2`, auth.Tenant, id).Scan(&device)
				if e == pgx.ErrNoRows {
					return k.NotFound("device")
				}
				if e != nil {
					return e
				}
			}
			target := device
			if objectKind == "output" {
				target = id
			}
			if e = projectReadable(ctx, tx, auth, objectKind, target, false); e != nil {
				return e
			}
			if _, e = loadDevice(ctx, tx, auth.Tenant, device, true); e != nil {
				return e
			}
		case "monitoring_source":
			var device, output, channel string
			e = tx.QueryRow(ctx, `SELECT device_id::text,output_id::text,channel_id::text FROM monitoring_source_bindings WHERE tenant_id=$1 AND id=$2`, auth.Tenant, id).Scan(&device, &output, &channel)
			if e == pgx.ErrNoRows {
				return k.NotFound("device")
			}
			if e != nil {
				return e
			}
			if scope.Entry == "tenant" {
				if e = require(access.TenantDeviceRead, access.TenantEngineeringResourceRead); e != nil {
					return e
				}
			} else {
				if e = require(access.ProjectDeviceRead, access.ProjectEngineeringResourceRead); e != nil {
					return e
				}
				if e = projectReadable(ctx, tx, auth, "output", output, false); e != nil {
					return e
				}
				var visible bool
				e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.channel_id=$3 AND g.ended_at IS NULL)`, auth.Tenant, auth.Project, channel).Scan(&visible)
				if e != nil {
					return e
				}
				if !visible {
					return k.NotFound("device")
				}
			}
		case "project_device_grant_set":
			if scope.Entry != "tenant" {
				return k.NotFound("device")
			}
			if e = require(access.TenantDeviceAuthorizationRead, access.TenantDeviceRead, access.TenantProjectRead); e != nil {
				return e
			}
			if _, e = grantStateTx(ctx, tx, scope.Tenant, id); e != nil {
				return e
			}
		default:
			return k.NotFound("device")
		}
		out = result
		return nil
	})
	return out, e
}
