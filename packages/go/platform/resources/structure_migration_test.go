package resources_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

// Seed through the actual historical schema and its constraints, then run all
// upgrade scripts. No new-schema downgrade or trigger bypass prepares this test.
func TestStructureUpgradePreservesLegacyFactsAndEverySectionOccurrence(t *testing.T) {
	f := resourcesFromFixture(t, setupAtMigration(t, 46))
	facility, component, point, channel := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	position := `{"description":"原位置","sections":[{"name":"同名断面","value":"K1"},{"name":"同名断面","value":"K1"}]}`
	_, e := f.env.Pool.Exec(ctx, `INSERT INTO engineering_facilities(id,tenant_id,code,name,type_version_id,values,position,created_by,updated_by) VALUES($1,$2,'LEGACY','旧设施',$3,'{"length":120}',$4,$5,$5)`, facility, f.tenant, f.typ, position, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_components(id,tenant_id,facility_id,code,name,category_key,position,created_by,updated_by) VALUES($1,$2,$3,'C','旧构件','span',$4,$5,$5)`, component, f.tenant, facility, position, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_points(id,tenant_id,facility_id,component_id,code,name,position,created_by,updated_by) VALUES($1,$2,$3,$4,'P','旧测点',$5,$6,$6)`, point, f.tenant, facility, component, position, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_channels(id,tenant_id,facility_id,point_id,code,name,metric_version_id,unit_version_id,timestamp_meaning,created_by,updated_by) VALUES($1,$2,$3,$4,'CH','旧通道',$5,$6,'observation_time',$7,$7)`, channel, f.tenant, facility, point, f.metric, f.unit, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_resource_changes(tenant_id,facility_id,resource_kind,resource_id,resource_revision,operation,actor_id,actor_name,request_id,reason,before,after,changed_fields,definition_versions) VALUES($1,$2,'point',$3,1,'update',$4,'旧操作者',$5,'历史原文',jsonb_build_object('position',$6::jsonb),jsonb_build_object('position',$6::jsonb),'[]','[]')`, f.tenant, facility, point, f.actor.ID, uuid.NewString(), position)
	must(t, e)
	manufacturer, product, version, device, installation := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	_, e = tx.Exec(ctx, `INSERT INTO device_manufacturers(id,code,name,created_by,updated_by) VALUES($1,'LEGACY','旧厂商',$2,$2)`, manufacturer, f.actor.ID)
	must(t, e)
	_, e = tx.Exec(ctx, `INSERT INTO device_products(id,manufacturer_id,code,created_by,updated_by) VALUES($1,$2,'LEGACY',$3,$3)`, product, manufacturer, f.actor.ID)
	must(t, e)
	_, e = tx.Exec(ctx, `INSERT INTO device_product_versions(id,product_id,number,schema_version,content,content_hash,published_by) VALUES($1,$2,1,1,'{"schema_version":1,"name":"旧无输出设备","output_slots":[]}',repeat('a',64),$3)`, version, product, f.actor.ID)
	must(t, e)
	_, e = tx.Exec(ctx, `INSERT INTO device_product_version_states(version_id,updated_by) VALUES($1,$2)`, version, f.actor.ID)
	must(t, e)
	must(t, tx.Commit(ctx))
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO devices(id,tenant_id,code,name,product_id,product_version_id,created_by,updated_by) VALUES($1,$2,'LEGACY','旧安装设备',$3,$4,$5,$5)`, device, f.tenant, product, version, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO device_installations(id,tenant_id,device_id,target_kind,facility_id,point_id,position,installed_at,recorded_by,updated_by) VALUES($1,$2,$3,'point',$4,$5,$6,now(),$7,$7)`, installation, f.tenant, device, facility, point, position, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO device_resource_changes(tenant_id,device_id,installation_id,operation,actor_id,actor_name,actor_scope,tenant_membership_id,request_id,before,after,changed_fields) SELECT $1,$2,$3,'installation.create',$4,'旧操作者','tenant',id,$5,NULL,jsonb_build_object('installation',jsonb_build_object('id',$3::uuid::text,'revision',1,'facility_id',$6::uuid::text,'position',$7::jsonb)),'[]' FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$4`, f.tenant, device, installation, f.actor.ID, uuid.NewString(), facility, position)
	must(t, e)
	before := map[string]string{}
	for _, table := range []string{"engineering_versions", "engineering_dependencies", "engineering_channels", "engineering_resource_changes", "devices", "device_product_versions", "device_installation_target_refs", "device_resource_changes"} {
		var digest string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r)-'monitoring_section_id'-'borehole_id' ORDER BY to_jsonb(r)::text)::text,'[]')) FROM `+table+` r`).Scan(&digest))
		before[table] = digest
	}
	must(t, migrations.Up(ctx, f.env.Config.DatabaseURL))
	must(t, migrations.Check(ctx, f.env.Pool))
	for table, want := range before {
		var got string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r)-'monitoring_section_id'-'borehole_id' ORDER BY to_jsonb(r)::text)::text,'[]')) FROM `+table+` r`).Scan(&got))
		if got != want {
			t.Fatal("migration rewrote retained facts", table)
		}
	}
	var total, current, historical, entities int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE NOT historical),count(*) FILTER(WHERE historical) FROM engineering_legacy_section_evidence WHERE tenant_id=$1`, f.tenant).Scan(&total, &current, &historical))
	if total != 14 || current != 8 || historical != 6 {
		t.Fatalf("lost duplicate occurrences: %d/%d/%d", total, current, historical)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM engineering_monitoring_sections)+(SELECT count(*) FROM engineering_boreholes)`).Scan(&entities))
	if entities != 0 {
		t.Fatal("text fabricated real entities", entities)
	}
	for _, kind := range []string{"facility", "component", "point"} {
		var raw []byte
		must(t, f.env.Pool.QueryRow(ctx, `SELECT data->'position' FROM engineering_resource_records WHERE tenant_id=$1 AND kind=$2`, f.tenant, kind).Scan(&raw))
		var data map[string]any
		must(t, json.Unmarshal(raw, &data))
		if _, ok := data["sections"]; ok || data["description"] != "原位置" {
			t.Fatal(string(raw))
		}
	}
	for _, want := range []struct{ kind, id, parentKind, parent string }{{"component", component, "facility", facility}, {"point", point, "component", component}, {"channel", channel, "point", point}} {
		var kind, id string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT parent_kind,parent_id::text FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3`, f.tenant, want.kind, want.id).Scan(&kind, &id))
		if kind != want.parentKind || id != want.parent {
			t.Fatal(want, kind, id)
		}
	}
	var state string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM device_effective_classifications WHERE device_id=$1`, device).Scan(&state))
	if state != "unclassified" {
		t.Fatal(state)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_points SET position=$2 WHERE id=$1`, point, position)
	if e == nil {
		t.Fatal("legacy writer reintroduced text sections")
	}
	// The current service can read the old point using its stable ID after upgrade.
	detail, e := f.srv.Detail(ctx, f.actor, r.TenantScope(f.tenant), r.Target{Kind: r.Point, ID: point, Facility: facility})
	must(t, e)
	if detail.ID != point || detail.ParentRef.ID != component {
		t.Fatal(detail)
	}
	db, e := sql.Open("pgx", f.env.Config.DatabaseURL)
	must(t, e)
	defer db.Close()
	dir, e := filepath.Abs("../migrations")
	must(t, e)
	provider, e := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	must(t, e)
	// A single migration transaction cannot discard the retained evidence.
	_, e = provider.DownTo(ctx, 46)
	if e == nil {
		t.Fatal("destructive historical downgrade allowed")
	}
	must(t, migrations.Up(ctx, f.env.Config.DatabaseURL))
	must(t, migrations.Check(ctx, f.env.Pool))
	t.Log("v46→current preserves v1 digests, channels, device/product/install-target/history facts; 14 independent occurrences remain; 0 fabricated sections; incompatible rollback rejected")
}
