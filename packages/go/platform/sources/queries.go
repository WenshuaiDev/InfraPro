package sources

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) History(ctx context.Context, a access.Actor, scope Scope, target Target, f Filter) (k.Page[Projection], error) {
	out := k.Page[Projection]{Items: []Projection{}}
	e := s.transact(ctx, a, scope, "source", "history", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, _, e := s.channelTx(ctx, tx, a, scope, target, false); e != nil {
			return e
		}
		if e := f.Validate(); e != nil {
			return e
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"source-history", target}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(b)||jsonb_build_object('revision',revision::text) FROM monitoring_source_bindings b WHERE tenant_id=$1 AND channel_id=$2 AND ($3::timestamptz IS NULL OR (started_at,id)<($3,$4::uuid)) ORDER BY started_at DESC,id DESC LIMIT $5`, scope.Tenant, target.ChannelID, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		bindings := []Binding{}
		for rows.Next() {
			var raw []byte
			var b Binding
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(raw, &b); e != nil {
				rows.Close()
				return e
			}
			bindings = append(bindings, b)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(bindings) > f.Limit {
			bindings = bindings[:f.Limit]
			last := bindings[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.StartedAt
			out.NextCursor = cursor.Encode()
		}

		out.Items, e = s.projectBindings(ctx, tx, a, auth, bindings, true)
		return e

	})
	return out, e
}
func (s *Service) Candidates(ctx context.Context, a access.Actor, scope Scope, target Target, f Filter) (k.Page[Candidate], error) {
	out := k.Page[Candidate]{Items: []Candidate{}}
	e := s.transact(ctx, a, scope, "source", "candidates", false, func(tx pgx.Tx, auth k.Authorization) error {
		channel, semantic, e := s.channelTx(ctx, tx, a, scope, target, true)
		if e != nil {
			return e
		}
		if e = f.Validate(); e != nil {
			return e
		}
		compatibleOnly := f.CompatibleOnly == nil || *f.CompatibleOnly
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"source-candidates", target, channel.Resource.Revision}))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT o.id::text,o.started_at FROM device_outputs o JOIN devices d ON d.id=o.device_id WHERE o.tenant_id=$1 AND o.ended_at IS NULL AND d.deleted_at IS NULL
   AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=o.tenant_id AND g.project_id=$2 AND g.kind='output' AND g.output_id=o.id AND g.ended_at IS NULL AND g.can_use_source))
   AND ($3='' OR o.name ILIKE '%'||$3||'%' OR d.name ILIKE '%'||$3||'%' OR d.code ILIKE '%'||$3||'%') AND ($4::uuid IS NULL OR o.device_id=$4)
   AND ($5::uuid IS NULL OR o.metric_version_id=$5) AND ($6::uuid IS NULL OR o.unit_version_id=$6)
   AND (NOT $7 OR (o.metric_version_id=$8 AND o.unit_version_id IS NOT DISTINCT FROM $9::uuid AND o.value_shape=$10 AND o.timestamp_meaning=$11 AND device_semantic_text(o.aggregation_description)=$12 AND device_semantic_text(o.observation_direction)=$13 AND device_semantic_text(o.reference_basis)=$14))
   AND ($15::timestamptz IS NULL OR (o.started_at,o.id)>($15,$16::uuid)) ORDER BY o.started_at,o.id LIMIT $17`, auth.Tenant, k.Nullable(auth.Project), f.Query, k.Nullable(f.DeviceID), k.Nullable(f.MetricID), k.Nullable(f.UnitID), compatibleOnly, semantic.MetricVersionID, semantic.UnitVersionID, semantic.ValueShape, semantic.TimestampMeaning, semantic.AggregationDescription, semantic.ObservationDirection, semantic.ReferenceBasis, cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		times := []time.Time{}
		for rows.Next() {
			var id string
			var at time.Time
			if e = rows.Scan(&id, &at); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
			times = append(times, at)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(ids) > f.Limit {
			ids = ids[:f.Limit]
			cursor.ID = ids[f.Limit-1]
			cursor.At = times[f.Limit-1]
			out.NextCursor = cursor.Encode()
		}
		outputs, e := s.devices.OutputsByIDTx(ctx, tx, a, scope, ids, true)
		if e != nil {
			return e
		}
		for _, id := range ids {
			o, ok := outputs[id]
			if !ok {
				continue
			}
			issues := c.Compatible(o.Semantics, semantic)
			v := Candidate{Output: o, Compatible: len(issues) == 0, Available: len(issues) == 0, Issues: issues}
			if o.DeviceStatus != "enabled" {
				v.Available = false
				v.Issues = append(v.Issues, Issue{Path: "/device", Code: "device_disabled", Message: "设备已停用。"})
			}
			if o.Occupied {
				v.Available = false
				v.Issues = append(v.Issues, Issue{Path: "/output", Code: "output_occupied", Message: "输出当前已被使用。"})
			}
			if channel.Retired {
				v.Available = false
				v.Issues = append(v.Issues, Issue{Path: "/channel", Code: "target_retired", Message: "通道或祖先已退役。"})
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, e
}
