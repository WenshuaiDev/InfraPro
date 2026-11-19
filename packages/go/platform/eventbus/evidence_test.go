package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/dispatch"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func repo(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("INFRAPRO_TEST_REPO_ROOT"); r != "" {
		return r
	}
	dir, err := filepath.Abs("../../../..")
	must(t, err)
	return dir
}

func TestVersionedWireContract(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile(filepath.Join(repo(t), "contracts/events/v1.schema.json"))
	must(t, err)
	for _, topic := range consumerTopics {
		e := Event{ID: uuid.NewString(), Epoch: uuid.NewString(), Scope: "platform", Type: topic, Version: 1, OccurredAt: time.Now().UTC(), AggregateID: uuid.NewString()}
		data, err := json.Marshal(e)
		must(t, err)
		var rendered map[string]any
		must(t, json.Unmarshal(data, &rendered))
		must(t, schema.Validate(rendered))
		v, why := decode(data, e.Epoch, topic)
		require(t, why == "" && equal(v, e), "versioned producer and consumer differ")
		for _, field := range []string{"generation_or_position", "scope", "event_id", "occurred_at", "restore_epoch", "aggregate_id", "type", "version"} {
			saved := rendered[field]
			delete(rendered, field)
			data, err = json.Marshal(rendered)
			must(t, err)
			_, why = decode(data, e.Epoch, topic)
			require(t, why != "" && schema.Validate(rendered) != nil, "accepted missing field "+field)
			rendered[field] = saved
		}
		rendered["body"] = "test"
		data, err = json.Marshal(rendered)
		must(t, err)
		_, why = decode(data, e.Epoch, topic)
		require(t, why != "" && schema.Validate(rendered) != nil, "unexpected payload allowed")
		_, why = decode([]byte(strings.Repeat(" ", MaxBytes+1)), e.Epoch, topic)
		require(t, why == "bad_message", "unbounded envelope allowed")
	}
}

func TestFinalDeliverySupervision(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, err := Operate(ctx, p, Operation{Action: "enable", Reason: "last delivery crash"})
	must(t, err)
	_, cfg := cluster(t, 1, "arclast")
	cfg.MaxDeliver = 2
	cfg.AckWaitMS = 150
	b := provisionForTest(t, cfg, st.Epoch)
	id := uuid.NewString()
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
	facts, err := claimPublications(ctx, p, cfg, st.Epoch)
	must(t, err)
	require(t, len(facts) == 1, "one publication expected")
	ack, err := publish(ctx, b, facts[0])
	must(t, err)
	must(t, markPublication(ctx, p, cfg, facts[0], ack))
	// Start neither supervisor nor PG fallback. The final receiver exits
	// before handing the event to PG or acknowledging to JetStream.
	for attempt := 1; attempt <= cfg.MaxDeliver; attempt++ {
		m := next(t, b, "notification")
		meta, err := m.Metadata()
		must(t, err)
		require(t, int(meta.NumDelivered) == attempt, "unexpected delivery number")
	}
	stream := b.stream("auxiliary")
	con, err := b.js.Consumer(ctx, stream, "notification")
	must(t, err)
	// A new pull causes the broker to observe exhaustion across the expired lease.
	_, _ = con.Next(jetstream.FetchMaxWait(500 * time.Millisecond))
	notices, err := b.js.Consumer(ctx, stream, "notification-advisories")
	must(t, err)
	m, err := notices.Next(jetstream.FetchMaxWait(4 * time.Second))
	must(t, err)
	changed, err := recordAdvisory(ctx, p, b, "notification", stream, m)
	must(t, err)
	require(t, changed, "crashed final delivery was not isolated")
	must(t, m.DoubleAck(ctx))
	var reason, state string
	var attempts int
	must(t, p.QueryRow(ctx, `SELECT q.reason,q.attempts,w.state FROM event_quarantine q JOIN event_work w USING(consumer,restore_epoch,event_id) WHERE q.event_id=$1`, facts[0].ID).Scan(&reason, &attempts, &state))
	require(t, reason == "delivery_exhausted" && attempts == 2 && state == "quarantined", "invalid exhaustion evidence")
	_, err = Operate(ctx, p, Operation{Action: "redrive", Reason: "recheck original domain fact", Consumer: "notification", ID: facts[0].ID})
	must(t, err)
	// The short broker deadline above provokes the real last-delivery notice.
	// Manual recovery uses the ordinary domain lease, including on a busy PG.
	cfg.AckWaitMS = Defaults().AckWaitMS
	job, err := claimWork(ctx, p, cfg, "notification", st.Epoch, facts[0].ID)
	must(t, err)
	require(t, job != nil, "redrive did not resume")
	must(t, finishWork(ctx, p, cfg, *job, nil))
	evidence(t, "exhausted-delivery", map[string]any{"restore_epoch": st.Epoch, "source_id": id, "event_id": facts[0].ID, "stream_sequence": ack.Sequence, "attempts": attempts, "reason": reason, "state_before_redrive": state, "recovered": true})
}

func asGroup(t *testing.T, p *pgxpool.Pool, name string) *pgxpool.Pool {
	t.Helper()
	conf := p.Config()
	conf.ConnConfig.RuntimeParams["role"] = name
	restricted, err := pgxpool.NewWithConfig(context.Background(), conf)
	must(t, err)
	t.Cleanup(restricted.Close)
	var who string
	must(t, restricted.QueryRow(context.Background(), "SELECT current_user").Scan(&who))
	require(t, who == name, "database did not switch to restricted identity")
	return restricted
}
func denied(t *testing.T, p *pgxpool.Pool, sql string) {
	t.Helper()
	_, err := p.Exec(context.Background(), sql)
	var dbErr *pgconn.PgError
	require(t, errors.As(err, &dbErr) && dbErr.Code == "42501", "expected PostgreSQL privilege rejection")
}
func TestRestrictedDatabaseRoles(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	rejections := 0
	permitPath := func() string {
		t.Helper()
		var actual, expected string
		must(t, p.QueryRow(ctx, `SELECT opt.option_value, format('pg_catalog, %I, pg_temp', n.nspname)
 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
 CROSS JOIN LATERAL pg_options_to_table(p.proconfig) opt
 WHERE p.oid='event_publication_permit(boolean)'::regprocedure AND p.prosecdef AND opt.option_name='search_path'`).Scan(&actual, &expected))
		require(t, actual == expected, "unexpected publication permit search_path: "+actual)
		return actual
	}
	migratedPermitPath := permitPath()
	// Existing schema 59 instances can retain the migration session's path.
	execSQL(t, p, "ALTER FUNCTION event_publication_permit(boolean) SET search_path FROM CURRENT")
	reject := func(group *pgxpool.Pool, sql string) {
		t.Helper()
		denied(t, group, sql)
		rejections++
	}
	for _, file := range []string{"deploy/architecture/postgres-roles.sql", "deploy/events/postgres-grants.sql"} {
		body, err := os.ReadFile(filepath.Join(repo(t), file))
		must(t, err)
		_, err = p.Exec(ctx, string(body), pgx.QueryExecModeSimpleProtocol)
		must(t, err)
	}
	reinstalledPermitPath := permitPath()
	admin := asGroup(t, p, "infrapro_event_admin")
	relay := asGroup(t, p, "infrapro_event_relay")
	worker := asGroup(t, p, "infrapro_notification_worker")
	parser := asGroup(t, p, "infrapro_parsing_dispatch")
	pub := asGroup(t, p, "infrapro_monitoring_publish")
	compat := asGroup(t, p, "infrapro_compat_api")
	oldMonitor := asGroup(t, p, "infrapro_compat_monitoring_worker")
	oldIngest := asGroup(t, p, "infrapro_compat_ingest_worker")
	scope := tenant(t, p)
	st, err := Operate(ctx, admin, Operation{Action: "enable", Reason: "role acceptance"})
	must(t, err)
	id := uuid.NewString()
	must(t, pgx.BeginFunc(ctx, worker, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
	_, err = change(ctx, p, scope, 1)
	must(t, err)
	cfg := Defaults()
	facts, err := claimPublications(ctx, relay, cfg, st.Epoch)
	must(t, err)
	require(t, len(facts) == 2, "restricted relay did not lease two source facts")
	for i, f := range facts {
		must(t, markPublication(ctx, relay, cfg, f, &jetstream.PubAck{Stream: "acl", Sequence: uint64(i + 1)}))
	}
	var n int
	must(t, worker.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE type='monitoring.changed'`).Scan(&n))
	require(t, n == 0, "consumer can read another domain's facts")
	must(t, worker.QueryRow(ctx, `SELECT count(*) FROM event_work WHERE consumer<>'notification'`).Scan(&n))
	require(t, n == 0, "consumer can read another queue")
	reject(relay, `INSERT INTO event_outbox DEFAULT VALUES`)
	reject(relay, `DELETE FROM event_outbox`)
	reject(worker, `UPDATE event_protocol SET frozen=true`)
	reject(parser, `UPDATE dispatch_ownership SET mode='split'`)
	reject(relay, `SELECT * FROM account_sessions`)
	authorized, err := dispatch.Try(ctx, parser, "parsing-dispatch-worker", "compat", func(c context.Context) error {
		return pgx.BeginFunc(c, parser, func(tx pgx.Tx) error { return dispatch.CheckTx(c, tx) })
	})
	must(t, err)
	require(t, authorized, "legitimate dispatch grant denied")
	authorized, err = dispatch.Try(ctx, pub, "parsing-dispatch-worker", "compat", func(context.Context) error { return errors.New("unauthorized role") })
	must(t, err)
	require(t, !authorized, "cross-domain dispatch grant leaked")
	reject(worker, "SELECT event_publication_permit(true)")
	execSQL(t, p, "UPDATE event_pressure SET storage_level='soft' WHERE family='monitoring'")
	var permitted bool
	must(t, pub.QueryRow(ctx, "SELECT event_publication_permit(true)").Scan(&permitted))
	require(t, permitted, "soft water level blocked the first formal publication")
	// Hold the already reserved interval open while checking another login.
	execSQL(t, p, "UPDATE monitoring_scheduler_leases SET next_run=clock_timestamp()+interval '1 minute' WHERE lane='event-formal-publication'")
	must(t, pub.QueryRow(ctx, "SELECT event_publication_permit(true)").Scan(&permitted))
	require(t, !permitted, "concurrent formal publication bypassed the soft water level")
	execSQL(t, p, "UPDATE event_pressure SET storage_level='hard' WHERE family='monitoring'")
	must(t, pub.QueryRow(ctx, "SELECT event_publication_permit(false)").Scan(&permitted))
	require(t, !permitted, "hard water level consumed a formal job attempt")
	execSQL(t, p, "UPDATE event_pressure SET storage_level='normal' WHERE family='monitoring'")
	must(t, pub.QueryRow(ctx, "SELECT event_publication_permit(true)").Scan(&permitted))
	require(t, permitted, "normal water level did not resume formal publications immediately")
	oneTrial := func(c context.Context) error {
		return pgx.BeginFunc(c, compat, func(tx pgx.Tx) error { return dispatch.CheckTx(c, tx) })
	}
	ok, err := dispatch.Try(ctx, compat, "monitoring-incremental-worker", "compat", oneTrial)
	must(t, err)
	require(t, ok, "compat API could not run a fenced inline trial")
	_, err = Operate(ctx, admin, Operation{Action: "switch", Role: "monitoring-incremental-worker", Mode: "split", Expected: 1, Reason: "cut off inline trials"})
	must(t, err)
	for _, mode := range []string{"compat", "split"} {
		ok, err = dispatch.Try(ctx, compat, "monitoring-incremental-worker", mode, oneTrial)
		must(t, err)
		require(t, !ok, "compat API acquired a retired or split trial lease")
	}
	manual := func(c context.Context) error {
		return pgx.BeginFunc(c, oldMonitor, func(tx pgx.Tx) error {
			if err := dispatch.CheckTx(c, tx); err != nil {
				return err
			}
			var slot int
			return tx.QueryRow(c, `UPDATE parsing_executor_slots SET scope='platform',execution_id=gen_random_uuid(),
				automatic=false,lease_until=clock_timestamp()+interval '30 seconds' WHERE slot=4 RETURNING slot`).Scan(&slot)
		})
	}
	ok, err = dispatch.Try(ctx, oldMonitor, "parsing-dispatch-worker", "compat", manual)
	must(t, err)
	require(t, ok, "old monitoring worker could not run its manual parsing lane")
	reject(oldMonitor, "SELECT * FROM ingestion_receipts")
	reject(oldMonitor, "UPDATE parsing_executor_slots SET automatic=true WHERE slot=4")
	execSQL(t, parser, "UPDATE parsing_executor_slots SET automatic=true,lease_until=clock_timestamp()+interval '30 seconds' WHERE slot=1")
	tag, err := oldMonitor.Exec(ctx, "UPDATE parsing_executor_slots SET execution_id=NULL WHERE slot=1")
	must(t, err)
	require(t, tag.RowsAffected() == 0, "manual monitoring login altered a live automatic slot")
	_, err = Operate(ctx, admin, Operation{Action: "switch", Role: "parsing-dispatch-worker", Mode: "split", Expected: 1, Reason: "verify retired compatibility workers"})
	must(t, err)
	for _, old := range []*pgxpool.Pool{oldMonitor, oldIngest} {
		for _, mode := range []string{"compat", "split"} {
			ok, err := dispatch.Try(ctx, old, "parsing-dispatch-worker", mode, func(context.Context) error { return nil })
			must(t, err)
			require(t, !ok, "old worker acquired parsing authority after switching to split")
		}
	}
	job, err := claimWork(ctx, worker, cfg, "notification", st.Epoch, eventFor(t, p, id).ID)
	must(t, err)
	require(t, job != nil, "restricted worker failed to claim domain fact")
	bound := dispatch.WithEventWork(ctx, "notification", st.Epoch, job.ID, job.fence)
	must(t, pgx.BeginFunc(bound, worker, func(tx pgx.Tx) error { return dispatch.CheckTx(bound, tx) }))
	must(t, finishWork(ctx, worker, cfg, *job, nil))
	err = pgx.BeginFunc(bound, worker, func(tx pgx.Tx) error { return dispatch.CheckTx(bound, tx) })
	require(t, errors.Is(err, dispatch.ErrRetired), "retired auxiliary work could commit")
	_, err = Prune(ctx, relay, time.Second, 32)
	must(t, err)
	_, err = Operate(ctx, admin, Operation{Action: "freeze", Reason: "verify operator freeze"})
	must(t, err)
	rotated, err := Operate(ctx, admin, Operation{Action: "rotate-epoch", Epoch: st.Epoch, Reason: "verify operator restore"})
	must(t, err)
	require(t, rotated.Epoch != st.Epoch, "operator could not rebind outstanding work")
	evidence(t, "sql-permissions", map[string]any{"role_count": 8, "relay_published": len(facts), "unauthorized_statements_rejected": rejections, "cross_role_lease": authorized, "formal_publication_soft_pacing": true, "formal_publication_hard_pause": true, "normal_publication_resumed": true, "retired_aux_work_rejected": true, "compat_inline_trial": true, "compat_trial_after_cutover_rejected": true, "compat_manual_parsing": true, "compat_automatic_slot_hidden": true, "compat_workers_rejected_after_split": true, "can_rotate_old_epoch": true, "publication_permit_search_path_after_migration": migratedPermitPath, "publication_permit_search_path_after_role_install": reinstalledPermitPath})
}

func TestAllStreamDiskWatermarks(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, err := Operate(ctx, p, Operation{Action: "enable", Reason: "measure independent file stream capacities"})
	must(t, err)
	_, cfg := cluster(t, 1, "arcdisk")
	cfg.IngestionBytes = 65536
	cfg.MonitoringBytes = 65536
	cfg.AuxiliaryBytes = 65536
	cfg.DiskSoftPercent = 40
	cfg.DiskHardPercent = 80
	b := provisionForTest(t, cfg, st.Epoch)
	var data []map[string]any
	for _, topic := range []string{"ingestion.receipt.stored", "monitoring.changed", "notification.intent.ready"} {
		f := family(topic)
		// Each pressure sample has the production reference format and size.
		// Only this isolated broker receives these unassigned synthetic keys.
		e := Event{Type: topic, Scope: "platform", Version: 1, Epoch: st.Epoch, AggregateID: uuid.NewString(), Generation: 1, OccurredAt: time.Now().UTC()}
		stream, err := b.js.Stream(ctx, b.stream(f))
		must(t, err)
		sawSoft, refused := false, false
		for i := 0; i < 512; i++ {
			e.ID = uuid.NewString()
			body, err := json.Marshal(e)
			must(t, err)
			_, err = b.js.Publish(ctx, b.subject(topic), body, jetstream.WithMsgID(e.ID))
			if err != nil {
				require(t, strings.Contains(err.Error(), "maximum") || strings.Contains(err.Error(), "limits"), "unexpected broker refusal")
				refused = true
				break
			}
			if i%8 == 0 && !sawSoft {
				must(t, sampleDisk(ctx, p, b))
				var level string
				must(t, p.QueryRow(ctx, `SELECT storage_level FROM event_pressure WHERE family=$1`, f).Scan(&level))
				sawSoft = level == "soft"
			}
		}
		require(t, refused && sawSoft, "stream neither slowed nor refused new messages")
		must(t, sampleDisk(ctx, p, b))
		var level string
		var used, limit int64
		must(t, p.QueryRow(ctx, `SELECT storage_level,broker_bytes,broker_limit_bytes FROM event_pressure WHERE family=$1`, f).Scan(&level, &used, &limit))
		require(t, level == "hard" && limit == 65536 && used <= limit && used >= limit*80/100, "unobserved hard disk watermark")
		if f == "monitoring" {
			scope := tenant(t, p)
			_, err = change(ctx, p, scope, 1)
			var pgErr *pgconn.PgError
			require(t, errors.As(err, &pgErr) && pgErr.Code == "53300", "hard disk did not backpressure formal publication")
		}
		// Drop the synthetic hints in this test volume to model expired retention.
		must(t, stream.Purge(ctx))
		must(t, sampleDisk(ctx, p, b))
		var recovered string
		must(t, p.QueryRow(ctx, `SELECT storage_level FROM event_pressure WHERE family=$1`, f).Scan(&recovered))
		require(t, recovered == "normal", "capacity did not recover")
		data = append(data, map[string]any{"family": f, "bytes": used, "max_bytes": limit, "soft_seen": sawSoft, "hard_seen": level == "hard", "discard_new": refused, "recovered": recovered})
	}
	evidence(t, "disk-watermarks", data)
}
