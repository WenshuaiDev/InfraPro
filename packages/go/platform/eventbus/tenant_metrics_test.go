package eventbus

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantQueueSamplesCurrentEpoch(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, err := Operate(ctx, p, Operation{Action: "enable", Reason: "tenant diagnostics fixture"})
	must(t, err)
	const total = diagnosticreport.MaxQueueDimensions + 3
	tenants := make([]string, total)
	for i := range tenants {
		tenants[i] = uuid.NewString()
	}
	sort.Strings(tenants)
	put := func(scope, state, epoch string, n int) {
		t.Helper()
		execSQL(t, p, `INSERT INTO event_outbox(producer,type,business_effect_key,restore_epoch,scope,aggregate_id,generation_or_position,state)
		 SELECT 'tenant-queue-test','notification.intent.ready',gen_random_uuid()::text,$1,$2,gen_random_uuid(),1,$3 FROM generate_series(1,$4)`,
			epoch, scope, state, n)
	}
	for i, id := range tenants {
		put(id, "pending", st.Epoch, i+1)
		put(id, "leased", st.Epoch, 1)
		put(id, "sent", st.Epoch, 2)
	}
	put("platform", "pending", st.Epoch, 3)
	put(tenants[0], "pending", uuid.NewString(), 4)
	// Published rows dominate the retained log. Verify the normal planner
	// can answer the sample against the smaller pending-scope index.
	put("platform", "sent", st.Epoch, 8192)
	execSQL(t, p, "ANALYZE event_outbox")
	var plan string
	must(t, p.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+tenantPressureSQL, st.Epoch, "", diagnosticreport.MaxQueueDimensions).Scan(&plan))
	require(t, strings.Contains(plan, "event_outbox_tenant_pressure"), "tenant sample failed to use its partial index")
	pc := p.Config()
	pc.MaxConns = 1
	limited, err := pgxpool.NewWithConfig(ctx, pc)
	must(t, err)
	t.Cleanup(limited.Close)
	sampled := 0
	after := ""
	for page, size := range []int{diagnosticreport.MaxQueueDimensions, 3} {
		depths, next, err := tenantDepths(ctx, limited, st.Epoch, after)
		must(t, err)
		require(t, len(depths) == size, "unexpected tenant page size")
		for _, id := range tenants[sampled : sampled+size] {
			require(t, depths[id] == int64(sampled+2), fmt.Sprintf("wrong tenant depth at position %d", sampled))
			sampled++
		}
		if page == 0 {
			require(t, next == tenants[sampled-1], "lost next scope cursor")
		}
		after = next
	}
	require(t, sampled == total && after == "", "sample did not wrap at end of queue")
	execSQL(t, p, "UPDATE event_protocol SET frozen=true WHERE id=1")
	depths, _, err := tenantDepths(ctx, limited, st.Epoch, "")
	must(t, err)
	require(t, len(depths) == 0, "frozen epoch exposed tenant metrics")
	evidence(t, "tenant-queue", map[string]any{"sampled_tenants": total, "page_size": diagnosticreport.MaxQueueDimensions, "pg_connections": 1, "excludes_platform_sent_and_stale_epochs": true, "freeze_clears": true, "indexed_plan": "event_outbox_tenant_pressure", "retained_platform_rows": 8192})
}
