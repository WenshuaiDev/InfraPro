package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/dispatch"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
)

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func require(t *testing.T, ok bool, reason string) {
	t.Helper()
	if !ok {
		t.Fatal(reason)
	}
}
func fixture(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("INFRAPRO_ARCHITECTURE_EVENTS_TEST") == "" {
		t.Skip("run node scripts/test-architecture-events.mjs")
	}
	u, err := url.Parse(dsn)
	must(t, err)
	require(t, strings.HasSuffix(u.Path, "_test"), "requires isolated test database")
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	must(t, err)
	schema := "arc_events_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	must(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	must(t, migrations.Up(ctx, u.String()))
	pool, err := pgxpool.New(ctx, u.String())
	must(t, err)
	t.Cleanup(pool.Close)
	return pool
}
func notification(ctx context.Context, tx pgx.Tx, key string) error {
	_, e := tx.Exec(ctx, `INSERT INTO notification_events(id,source_module,source_key,event_type,schema_version,scope,target_kind,outcome,occurred_at,fingerprint,recipient_count)
 VALUES($1::uuid,'event-contract',$1::uuid::text,'test',1,'platform','test','succeeded',clock_timestamp(),'fixture',0)`, key)
	return e
}
func eventFor(t *testing.T, p *pgxpool.Pool, id string) Event {
	t.Helper()
	var e Event
	must(t, p.QueryRow(context.Background(), `SELECT `+columns+` FROM event_outbox o JOIN event_protocol ep USING(restore_epoch) WHERE aggregate_id=$1`, id).Scan(e.targets()...))
	return e
}
func execSQL(t *testing.T, p *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, e := p.Exec(context.Background(), sql, args...)
	must(t, e)
}
func evidence(t *testing.T, name string, record any) {
	t.Helper()
	dir := os.Getenv("INFRAPRO_EVENT_TEST_EVIDENCE")
	if dir == "" {
		return
	}
	must(t, os.MkdirAll(dir, 0700))
	bytes, err := json.MarshalIndent(record, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(dir, name+".json"), bytes, 0600))
}

type node struct {
	addr, cluster, status int
	file                  string
	cmd                   *exec.Cmd
	log                   *os.File
}

func port(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	must(t, e)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func cluster(t *testing.T, size int, env string) ([]*node, Config) {
	t.Helper()
	bin := os.Getenv("INFRAPRO_EVENT_TEST_SERVER")
	require(t, filepath.IsAbs(bin), "pinned offline nats-server binary is required")
	root := t.TempDir()
	nodes := make([]*node, size)
	routes := []string{}
	urls := []string{}
	used := map[int]bool{}
	unique := func() int {
		for {
			v := port(t)
			if !used[v] {
				used[v] = true
				return v
			}
		}
	}
	for i := range nodes {
		n := &node{addr: unique(), cluster: unique(), status: unique(), file: filepath.Join(root, fmt.Sprintf("node-%d.conf", i))}
		nodes[i] = n
		routes = append(routes, fmt.Sprintf("nats://127.0.0.1:%d", n.cluster))
		urls = append(urls, fmt.Sprintf("nats://127.0.0.1:%d", n.addr))
	}
	for i, n := range nodes {
		txt := fmt.Sprintf("server_name: node%d\nlisten: 127.0.0.1:%d\nhttp: 127.0.0.1:%d\njetstream {store_dir: %q, max_file_store: 128MB, max_memory_store: 16MB}\n", i, n.addr, n.status, filepath.Join(root, fmt.Sprintf("store-%d", i)))
		if size > 1 {
			list, _ := json.Marshal(routes)
			txt += fmt.Sprintf("cluster {name: arc, listen: 127.0.0.1:%d, routes: %s}\n", n.cluster, list)
		}
		must(t, os.WriteFile(n.file, []byte(txt), 0600))
		n.start(t)
		t.Cleanup(func() { n.stop() })
	}
	c := Defaults()
	c.Environment = env
	c.NATSURL = strings.Join(urls, ",")
	c.LoopbackTest = true
	c.Replicas = size
	c.Batch = 8
	c.MaxAckPending = 8
	c.MaxDeliver = 3
	c.AckWaitMS = 800
	c.BackoffMS = 100
	c.FallbackScanMS = 300
	c.DedupSeconds = 1
	c.IngestionBytes = 1 << 20
	c.MonitoringBytes = 1 << 20
	c.AuxiliaryBytes = 1 << 20
	return nodes, c
}
func (n *node) start(t *testing.T) {
	t.Helper()
	log, e := os.OpenFile(n.file+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	must(t, e)
	n.log = log
	n.cmd = exec.Command(os.Getenv("INFRAPRO_EVENT_TEST_SERVER"), "-c", n.file)
	n.cmd.Stdout = log
	n.cmd.Stderr = log
	must(t, n.cmd.Start())
}
func (n *node) stop() {
	if n.cmd == nil {
		return
	}
	_ = n.cmd.Process.Kill()
	_ = n.cmd.Wait()
	n.cmd = nil
	_ = n.log.Close()
}
func wait(t *testing.T, limit time.Duration, f func() bool, reason string) {
	t.Helper()
	until := time.Now().Add(limit)
	for !f() {
		if time.Now().After(until) {
			t.Fatalf("timed out: %s", reason)
		}
		time.Sleep(80 * time.Millisecond)
	}
}
func provisionForTest(t *testing.T, c Config, epoch string) *broker {
	t.Helper()
	var b *broker
	wait(t, 30*time.Second, func() bool {
		var e error
		if b == nil {
			b, e = connect(c, epoch, "admin")
			if e != nil {
				return false
			}
		}
		ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		return b.provision(ctx) == nil
	}, "real streams and durable consumers")
	t.Cleanup(b.nc.Close)
	return b
}
func next(t *testing.T, b *broker, consumer string) jetstream.Msg {
	t.Helper()
	ctx, done := context.WithTimeout(context.Background(), 4*time.Second)
	defer done()
	con, e := b.js.Consumer(ctx, b.stream(family(consumerTopics[consumer])), consumer)
	must(t, e)
	msg, e := con.Next(jetstream.FetchMaxWait(3 * time.Second))
	must(t, e)
	return msg
}
func markPublication(ctx context.Context, p *pgxpool.Pool, c Config, v publication, ack *jetstream.PubAck) error {
	_, err := markPublications(ctx, p, c, []published{{v, ack}})
	return err
}

func TestIdentifyHandoffResumesAfterLostExecutor(t *testing.T) {
	checkMachineHandoff(t, "identify", "ingest-custody-worker")
}

func TestParseHandoffResumesAfterLostExecutor(t *testing.T) {
	checkMachineHandoff(t, "parse", "parsing-dispatch-worker")
}

func checkMachineHandoff(t *testing.T, consumer, producer string) {
	t.Helper()
	p := fixture(t)
	ctx := context.Background()
	state, err := Operate(ctx, p, Operation{Action: "enable", Reason: "verify a keyed machine work handoff"})
	must(t, err)
	source := uuid.NewString()
	// A protocol fixture can exercise the same immutable identifiers as
	// ingestion without calling a machine parser or adding formal observations.
	execSQL(t, p, `WITH new_event AS (
		INSERT INTO event_outbox(producer,type,business_effect_key,restore_epoch,scope,aggregate_id,generation_or_position)
		VALUES ($3,$4,$1::uuid::text,$2,'platform',$1,1)
		RETURNING event_id,restore_epoch)
		INSERT INTO event_work(consumer,restore_epoch,event_id) SELECT $5,restore_epoch,event_id FROM new_event`, source, state.Epoch, producer, consumerTopics[consumer], consumer)
	fact := eventFor(t, p, source)
	_, cfg := cluster(t, 1, "arc"+consumer)
	cfg.AckWaitMS = 5000
	cfg.BackoffMS = 5000
	b := provisionForTest(t, cfg, state.Epoch)
	raw, err := json.Marshal(fact)
	must(t, err)
	deliver := func() handoff {
		_, err := b.js.Publish(ctx, b.subject(fact.Type), raw)
		must(t, err)
		h, err := receiveDelivery(ctx, p, b, consumer, next(t, b, consumer), true)
		must(t, err)
		return h
	}
	first := deliver()
	require(t, first.leased != nil && first.leased.fence > 0 && first.key == "", "broker did not claim the exact referenced work")
	replay := deliver()
	require(t, replay.leased == nil && !replay.quarantined, "duplicate granted a second active executor")
	current, err := claimWork(ctx, p, cfg, consumer, state.Epoch, fact.ID)
	must(t, err)
	require(t, current == nil, "PG took live broker work")
	must(t, finishWork(ctx, p, cfg, *first.leased, ErrDeferred))
	require(t, deliver().leased == nil, "duplicate bypassed the machine retry deadline")
	execSQL(t, p, `UPDATE event_work SET available_at=clock_timestamp()-interval '1 second' WHERE consumer=$1 AND event_id=$2`, consumer, fact.ID)
	retry := deliver()
	require(t, retry.leased != nil && retry.leased.fence > first.leased.fence && retry.leased.attempt == first.leased.attempt, "due machine retry failed to reuse its attempt budget")
	execSQL(t, p, `UPDATE event_work SET lease_until=clock_timestamp()-interval '1 second' WHERE consumer=$1 AND event_id=$2`, consumer, fact.ID)
	recovered, err := claimWork(ctx, p, cfg, consumer, state.Epoch, fact.ID)
	must(t, err)
	require(t, recovered != nil && recovered.fence > retry.leased.fence, "PG failed to recover the acknowledged executor")
	require(t, errors.Is(finishWork(ctx, p, cfg, *retry.leased, nil), dispatch.ErrRetired), "expired broker grant could still finish")
	must(t, finishWork(ctx, p, cfg, *recovered, nil))
	require(t, deliver().leased == nil, "terminal fact received a new grant")
	var via, workState string
	var duplicates int
	must(t, p.QueryRow(ctx, `SELECT c.via,c.duplicates,w.state FROM event_consumptions c JOIN event_work w
		USING(consumer,restore_epoch,event_id) WHERE c.consumer=$1 AND c.event_id=$2`, consumer, fact.ID).Scan(&via, &duplicates, &workState))
	require(t, via == "broker" && duplicates == 4 && workState == "done", "durable receipt and execution disagree")
	evidence(t, consumer+"-direct", map[string]any{"event_id": fact.ID, "receipt": via, "duplicates": duplicates, "first_fence": first.leased.fence, "retry_fence": retry.leased.fence, "recovered_fence": recovered.fence, "state": workState})
}

func TestTransactionalOutboxFaults(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, e := Operate(ctx, p, Operation{Action: "enable", Reason: "isolated test"})
	must(t, e)
	var rolled = errors.New("simulated source rollback")
	id := uuid.NewString()
	e = pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if e := notification(ctx, tx, id); e != nil {
			return e
		}
		return rolled
	})
	require(t, errors.Is(e, rolled), "failed to exercise transaction rollback")
	var count int
	must(t, p.QueryRow(ctx, `SELECT count(*) FROM event_outbox`).Scan(&count))
	require(t, count == 0, "rolled back fact leaked")
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
	original := eventFor(t, p, id)
	_, c := cluster(t, 1, "arc1")
	b := provisionForTest(t, c, st.Epoch)
	first, e := claimPublications(ctx, p, c, st.Epoch)
	must(t, e)
	require(t, len(first) == 1, "expected one outbox lease")
	ack, e := publish(ctx, b, first[0])
	must(t, e)
	require(t, !ack.Duplicate, "first ack unexpectedly duplicate")
	// Stop exactly after PubAck. No PG state mark is made; the expired lease is
	// reclaimed and a stale writer cannot mark it sent.
	execSQL(t, p, `UPDATE event_outbox SET lease_until=clock_timestamp()-interval '1 second' WHERE event_id=$1`, original.ID)
	second, e := claimPublications(ctx, p, c, st.Epoch)
	must(t, e)
	require(t, len(second) == 1, "recovery did not reclaim")
	require(t, errors.Is(markPublication(ctx, p, c, first[0], ack), errEpoch), "stale relay fence accepted")
	ack2, e := publish(ctx, b, second[0])
	must(t, e)
	must(t, markPublication(ctx, p, c, second[0], ack2))
	m := next(t, b, "notification")
	// Terminate the receiver after PG commit, before any ACK.
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		_, err := accept(ctx, tx, "notification", original, "broker")
		return err
	}))
	_ = m
	redelivery := next(t, b, "notification")
	meta, e := redelivery.Metadata()
	must(t, e)
	require(t, meta.NumDelivered >= 2, "missing redelivery")
	key, e := receive(ctx, p, b, "notification", redelivery)
	must(t, e)
	require(t, key.key == original.ID, "redelivery lost identity")
	job, e := claimWork(ctx, p, c, "notification", st.Epoch, key.key)
	must(t, e)
	require(t, job != nil, "missing durable job")
	must(t, finishWork(ctx, p, c, *job, nil))
	time.Sleep(1200 * time.Millisecond)
	late, e := publish(ctx, b, second[0])
	must(t, e)
	require(t, !late.Duplicate && late.Sequence > ack.Sequence, "late duplicate stayed inside broker dedup window")
	key, e = receive(ctx, p, b, "notification", next(t, b, "notification"))
	must(t, e)
	require(t, key.key == "" && !key.quarantined, "completed work got a second execution hint")
	none, e := claimWork(ctx, p, c, "notification", st.Epoch, original.ID)
	must(t, e)
	require(t, none == nil, "late duplicate revived terminal effect")
	var received, doneRows int
	var replays int64
	must(t, p.QueryRow(ctx, `SELECT count(*),sum(duplicates) FROM event_consumptions WHERE event_id=$1`, original.ID).Scan(&received, &replays))
	must(t, p.QueryRow(ctx, `SELECT count(*) FROM event_work WHERE event_id=$1 AND state='done'`, original.ID).Scan(&doneRows))
	require(t, received == 1 && doneRows == 1 && replays >= 2, "duplicate effect accounting")
	// Unsupported schema is durably isolated. Redrive reads the original PG
	// evidence, retaining both the quarantine record and the operational receipt.
	badID := uuid.NewString()
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, badID) }))
	bad := eventFor(t, p, badID)
	bad.Version = 2
	body, e := json.Marshal(bad)
	must(t, e)
	_, e = b.js.Publish(ctx, b.subject(bad.Type), body)
	must(t, e)
	key, e = receive(ctx, p, b, "notification", next(t, b, "notification"))
	must(t, e)
	require(t, key.key == "" && key.quarantined, "unknown version was dispatched")
	var reason string
	must(t, p.QueryRow(ctx, `SELECT reason FROM event_quarantine WHERE event_id=$1 AND state='open'`, bad.ID).Scan(&reason))
	require(t, reason == "unsupported_version", "wrong quarantine reason")
	_, e = Operate(ctx, p, Operation{Action: "redrive", Reason: "canonical v1 fact verified", Consumer: "notification", ID: bad.ID})
	must(t, e)
	job, e = claimWork(ctx, p, c, "notification", st.Epoch, bad.ID)
	must(t, e)
	require(t, job != nil && job.Version == 1, "redrive did not use stored fact")
	must(t, finishWork(ctx, p, c, *job, nil))
	evidence(t, "transaction-boundaries", map[string]any{"event_id": original.ID, "source_rollback_count": 0, "puback_sequence": ack.Sequence, "replacement_fence": second[0].fence, "ack_redeliveries": meta.NumDelivered, "late_sequence": late.Sequence, "consumptions": received, "completed": doneRows, "duplicates": replays, "quarantine": reason, "redriven_id": bad.ID})
}

func TestEpochAndDispatchFences(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, e := Operate(ctx, p, Operation{Action: "enable", Reason: "isolated fence test"})
	must(t, e)
	// Preserve a leased context to model a delayed parser result crossing cutover.
	var old context.Context
	ready := make(chan struct{})
	resume := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, e := dispatch.Try(ctx, p, "parsing-dispatch-worker", "compat", func(leased context.Context) error {
			old = leased
			close(ready)
			<-resume
			return pgx.BeginFunc(leased, p, func(tx pgx.Tx) error { return dispatch.CheckTx(leased, tx) })
		})
		result <- e
	}()
	select {
	case <-ready:
	case e := <-result:
		t.Fatalf("compat did not receive lease: %v", e)
	case <-time.After(3 * time.Second):
		t.Fatal("compat lease timeout")
	}
	_, e = Operate(ctx, p, Operation{Action: "switch", Role: "parsing-dispatch-worker", Mode: "split", Expected: 1, Reason: "cutover test"})
	must(t, e)
	close(resume)
	require(t, errors.Is(<-result, dispatch.ErrRetired), "old owner committed")
	ok, e := dispatch.Try(ctx, p, "parsing-dispatch-worker", "split", func(leased context.Context) error {
		return pgx.BeginFunc(leased, p, func(tx pgx.Tx) error { return dispatch.CheckTx(leased, tx) })
	})
	must(t, e)
	require(t, ok, "new generation unavailable")
	ok, e = dispatch.Try(ctx, p, "parsing-dispatch-worker", "compat", func(context.Context) error { return errors.New("wrong role acquired") })
	must(t, e)
	require(t, !ok, "compat still owns role")
	id := uuid.NewString()
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
	before := eventFor(t, p, id)
	_, e = Operate(ctx, p, Operation{Action: "freeze", Reason: "recorded restore window"})
	must(t, e)
	execSQL(t, p, `UPDATE audit_protocol SET restore_epoch=gen_random_uuid() WHERE id=1`)
	_, e = Operate(ctx, p, Operation{Action: "resume", Epoch: st.Epoch, Reason: "attempt to resume restored old epoch"})
	require(t, e != nil, "restored data resumed in the retired namespace")
	rotated, e := Operate(ctx, p, Operation{Action: "rotate-epoch", Epoch: st.Epoch, Reason: "isolated restore"})
	must(t, e)
	require(t, rotated.Frozen && rotated.Epoch != st.Epoch, "rotation did not preserve guard")
	after := eventFor(t, p, id)
	require(t, after.ID != before.ID && after.Cause == before.ID, "unfinished fact not rebound")
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if !errors.Is(dispatch.CheckTx(context.WithoutCancel(old), tx), dispatch.ErrRetired) {
			return errors.New("old ticket valid after restore")
		}
		return nil
	}))
	evidence(t, "epoch-fence", map[string]any{"old_epoch": st.Epoch, "restored_epoch": rotated.Epoch, "old_event": before.ID, "recreated_from": after.Cause, "guard_frozen": rotated.Frozen, "resume_before_rotation_rejected": true, "fence_rejected": true})
}

func TestJetStreamQuorumAndCapacity(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, e := Operate(ctx, p, Operation{Action: "enable", Reason: "isolated quorum test"})
	must(t, e)
	nodes, c := cluster(t, 3, "arc3")
	b := provisionForTest(t, c, st.Epoch)
	hc := &http.Client{Timeout: 500 * time.Millisecond}
	for _, n := range nodes {
		wait(t, 15*time.Second, func() bool {
			res, e := hc.Get("http://127.0.0.1:" + strconv.Itoa(n.status) + "/healthz?js-enabled-only=true")
			if e != nil {
				return false
			}
			defer res.Body.Close()
			return res.StatusCode == 200
		}, "all replicas healthy")
	}
	makeFact := func() publication {
		id := uuid.NewString()
		must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
		facts, e := claimPublications(ctx, p, c, st.Epoch)
		must(t, e)
		require(t, len(facts) == 1, "expected one new fact")
		return facts[0]
	}
	n1 := makeFact()
	a1, e := publish(ctx, b, n1)
	must(t, e)
	must(t, markPublication(ctx, p, c, n1, a1))
	nodes[2].stop()
	n2 := makeFact()
	var a2 *jetstream.PubAck
	wait(t, 15*time.Second, func() bool { a2, e = publish(ctx, b, n2); return e == nil }, "two replicas should still confirm")
	must(t, markPublication(ctx, p, c, n2, a2))
	nodes[1].stop()
	n3 := makeFact()
	a3, e := publish(ctx, b, n3)
	require(t, e != nil && a3 == nil, "lost majority acknowledged a write")
	must(t, markPublication(ctx, p, c, n3, nil))
	var state string
	must(t, p.QueryRow(ctx, `SELECT state FROM event_outbox WHERE event_id=$1`, n3.ID).Scan(&state))
	require(t, state == "pending", "no-quorum fact not retained")
	nodes[1].start(t)
	nodes[2].start(t)
	var a4 *jetstream.PubAck
	var j publication
	wait(t, 25*time.Second, func() bool {
		batch, err := claimPublications(ctx, p, c, st.Epoch)
		if err != nil || len(batch) == 0 {
			return false
		}
		j = batch[0]
		a4, err = publish(ctx, b, j)
		if err != nil {
			_ = markPublication(ctx, p, c, j, nil)
			return false
		}
		return true
	}, "new majority accepts retained fact")
	must(t, markPublication(ctx, p, c, j, a4))
	// Stream and durable elections finish independently. Use bounded pulls
	// through reconnects and require a handoff for each original source key.
	drain := func(facts ...publication) {
		remaining := make(map[string]bool, len(facts))
		for _, fact := range facts {
			remaining[fact.ID] = true
		}
		wait(t, 30*time.Second, func() bool {
			probe, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			consumer, err := b.js.Consumer(probe, b.stream("auxiliary"), "notification")
			if err != nil {
				return false
			}
			msg, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
			if err != nil {
				return false
			}
			if _, err = receive(ctx, p, b, "notification", msg); err != nil {
				return false
			}
			delete(remaining, msg.Headers().Get("Nats-Msg-Id"))
			return len(remaining) == 0
		}, "restored notification durable delivered every retained fact")
		for _, fact := range facts {
			work, err := claimWork(ctx, p, c, "notification", st.Epoch, fact.ID)
			must(t, err)
			require(t, work != nil && work.AggregateID == fact.AggregateID, "wrong notification intent")
			var recipients int
			must(t, p.QueryRow(ctx, "SELECT recipient_count FROM notification_events WHERE id=$1", work.AggregateID).Scan(&recipients))
			require(t, recipients == 0, "zero-recipient fixture has unexpected deliveries")
			must(t, finishWork(ctx, p, c, *work, nil))
		}
	}
	drain(n1, n2, n3)
	var total int
	must(t, p.QueryRow(ctx, `SELECT count(*) FROM event_consumptions WHERE consumer='notification'`).Scan(&total))
	require(t, total == 3, "missing durable handoffs")
	capTest := makeFact()
	stream, e := b.js.Stream(ctx, b.stream("auxiliary"))
	must(t, e)
	info, e := stream.Info(ctx)
	must(t, e)
	capCfg := info.Config
	capCfg.MaxMsgs = int64(info.State.Msgs)
	wait(t, 30*time.Second, func() bool {
		update, done := context.WithTimeout(ctx, 3*time.Second)
		defer done()
		_, err := b.js.UpdateStream(update, capCfg)
		return err == nil
	}, "stream configuration majority after recovery")
	capAck, capErr := publish(ctx, b, capTest)
	require(t, capAck == nil && capErr != nil, "DiscardNew accepted over capacity")
	must(t, markPublication(ctx, p, c, capTest, nil))
	capCfg.MaxMsgs = -1
	wait(t, 30*time.Second, func() bool {
		update, done := context.WithTimeout(ctx, 3*time.Second)
		defer done()
		_, err := b.js.UpdateStream(update, capCfg)
		return err == nil
	}, "stream configuration majority after recovery")
	wait(t, 4*time.Second, func() bool {
		batch, e := claimPublications(ctx, p, c, st.Epoch)
		if e != nil || len(batch) == 0 {
			return false
		}
		ack, e := publish(ctx, b, batch[0])
		if e != nil {
			return false
		}
		return markPublication(ctx, p, c, batch[0], ack) == nil
	}, "recover from full stream")
	drain(capTest)
	var completed int
	must(t, p.QueryRow(ctx, "SELECT count(*) FROM event_work WHERE consumer='notification' AND state='done'").Scan(&completed))
	require(t, completed == 4, "accepted notification intents did not all reach terminal state")
	evidence(t, "quorum", map[string]any{"replicas": 3, "healthy_sequence": a1.Sequence, "one_node_down_sequence": a2.Sequence, "majority_lost_state": state, "recovered_sequence": a4.Sequence, "completed_handoffs": total, "completed_business_keys": completed, "discard_new_refused": capErr != nil})
}
