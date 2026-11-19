package parsing_test

import (
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
)

func TestArchitectureResponseEvidenceRetry(t *testing.T) {
	f := setupIngestion(t)
	_, devices, _, frame := f.receiveGatewayReadings(t)
	var receipt string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT r.id::text FROM ingestion_receipts r JOIN ingestion_objects o ON o.id=r.object_id WHERE o.byte_length=$1`, len(frame)).Scan(&receipt))
	// The domain starts its real transactions with a 10s statement timeout.
	// Delay only this fixture's new response object, after custody has committed.
	_, err := f.env.Pool.Exec(ctx, `CREATE FUNCTION slow_staged_response() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM pg_sleep(15); RETURN NEW; END $$;
		CREATE TRIGGER slow_staged_response BEFORE INSERT ON ingestion_objects FOR EACH ROW EXECUTE FUNCTION slow_staged_response()`)
	must(t, err)
	started := time.Now()
	_, err = f.s.IdentifyReceipt(ctx, f.tenant, receipt, 1)
	must(t, err)
	if time.Since(started) < 9*time.Second {
		t.Fatal("the original PostgreSQL statement timeout was not reached")
	}
	_, err = f.env.Pool.Exec(ctx, `DROP TRIGGER slow_staged_response ON ingestion_objects`)
	must(t, err)
	var decision, machine string
	var unitsBefore int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,response->>'execution_id',(SELECT count(*) FROM ingestion_units WHERE receipt_id=$1)
		FROM ingestion_identity_decisions WHERE receipt_id=$1 AND response->>'status'='identified' AND response_object_id IS NULL`, receipt).Scan(&decision, &machine, &unitsBefore))
	if machine == "" || unitsBefore != 0 {
		t.Fatal("partially staged identification became selected", unitsBefore)
	}
	// Each stage must persist the failed technical attempt, apply the original
	// backoff and retain its fence before the next legitimate owner can retry.
	waitForRetry := func(stage, table, id, expectedState string) {
		t.Helper()
		var state, reason string
		var count, fence int
		var remaining float64
		var deferred, failedWithoutObject bool
		must(t, f.env.Pool.QueryRow(ctx, `SELECT x.state,x.error_code,x.attempts,x.fence,
			greatest(extract(epoch FROM (x.available_at-clock_timestamp())),0)::float8,
			x.available_at>=a.finished_at+interval '900ms',a.state='failed' AND a.error_code='storage_unavailable' AND a.response_object_id IS NULL
			FROM `+table+` x JOIN ingestion_attempts a ON a.target_id=x.id AND a.stage=$2 AND a.attempt=1 WHERE x.id=$1`, id, stage).
			Scan(&state, &reason, &count, &fence, &remaining, &deferred, &failedWithoutObject))
		if state != expectedState || reason != "storage_unavailable" || count != 1 || fence != 1 || !deferred || !failedWithoutObject {
			t.Fatalf("%s retry evidence: state=%s reason=%s attempts=%d fence=%d backoff=%v failed=%v", stage, state, reason, count, fence, deferred, failedWithoutObject)
		}
		time.Sleep(time.Duration(remaining*float64(time.Second)) + 25*time.Millisecond)
	}
	waitForRetry("identify", "ingestion_identity_decisions", decision, "retry_wait")
	_, err = f.s.IdentifyReceipt(ctx, f.tenant, receipt, 1)
	must(t, err)
	var parent string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT u.id::text FROM ingestion_units u JOIN ingestion_identity_decisions d ON d.id=u.decision_id
		WHERE u.receipt_id=$1 AND d.state='identified' AND d.fence=2 AND d.response_object_id IS NOT NULL AND d.response->>'execution_id'=$2`, receipt, machine).Scan(&parent))
	objects, err := blobstore.Open(fixtureObjectEndpoint(), "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, err)
	f.s.WithStorage(failIngestionPut{objects})
	_, err = f.s.PlanUnit(ctx, f.tenant, parent, 1)
	must(t, err)
	waitForRetry("demultiplex", "ingestion_units", parent, "demultiplex_retry")
	f.s.WithStorage(objects)
	_, err = f.s.PlanUnit(ctx, f.tenant, parent, 1)
	must(t, err)
	for range devices {
		_, err = f.s.PlanOne(ctx)
		must(t, err)
	}
	var plan string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_parse_plans ORDER BY id LIMIT 1`).Scan(&plan))
	f.s.WithStorage(failIngestionPut{objects})
	_, err = f.s.ParsePlan(ctx, f.tenant, plan, 1)
	must(t, err)
	waitForRetry("parse", "ingestion_parse_plans", plan, "retry_wait")
	f.s.WithStorage(objects)
	_, err = f.s.ParsePlan(ctx, f.tenant, plan, 1)
	must(t, err)
	f.work(t, 6)
	var recovered, demultiplexed, results, observations int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ingestion_attempts WHERE target_id=ANY($1::uuid[]) AND attempt=2 AND fence=2 AND state='succeeded' AND response_object_id IS NOT NULL),
		(SELECT count(*) FROM ingestion_demultiplex_decisions WHERE unit_id=$2::uuid),
		(SELECT count(*) FROM ingestion_results),(SELECT count(*) FROM monitoring_publications)`, []string{decision, parent, plan}, parent).
		Scan(&recovered, &demultiplexed, &results, &observations))
	if recovered != 3 || demultiplexed != 1 || results != 2 || observations != 0 {
		t.Fatalf("retry reconciliation: recovered=%d demultiplexed=%d results=%d observations=%d", recovered, demultiplexed, results, observations)
	}
	for _, device := range devices {
		var unit string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE device_id=$1 AND state='parsed'`, device).Scan(&unit))
		entries, _, err := f.s.Results(ctx, f.owner, devicecatalog.TenantScope(f.tenant), unit, "")
		must(t, err)
		if len(entries) != 1 {
			t.Fatalf("selected results for %s: %d", unit, len(entries))
		}
	}
	t.Log("actual PostgreSQL statement timeout and two object-store failures retried with unchanged identifiers, three new fences, and two owner-readable results")
}
