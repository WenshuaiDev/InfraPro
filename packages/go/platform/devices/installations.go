package devices

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

// InstallationCommand is used only after decoding one of the strict public DTOs.
type InstallationCommand struct {
	Action string `json:"action"`
	ID     string `json:"installation_id,omitempty"`
	RevisionInput
	InstallationFields
	InstallationRevision string  `json:"expected_installation_revision"`
	InstalledAt          string  `json:"installed_at,omitempty"`
	UninstalledAt        *string `json:"uninstalled_at,omitempty"`
	MovedAt              string  `json:"moved_at,omitempty"`
}

func InstallationCreateCommand(in InstallationCreate) InstallationCommand {
	var end *string
	if in.UninstalledAt != "" {
		end = &in.UninstalledAt
	}
	return InstallationCommand{Action: "create", RevisionInput: RevisionInput{Meta: in.Meta}, InstallationFields: in.InstallationFields, InstallationRevision: in.InstallationRevision, InstalledAt: in.InstalledAt, UninstalledAt: end}
}
func InstallationEndCommand(id string, in InstallationEnd) InstallationCommand {
	return InstallationCommand{Action: "end", ID: id, RevisionInput: in.RevisionInput, InstallationRevision: in.InstallationRevision, UninstalledAt: &in.UninstalledAt}
}
func InstallationRelocateCommand(id string, in InstallationRelocate) InstallationCommand {
	return InstallationCommand{Action: "relocate", ID: id, RevisionInput: in.RevisionInput, InstallationFields: in.InstallationFields, InstallationRevision: in.InstallationRevision, MovedAt: in.MovedAt}
}
func InstallationCorrectCommand(id string, in InstallationCorrect) InstallationCommand {
	return InstallationCommand{Action: "correct", ID: id, RevisionInput: in.RevisionInput, InstallationFields: in.InstallationFields, InstallationRevision: in.InstallationRevision, InstalledAt: in.InstalledAt, UninstalledAt: in.UninstalledAt}
}
func installationInput(in *InstallationCommand) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	if !slices.Contains([]string{"create", "end", "relocate", "correct"}, in.Action) {
		return k.Problem("invalid_request", "不支持的安装操作。", 400)
	}
	if in.Action != "create" {
		if !access.ValidID(in.ID) {
			return k.NotFound("device")
		}
		if e := in.Meta.RequireReason(); e != nil {
			return e
		}
	}
	if in.Action == "create" && in.UninstalledAt != nil && in.InstalledAt == "" {
		return k.Invalid("/installed_at", "required", "补录已结束历史必须填写实际安装时间。")
	}
	if in.Action == "correct" && in.InstalledAt == "" {
		return k.Invalid("/installed_at", "required", "纠错须提供完整的实际安装时间。")
	}
	if in.Action != "end" {
		if !slices.Contains([]string{"facility", "component", "point", "location"}, in.Target.Kind) {
			return k.Invalid("/target/kind", "target_kind", "请选择安装目标类型。")
		}
		if in.Target.Kind == "location" {
			if in.Target.ID != "" {
				return k.Problem("invalid_request", "独立位置不能携带资源 ID。", 400)
			}
		} else if !access.ValidID(in.Target.ID) {
			return k.Invalid("/target/id", "uuid", "请选择有效的工程资源。")
		}
		in.LocationDescription = k.Text(in.LocationDescription)
		if e := k.String(in.LocationDescription, "/location_description", 2000, in.Target.Kind == "location"); e != nil {
			return e
		}
		if e := location.ValidatePosition(in.Position); e != nil {
			var le *location.Error
			if errors.As(e, &le) {
				out := k.Problem("validation_failed", le.Message, le.Status)
				for _, i := range le.Issues {
					out.Issues = append(out.Issues, Issue{Path: i.Path, Code: i.Code, Message: i.Message})
				}
				return out
			}
			return e
		}
	}
	normalizeTime := func(v *string, path string) error {
		if *v == "" {
			return nil
		}
		parsed, e := k.Time(*v, path)
		if e != nil {
			return e
		}
		*v = parsed.Format(time.RFC3339Nano)
		return nil
	}
	if e := normalizeTime(&in.InstalledAt, "/installed_at"); e != nil {
		return e
	}
	if e := normalizeTime(&in.MovedAt, "/moved_at"); e != nil {
		return e
	}
	if in.UninstalledAt != nil {
		value := *in.UninstalledAt
		if e := normalizeTime(&value, "/uninstalled_at"); e != nil {
			return e
		}
		in.UninstalledAt = &value
	}
	return nil
}
func installationTx(ctx context.Context, tx pgx.Tx, tenant, device, id string) (Installation, error) {
	var out Installation
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(i)||jsonb_build_object('revision',revision::text) FROM device_installations i WHERE tenant_id=$1 AND device_id=$2 AND id=$3`, tenant, device, id).Scan(&raw)
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
func (s *Service) prepareInstallation(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, device string, in InstallationCommand, write bool) (Preview, Device, *Installation, Installation, error) {
	var preview Preview
	var old *Installation
	var proposed Installation
	if e := auth.Require(access.TenantDeviceInstall); e != nil {
		return preview, Device{}, old, proposed, e
	}
	d, e := loadDevice(ctx, tx, auth.Tenant, device, false)
	if e != nil {
		return preview, d, old, proposed, e
	}
	if e = k.Expect(in.InstallationRevision, d.InstallationRevision); e != nil {
		return preview, d, old, proposed, e
	}
	if in.Action != "create" {
		v, e := installationTx(ctx, tx, auth.Tenant, device, in.ID)
		if e != nil {
			return preview, d, old, proposed, e
		}
		old = &v
		if old.TargetKind != "location" {
			if e = auth.Require(access.TenantEngineeringResourceRead); e != nil {
				return preview, d, old, proposed, e
			}
		}
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return preview, d, old, proposed, e
		}
		if in.Action != "correct" && old.UninstalledAt != nil {
			return preview, d, old, proposed, k.Problem("revision_conflict", "该安装记录已结束。", 409)
		}
	}
	if in.Action == "end" {
		proposed = *old
	} else {
		if in.Target.Kind == "location" {
			proposed = Installation{TargetKind: "location", Visibility: "full"}
		} else {
			if e = auth.Require(access.TenantEngineeringResourceRead); e != nil {
				return preview, d, old, proposed, e
			}
			if s.resources == nil {
				return preview, d, old, proposed, k.Problem("unavailable", "资源目标读取暂不可用。", 503)
			}
			proposed, e = s.resources.ResolveInstallationTx(ctx, tx, a, auth.Scope, in.Target, in.UninstalledAt != nil)
			if e != nil {
				return preview, d, old, proposed, e
			}
		}
		proposed.LocationDescription = in.LocationDescription
		proposed.Position = in.Position
	}
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	if _, e = tx.Exec(ctx, `SELECT id FROM devices WHERE tenant_id=$1 AND id=$2`+lock, auth.Tenant, device); e != nil {
		return preview, d, old, proposed, e
	}
	if old != nil {
		if _, e = tx.Exec(ctx, `SELECT id FROM device_installations WHERE tenant_id=$1 AND id=$2`+lock, auth.Tenant, old.ID); e != nil {
			return preview, d, old, proposed, e
		}
	}

	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return preview, d, old, proposed, e
	}
	parse := func(raw, path string) (time.Time, error) {
		if raw == "" {
			return now, nil
		}
		return k.Time(raw, path)
	}
	start := now
	var end *time.Time
	if in.Action == "create" || in.Action == "correct" {
		start, e = parse(in.InstalledAt, "/installed_at")
		if e != nil {
			return preview, d, old, proposed, e
		}
		if in.UninstalledAt != nil {
			t, e := k.Time(*in.UninstalledAt, "/uninstalled_at")
			if e != nil {
				return preview, d, old, proposed, e
			}
			end = &t
		}
	}
	if in.Action == "end" {
		start = old.InstalledAt
		t, e := parse(value(in.UninstalledAt), "/uninstalled_at")
		if e != nil {
			return preview, d, old, proposed, e
		}
		end = &t
	}
	if in.Action == "relocate" {
		start, e = parse(in.MovedAt, "/moved_at")
		if e != nil {
			return preview, d, old, proposed, e
		}
		if !start.After(old.InstalledAt) {
			return preview, d, old, proposed, k.Invalid("/moved_at", "period", "迁移时间须晚于原安装时间。")
		}
	}
	if start.After(now) || end != nil && end.After(now) {
		return preview, d, old, proposed, k.Invalid("/installed_at", "future_time", "实际安装或拆除时间不能晚于服务器当前时间。")
	}
	if end != nil && !end.After(start) {
		return preview, d, old, proposed, k.Invalid("/uninstalled_at", "period", "拆除时间必须严格晚于安装时间。")
	}
	proposed.DeviceID = device
	proposed.InstalledAt = start
	proposed.UninstalledAt = end
	var overlap bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_installations WHERE tenant_id=$1 AND device_id=$2 AND ($3::uuid IS NULL OR id<>$3) AND tstzrange(installed_at,uninstalled_at,'[)') && tstzrange($4::timestamptz,$5::timestamptz,'[)'))`, auth.Tenant, device, k.Nullable(in.ID), start, end).Scan(&overlap)
	if e != nil {
		return preview, d, old, proposed, e
	}
	// Relocation also validates the shortened old interval, including backdated moves.
	impact, e := impactTx(ctx, tx, auth.Tenant, device)
	if e != nil {
		return preview, d, old, proposed, e
	}
	in.PreviewHash = ""
	preview = k.NewPreview([]any{device, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, d, old, impact.Hash}, map[string]any{"before": old, "after": proposed, "preserves_sources": impact.CurrentSources}, in.Action != "create" || impact.Shared || impact.CurrentSources, impact.Shared)
	if overlap {
		preview.CanSubmit = false
		preview.Blockers = append(preview.Blockers, Issue{Path: "/installed_at", Code: "installation_overlap", Message: "安装期间与设备已有记录重叠。"})
	}
	if impact.Shared && in.Reason == "" {
		preview.CanSubmit = false
		preview.Blockers = append(preview.Blockers, Issue{Path: "/reason", Code: "required", Message: "共享设备安装变更须填写原因。"})
	}
	return preview, d, old, proposed, nil
}
func (s *Service) PreviewInstallation(ctx context.Context, a access.Actor, scope Scope, device string, in InstallationCommand) (Preview, error) {
	var out Preview
	if e := installationInput(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "device", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, _, _, _, e = s.prepareInstallation(ctx, tx, a, auth, device, in, false)
		return e
	})
	return out, e
}
func insertInstallation(ctx context.Context, tx pgx.Tx, auth k.Authorization, i Installation) (Installation, error) {
	var id string
	e := tx.QueryRow(ctx, `INSERT INTO device_installations(tenant_id,device_id,target_kind,facility_id,component_id,point_id,location_description,position,installed_at,uninstalled_at,recorded_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8::jsonb,'null'::jsonb),$9,$10,$11,$11) RETURNING id::text`, auth.Tenant, i.DeviceID, i.TargetKind, i.FacilityID, i.ComponentID, i.PointID, i.LocationDescription, k.JSON(i.Position), i.InstalledAt, i.UninstalledAt, auth.Account).Scan(&id)
	if e != nil {
		return i, e
	}
	return installationTx(ctx, tx, auth.Tenant, i.DeviceID, id)
}
func (s *Service) Install(ctx context.Context, a access.Actor, scope Scope, device string, in InstallationCommand, meta access.Meta) (Result, error) {
	var out Result
	if e := installationInput(&in); e != nil {
		return out, e
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.transact(ctx, a, scope, "installation", in.Action, true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("installation", in.Action)
		input := []any{device, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		preview, _, old, proposed, e := s.prepareInstallation(ctx, tx, a, auth, device, in, true)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if e = preview.Check(m); e != nil {
			return e
		}
		if in.Action == "relocate" {
			_, e = tx.Exec(ctx, `UPDATE device_installations SET uninstalled_at=$2,revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE id=$1`, old.ID, proposed.InstalledAt, a.ID)
			if e != nil {
				return e
			}
		}
		if in.Action == "create" || in.Action == "relocate" {
			proposed, e = insertInstallation(ctx, tx, auth, proposed)
		} else {
			_, e = tx.Exec(ctx, `UPDATE device_installations SET target_kind=$2,facility_id=$3,component_id=$4,point_id=$5,location_description=$6,position=NULLIF($7::jsonb,'null'::jsonb),installed_at=$8,uninstalled_at=$9,revision=revision+1,updated_at=clock_timestamp(),updated_by=$10 WHERE id=$1`, old.ID, proposed.TargetKind, proposed.FacilityID, proposed.ComponentID, proposed.PointID, proposed.LocationDescription, k.JSON(proposed.Position), proposed.InstalledAt, proposed.UninstalledAt, a.ID)
			if e == nil {
				proposed, e = installationTx(ctx, tx, auth.Tenant, device, old.ID)
			}
		}
		if e != nil {
			return e
		}
		out = Result{ID: device, InstallationID: proposed.ID, Revision: proposed.Revision, Changed: true}
		e = tx.QueryRow(ctx, `UPDATE devices SET installation_revision=installation_revision+1 WHERE id=$1 RETURNING installation_revision::text`, device).Scan(&out.InstallationRevision)
		if e != nil {
			return e
		}
		after := map[string]any{"installation": proposed}
		if in.Action == "relocate" {
			closed, e := installationTx(ctx, tx, auth.Tenant, device, old.ID)
			if e != nil {
				return e
			}
			after["previous_installation"] = closed
		}
		return complete(ctx, tx, auth, m, op, input, "device_installation", proposed.ID, device, "", proposed.ID, old, after, []string{"installations"}, out, meta)
	})
	return out, e
}
func (s *Service) projectInstallation(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, i Installation) (Installation, error) {
	if s.resources == nil {
		return Installation{}, k.Problem("unavailable", "安装目标读取暂不可用。", 503)
	}
	out, e := s.resources.ProjectInstallationsTx(ctx, tx, a, auth.Scope, []Installation{i})
	if e != nil {
		return Installation{}, e
	}
	return out[0], nil
}
func (s *Service) Installations(ctx context.Context, a access.Actor, scope Scope, device string, f Filter) (k.Page[Installation], error) {
	out := k.Page[Installation]{Items: []Installation{}}
	e := s.transact(ctx, a, scope, "installation", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := projectReadable(ctx, tx, auth, "device", device, false); e != nil {
			return e
		}
		if _, e := loadDevice(ctx, tx, auth.Tenant, device, false); e != nil {
			return e
		}
		if e := f.Validate(); e != nil {
			return e
		}
		if f.State == "" {
			f.State = "all"
		}
		if !slices.Contains([]string{"all", "current", "ended"}, f.State) {
			return k.Problem("invalid_request", "安装状态筛选无效。", 400)
		}
		if f.State != "current" {
			permission := access.TenantDeviceHistoryRead
			if auth.Entry == "project" {
				permission = access.ProjectDeviceHistoryRead
			}
			if e := auth.Require(permission); e != nil {
				return e
			}
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"installations", device}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(i)||jsonb_build_object('revision',revision::text) FROM device_installations i WHERE tenant_id=$1 AND device_id=$2 AND ($3='all' OR ($3='current')=(uninstalled_at IS NULL)) AND ($4::timestamptz IS NULL OR (installed_at,id)<($4,$5::uuid)) ORDER BY installed_at DESC,id DESC LIMIT $6`, auth.Tenant, device, f.State, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw []byte
			var i Installation
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(raw, &i); e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, i)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.At = last.InstalledAt
			cursor.ID = last.ID
			out.NextCursor = cursor.Encode()
		}

		if s.resources == nil {
			return k.Problem("unavailable", "安装目标读取暂不可用。", 503)
		}
		out.Items, e = s.resources.ProjectInstallationsTx(ctx, tx, a, scope, out.Items)
		return e

	})
	return out, e
}
func (s *Service) InstallationTargets(ctx context.Context, a access.Actor, scope Scope, device string, f Filter, history bool) (k.Page[InstallationTarget], error) {
	out := k.Page[InstallationTarget]{Items: []InstallationTarget{}}
	e := s.transact(ctx, a, scope, "installation", "targets", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := loadDevice(ctx, tx, auth.Tenant, device, false); e != nil {
			return e
		}
		if e := auth.Require(access.TenantEngineeringResourceRead); e != nil {
			return e
		}
		if s.resources == nil {
			return k.Problem("unavailable", "资源目标读取暂不可用。", 503)
		}
		var e error
		out, e = s.resources.InstallationTargetsTx(ctx, tx, a, scope, f, history)
		return e
	})
	return out, e
}
