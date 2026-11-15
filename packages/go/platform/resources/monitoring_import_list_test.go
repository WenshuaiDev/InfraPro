//go:build monitoringintegration

package resources_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func TestMonitoringImportListFilteringAndStablePagination(t *testing.T) {
	f := monitoringSetup(t)
	ids := []string{}
	for i := 0; i < 53; i++ {
		name := fmt.Sprintf("batch-%02d.csv", i)
		if i%3 == 0 {
			name = fmt.Sprintf("batch-%02d.xlsx", i)
		}
		if i == 0 {
			name = "literal_%-target.csv"
		}
		b, e := f.service.CreateImport(ctx, f.actor, f.scope, md.ImportInput{WriteMeta: f.meta("0"), Name: name, Size: "4"})
		must(t, e)
		ids = append(ids, b.ID)
		if i < 51 {
			_, e = f.service.CancelImport(ctx, f.actor, f.scope, b.ID, f.meta(b.Revision))
			must(t, e)
		}
	}
	// Timestamp ties deliberately exercise the second half of the keyset ordering.
	_, e := f.env.Pool.Exec(ctx, `UPDATE monitoring_import_batches SET created_at='2026-01-01T00:00:00Z' WHERE tenant_id=$1`, f.tenant)
	must(t, e)
	slices.Sort(ids)
	for _, order := range []string{"newest", "oldest"} {
		q := md.ImportQuery{Limit: 10, Sort: order}
		got := []string{}
		pages := 0
		for {
			page, e := f.service.QueryImports(ctx, f.actor, f.scope, q)
			must(t, e)
			pages++
			for _, b := range page.Items {
				got = append(got, b.ID)
			}
			if len(page.Items) > 10 {
				t.Fatal("page exceeds requested size")
			}
			if page.NextCursor == "" {
				break
			}
			q.Cursor = page.NextCursor
			if pages > 6 {
				t.Fatal("cursor loop")
			}
		}
		expected := slices.Clone(ids)
		if order == "newest" {
			slices.Reverse(expected)
		}
		if !slices.Equal(got, expected) || pages != 6 {
			t.Fatalf("unstable %s pages: %d items %d pages", order, len(got), pages)
		}
	}
	for _, q := range []md.ImportQuery{{Search: "literal_%", Limit: 10}, {Search: strings.ToUpper(ids[0]), Limit: 10}} {
		page, e := f.service.QueryImports(ctx, f.actor, f.scope, q)
		must(t, e)
		if len(page.Items) != 1 {
			t.Fatalf("full-set exact/literal search returned %d", len(page.Items))
		}
	}
	q := md.ImportQuery{States: "cancelled,uploading", Formats: "xlsx", Limit: 100}
	page, e := f.service.QueryImports(ctx, f.actor, f.scope, q)
	must(t, e)
	if len(page.Items) != 17 {
		t.Fatalf("format filter: %d", len(page.Items))
	}
	q = md.ImportQuery{States: "cancelled", Limit: 10}
	page, e = f.service.QueryImports(ctx, f.actor, f.scope, q)
	must(t, e)
	for _, b := range page.Items {
		if b.State != "cancelled" {
			t.Fatal("state filter ignored")
		}
	}
	q.Cursor = page.NextCursor
	q.States = "uploading"
	if _, e = f.service.QueryImports(ctx, f.actor, f.scope, q); e == nil {
		t.Fatal("query-bound cursor accepted changed filter")
	}
	if _, e = f.service.QueryImports(ctx, f.actor, md.TenantScope("00000000-0000-4000-8000-000000000001"), md.ImportQuery{Search: ids[0], Limit: 10}); e == nil {
		t.Fatal("foreign tenant permitted")
	}
}
