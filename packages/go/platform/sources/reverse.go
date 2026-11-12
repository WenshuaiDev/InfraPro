package sources

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// Authorization is part of the relational candidate set, before keyset paging.
// Submission still resolves the full target through resources.DeviceTargetTx.
const channelVisible = `($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=c.tenant_id AND g.project_id=$2 AND g.channel_id=c.id AND g.resource_kind='channel' AND g.ended_at IS NULL AND (NOT $3 OR g.can_source_manage)))`

func (s *Service) ChannelCandidates(ctx context.Context, a access.Actor, scope Scope, output string, f Filter) (k.Page[ChannelCandidate], error) {
	out := k.Page[ChannelCandidate]{Items: []ChannelCandidate{}}
	e := s.transact(ctx, a, scope, "output", "channel-candidates", false, func(tx pgx.Tx, auth k.Authorization) error {
		o, e := s.devices.ResolveOutputTx(ctx, tx, a, scope, output, true)
		if e != nil {
			return e
		}
		if e = f.Validate(); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"channel-candidates", output, o.Revision}))
		if e != nil {
			return e
		}
		only := f.CompatibleOnly == nil || *f.CompatibleOnly
		rows, e := tx.Query(ctx, `SELECT c.id::text,c.facility_id::text,c.point_id::text,c.code,c.name,c.revision::text,
   c.metric_version_id::text,c.unit_version_id::text,v.content->'metric'->>'shape',c.timestamp_meaning,device_semantic_text(c.aggregation_description),device_semantic_text(c.observation_direction),device_semantic_text(c.reference_basis),c.created_at,
   EXISTS(SELECT 1 FROM monitoring_source_bindings b WHERE b.channel_id=c.id AND b.ended_at IS NULL),
   EXISTS(WITH RECURSIVE ancestors AS(SELECT * FROM engineering_resource_nodes WHERE tenant_id=c.tenant_id AND kind='channel' AND id=c.id UNION ALL SELECT p.* FROM engineering_resource_nodes p JOIN ancestors a ON p.tenant_id=a.tenant_id AND p.kind=a.parent_kind AND p.id=a.parent_id)SELECT 1 FROM ancestors WHERE status<>'normal' OR deleted_at IS NOT NULL)
   FROM engineering_channels c JOIN engineering_versions v ON v.id=c.metric_version_id WHERE c.tenant_id=$1 AND c.deleted_at IS NULL AND `+channelVisible+`
   AND ($4='' OR c.code ILIKE '%'||$4||'%' OR c.name ILIKE '%'||$4||'%')
   AND (NOT $5 OR(c.metric_version_id=$6 AND c.unit_version_id IS NOT DISTINCT FROM $7::uuid AND v.content->'metric'->>'shape'=$8 AND c.timestamp_meaning=$9 AND device_semantic_text(c.aggregation_description)=$10 AND device_semantic_text(c.observation_direction)=$11 AND device_semantic_text(c.reference_basis)=$12))
   AND ($13::timestamptz IS NULL OR (c.created_at,c.id)>($13,$14::uuid)) ORDER BY c.created_at,c.id LIMIT $15`, scope.Tenant, k.Nullable(scope.Project), true, f.Query, only, o.MetricVersionID, o.UnitVersionID, o.ValueShape, o.TimestampMeaning, o.AggregationDescription, o.ObservationDirection, o.ReferenceBasis, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v ChannelCandidate
			var occupied, retired bool
			if e = rows.Scan(&v.ChannelID, &v.FacilityID, &v.PointID, &v.Code, &v.Name, &v.Revision, &v.Semantics.MetricVersionID, &v.Semantics.UnitVersionID, &v.Semantics.ValueShape, &v.Semantics.TimestampMeaning, &v.Semantics.AggregationDescription, &v.Semantics.ObservationDirection, &v.Semantics.ReferenceBasis, &v.CreatedAt, &occupied, &retired); e != nil {
				return e
			}
			v.Issues = c.Compatible(o.Semantics, v.Semantics)
			v.Compatible = len(v.Issues) == 0
			issue := func(code, message string) {
				v.Issues = append(v.Issues, Issue{Path: "/channel", Code: code, Message: message})
			}
			if retired {
				issue("target_retired", "通道或祖先已退役。")
			}
			if o.EndedAt != nil {
				issue("output_ended", "该输出代次已结束。")
			}
			if o.DeviceStatus != "enabled" {
				issue("device_disabled", "设备已停用。")
			}
			if o.Occupied {
				issue("output_occupied", "输出当前已被使用。")
			}
			// A configured channel may be selected for Switch; Current supplies the old
			// endpoint and rechecks the old output grant before exposing that action.
			v.Configured = occupied
			v.Available = len(v.Issues) == 0
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ChannelID
			cursor.At = last.CreatedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}

type OutputSource struct {
	Projection
	Target
	ChannelName string `json:"channel_name"`
	ChannelCode string `json:"channel_code"`
}

func (s *Service) OutputHistory(ctx context.Context, a access.Actor, scope Scope, output string, f Filter) (k.Page[OutputSource], error) {
	out := k.Page[OutputSource]{Items: []OutputSource{}}
	e := s.transact(ctx, a, scope, "output", "source-history", false, func(tx pgx.Tx, auth k.Authorization) error {
		o, e := s.devices.ResolveOutputTx(ctx, tx, a, scope, output, false)
		if e != nil {
			return e
		}
		if e = f.Validate(); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"output-source-history", output}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(b)||jsonb_build_object('revision',b.revision::text),c.name,c.code FROM monitoring_source_bindings b JOIN engineering_channels c ON c.id=b.channel_id AND c.tenant_id=b.tenant_id WHERE b.tenant_id=$1 AND `+channelVisible+` AND b.output_id=$4 AND ($5::timestamptz IS NULL OR (b.started_at,b.id)<($5,$6::uuid)) ORDER BY b.started_at DESC,b.id DESC LIMIT $7`, scope.Tenant, k.Nullable(scope.Project), false, output, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var b Binding
			var v OutputSource
			if e = rows.Scan(&raw, &v.ChannelName, &v.ChannelCode); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &b); e != nil {
				return e
			}
			v.Target = Target{FacilityID: b.FacilityID, PointID: b.PointID, ChannelID: b.ChannelID}
			v.Projection = Projection{Configured: true, Visibility: "full", ID: b.ID, Revision: b.Revision, Output: &o, StartedAt: &b.StartedAt, EndedAt: b.EndedAt, OutputSnapshot: b.OutputSnapshot, ChannelSnapshot: b.ChannelSnapshot}
			if scope.Entry == "tenant" || b.CreatedProjectID != nil && *b.CreatedProjectID == scope.Project {
				v.Reason = b.CreatedReason
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = *last.StartedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
