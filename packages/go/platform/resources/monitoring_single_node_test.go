//go:build monitoringintegration

package resources_test

import (
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func TestMonitoringSingleNodePublicationAndOnlineHistory(t *testing.T) {
	f := monitoringSetup(t, func(cfg *md.StorageConfig) { cfg.Replicas = cfg.Replicas[:1] })
	c, err := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:59000"}})
	must(t, err)
	defer c.Close()
	var total, active uint32
	must(t, c.QueryRow(ctx, "SELECT total_replicas,active_replicas FROM system.replicas WHERE database=? AND table='observations'", f.database).Scan(&total, &active))
	if total != 1 || active != 1 {
		t.Fatalf("test must use a real single-replica table: total=%d active=%d", total, active)
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,-99999999999999999999999999.000000000001,good\n2262-04-11T23:47:16.854775807Z,0.000000000001,good\n")))
	page, err := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, err)
	if len(page.Items) != 2 || page.Items[0].At != "2262-04-11T23:47:16.854775807Z" || page.Items[1].Value != "-99999999999999999999999999.000000000001" {
		t.Fatal("single-node publication lost exact values or time bounds", page)
	}
	var coldRows uint64
	must(t, c.QueryRow(ctx, "SELECT sum(rows) FROM system.parts WHERE database=? AND table='observations' AND active AND disk_name='monitoring_cache'", f.database).Scan(&coldRows))
	if coldRows != 1 {
		t.Fatal("single-node history did not retain the online S3 tier", coldRows)
	}
	must(t, f.storage.Check(ctx))
	t.Log("single real replica: initialization, CSV validation/publication, exact reads and online history passed")
}
