package resources_test

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"testing"
)

// These assertions run after real storage publication/worker paths, including
// their failure injection. They do not call the notification producer directly.
func verifyTerminalNotifications(t *testing.T, env *testsupport.Environment) {
	t.Helper()
	if t.Failed() {
		return
	}
	for _, tc := range []struct{ name, query string }{
		{"monitoring", `SELECT count(*) FROM monitoring_jobs j WHERE actor_id IS NOT NULL AND kind IN ('validate','publish','export','derive','analysis','analysis_compare','derived_replay','analysis_export','derived_evidence_export') AND state IN ('succeeded','failed','needs_action','cancelled') AND NOT EXISTS(SELECT 1 FROM notification_events e WHERE e.source_module='monitoring' AND e.source_key=j.id::text||':'||j.notification_generation::text||':'||CASE WHEN j.state='succeeded' AND j.kind='validate' THEN CASE WHEN EXISTS(SELECT 1 FROM notification_events ve WHERE ve.source_module='monitoring' AND ve.source_key=j.id::text||':'||j.notification_generation::text||':needs_action') THEN 'needs_action' ELSE 'validated' END WHEN j.state='succeeded' AND j.kind='publish' THEN 'adopted' ELSE j.state END)`},
		{"spatial", `SELECT count(*) FROM spatial_processing_jobs j WHERE state IN ('succeeded','failed','canceled') AND NOT EXISTS(SELECT 1 FROM notification_events e WHERE e.source_module='spatial' AND e.source_key=j.id::text||':'||j.notification_generation::text||':'||j.state)`},
		{"cancellation reason", `SELECT count(*) FROM notification_events WHERE event_type IN ('task.result','spatial.result') AND outcome IN ('cancelled','canceled') AND reason_code=''`},
	} {
		var missing int
		if e := env.Pool.QueryRow(ctx, tc.query).Scan(&missing); e != nil {
			t.Error(e)
		} else if missing != 0 {
			t.Errorf("%s terminal transactions missing notification facts: %d", tc.name, missing)
		}
	}
}
