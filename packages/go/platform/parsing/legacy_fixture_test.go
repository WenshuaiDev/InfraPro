package parsing_test

import (
	"encoding/json"
	"testing"

	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
)

// Historical migration fixtures write through the old schema and its real
// constraints. Current product/device commands require later classification
// tables and are deliberately not supported before the database is upgraded.
func seedLegacyProductDevice(t *testing.T, f fixture, manufacturer, code string) (c.Result, devices.Result) {
	t.Helper()
	product := c.Result{ID: uuid.NewString(), VersionID: uuid.NewString(), Number: 1, Revision: "1"}
	device := devices.Result{ID: uuid.NewString(), Revision: "1"}
	slot := f.fixed()
	content := c.Content{SchemaVersion: 1, Name: "历史位移传感器", Model: "Sensor-X", OutputSlots: []c.Slot{slot}}
	must(t, c.Normalize(&content))
	body, err := json.Marshal(content)
	must(t, err)
	slotBody, err := json.Marshal(slot)
	must(t, err)
	semantics := c.Semantics{MetricVersionID: f.metric, UnitVersionID: &f.unit, ValueShape: "decimal", TimestampMeaning: "observation_time"}
	tx, err := f.env.Pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := tx.Exec(ctx, query, args...)
		must(t, err)
	}
	exec(`INSERT INTO device_products(id,manufacturer_id,code,created_by,updated_by) VALUES($1,$2,$3,$4,$4)`, product.ID, manufacturer, code, f.actor().ID)
	exec(`INSERT INTO device_product_versions(id,product_id,number,schema_version,content,content_hash,published_by) VALUES($1,$2,1,1,$3,$4,$5)`, product.VersionID, product.ID, body, wire.Digest(body), f.actor().ID)
	exec(`INSERT INTO device_product_version_states(version_id,updated_by) VALUES($1,$2)`, product.VersionID, f.actor().ID)
	exec(`INSERT INTO device_product_slot_keys(product_id,key,mode,core_signature) VALUES($1,'x','fixed',$2)`, product.ID, slot.Core())
	exec(`INSERT INTO device_product_version_slots(version_id,product_id,slot_key,mode,slot_order,content,metric_version_id,unit_version_id) VALUES($1,$2,'x','fixed',0,$3,$4,$5)`, product.VersionID, product.ID, slotBody, f.metric, f.unit)
	exec(`INSERT INTO device_product_dependencies(product_version_id,engineering_version_id,path) VALUES($1,$2,'/output_slots/0/metric_version_id'),($1,$3,'/output_slots/0/unit_version_id')`, product.VersionID, f.metric, f.unit)
	exec(`INSERT INTO devices(id,tenant_id,code,name,product_id,product_version_id,output_revision,created_by,updated_by) VALUES($1,$2,$3,'历史设备',$4,$5,1,$6,$6)`, device.ID, f.tenant, code, product.ID, product.VersionID, f.owner.ID)
	exec(`INSERT INTO device_outputs(tenant_id,device_id,product_version_id,slot_key,generation,name,metric_version_id,unit_version_id,value_shape,timestamp_meaning,semantic_hash,created_by,updated_by) VALUES($1,$2,$3,'x',1,'X 输出',$4,$5,'decimal','observation_time',$6,$7,$7)`, f.tenant, device.ID, product.VersionID, f.metric, f.unit, semantics.Hash(), f.owner.ID)
	must(t, tx.Commit(ctx))
	return product, device
}

// Seed a v44 inherited adoption and queued run without invoking new catalog
// readers. The real parser worker executes it before the migration, and the
// same frozen input is replayed through the current service after the migration.
func seedLegacyParsingRun(t *testing.T, f fixture, product c.Result, device devices.Result, sampleID string) parse.Result {
	t.Helper()
	var entry wire.Entry
	must(t, f.env.Pool.QueryRow(ctx, `SELECT entry FROM parsing_method_versions WHERE method_id=$1 AND method_version='1.0.0'`, example).Scan(&entry))
	if len(entry.Outputs) != 1 || entry.Outputs[0].Key != "displacement" {
		t.Fatal("historical parsing fixture requires the original displacement contract")
	}
	sample, err := f.ps.Sample(ctx, f.owner, c.TenantScope(f.tenant), sampleID)
	must(t, err)
	semantics := c.Semantics{MetricVersionID: f.metric, UnitVersionID: &f.unit, ValueShape: "decimal", TimestampMeaning: "observation_time"}
	mapping := parse.Mapping{OutputKey: "displacement", SlotKey: "x", Status: "semantic_unknown", MethodSemantics: entry.Outputs[0].Semantics, SlotSemantics: &semantics}
	snapshot := parse.Snapshot{OwnerKind: "product", OwnerID: product.ID, ProductVersionID: product.VersionID, Method: entry.Method(), OutputContractDigest: entry.OutputContractDigest, Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "x", Confirmed: true}}, Mapping: []parse.Mapping{mapping}, Example: entry.Example}
	snapshotBody, err := json.Marshal(snapshot)
	must(t, err)
	configID, revisionID, adoptionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	run := parse.Result{ID: uuid.NewString(), Generation: "1", Revision: "1", State: "queued"}
	tx, err := f.env.Pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := tx.Exec(ctx, query, args...)
		must(t, err)
	}
	exec(`INSERT INTO parsing_configs(id,owner_kind,product_id,product_version_id,name,created_by) VALUES($1,'product',$2,$3,'历史默认解析',$4)`, configID, product.ID, product.VersionID, f.actor().ID)
	exec(`INSERT INTO parsing_config_revisions(id,config_id,product_version_id,number,method_id,method_version,snapshot,content_digest,reason,created_by) VALUES($1,$2,$3,1,$4,'1.0.0',$5,$6,'历史解析配置',$7)`, revisionID, configID, product.VersionID, example, snapshotBody, wire.Digest(snapshotBody), f.actor().ID)
	exec(`INSERT INTO parsing_product_defaults(product_version_id,config_revision_id,revision,updated_by) VALUES($1,$2,1,$3)`, product.VersionID, revisionID, f.actor().ID)
	exec(`INSERT INTO parsing_device_heads(tenant_id,device_id,revision) VALUES($1,$2,1)`, f.tenant, device.ID)
	exec(`INSERT INTO parsing_device_adoptions(id,tenant_id,device_id,product_version_id,mode,config_revision_id,source_default_revision,snapshot,revision,actor_id,reason) VALUES($1,$2,$3,$4,'inherit_snapshot',$5,1,$6,1,$7,'历史继承采用')`, adoptionID, f.tenant, device.ID, product.VersionID, revisionID, snapshotBody, f.owner.ID)
	var config parse.ConfigRevision
	must(t, tx.QueryRow(ctx, `SELECT to_jsonb(r)||jsonb_build_object('number',r.number::text,'name',c.name,'enabled',c.enabled,'revision',c.revision::text) FROM parsing_config_revisions r JOIN parsing_configs c ON c.id=r.config_id WHERE r.id=$1`, revisionID).Scan(&config))
	must(t, tx.QueryRow(ctx, `SELECT id::text,generation,semantic_hash FROM device_outputs WHERE device_id=$1`, device.ID).Scan(&mapping.OutputID, &mapping.OutputGeneration, &mapping.OutputSemanticHash))
	frozen := parse.FrozenInput{Sample: sample, Context: wire.Context{ReceivedAt: sample.ReceivedAt, Transport: "tcp"}, Entry: entry, Config: &config, AdoptionID: adoptionID, Mapping: []parse.Mapping{mapping}, DeviceStatus: "enabled", TraceID: uuid.NewString()}
	frozenBody, err := json.Marshal(frozen)
	must(t, err)
	exec(`INSERT INTO parsing_runs(id,scope_key,tenant_id,kind,sample_id,method_id,method_version,config_revision_id,product_version_id,device_id,adoption_id,input_snapshot,created_by) VALUES($1,$2,$3,'device_test',$4,$5,'1.0.0',$6,$7,$8,$9,$10,$11)`, run.ID, "tenant:"+f.tenant, f.tenant, sampleID, example, revisionID, product.VersionID, device.ID, adoptionID, frozenBody, f.owner.ID)
	exec(`INSERT INTO parsing_run_executions(run_id,generation,actor_id,state) VALUES($1,1,$2,'queued')`, run.ID, f.owner.ID)
	must(t, tx.Commit(ctx))
	return run
}
