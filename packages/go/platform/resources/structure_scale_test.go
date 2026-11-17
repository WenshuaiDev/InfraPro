package resources_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func TestStructureLargePagingAndExplicitApplicability(t *testing.T) {
	f := resourcesSetup(t)
	withHole := f.typedFacility(t)
	noHole := f.structureVersion(t, f.typ, false)
	withoutHole, e := f.write(t, r.Target{Kind: r.Facility}, "create", map[string]any{"code": "NO_HOLE", "name": "明确无孔", "type_version_id": noHole, "values": map[string]any{"length": 120}})
	must(t, e)
	legacy := f.facility(t, "UNDECLARED")
	p := f.project(t, "StructurePages")
	for _, facility := range []r.Result{withHole, withoutHole, legacy} {
		g := f.accessInput(t, p, facility.ID, "create_link")
		g.Add = []r.GrantInput{{Kind: r.Facility, Capabilities: []string{"read"}}}
		f.grant(t, p, facility.ID, g)
	}
	start := time.Now()
	_, e = f.env.Pool.Exec(ctx, `WITH inserted AS (INSERT INTO engineering_monitoring_sections(tenant_id,facility_id,code,name,created_by,updated_by) SELECT $1,$2,'PAGE-'||n,'分页断面 '||n,$3,$3 FROM generate_series(1,620)n RETURNING id) INSERT INTO project_resource_grants(tenant_id,project_id,facility_id,link_id,resource_kind,monitoring_section_id,source,granted_by) SELECT $1,$4,$2,l.id,'monitoring_section',i.id,'tenant_grant',$3 FROM inserted i CROSS JOIN project_facility_links l WHERE l.project_id=$4 AND l.facility_id=$2 AND l.status='active'`, f.tenant, withHole.ID, f.actor.ID, p.id)
	must(t, e)
	b := runtime.Compose(displayReadPlanAccess(t, f))
	d := dr.Definition{ID: "resources", StructureModel: 2, Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	v := d
	v.ID = ""
	v.ScopeID = d.ID
	h := createReadDisplay(t, f, p, b, []dr.Definition{d}, map[string]dr.Definition{"resources": v}, []string{"views.read", "queries.read", "resources.directory", "resources.detail", "statistics.structure"})
	target := dr.Target{View: "resources"}
	result := readCall(t, h, "resources.query", dr.ResourcesRequest{Target: target, Paging: dr.Paging{Limit: 100}})
	seen := map[string]bool{}
	for {
		page := readData[dr.Page[dr.ResourceSummary]](t, result)
		if page.Total != "623" {
			t.Fatal("directory truncated", page.Total)
		}
		for _, item := range page.Items {
			key := item.Kind + ":" + item.ID
			if seen[key] {
				t.Fatal("duplicate page item", key)
			}
			seen[key] = true
		}
		if !page.HasMore {
			break
		}
		result = readCall(t, h, "queries.get", dr.QueryRequest{QueryID: result.Query.ID, Cursor: page.NextCursor, Limit: 100})
	}
	if len(seen) != 623 {
		t.Fatal(len(seen))
	}
	statistics := readData[dr.StructureStatistics](t, readCall(t, h, "statistics.structure", dr.StructureStatisticsRequest{Target: target, GroupBy: "none", Membership: "nearest"}))
	if statistics.Totals.MonitoringSectionCount != "620" || statistics.Totals.BoreholeCount != "0" {
		t.Fatal(statistics)
	}
	for _, state := range []string{"applicable", "not_applicable", "undeclared"} {
		if statistics.Applicability["borehole"][state] != "1" {
			t.Fatal(statistics.Applicability)
		}
	}
	for _, basis := range []struct {
		node  r.Result
		state string
	}{{withHole, "applicable"}, {withoutHole, "not_applicable"}, {legacy, "undeclared"}} {
		detail := readData[dr.ResourceDetail](t, readCall(t, h, "resources.detail", dr.ResourceDetailRequest{Target: target, ID: basis.node.ID, Kind: "facility"}))
		if detail.Summary.StructureApplicability["borehole"] != basis.state {
			t.Fatal(detail)
		}
	}
	options := display.ReadScopeOptionsInput{Purpose: "grant", Domain: "resources", Kind: "monitoring_section", StructureModel: 2, Limit: 100}
	optionIDs := map[string]bool{}
	for {
		page, e := h.s.ReadScopeOptions(ctx, p.actor, h.scope, h.id, options)
		must(t, e)
		if page.Total != "620" {
			t.Fatal(page.Total)
		}
		for _, item := range page.Items {
			optionIDs[item.Kind+":"+item.ID] = true
		}
		if !page.HasMore {
			break
		}
		options.Cursor = page.NextCursor
	}
	if len(optionIDs) != 620 {
		t.Fatal(len(optionIDs))
	}
	// The database/import boundary independently refuses holes under the fixed no-hole type.
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_boreholes(tenant_id,facility_id,code,name,created_by,updated_by) VALUES($1,$2,'INVALID','不允许的孔位',$3,$3)`, f.tenant, withoutHole.ID, f.actor.ID)
	if e == nil {
		t.Fatal("no-hole type accepted direct database import")
	}
	t.Logf("623 resources completely paged through runtime; 620 sections through selector; 620 exact sections; three applicability states; elapsed=%s", time.Since(start))
}

func TestStructureDepthBoundaryIncludesChannels(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.typedFacility(t)
	start := time.Now()
	_, e := f.env.Pool.Exec(ctx, `INSERT INTO engineering_monitoring_sections(tenant_id,facility_id,code,name,created_by,updated_by) SELECT $1,$2,'DEPTH-'||n,'深度断面 '||n,$3,$3 FROM generate_series(1,511)n`, f.tenant, facility.ID, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_structure_nodes n SET parent_kind='monitoring_section',parent_id=p.id FROM engineering_monitoring_sections c JOIN engineering_monitoring_sections p ON p.tenant_id=c.tenant_id AND p.facility_id=c.facility_id AND p.code='DEPTH-'||((substring(c.code FROM 7))::int-1) WHERE n.tenant_id=$1 AND n.facility_id=$2 AND n.kind='monitoring_section' AND n.id=c.id AND c.code<>'DEPTH-1'`, f.tenant, facility.ID)
	must(t, e)
	var parent string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM engineering_monitoring_sections WHERE tenant_id=$1 AND code='DEPTH-511'`, f.tenant).Scan(&parent))
	point := f.structureCreate(t, facility, r.Point, "DEEP_POINT", r.StructureRef{Kind: r.MonitoringSection, ID: parent}, nil)
	ct := target(facility, r.Channel, "")
	ct.Point = point.ID
	channel, e := f.write(t, ct, "create", map[string]any{"code": "DEEP_CH", "name": "最深测点的通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	ct.ID = channel.ID
	detail, e := f.srv.Detail(ctx, f.actor, r.TenantScope(f.tenant), ct)
	must(t, e)
	if len(detail.Ancestors) != 513 {
		t.Fatal(len(detail.Ancestors))
	}
	last := f.structureCreate(t, facility, r.MonitoringSection, "DEPTH-512", r.StructureRef{Kind: r.MonitoringSection, ID: parent}, nil)
	root, e := f.srv.Detail(ctx, f.actor, r.TenantScope(f.tenant), target(facility, r.Facility, facility.ID))
	must(t, e)
	_, e = f.write(t, target(facility, r.Borehole, ""), "create", map[string]any{"code": "TOO_DEEP", "name": "不得截断的超深孔位", "parent_ref": r.StructureRef{Kind: r.MonitoringSection, ID: last.ID}, "expected_structure_revision": root.StructureRevision})
	if e == nil {
		t.Fatal("513 structural levels accepted")
	}
	t.Logf("512 structural levels and attached channel accepted; deeper write rejected: %v; elapsed=%s", e, time.Since(start))
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_boreholes WHERE tenant_id=$1`, f.tenant).Scan(&count))
	if count != 0 {
		t.Fatal(fmt.Sprint("failed write leaked entity: ", count))
	}
}
