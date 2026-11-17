package audit_test

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/google/uuid"
)

func TestAuditEngineeringHistoryFiltersBeforeCandidateBudget(t *testing.T) {
	f := setup(t)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	engineering := eng.New(f.env.Service.Access())
	meta := func() eng.WriteMeta {
		return eng.WriteMeta{RequestID: uuid.NewString(), Reason: "工程操作记录回归"}
	}
	publish := func(kind eng.Kind, code string, content eng.Content) eng.Result {
		t.Helper()
		created, e := engineering.Create(ctx, f.actor, eng.Scope{Kind: kind}, eng.CreateInput{WriteMeta: meta(), Code: code, Content: content}, access.Meta{})
		must(t, e)
		published, e := engineering.Publish(ctx, f.actor, eng.Scope{Kind: kind}, created.DefinitionID, eng.RevisionInput{WriteMeta: meta(), Revision: created.Revision}, access.Meta{})
		must(t, e)
		return published
	}
	_, e := engineering.SaveCategory(ctx, f.actor, eng.CategoryInput{WriteMeta: meta(), Key: "length", Name: "长度", Description: "空间长度"}, access.Meta{})
	must(t, e)
	unit := publish(eng.UnitDefinition, "mm", eng.Content{MetamodelVersion: "1", Name: "毫米", Unit: &eng.Unit{Symbol: "mm", Category: "length"}})
	typ := publish(eng.TypeDefinition, "bridge", eng.Content{MetamodelVersion: "1", Name: "桥梁", Components: []eng.Component{{Key: "beam", Name: "主梁"}}, Relations: []eng.Relation{{Parent: "root", Child: "beam", MinCount: 1}}})
	metric := publish(eng.MetricDefinition, "displacement", eng.Content{MetamodelVersion: "1", Name: "位移", Metric: &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: unit.VersionID, AllowedUnits: []string{unit.VersionID}}})
	template := publish(eng.MonitoringTemplate, "bridge_monitoring", eng.Content{MetamodelVersion: "1", Name: "桥梁监测", Template: &eng.Template{
		TypeVersions: []string{typ.VersionID},
		Items:        []eng.Item{{Key: "movement", Name: "位移", Enabled: true, Targets: []eng.Target{{TypeVersion: typ.VersionID, Component: "beam"}}, Bindings: []eng.Binding{{MetricVersion: metric.VersionID, UnitVersion: unit.VersionID, ResourceKind: "observation_channel", MinCount: 1}}}},
	}})
	// Newer, unrelated facts must not consume the budget of a specific object's
	// history. These exceed the production cap, not just the page size.
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO account_security_events(event_type,object_type,object_id,actor_type,outcome,visibility_class,reason)
		SELECT 'role_created','role',gen_random_uuid()::text,'system','success','platform','unrelated history' FROM generate_series(1,$1)`, config.DefaultAudit().CandidateBudget+1)
	must(t, e)
	for kind, definition := range map[eng.Kind]eng.Result{eng.TypeDefinition: typ, eng.MetricDefinition: metric, eng.UnitDefinition: unit, eng.MonitoringTemplate: template} {
		t.Run(string(kind), func(t *testing.T) {
			filter := audit.Filter{ObjectType: "engineering_definition", ObjectID: definition.DefinitionID, Limit: 20}
			page, e := s.Events(ctx, f.actor, scope, filter)
			must(t, e)
			if len(page.Items) != 2 || page.NextCursor != "" || page.Items[0].EventType != "engineering.publish" || page.Items[1].EventType != "engineering.create" {
				t.Fatalf("object history missing or mixed with unrelated facts: %+v", page)
			}
			// The engineering tabs use the legacy adapter; object-history links use
			// the new reader. Both must retain identical filtering and pagination.
			legacyFilter := access.Filter{ObjectType: filter.ObjectType, ObjectID: filter.ObjectID, Limit: 1}
			for i, expected := range page.Items {
				legacy, e := s.LegacyEvents(ctx, f.actor, scope, legacyFilter)
				must(t, e)
				if len(legacy.Items) != 1 || legacy.Items[0].ID != expected.ID || (legacy.NextCursor == "") != (i == len(page.Items)-1) {
					t.Fatalf("legacy history page %d lost its order or terminal cursor: %+v", i, legacy)
				}
				legacyFilter.Cursor = legacy.NextCursor
			}
			filter.Query = "no matching operation reason"
			page, e = s.Events(ctx, f.actor, scope, filter)
			must(t, e)
			if len(page.Items) != 0 || page.NextCursor != "" {
				t.Fatal("empty object search returned rows or a cursor")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		filter audit.Filter
		count  int
	}{
		{"object type", audit.Filter{ObjectType: "engineering_definition"}, 9}, // Includes the unit-category change.
		{"object ID", audit.Filter{ObjectID: metric.DefinitionID}, 2},
		{"event type", audit.Filter{EventType: "engineering.publish"}, 4},
		{"combined", audit.Filter{ObjectType: "engineering_definition", ObjectID: metric.DefinitionID, EventType: "engineering.publish"}, 1},
		{"missing object", audit.Filter{ObjectType: "engineering_definition", ObjectID: uuid.NewString()}, 0},
		{"wrong type", audit.Filter{ObjectType: "role", ObjectID: metric.DefinitionID}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, e := s.Events(ctx, f.actor, scope, tc.filter)
			must(t, e)
			if len(page.Items) != tc.count || page.NextCursor != "" {
				t.Fatalf("wanted %d matching facts without a next cursor, got %d (has cursor: %t)", tc.count, len(page.Items), page.NextCursor != "")
			}
		})
	}
	// Unbounded text searches still fail closed at the configured cap.
	_, e = s.Events(ctx, f.actor, scope, audit.Filter{Query: "no matching operation reason"})
	errorCode(t, e, "audit_query_budget_exceeded")
}

func TestAuditTenantNoticeObjectFiltersUseDisclosedSubject(t *testing.T) {
	f := setup(t)
	tenant := f.tenant(t, "notice-filter")
	other := f.tenant(t, "other-notice-filter")
	var id string
	// Tenant notices deliberately project their subject to the current tenant,
	// even when the source fact names a platform object.
	must(t, f.env.Pool.QueryRow(ctx, `INSERT INTO account_security_events(event_type,object_type,object_id,actor_type,outcome,visibility_class,tenant_id,reason)
		VALUES('tenant.disabled','account',$1,'system','success','platform_tenant_notice',$2,'PRIVATE_PLATFORM_REASON') RETURNING id::text`, f.actor.ID, tenant).Scan(&id))
	for _, tc := range []struct {
		name   string
		scope  audit.Scope
		filter audit.Filter
		count  int
	}{
		{"tenant subject", audit.Scope{Kind: "tenant", Tenant: tenant}, audit.Filter{ObjectType: "tenant", ObjectID: tenant}, 1},
		{"tenant type", audit.Scope{Kind: "tenant", Tenant: tenant}, audit.Filter{ObjectType: "tenant"}, 1},
		{"tenant ID", audit.Scope{Kind: "tenant", Tenant: tenant}, audit.Filter{ObjectID: tenant}, 1},
		{"hidden source", audit.Scope{Kind: "tenant", Tenant: tenant}, audit.Filter{ObjectType: "account", ObjectID: f.actor.ID}, 0},
		{"different tenant", audit.Scope{Kind: "tenant", Tenant: other}, audit.Filter{ObjectType: "tenant", ObjectID: tenant}, 0},
		{"platform source", audit.Scope{Kind: "platform"}, audit.Filter{ObjectType: "account", ObjectID: f.actor.ID}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.filter.EventType = "tenant.disabled"
			page, e := f.business.Audit.Events(ctx, f.actor, tc.scope, tc.filter)
			must(t, e)
			if len(page.Items) != tc.count || tc.count == 1 && page.Items[0].ID != id {
				t.Fatalf("notice subject filter changed disclosure: %+v", page)
			}
			if tc.count == 1 && tc.scope.Kind == "tenant" && (page.Items[0].Actor.ID != "" || page.Items[0].Reason != "" || page.Items[0].Technical != nil) {
				t.Fatal("tenant notice exposed platform details")
			}
		})
	}
}
