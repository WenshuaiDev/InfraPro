package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	access    *access.Service
	resources InstallationResources
}

func New(a *access.Service, r InstallationResources) *Service {
	return &Service{access: a, resources: r}
}
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, kind, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	if scope.Entry == "platform" {
		return k.NotFound("device")
	}
	return k.Transact(ctx, s.access, a, scope, scope.Operation(kind, action), k.Locks{TenantWrite: write}, fn)
}

const deviceJSON = `to_jsonb(d)||jsonb_build_object('revision',d.revision::text,'output_revision',d.output_revision::text,'installation_revision',d.installation_revision::text,
 'product',jsonb_build_object('id',p.id,'code',p.code,'name',v.content->>'name','model',v.content->>'model','version_id',v.id,'number',v.number,'manufacturer_id',m.id,'manufacturer_name',m.name,'source',CASE WHEN p.owner_tenant_id IS NULL THEN 'public' ELSE 'private' END))`
const deviceFrom = ` FROM devices d JOIN device_products p ON p.id=d.product_id JOIN device_product_versions v ON v.id=d.product_version_id JOIN device_manufacturers m ON m.id=p.manufacturer_id `

func loadDevice(ctx context.Context, tx pgx.Tx, tenant, id string, deleted bool) (Device, error) {
	var out Device
	if !access.ValidID(id) {
		return out, k.NotFound("device")
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT `+deviceJSON+deviceFrom+` WHERE d.tenant_id=$1 AND d.id=$2 AND ($3 OR d.deleted_at IS NULL)`, tenant, id, deleted).Scan(&raw)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("device")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	out.Visibility = "full"
	return out, e
}

const outputJSON = `to_jsonb(o)||jsonb_build_object('revision',o.revision::text,'device_status',d.status,'device_revision',d.revision::text,'device',jsonb_build_object('id',d.id,'code',d.code,'name',d.name,'product_name',v.content->>'name','product_model',v.content->>'model'),'template',s.content,'occupied',EXISTS(SELECT 1 FROM monitoring_source_bindings b WHERE b.output_id=o.id AND b.ended_at IS NULL))`
const outputFrom = ` FROM device_outputs o JOIN devices d ON d.id=o.device_id JOIN device_product_versions v ON v.id=o.product_version_id JOIN device_product_version_slots s ON s.version_id=o.product_version_id AND s.slot_key=o.slot_key `

func loadOutput(ctx context.Context, tx pgx.Tx, tenant, device, id string) (Output, error) {
	var out Output
	if !access.ValidID(id) {
		return out, k.NotFound("device")
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT `+outputJSON+outputFrom+` WHERE o.tenant_id=$1 AND o.id=$2 AND ($3::uuid IS NULL OR o.device_id=$3) AND d.deleted_at IS NULL`, tenant, id, k.Nullable(device)).Scan(&raw)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("device")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	out.Visibility = "full"
	if e == nil {
		e = outputDefinitions(ctx, tx, []*Output{&out})
	}
	return out, e
}
func projectReadable(ctx context.Context, tx pgx.Tx, auth k.Authorization, kind, id string, use bool) error {
	if !access.ValidID(id) {
		return k.NotFound("device")
	}
	if auth.Entry != "project" {
		return nil
	}
	var exists bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_device_grants WHERE tenant_id=$1 AND project_id=$2 AND kind=$3 AND CASE WHEN kind='device' THEN device_id ELSE output_id END=$4 AND ended_at IS NULL AND (NOT $5 OR can_use_source))`, auth.Tenant, auth.Project, kind, id, use).Scan(&exists)
	if e != nil {
		return e
	}
	if !exists {
		return k.NotFound("device")
	}
	return nil
}
func deviceActions(auth k.Authorization, d Device) Actions {
	out := Actions{History: auth.Has(access.TenantDeviceHistoryRead) || auth.Has(access.ProjectDeviceHistoryRead)}
	if auth.Entry != "tenant" || d.DeletedAt != nil {
		return out
	}
	out.Edit = auth.Has(access.TenantDeviceManage)
	out.Delete = out.Edit
	out.State = auth.Has(access.TenantDeviceState)
	out.Install = auth.Has(access.TenantDeviceInstall)
	out.Configure = auth.Has(access.TenantDeviceOutputManage)
	return out
}
func outputActions(auth k.Authorization, o Output, use bool) Actions {
	out := Actions{History: auth.Has(access.TenantDeviceHistoryRead) || auth.Has(access.ProjectDeviceHistoryRead)}
	if auth.Entry == "tenant" {
		out.Edit = auth.Has(access.TenantDeviceOutputManage)
		out.Configure = out.Edit
		out.UseSource = auth.Has(access.TenantMonitoringSourceManage)
	} else {
		out.UseSource = use && auth.Has(access.ProjectDeviceOutputUse) && auth.Has(access.ProjectMonitoringSourceManage)
	}
	if o.EndedAt != nil {
		out.Edit = false
		out.Configure = false
		out.UseSource = false
		out.Reason = "该输出代次已结束，仅供历史查看。"
	} else if o.DeviceStatus != "enabled" {
		out.UseSource = false
		out.Reason = "设备已停用，原来源配置仍保留。"
	}
	if auth.ProjectStatus == "archived" {
		out.UseSource = false
		out.Reason = "项目已归档，只能读取。"
	}
	return out
}

// ResolveOutputTx is used by source commands inside their existing authorized
// transaction. It independently checks the explicit scope and object grant; it
// never infers output visibility from a device grant or installation target.
func (s *Service) ResolveOutputTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, use bool) (Output, error) {
	op := scope.Operation("output", "detail")
	if use {
		op = scope.Operation("source", "bind")
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, op)
	if e != nil {
		return Output{}, e
	}
	if e = projectReadable(ctx, tx, auth, "output", id, use); e != nil {
		return Output{}, e
	}
	out, e := loadOutput(ctx, tx, scope.Tenant, "", id)
	if e != nil {
		return out, e
	}
	out.Actions = outputActions(auth, out, use)
	return out, nil
}
func writeChange(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, op access.Operation, device, output, installation, binding string, before, after any, fields []string) error {
	_, e := tx.Exec(ctx, `INSERT INTO device_resource_changes(tenant_id,device_id,output_id,installation_id,binding_id,operation,actor_id,actor_name,actor_scope,actor_project_id,tenant_membership_id,project_membership_id,request_id,reason,before,after,changed_fields) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, auth.Tenant, device, k.Nullable(output), k.Nullable(installation), k.Nullable(binding), string(op), auth.Account, auth.Name, auth.Entry, k.Nullable(auth.Project), auth.TenantMembership, k.Nullable(auth.ProjectMembership), m.RequestID, m.Reason, k.JSON(before), k.JSON(after), k.JSON(fields))
	return e
}
func complete(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, op access.Operation, input any, kind, id, device, output, installation string, before, after any, fields []string, result Result, meta access.Meta) error {
	if e := writeChange(ctx, tx, auth, m, op, device, output, installation, "", before, after, fields); e != nil {
		return e
	}
	if e := k.Audit(ctx, tx, auth, m, string(op), kind, id, result, meta); e != nil {
		return e
	}
	return k.SaveReceipt(ctx, tx, auth, m, op, input, kind, id, result)
}
func currentSlots(ctx context.Context, tx pgx.Tx, d Device) ([]c.Slot, error) {
	var body c.Content
	e := tx.QueryRow(ctx, `SELECT content FROM device_product_versions WHERE id=$1`, d.ProductVersionID).Scan(&body)
	return body.OutputSlots, e
}
func slotTx(ctx context.Context, tx pgx.Tx, d Device, key string) (c.Slot, error) {
	var slot c.Slot
	e := tx.QueryRow(ctx, `SELECT content FROM device_product_version_slots WHERE version_id=$1 AND slot_key=$2`, d.ProductVersionID, key).Scan(&slot)
	if e == pgx.ErrNoRows {
		return slot, k.NotFound("device")
	}
	return slot, e
}
