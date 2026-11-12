package devicecatalog_test

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

func TestPublicationDatabaseGuards(t *testing.T) {
	f := setup(t)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "fixed", []c.Slot{f.fixed()})
	private := f.product(t, c.TenantScope(f.tenant), f.owner, m.ID, "private", nil)
	for _, tc := range []struct{ name, mutation string }{
		{"projected unit", `UPDATE device_product_version_slots SET unit_version_id=NULL WHERE version_id=$1`},
		{"fixed meaning", `UPDATE device_product_version_slots SET content=jsonb_set(content,'{timestamp_meaning}','"interval_end"') WHERE version_id=$1`},
		{"missing dependencies", `DELETE FROM device_product_dependencies WHERE product_version_id=$1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, e := f.env.Pool.Begin(ctx)
			must(t, e)
			defer tx.Rollback(ctx)
			var id string
			must(t, tx.QueryRow(ctx, `INSERT INTO device_product_versions(product_id,number,schema_version,content,content_hash,published_by) SELECT product_id,2,1,content,content_hash,published_by FROM device_product_versions WHERE id=$1 RETURNING id::text`, p.VersionID).Scan(&id))
			_, e = tx.Exec(ctx, `INSERT INTO device_product_version_states(version_id,updated_by) VALUES($1,$2)`, id, f.actor().ID)
			must(t, e)
			_, e = tx.Exec(ctx, `INSERT INTO device_product_version_slots SELECT $1,product_id,slot_key,mode,slot_order,content,metric_version_id,unit_version_id,allowed_shapes FROM device_product_version_slots WHERE version_id=$2`, id, p.VersionID)
			must(t, e)
			_, e = tx.Exec(ctx, `INSERT INTO device_product_dependencies SELECT $1,engineering_version_id,path FROM device_product_dependencies WHERE product_version_id=$2`, id, p.VersionID)
			must(t, e)
			// Raw updates are themselves forbidden, even before deferred completeness checks.
			_, e = tx.Exec(ctx, tc.mutation, id)
			var pe *pgconn.PgError
			if !errors.As(e, &pe) || pe.Code != "23514" {
				t.Fatalf("expected database invariant, got %v", e)
			}
		})
	}
	for _, sql := range []string{`DELETE FROM device_product_version_states WHERE version_id=$1`, `UPDATE device_product_version_states SET revision=revision+2 WHERE version_id=$1`} {
		_, e = f.env.Pool.Exec(ctx, sql, p.VersionID)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" {
			t.Fatalf("state guard: %v", e)
		}
	}
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `INSERT INTO device_product_versions(product_id,number,schema_version,content,content_hash,source_version_id,published_by) SELECT product_id,2,1,content,content_hash,$2,published_by FROM device_product_versions WHERE id=$1`, p.VersionID, private.VersionID)
	must(t, e)
	e = tx.Commit(ctx)
	var pe *pgconn.PgError
	if !errors.As(e, &pe) || pe.Code != "23514" {
		t.Fatalf("copy provenance: %v", e)
	}
	// A version without its state/slot/dependency projections must fail at commit.
	tx, e = f.env.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `INSERT INTO device_product_versions(product_id,number,schema_version,content,content_hash,published_by) SELECT product_id,2,1,content,content_hash,published_by FROM device_product_versions WHERE id=$1`, p.VersionID)
	must(t, e)
	e = tx.Commit(ctx)
	if !errors.As(e, &pe) || pe.Code != "23514" {
		t.Fatalf("incomplete publication: %v", e)
	}
}
