package devices

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func DisplaySemantics(v c.Semantics) displayread.OutputSemantics {
	return displayread.OutputSemantics{Metric: displayread.MetricRef{Kind: "engineering", VersionID: v.MetricVersionID}, UnitVersionID: v.UnitVersionID, ValueShape: v.ValueShape, TimestampMeaning: v.TimestampMeaning, AggregationDescription: v.AggregationDescription, ObservationDirection: v.ObservationDirection, ReferenceBasis: v.ReferenceBasis}
}
func displaySemantics(v c.Semantics) displayread.OutputSemantics { return DisplaySemantics(v) }

func (s *Service) displayHistoryTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, q displayread.DeviceHistoryRequest) (displayread.Materialized, error) {
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	o := p.Authority.Owner
	scope := p.Scopes["target"]
	if _, e := access.RequireActorProjectTx(ctx, tx, p.Authority.Subject.ReadActor(), access.DeviceOperation("project", "device", "changes"), o.Tenant, o.Project); e != nil {
		return out, e
	}
	if q.Kind == "output_generation" {
		values, e := s.DisplayOutputsTx(ctx, tx, p.Authority.Subject.ReadActor(), o.Tenant, o.Project, scope, q.ID, true)
		if e != nil {
			return out, e
		}
		for _, v := range values {
			out.Rows = append(out.Rows, displayread.Bytes(v))
		}
		return out, nil
	}
	if !slices.Contains(scope.IDs("device"), q.ID) {
		return out, displayread.Hidden()
	}
	if q.Kind == "installation" {
		values, e := s.DisplayInstallationsTx(ctx, tx, p, []string{q.ID}, true)
		if e != nil {
			return out, e
		}
		for _, v := range values[q.ID] {
			out.Rows = append(out.Rows, displayread.Bytes(v))
		}
		return out, nil
	}
	// Device changes have their own projection. Output/source/installation events
	// use the corresponding typed history and cannot leak through this endpoint.
	rows, e := tx.Query(ctx, `SELECT id::text,changed_at,operation,before,after FROM device_resource_changes WHERE tenant_id=$1 AND device_id=$2 AND split_part(operation,':',3)='device' ORDER BY changed_at DESC,id DESC`, o.Tenant, q.ID)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v displayread.DeviceChange
		var before, after []byte
		if e = rows.Scan(&v.ID, &v.ChangedAt, &v.Operation, &before, &after); e != nil {
			return out, e
		}
		v.Before = deviceHistoryValue(before, scope.FieldGroups)
		v.After = deviceHistoryValue(after, scope.FieldGroups)
		v.ChangedFields = []string{}
		var b, a map[string]json.RawMessage
		_ = json.Unmarshal(displayread.Bytes(v.Before), &b)
		_ = json.Unmarshal(displayread.Bytes(v.After), &a)
		for key, value := range a {
			if string(b[key]) != string(value) {
				v.ChangedFields = append(v.ChangedFields, key)
			}
		}
		for key := range b {
			if _, ok := a[key]; !ok {
				v.ChangedFields = append(v.ChangedFields, key)
			}
		}
		slices.Sort(v.ChangedFields)
		out.Rows = append(out.Rows, displayread.Bytes(v))
	}
	return out, rows.Err()
}
func deviceHistoryValue(raw []byte, fields []string) *displayread.DeviceHistoryValues {
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil || len(v) == 0 {
		return nil
	}
	if b, ok := v["device"]; ok {
		_ = json.Unmarshal(b, &v)
	}
	text := func(key string) string {
		var t string
		if json.Unmarshal(v[key], &t) == nil {
			return t
		}
		var n json.Number
		if json.Unmarshal(v[key], &n) == nil {
			return n.String()
		}
		return ""
	}
	out := &displayread.DeviceHistoryValues{Code: text("code"), Name: text("name"), Status: text("status"), Revision: text("revision"), ProductVersionID: text("product_version_id")}
	if slices.Contains(fields, "device.serial_number") {
		v := text("serial_number")
		out.SerialNumber = &v
	}
	if slices.Contains(fields, "device.description") {
		v := text("description")
		out.Description = &v
	}
	return out
}

// Source membership is supplied by its owning domain; installation membership
// remains in devices. Neither relation creates a device or output grant.
func (s *Service) WithDisplaySourceRelations(fn func(context.Context, pgx.Tx, displayread.Plan) (map[string][]string, error)) {
	s.displaySources = fn
}
func (s *Service) DisplayRelationsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, relation string) (map[string][]string, error) {
	if relation == "source" {
		if s.displaySources == nil {
			return nil, displayread.Problem("dependency_unavailable", "来源读取尚未就绪。", 503)
		}
		return s.displaySources(ctx, tx, p)
	}
	out := map[string][]string{}
	if relation != "installation" {
		return out, nil
	}
	rows, e := tx.Query(ctx, `SELECT device_id::text,facility_id::text FROM device_installations WHERE tenant_id=$1 AND device_id=ANY($2::uuid[]) AND uninstalled_at IS NULL ORDER BY device_id,id`, p.Authority.Owner.Tenant, p.Scopes["target"].IDs("device"))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var facility *string
		if e = rows.Scan(&id, &facility); e != nil {
			return nil, e
		}
		if facility != nil {
			out[id] = append(out[id], *facility)
		}
	}
	return out, rows.Err()
}
func deviceCounts(items []Device) displayread.DeviceCounts {
	counts := map[string]string{"enabled": "0", "disabled": "0"}
	seen := map[string]bool{}
	for _, d := range items {
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		n, _ := strconv.Atoi(counts[d.Status])
		counts[d.Status] = strconv.Itoa(n + 1)
	}
	return displayread.DeviceCounts{UniqueDeviceCount: strconv.Itoa(len(seen)), StatusCounts: counts}
}

func (s *Service) DisplayStatisticsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, resources []displayread.ResourceSummary, memberships ...map[string][]displayread.StructureGroupRef) (displayread.Materialized, error) {
	q := p.Request.(*displayread.DeviceStatisticsRequest)
	out := displayread.Materialized{Rows: []json.RawMessage{}}
	items, e := s.DisplayDevicesTx(ctx, tx, p, q.Filters, "")
	if e != nil {
		return out, e
	}
	relation := q.Relation
	if q.Population == "monitoring_bound" {
		relation = "source"
	}
	relations, e := s.DisplayRelationsTx(ctx, tx, p, relation)
	if e != nil {
		return out, e
	}
	if q.Population == "monitoring_bound" {
		items = slices.DeleteFunc(items, func(d Device) bool { return len(relations[d.ID]) == 0 })
	}
	type bucket struct {
		label, visibility string
		ref               *displayread.Ref
		items             []Device
	}
	groups := map[string]*bucket{}
	structureGroups := map[string][]displayread.StructureGroupRef{}
	if len(memberships) > 0 {
		structureGroups = memberships[0]
	}
	facilities := map[string]displayread.ResourceSummary{}
	for _, r := range resources {
		if r.Kind == "facility" && r.Visibility == "full" {
			facilities[r.ID] = r
		}
	}
	add := func(key, label, visibility string, ref *displayread.Ref, d *Device) {
		g := groups[key]
		if g == nil {
			g = &bucket{label: label, visibility: visibility, ref: ref}
			groups[key] = g
		}
		if d != nil {
			g.items = append(g.items, *d)
		}
	}
	// The requested visible facility population includes legitimate empty groups.
	if displayread.NewStructureKind(q.GroupBy) {
		for _, r := range resources {
			if r.Kind == q.GroupBy && r.Visibility == "full" {
				add(r.ID, r.Name, "full", &displayread.Ref{Kind: r.Kind, ID: r.ID}, nil)
			}
		}
	}
	for _, f := range facilities {
		if len(q.Filters.FacilityIDs) > 0 && !slices.Contains(q.Filters.FacilityIDs, f.ID) {
			continue
		}
		if q.GroupBy == "facility" {
			add(f.ID, f.Name, "full", &displayread.Ref{Kind: "facility", ID: f.ID}, nil)
		} else if q.GroupBy == "facility_type" && f.Type != nil {
			add(f.Type.DefinitionID, f.Type.Name, "full", &displayread.Ref{Kind: "facility_type", ID: f.Type.DefinitionID}, nil)
		}
	}
	for _, d := range items {
		if displayread.NewStructureKind(q.GroupBy) {
			values := structureGroups[d.ID]
			if len(values) == 0 {
				add("unassigned", "未分配工程归属", "unassigned", nil, &d)
			}
			for _, g := range values {
				add(g.Key, g.Label, g.Visibility, g.Ref, &d)
			}
			continue
		}
		if slices.Contains([]string{"manufacturer", "product", "product_model"}, q.GroupBy) && !slices.Contains(p.Scopes["target"].FieldGroups, "device.metadata") {
			add("restricted_context", "受限设备分类", "restricted", nil, &d)
			continue
		}
		switch q.GroupBy {
		case "none":
		case "manufacturer":
			add(d.Product.ManufacturerID, d.Product.ManufacturerName, "full", &displayread.Ref{Kind: "manufacturer", ID: d.Product.ManufacturerID}, &d)
		case "product":
			add(d.ProductID, d.Product.Name, "full", &displayread.Ref{Kind: "product", ID: d.ProductID}, &d)
		case "product_model":
			add(strings.Join([]string{d.Product.ManufacturerID, d.ProductID, d.Product.Model}, ":"), d.Product.Model, "full", &displayread.Ref{Kind: "product", ID: d.ProductID}, &d)
		default:
			fs := relations[d.ID]
			if len(fs) == 0 {
				add("unassigned", "未分配工程归属", "unassigned", nil, &d)
			}
			for _, id := range fs {
				f, ok := facilities[id]
				if !ok {
					add("restricted_context", "受限工程归属", "restricted", nil, &d)
					continue
				}
				if q.GroupBy == "facility" {
					add(id, f.Name, "full", &displayread.Ref{Kind: "facility", ID: id}, &d)
				} else if f.Type != nil {
					add(f.Type.DefinitionID, f.Type.Name, "full", &displayread.Ref{Kind: "facility_type", ID: f.Type.DefinitionID}, &d)
				}
			}
		}
	}
	keys := []string{}
	for k := range groups {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	additivity := "disjoint"
	if q.GroupBy == "facility" || q.GroupBy == "facility_type" || displayread.NewStructureKind(q.GroupBy) {
		additivity = "overlapping"
	}
	out.Metadata = map[string]any{"definitions_version": "1", "population": q.Population, "group_by": q.GroupBy, "relation": relation, "filters": q.Filters, "additivity": additivity, "totals": deviceCounts(items), "count_basis": map[string]string{"deduplicate_by": "device_id", "metadata": "current_read_snapshot", "monitoring_bound": "current_authorized_channel_and_output_binding"}}
	if displayread.NewStructureKind(q.GroupBy) {
		out.Metadata["structure_model"] = 2
		out.Metadata["membership"] = q.Membership
		out.Metadata["definitions_version"] = "2"
	}
	for _, key := range keys {
		g := groups[key]
		out.Rows = append(out.Rows, displayread.Bytes(displayread.StatisticsGroup[displayread.DeviceCounts]{Key: key, Label: g.label, Visibility: g.visibility, Ref: g.ref, Totals: deviceCounts(g.items)}))
	}
	if len(items) == 0 {
		out.State = "empty"
	}
	return out, nil
}
