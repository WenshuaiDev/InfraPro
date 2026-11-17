package devices

import (
	"context"
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// LegacySection is immutable migration evidence. Resolution never changes an
// installation target, creates an engineering node, or binds a device output.
type LegacySection struct {
	ID               string          `json:"id"`
	InstallationID   string          `json:"installation_id"`
	FacilityID       *string         `json:"facility_id"`
	SourceTable      string          `json:"source_table"`
	SourceID         string          `json:"source_id"`
	SourceRevision   string          `json:"source_revision"`
	SourcePath       string          `json:"source_path"`
	EntryIndex       int             `json:"entry_index"`
	Original         json.RawMessage `json:"original"`
	Historical       bool            `json:"historical"`
	CapturedAt       time.Time       `json:"captured_at"`
	Status           string          `json:"status"`
	ResolutionID     *string         `json:"resolution_id"`
	ResolutionReason *string         `json:"resolution_reason"`
	TargetID         *string         `json:"target_id"`
	TargetName       *string         `json:"target_name"`
}
type LegacySectionResolution struct {
	Meta
	Status string  `json:"status"`
	Target *Target `json:"target,omitempty"`
}

const legacySectionSelect = `SELECT e.id::text,e.resource_id::text,e.facility_id::text,e.source_table,e.source_id::text,e.source_revision,e.source_path,e.entry_index,e.original,e.historical,e.captured_at,
 COALESCE(r.status,CASE WHEN e.historical THEN 'historical' ELSE 'pending' END),r.id::text,r.reason,r.target_id::text,n.name
 FROM engineering_legacy_section_evidence e
 LEFT JOIN LATERAL (SELECT * FROM engineering_legacy_section_resolutions r WHERE r.evidence_id=e.id ORDER BY created_at DESC,id DESC LIMIT 1) r ON true
 LEFT JOIN engineering_resource_nodes n ON n.tenant_id=e.tenant_id AND n.kind=r.target_kind AND n.id=r.target_id
 WHERE e.tenant_id=$1 AND e.device_id=$2 AND e.resource_kind='installation'`

func scanLegacySection(row pgx.Row) (LegacySection, error) {
	var v LegacySection
	err := row.Scan(&v.ID, &v.InstallationID, &v.FacilityID, &v.SourceTable, &v.SourceID, &v.SourceRevision, &v.SourcePath, &v.EntryIndex, &v.Original, &v.Historical, &v.CapturedAt, &v.Status, &v.ResolutionID, &v.ResolutionReason, &v.TargetID, &v.TargetName)
	if err == pgx.ErrNoRows {
		err = k.NotFound("device")
	}
	return v, err
}

func (s *Service) LegacySections(ctx context.Context, a access.Actor, scope Scope, device string, f Filter) (k.Page[LegacySection], error) {
	out := k.Page[LegacySection]{Items: []LegacySection{}}
	if scope.Entry != "tenant" {
		return out, access.ErrDenied
	}
	err := s.transact(ctx, a, scope, "installation", "legacy-sections", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, err := loadDevice(ctx, tx, auth.Tenant, device, true); err != nil {
			return err
		}
		if err := f.Validate(); err != nil {
			return err
		}
		cursor, err := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"legacy-sections", device}))
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, legacySectionSelect+` AND ($3::timestamptz IS NULL OR (e.captured_at,e.id)>($3,$4::uuid)) ORDER BY e.captured_at,e.id LIMIT $5`, auth.Tenant, device, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanLegacySection(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, v)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.CapturedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, err
}

func legacySectionInput(in *LegacySectionResolution) error {
	if err := in.Meta.Normalize(); err != nil {
		return err
	}
	if err := in.RequireReason(); err != nil {
		return err
	}
	if in.Status != "pending" && in.Status != "historical" && in.Status != "mapped" {
		return k.Invalid("/status", "resolution", "请选择待核对、历史依据或映射正式断面。")
	}
	if in.Status == "mapped" {
		if in.Target == nil || in.Target.Kind != "monitoring_section" || !access.ValidID(in.Target.ID) {
			return k.Invalid("/target", "resolution_target", "请选择正式监测断面。")
		}
	} else if in.Target != nil {
		return k.Invalid("/target", "resolution_target", "只有实体映射可以指定正式断面。")
	}
	return nil
}
func (s *Service) prepareLegacySection(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, device, evidence string, in LegacySectionResolution) (Preview, LegacySection, error) {
	var preview Preview
	if err := auth.Require(access.TenantDeviceInstall, access.TenantDeviceHistoryRead, access.TenantEngineeringResourceRead); err != nil {
		return preview, LegacySection{}, err
	}
	d, err := loadDevice(ctx, tx, auth.Tenant, device, true)
	if err != nil {
		return preview, LegacySection{}, err
	}
	old, err := scanLegacySection(tx.QueryRow(ctx, legacySectionSelect+` AND e.id=$3`, auth.Tenant, device, evidence))
	if err != nil {
		return preview, old, err
	}
	var target Installation
	if in.Target != nil {
		if s.resources == nil {
			return preview, old, k.Problem("unavailable", "资源目标读取暂不可用。", 503)
		}
		target, err = s.resources.ResolveInstallationTx(ctx, tx, a, auth.Scope, *in.Target, true)
		if err != nil {
			return preview, old, err
		}
		if old.FacilityID != nil && (target.FacilityID == nil || *old.FacilityID != *target.FacilityID) {
			return preview, old, k.Invalid("/target", "facility_mismatch", "应选择旧记录所属设施内的断面；不同设施的同名文字不能自动合并。")
		}
	}
	in.PreviewHash = ""
	preview = k.NewPreview([]any{device, evidence, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, auth.Revision, d.InstallationRevision, old, target}, map[string]any{"before": old, "status": in.Status, "target": target}, true, false)
	return preview, old, nil
}
func (s *Service) PreviewLegacySection(ctx context.Context, a access.Actor, scope Scope, device, evidence string, in LegacySectionResolution) (Preview, error) {
	var out Preview
	if scope.Entry != "tenant" {
		return out, access.ErrDenied
	}
	if !access.ValidID(evidence) {
		return out, k.NotFound("device")
	}
	if err := legacySectionInput(&in); err != nil {
		return out, err
	}
	err := s.transact(ctx, a, scope, "installation", "legacy-preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var err error
		out, _, err = s.prepareLegacySection(ctx, tx, a, auth, device, evidence, in)
		return err
	})
	return out, err
}
func (s *Service) ResolveLegacySection(ctx context.Context, a access.Actor, scope Scope, device, evidence string, in LegacySectionResolution, meta access.Meta) (Result, error) {
	var out Result
	if scope.Entry != "tenant" {
		return out, access.ErrDenied
	}
	if !access.ValidID(evidence) {
		return out, k.NotFound("device")
	}
	if err := legacySectionInput(&in); err != nil {
		return out, err
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	err := s.transact(ctx, a, scope, "installation", "legacy-resolve", true, func(tx pgx.Tx, auth k.Authorization) error {
		// Recheck current authority and device identity even when replaying a receipt.
		if err := auth.Require(access.TenantDeviceHistoryRead, access.TenantEngineeringResourceRead); err != nil {
			return err
		}
		current, err := loadDevice(ctx, tx, auth.Tenant, device, true)
		if err != nil {
			return err
		}
		op := scope.Operation("installation", "legacy-resolve")
		input := []any{device, evidence, in}
		v, found, err := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if err != nil {
			return err
		}
		if found {
			out = v
			return nil
		}
		preview, old, err := s.prepareLegacySection(ctx, tx, a, auth, device, evidence, in)
		if err != nil {
			return err
		}
		m := in.Meta
		m.PreviewHash = hash
		if err = preview.Check(m); err != nil {
			return err
		}
		var kind, id any
		if in.Target != nil {
			kind = in.Target.Kind
			id = in.Target.ID
		}
		_, err = tx.Exec(ctx, `INSERT INTO engineering_legacy_section_resolutions(evidence_id,status,target_kind,target_id,reason,actor_id,request_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, evidence, in.Status, kind, id, in.Reason, a.ID, in.RequestID)
		if err != nil {
			return err
		}
		after, err := scanLegacySection(tx.QueryRow(ctx, legacySectionSelect+` AND e.id=$3`, auth.Tenant, device, evidence))
		if err != nil {
			return err
		}
		out = Result{ID: device, Revision: current.Revision, InstallationRevision: current.InstallationRevision, InstallationID: old.InstallationID, Changed: true}
		return complete(ctx, tx, auth, m, op, input, "device_installation", old.InstallationID, device, "", old.InstallationID, old, after, []string{"legacy_section_resolution"}, out, meta)
	})
	return out, err
}
