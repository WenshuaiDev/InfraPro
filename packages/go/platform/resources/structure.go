package resources

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

const MaxStructureDepth = 512
const MaxStructureNodes = 100000

func isStructureContainer(k Kind) bool {
	return k == Facility || k == Component || k == MonitoringSection || k == Borehole
}
func inferParentRef(r Resource) *StructureRef {
	if r.Kind == Facility {
		return nil
	}
	k, id := Facility, r.FacilityID
	switch r.Kind {
	case Component:
		if r.ParentComponentID != nil {
			k, id = Component, *r.ParentComponentID
		}
	case Point:
		if r.ComponentID != nil {
			k, id = Component, *r.ComponentID
		}
	case Channel:
		k = Kind(r.OwnerKind)
		switch k {
		case Point:
			id = r.PointID
		case Component:
			id = ptrValue(r.ComponentID)
		case MonitoringSection:
			id = ptrValue(r.MonitoringSectionID)
		case Borehole:
			id = ptrValue(r.BoreholeID)
		}
	}
	return &StructureRef{Kind: k, ID: id}
}
func loadParentRef(ctx context.Context, tx pgx.Tx, r *Resource) error {
	if r.Kind == Facility {
		return nil
	}
	p := StructureRef{}
	e := tx.QueryRow(ctx, `SELECT parent_kind,parent_id::text FROM engineering_resource_nodes WHERE tenant_id=$1 AND facility_id=$2 AND kind=$3 AND id=$4`, r.TenantID, r.FacilityID, r.Kind, r.ID).Scan(&p.Kind, &p.ID)
	if e == pgx.ErrNoRows {
		return notFound()
	}
	if e == nil {
		r.ParentRef = &p
	}
	return e
}
func structureAncestorsTx(ctx context.Context, tx pgx.Tx, t scopedTarget, r, facility Resource) ([]Resource, error) {
	out := []Resource{}
	if r.Kind == Facility {
		return out, nil
	}
	p := r.ParentRef
	if p == nil {
		p = inferParentRef(r)
	}
	if p == nil || !access.ValidID(p.ID) || !(isStructureContainer(p.Kind) || r.Kind == Channel && p.Kind == Point) {
		return nil, invalid("/parent_ref", "parent_reference", "请选择合法的同设施父对象。")
	}
	rows, e := tx.Query(ctx, `WITH RECURSIVE path AS (
 SELECT n.*,1 AS depth FROM engineering_resource_nodes n WHERE n.tenant_id=$1 AND n.facility_id=$2 AND n.kind=$3 AND n.id=$4 AND n.deleted_at IS NULL
 UNION ALL SELECT n.*,p.depth+1 FROM path p JOIN engineering_resource_nodes n ON n.tenant_id=p.tenant_id AND n.facility_id=p.facility_id AND n.kind=p.parent_kind AND n.id=p.parent_id WHERE n.deleted_at IS NULL AND p.depth<513
 ) SELECT r.data,p.depth FROM path p JOIN engineering_resource_records r USING(tenant_id,facility_id,kind,id) ORDER BY p.depth DESC`, t.Tenant, t.Facility, p.Kind, p.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	budget := MaxStructureDepth
	if r.Kind == Channel {
		budget++
	}
	for rows.Next() {
		var raw []byte
		var depth int
		var ancestor Resource
		if e = rows.Scan(&raw, &depth); e != nil {
			return nil, e
		}
		if depth > budget {
			return nil, problem("structure_budget_exceeded", "工程树深度超过 512 层预算。", 422)
		}
		if e = json.Unmarshal(raw, &ancestor); e != nil {
			return nil, e
		}
		if ancestor.Kind == r.Kind && ancestor.ID == r.ID {
			return nil, invalid("/parent_ref", "cycle", "不能指向自身或后代。")
		}
		out = append(out, ancestor)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) == 0 || out[0].Kind != Facility || out[0].ID != facility.ID {
		return nil, notFound()
	}
	return out, nil
}
func validateStructureBasic(r Resource) error {
	if p := r.ParentRef; p != nil && (!access.ValidID(p.ID) || !(isStructureContainer(p.Kind) || r.Kind == Channel && p.Kind == Point)) {
		return invalid("/parent_ref", "parent_reference", "父对象须有明确种类和有效身份。")
	}
	b := r.Borehole
	if b == nil {
		return nil
	}
	if r.Kind != Borehole {
		return invalid("/borehole", "not_allowed", "只有孔位维护孔口与孔深资料。")
	}
	for key, n := range map[string]*float64{"collar_elevation": b.CollarElevation, "design_depth_m": b.DesignDepthM, "measured_depth_m": b.MeasuredDepthM, "diameter_mm": b.DiameterMM, "azimuth": b.Azimuth, "inclination": b.Inclination} {
		if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || math.Abs(*n) > engineering.MaxSafeInteger || slices.Contains([]string{"design_depth_m", "measured_depth_m", "diameter_mm"}, key) && *n < 0) {
			return invalid("/borehole/"+key, "invalid_number", "请填写有限且符合物理含义的数值；未知请留空。")
		}
	}
	if (b.CollarElevation != nil) != (strings.TrimSpace(b.ElevationDatum) != "") {
		return invalid("/borehole/elevation_datum", "datum_required", "孔口高程与高程基准须同时提供。")
	}
	if (b.Azimuth != nil || b.Inclination != nil) && strings.TrimSpace(b.DirectionReference) == "" {
		return invalid("/borehole/direction_reference", "direction_required", "角度须说明相对方向和参考依据。")
	}
	if b.Azimuth != nil && (*b.Azimuth < 0 || *b.Azimuth >= 360) || b.Inclination != nil && (*b.Inclination < -180 || *b.Inclination > 180) {
		return invalid("/borehole", "angle_range", "方位角为 0–360 度（不含 360）；倾角为 -180–180 度。")
	}
	if b.MeasuredAt != "" {
		if _, e := time.Parse("2006-01-02", b.MeasuredAt); e != nil {
			return invalid("/borehole/measured_at", "date", "请填写真实测量日期。")
		}
	}
	for _, v := range []string{b.ElevationDatum, b.DirectionReference, b.Source} {
		if len([]rune(v)) > 2000 {
			return invalid("/borehole", "input_limit", "说明最多 2000 字符。")
		}
	}
	return nil
}

// prepareStructureWrite adapts legacy writes without ever flattening an omitted
// parent. A typed parent is only accepted on creation; moves use a fixed preview.
func prepareStructureWrite(ctx context.Context, tx pgx.Tx, t scopedTarget, r *Resource, before *Resource, in map[string]any, facility Resource) error {
	if r.Kind == Facility || r.Kind == Channel {
		return nil
	}
	legacy := ""
	if r.Kind == Component {
		legacy = "parent_component_id"
	}
	if r.Kind == Point {
		legacy = "component_id"
	}
	if before != nil && legacy != "" && hasKey(in, legacy) {
		if before.ParentRef != nil && !slices.Contains([]Kind{Facility, Component}, before.ParentRef.Kind) {
			return problem("structure_model_required", "当前对象位于混合工程树中，请使用结构移动入口。", 409)
		}
		r.ParentRef = inferParentRef(*r)
	}
	if r.ParentRef == nil {
		r.ParentRef = inferParentRef(*r)
	}
	if hasKey(in, "parent_ref") && legacy != "" && hasKey(in, legacy) && fingerprint(r.ParentRef) != fingerprint(inferParentRef(*r)) {
		return invalid("/parent_ref", "conflicting_parent", "不能提交两种矛盾的父级。")
	}
	ancestors, e := ancestorsTx(ctx, tx, t, *r, facility)
	if e != nil {
		return e
	}
	var component *string
	for _, a := range ancestors {
		if a.Kind == Component {
			id := a.ID
			component = &id
		}
	}
	if r.Kind == Component {
		r.ParentComponentID = component
	}
	if r.Kind == Point {
		r.ComponentID = component
	}
	return nil
}
func storeStructureParent(ctx context.Context, tx pgx.Tx, r Resource) error {
	if r.Kind == Facility || r.Kind == Channel || r.ParentRef == nil {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE engineering_structure_nodes SET parent_kind=$5,parent_id=$6,revision=revision+1 WHERE tenant_id=$1 AND facility_id=$2 AND kind=$3 AND id=$4 AND (parent_kind,parent_id) IS DISTINCT FROM ($5::text,$6::uuid)`, r.TenantID, r.FacilityID, r.Kind, r.ID, r.ParentRef.Kind, r.ParentRef.ID)
	return e
}
func applyStructureActions(d *Detail, auth authorization) {
	var c engineering.Content
	for _, v := range d.Definitions {
		if v.Kind == engineering.TypeDefinition {
			c = v.Content
			break
		}
	}
	if isStructureContainer(d.Kind) {
		section := engineering.StructureApplicability(c, "monitoring_section") == "applicable"
		hole := engineering.StructureApplicability(c, "borehole") == "applicable"
		if auth.Entry == "project" {
			d.Actions.CreateMonitoringSection = d.Actions.CreateMonitoringSection && section
			d.Actions.CreateBorehole = d.Actions.CreateBorehole && hole
		} else {
			d.Actions.CreateMonitoringSection = d.Actions.Add && section
			d.Actions.CreateBorehole = d.Actions.Add && hole
		}
	}

	d.Actions.Move = d.Actions.Edit && d.Kind != Facility && d.Kind != Channel
	d.Actions.AdoptType = d.Kind == Facility && auth.Entry == "tenant" && d.Actions.Edit
}
