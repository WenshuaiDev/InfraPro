package sources

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// Current binding membership is owned here. Device statistics receive only
// qualified device/facility pairs, never an unqualified count of relation rows.
func (s *Service) DisplayDeviceRelationsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) (map[string][]string, error) {
	return s.displayDeviceRelationsTx(ctx, tx, p, "facility_id")
}
func (s *Service) DisplayDeviceStructureTargetsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) (map[string][]displayread.Ref, error) {
	values, e := s.displayDeviceRelationsTx(ctx, tx, p, "channel_id")
	if e != nil {
		return nil, e
	}
	out := map[string][]displayread.Ref{}
	for device, ids := range values {
		for _, id := range ids {
			out[device] = append(out[device], displayread.Ref{Kind: "channel", ID: id})
		}
	}
	return out, nil
}
func (s *Service) displayDeviceRelationsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, column string) (map[string][]string, error) {
	out := map[string][]string{}
	o := p.Authority.Owner
	scope := p.Scopes["channel_target"]
	if scope.Domain != "channels" {
		return nil, displayread.Invalid("/channel_target", "来源关系需要通道读取视图。")
	}
	if _, e := access.RequireActorProjectTx(ctx, tx, p.Authority.Subject.ReadActor(), access.DeviceOperation("project", "source", "detail"), o.Tenant, o.Project); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT DISTINCT b.device_id::text,b.`+column+`::text FROM monitoring_source_bindings b
 JOIN project_device_grants g ON g.tenant_id=b.tenant_id AND g.project_id=$2 AND g.kind='output' AND g.output_id=b.output_id AND g.ended_at IS NULL
 WHERE b.tenant_id=$1 AND b.ended_at IS NULL AND b.device_id=ANY($3::uuid[]) AND b.output_id=ANY($4::uuid[]) AND b.channel_id=ANY($5::uuid[]) ORDER BY 1,2`, o.Tenant, o.Project, p.Scopes["target"].IDs("device"), p.Scopes["target"].IDs("output"), scope.IDs("channel"))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var device, facility string
		if e = rows.Scan(&device, &facility); e != nil {
			return nil, e
		}
		out[device] = append(out[device], facility)
	}
	return out, rows.Err()
}

func (s *Service) DisplayReadTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	scope := p.Scopes["target"]
	history := p.Method == "sources.bindingHistory"
	action := "detail"
	if history {
		action = "history"
	}
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.DeviceOperation("project", "source", action), o.Tenant, o.Project); e != nil {
		return out, e
	}
	ids := scope.IDs("channel")
	points := []string{}
	if q, ok := p.Request.(*displayread.SourceHistoryRequest); ok {
		if !slices.Contains(ids, q.ID) {
			return out, displayread.Hidden()
		}
		ids = []string{q.ID}
	}
	if q, ok := p.Request.(*displayread.SourceBindingsRequest); ok {
		points = q.PointIDs
		if len(q.ChannelIDs) > 0 {
			for _, id := range q.ChannelIDs {
				if !slices.Contains(ids, id) {
					return out, displayread.Hidden()
				}
			}
			ids = q.ChannelIDs
		}
	}
	// The channel population is independently qualified; device scopes only
	// determine the nested projection, never which hidden sources are searchable.
	rows, e := tx.Query(ctx, `SELECT c.id::text,c.point_id::text,c.source_kind,CASE WHEN b.id IS NULL THEN NULL ELSE to_jsonb(b)||jsonb_build_object('revision',b.revision::text) END
 FROM engineering_channels c LEFT JOIN monitoring_source_bindings b ON b.tenant_id=c.tenant_id AND b.channel_id=c.id AND ($3 OR b.ended_at IS NULL)
 WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL AND (COALESCE(cardinality($4::uuid[]),0)=0 OR c.point_id=ANY($4))
 AND (NOT $3 OR b.id IS NOT NULL) ORDER BY CASE WHEN NOT $3 THEN c.id END,b.started_at DESC,b.id DESC`, o.Tenant, ids, history, points)
	if e != nil {
		return out, e
	}
	type row struct {
		id      string
		point   *string
		source  string
		binding *Binding
	}
	values := []row{}
	for rows.Next() {
		var v row
		var raw []byte
		if e = rows.Scan(&v.id, &v.point, &v.source, &raw); e != nil {
			rows.Close()
			return out, e
		}
		if len(raw) > 0 {
			v.binding = &Binding{}
			if e = json.Unmarshal(raw, v.binding); e != nil {
				rows.Close()
				return out, e
			}
		}
		values = append(values, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	outputs := map[string]displayread.DeviceOutput{}
	if deviceScope, ok := p.Scopes["device_target"]; ok {
		if history {
			if _, e = access.RequireActorProjectTx(ctx, tx, a, access.DeviceOperation("project", "device", "changes"), o.Tenant, o.Project); e != nil {
				return out, e
			}
		}
		readable, e := s.devices.DisplayOutputsTx(ctx, tx, a, o.Tenant, o.Project, deviceScope, "", true)
		if e != nil {
			return out, e
		}
		for _, v := range readable {
			outputs[v.ID] = v
		}
	}
	for _, v := range values {
		item := displayread.SourceBindingItem{Channel: displayread.Ref{Kind: "channel", ID: v.id}, State: "unbound"}
		if v.point != nil {
			item.Point = &displayread.Ref{Kind: "point", ID: *v.point}
		}
		if v.source == "derived" {
			item.State = "not_applicable"
		} else if b := v.binding; b != nil {
			item.State = "restricted"
			item.Source = &displayread.SourceProjection{Configured: true, Visibility: "restricted"}
			if output, ok := outputs[b.OutputID]; ok {
				item.State = "bound"
				if b.EndedAt != nil {
					item.State = "ended"
				} else if output.State != "active" {
					item.State = "attention"
				}
				item.Source = &displayread.SourceProjection{Configured: true, Visibility: "full", ID: b.ID, Revision: b.Revision, Device: &output.Device, Output: &output, StartedAt: &b.StartedAt, EndedAt: b.EndedAt}
				if history {
					item.Source.OutputSemantics = historicalDisplaySemantics(b.OutputSnapshot)
					item.Source.ChannelSemantics = historicalDisplaySemantics(b.ChannelSnapshot)
				}
			}
		}
		out.Rows = append(out.Rows, displayread.Bytes(item))
	}
	return out, nil
}
func historicalDisplaySemantics(raw json.RawMessage) *displayread.OutputSemantics {
	var v struct {
		Semantics c.Semantics `json:"semantics"`
	}
	if json.Unmarshal(raw, &v) != nil || !access.ValidID(v.Semantics.MetricVersionID) {
		return nil
	}
	out := devices.DisplaySemantics(v.Semantics)
	return &out
}
