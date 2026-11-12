//go:build monitoringintegration

package resources_test

import (
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func TestMonitoringStorageLayoutAndOnlineHistory(t *testing.T) {
	f := monitoringSetup(t)
	must(t, f.storage.Initialize(ctx))
	must(t, f.storage.Check(ctx))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,-99999999999999999999999999.000000000001,good\n2000-01-01T00:00:00.000000001Z,99999999999999999999999999.000000000003,good\n2262-04-11T23:47:16.854775807Z,0.000000000001,good\n")))
	for _, port := range []string{"59000", "59001"} {
		c, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:" + port}})
		must(t, e)
		rows, e := c.Query(ctx, `SELECT partition,disk_name,sum(rows) FROM system.parts WHERE database=? AND table='observations' AND active GROUP BY partition,disk_name ORDER BY partition`, f.database)
		must(t, e)
		parts := map[string]string{}
		for rows.Next() {
			var partition, disk string
			var n uint64
			must(t, rows.Scan(&partition, &disk, &n))
			if n != 1 {
				t.Fatal("unexpected raw part count", n)
			}
			parts[partition] = disk
		}
		must(t, rows.Err())
		rows.Close()
		must(t, c.Close())
		if parts["190001"] != "monitoring_cache" || parts["200001"] != "monitoring_cache" || parts["226204"] != "default" {
			t.Fatal("historical or upper-endpoint tiering failed", parts)
		}
	}
	query := f.query(t)
	query.Limit = 10
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 3 || page.Items[0].At != "2262-04-11T23:47:16.854775807Z" || page.Items[2].Value != "-99999999999999999999999999.000000000001" {
		t.Fatal(page)
	}
	// Distinct endpoint spellings must not masquerade as independent replicas.
	cfg := md.StorageConfig{Replicas: []string{"127.0.0.1:59000", "localhost:59000"}, Database: f.database, Policy: "monitoring_tiered", ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only", Bucket: "contract-source"}
	alias, e := md.OpenStorage(ctx, cfg)
	must(t, e)
	defer alias.Close()
	if e = alias.Check(ctx); e == nil || !strings.Contains(e.Error(), "independent replicas") {
		t.Fatal("same replica accepted twice", e)
	}
	c, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:59000"}})
	must(t, e)
	defer c.Close()
	must(t, c.Exec(ctx, "ALTER TABLE "+f.database+".observations DROP CONSTRAINT typed_value SETTINGS alter_sync=2"))
	must(t, c.Exec(ctx, "ALTER TABLE "+f.database+".observations ADD CONSTRAINT typed_value CHECK 1=1 SETTINGS alter_sync=2"))
	if e = f.storage.Check(ctx); e == nil || !strings.Contains(e.Error(), "typed-value") {
		t.Fatal("same constraint name concealed a changed predicate", e)
	}
	must(t, c.Exec(ctx, "ALTER TABLE "+f.database+".observations DROP CONSTRAINT typed_value SETTINGS alter_sync=2"))
	must(t, c.Exec(ctx, "ALTER TABLE "+f.database+".observations ADD CONSTRAINT typed_value CHECK (value_kind='integer' AND isNotNull(integer_value) AND isNull(decimal_value) AND isNull(enum_value)) OR (value_kind='decimal' AND isNull(integer_value) AND isNotNull(decimal_value) AND isNull(enum_value)) OR (value_kind='enum' AND isNull(integer_value) AND isNull(decimal_value) AND isNotNull(enum_value)) SETTINGS alter_sync=2"))
	must(t, f.storage.Check(ctx))
	must(t, c.Exec(ctx, "ALTER TABLE "+f.database+".observations MODIFY COLUMN source_precision UInt16 SETTINGS alter_sync=2"))
	if e = f.storage.Check(ctx); e == nil || !strings.Contains(e.Error(), "column layout") {
		t.Fatal("changed raw schema accepted", e)
	}
}
