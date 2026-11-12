//go:build monitoringintegration

package resources_test

import (
	"fmt"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"testing"
)

func TestMonitoringRecentListsUseStableChronologicalPages(t *testing.T) {
	f := monitoringSetup(t)
	var expected []string
	var batch md.ImportBatch
	for n := 0; n < 55; n++ {
		var e error
		batch, e = f.service.CreateImport(ctx, f.actor, f.scope, md.ImportInput{WriteMeta: f.meta("0"), Name: fmt.Sprintf("synthetic-%02d.csv", n), Size: "1"})
		must(t, e)
		expected = append([]string{batch.ID}, expected...)
		_, e = f.service.CancelImport(ctx, f.actor, f.scope, batch.ID, f.meta(batch.Revision))
		must(t, e)
	}
	var actual []string
	for cursor := ""; ; {
		page, e := f.service.Imports(ctx, f.actor, f.scope, cursor)
		must(t, e)
		if len(page.Items) > 50 {
			t.Fatal("unbounded import list")
		}
		for _, b := range page.Items {
			actual = append(actual, b.ID)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if fmt.Sprint(actual) != fmt.Sprint(expected) {
		t.Fatal("import pagination lost chronological order", actual, expected)
	}
	// Hidden maintenance jobs must not shift the page boundary or include the
	// extra 51st candidate on both pages.
	_, e := f.env.Pool.Exec(ctx, `INSERT INTO monitoring_jobs(tenant_id,actor_id,kind,target_id,target_revision,input_hash,input,state,created_at)
 SELECT $1,$2,CASE WHEN n IN (10,20,40) THEN 'summary' ELSE 'validate' END,$3,0,repeat(lpad(to_hex(n),2,'0'),32),'{}'::jsonb,'cancelled',clock_timestamp()-make_interval(secs=>n) FROM generate_series(0,54) n`, f.tenant, f.actor.ID, batch.ID)
	must(t, e)
	seen := map[string]bool{}
	var previous md.Job
	pages := 0
	for cursor := ""; ; {
		page, e := f.service.Jobs(ctx, f.actor, f.scope, cursor)
		must(t, e)
		pages++
		for _, j := range page.Items {
			if seen[j.ID] {
				t.Fatal("job duplicated across pages", j.ID)
			}
			if previous.ID != "" && j.CreatedAt.After(previous.CreatedAt) {
				t.Fatal("jobs not newest first")
			}
			previous = j
			seen[j.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
		if pages > 3 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != 52 || pages != 2 {
		t.Fatal(len(seen), pages)
	}
	if _, e = f.service.Jobs(ctx, f.actor, f.scope, "invalid-cursor"); e == nil {
		t.Fatal("malformed cursor accepted")
	}
}
