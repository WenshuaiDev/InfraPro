package parsing_test

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	ig "github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// An opaque TCP switch interrupts only the private handoff. The independently
// deployed relay retains both external mTLS listeners and its durable spool.
type handoffLink struct {
	net.Listener
	mu     sync.Mutex
	closed bool
	live   map[net.Conn]bool
}

func newHandoffLink(t *testing.T, upstream string) *handoffLink {
	t.Helper()
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	p := &handoffLink{Listener: ln, live: map[net.Conn]bool{}}
	go func() {
		for {
			front, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer front.Close()
				p.mu.Lock()
				if p.closed {
					p.mu.Unlock()
					return
				}
				p.live[front] = true
				p.mu.Unlock()
				defer func() { p.mu.Lock(); delete(p.live, front); p.mu.Unlock() }()
				back, e := net.DialTimeout("tcp", upstream, time.Second)
				if e != nil {
					return
				}
				defer back.Close()
				done := make(chan struct{})
				go func() { _, _ = io.Copy(back, front); _ = back.Close(); close(done) }()
				_, _ = io.Copy(front, back)
				_ = front.Close()
				<-done
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close(); p.offline(true) })
	return p
}
func (p *handoffLink) offline(off bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = off
	if off {
		for c := range p.live {
			_ = c.Close()
		}
	}
}
func archJSON(t *testing.T, file string, value any) {
	t.Helper()
	data, e := json.Marshal(value)
	must(t, e)
	must(t, os.WriteFile(file+".tmp", append(data, '\n'), 0600))
	must(t, os.Rename(file+".tmp", file))
}
func archWait(t *testing.T, d time.Duration, predicate func() bool, what string) {
	t.Helper()
	until := time.Now().Add(d)
	for !predicate() {
		if time.Now().After(until) {
			t.Fatalf("deadline waiting for %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func archHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

type archReceipt struct {
	LocalID, CloudID, Epoch, Source, Tenant, Project, ObjectSHA, EnvelopeSHA, State, Error, Ack string
	Sequence                                                                                    int64
	Received, Stored                                                                            time.Time
}
type archFrame struct {
	Plan, Unit, Result, Receipt, Tenant, Version, Status, Method, Source, UnitKey string
	Value, ValueUnit, OutputKey, Mapping, Quality                                 string
	Ready, FirstClaim, Complete, Visible                                          time.Time
	E2EMs, ClaimMs                                                                float64
}
type archCloud struct {
	CloudID, LocalID, Epoch, Source, SHA, EnvelopeSHA, ProducerKey, PayloadSHA string
	Sequence                                                                   int64
	Received, Confirmed                                                        time.Time
}
type archCount struct {
	Receipts, Results, PendingAcks, InFlight, Quarantine, Waiting, Locks, Connections int
	OldestSeconds                                                                     float64
}

func (f ingestionFixture) archCounts(t *testing.T, start time.Time, tag string) archCount {
	t.Helper()
	var out archCount
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ingestion_receipts WHERE received_at>=$1),
	(SELECT count(*) FROM ingestion_results z JOIN ingestion_parse_plans p ON p.id=z.plan_id JOIN ingestion_units u ON u.id=p.unit_id JOIN ingestion_receipts r ON r.id=u.receipt_id WHERE r.received_at>=$1),
	(SELECT count(*) FROM ingestion_ack_outbox WHERE state<>'acknowledged'),
	(SELECT count(*) FROM ingestion_receipts WHERE state IN ('stored','identifying','retry_wait'))+
	(SELECT count(*) FROM ingestion_units WHERE state IN ('identified','demultiplexing','demultiplex_retry'))+
	(SELECT count(*) FROM ingestion_parse_plans WHERE state IN ('queued','parsing','retry_wait')),
	(SELECT count(*) FROM ingestion_identity_decisions WHERE state IN ('rejected','failed')),
	(SELECT count(*) FROM pg_stat_activity WHERE application_name=$2 AND wait_event_type IS NOT NULL AND state='active'),
	(SELECT count(*) FROM pg_stat_activity WHERE application_name=$2 AND wait_event_type='Lock'),
	(SELECT count(*) FROM pg_stat_activity WHERE application_name=$2),
	COALESCE((SELECT extract(epoch FROM clock_timestamp()-min(at)) FROM (
	SELECT min(stored_at) at FROM ingestion_receipts WHERE state IN ('stored','identifying','retry_wait') UNION ALL SELECT min(created_at) FROM ingestion_units WHERE state IN ('identified','demultiplexing','demultiplex_retry') UNION ALL SELECT min(created_at) FROM ingestion_parse_plans WHERE state IN ('queued','parsing','retry_wait')) q),0)`, start, tag).Scan(&out.Receipts, &out.Results, &out.PendingAcks, &out.InFlight, &out.Quarantine, &out.Waiting, &out.Locks, &out.Connections, &out.OldestSeconds))
	return out
}

type archProducer struct {
	client       *http.Client
	connection   net.Conn
	httpURL      string
	frame, batch []byte
}

func (s archProducer) ddp(b []byte, ack byte) error {
	if e := s.connection.SetDeadline(time.Now().Add(15 * time.Second)); e != nil {
		return e
	}
	if _, e := s.connection.Write(b); e != nil {
		return e
	}
	res, e := ddp.Read(s.connection)
	if e == nil && res[1] != ack {
		return fmt.Errorf("unexpected DDP ack %d", res[1])
	}
	return e
}
func (s archProducer) send(payload []byte, id string) error {
	req, e := http.NewRequest("POST", s.httpURL, bytes.NewReader(payload))
	if e != nil {
		return e
	}
	req.Header.Set("Idempotency-Key", id)
	r, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if _, e = io.Copy(io.Discard, r.Body); e != nil {
		return e
	}
	if r.StatusCode != 202 {
		return fmt.Errorf("HTTPS %s: %d", id, r.StatusCode)
	}
	return nil
}

type archInput struct {
	Source, Key, SHA        string
	Sequence, Frames        int
	Due, Sent, Acknowledged time.Time
}

// Observe committed visibility from an independent PG connection. Scans overlap
// to include writers that started earlier and committed out of order. The final
// unrestricted pass records every tail result, including unusually late commits.
type visibilitySampler struct {
	stop  chan struct{}
	done  chan error
	first map[string]time.Time
}

func startVisibility(pool *pgxpool.Pool, started time.Time) *visibilitySampler {
	v := &visibilitySampler{make(chan struct{}), make(chan error, 1), map[string]time.Time{}}
	go func() {
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			final := false
			select {
			case <-tick.C:
			case <-v.stop:
				final = true
			}
			floor := time.Now().Add(-5 * time.Second)
			if final || floor.Before(started) {
				floor = started
			}
			rows, e := pool.Query(ctx, `SELECT id::text,statement_timestamp() FROM ingestion_results WHERE created_at>=$1`, floor)
			if e != nil {
				v.done <- e
				return
			}
			for rows.Next() {
				var id string
				var at time.Time
				if e = rows.Scan(&id, &at); e != nil {
					break
				}
				if _, ok := v.first[id]; !ok {
					v.first[id] = at
				}
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil || final {
				v.done <- e
				return
			}
		}
	}()
	return v
}

// Exercise the same object-backed, authorized Results path used by the final
// audit. Repeating this after an operator-held start keeps the client warm.
func (f ingestionFixture) warmOwnerResults(t *testing.T, secondActor access.Actor, secondTenant string) {
	t.Helper()
	rows, err := f.env.Pool.Query(ctx, `SELECT u.tenant_id::text,u.id::text,z.id::text FROM ingestion_results z
		JOIN ingestion_parse_plans p ON p.id=z.plan_id JOIN ingestion_units u ON u.id=p.unit_id ORDER BY u.id`)
	must(t, err)
	type warmResult struct{ tenant, unit, id string }
	var list []warmResult
	for rows.Next() {
		var v warmResult
		must(t, rows.Scan(&v.tenant, &v.unit, &v.id))
		list = append(list, v)
	}
	must(t, rows.Err())
	rows.Close()
	if len(list) != 3 {
		t.Fatalf("expected three warm owner reads, got %d", len(list))
	}
	for _, v := range list {
		actor := f.owner
		if v.tenant == secondTenant {
			actor = secondActor
		}
		views, _, err := f.s.Results(ctx, actor, c.TenantScope(v.tenant), v.unit, "")
		must(t, err)
		if len(views) != 1 || views[0].ID != v.id || len(views[0].Response.Records) != 1 {
			t.Fatal("warm result not readable by original owner")
		}
	}
}

func TestArchitectureIngestionLoad(t *testing.T) {
	dir := os.Getenv("INFRAPRO_ARCH_INGESTION_ROUND")
	if dir == "" {
		t.Skip("architecture load driver required")
	}
	f := setupIngestion(t)
	port, secondActor, secondTenant, secondSource := setupLoadTenant(t, f)
	// A terminal genuinely belongs to the first tenant. Its payload on the
	// second tenant's source must stay unassigned and invisible to both owners.
	foreignTerm, e := f.s.CreateTerminal(ctx, f.owner, c.TenantScope(f.tenant), ig.TerminalInput{Meta: f.meta(t), SourceID: f.ddpSource, Key: "00000000009", Name: "隔离地址", Mode: "single"})
	must(t, e)
	_, e = f.s.ChangeAssociation(ctx, f.owner, c.TenantScope(f.tenant), foreignTerm.ID, ig.AssociationInput{Meta: f.meta(t), Revision: "1", DeviceID: f.device, InputView: "complete_frame"}, false)
	must(t, e)
	_, e = f.s.ChangeOne(ctx)
	must(t, e)
	upstream, e := url.Parse(f.opts.Relays[0].URL)
	must(t, e)
	switcher := newHandoffLink(t, upstream.Host)
	workerConfig, e := os.ReadFile(os.Getenv("INFRAPRO_INGESTION_FILE"))
	must(t, e)
	var private map[string]any
	must(t, json.Unmarshal(workerConfig, &private))
	private["relays"].([]any)[0].(map[string]any)["url"] = "https://" + switcher.Addr().String()
	netCfg := filepath.Join(dir, "private", "handoff.json")
	archJSON(t, netCfg, private)
	db, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	q := db.Query()
	tag := "arc_" + q.Get("search_path")
	q.Set("application_name", tag)
	db.RawQuery = q.Encode()
	archJSON(t, filepath.Join(dir, "private", "service.json"), map[string]any{
		"database_url": db.String(), "redis_url": f.env.Config.RedisURL, "redis_key_prefix": f.env.Config.RedisKeyPrefix, "public_console_url": f.env.Config.PublicConsoleURL, "trusted_origins": f.env.Config.TrustedOrigins[0], "csrf_auth_key": hex.EncodeToString(f.env.Config.CSRFKey), "mfa_keyring_file": f.env.Config.MFAKeyringFile, "ingestion_file": netCfg, "log_level": "warn", "support_contact": "架构验收",
		"parsing": map[string]any{"object_endpoint": fixtureObjectEndpoint(), "object_access_key": "contract-admin", "object_secret_key": "contract-admin-test-only", "bucket": f.bucket, "enabled": "true", "url": os.Getenv("INFRAPRO_PARSING_URL"), "token_file": os.Getenv("INFRAPRO_PARSING_TOKEN_FILE")}})
	var epoch string
	if os.Getenv("INFRAPRO_ARCH_TRANSPORT") != "a" {
		must(t, f.env.Pool.QueryRow(ctx, `SELECT restore_epoch::text FROM event_protocol`).Scan(&epoch))
	}
	archJSON(t, filepath.Join(dir, "fixture.json"), map[string]any{"epoch": epoch, "schema": q.Get("search_path"), "bucket": f.bucket, "tag": tag})
	archWait(t, 2*time.Minute, func() bool { _, e := os.Stat(filepath.Join(dir, "workers.ready")); return e == nil }, "deployment ready")
	frame, e := os.ReadFile("../../../../samples/parsing/measurement.bin")
	must(t, e)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	conn, e := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ddpPort), f.tls)
	must(t, e)
	defer conn.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: f.tls, MaxIdleConnsPerHost: 10}, Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()
	send := archProducer{client, conn, fmt.Sprintf("https://127.0.0.1:%d/v1/push/load-batch", port), frame, batch}
	reg, e := ddp.Encode(1, "00000000001", make([]byte, 6))
	must(t, e)
	must(t, send.ddp(reg, 0x81))
	must(t, send.ddp(frame, 0x85))
	must(t, send.send(batch, "warmup"))
	archWait(t, 90*time.Second, func() bool {
		n := f.archCounts(t, time.Unix(0, 0), tag)
		return n.Results == 3 && n.InFlight+n.PendingAcks == 0
	}, "both warm paths")
	must(t, conn.Close())
	f.warmOwnerResults(t, secondActor, secondTenant)
	archJSON(t, filepath.Join(dir, "warm.json"), map[string]any{"at": time.Now().UTC(), "readable_results": 3})
	if os.Getenv("INFRAPRO_ARCH_WARM_CHECK") == "1" {
		var commits int
		archWait(t, 30*time.Second, func() bool {
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM event_work WHERE consumer IN ('identify','plan','parse') AND state<>'done'`).Scan(&commits))
			if commits != 0 {
				return false
			}
			if os.Getenv("INFRAPRO_ARCH_TRANSPORT") == "mq" {
				tail := brokerBacklog(t, dir)
				return tail["identify"]+tail["plan"]+tail["parse"] == 0
			}
			return true
		}, "warm work and broker acknowledgements")
		archJSON(t, filepath.Join(dir, "warm-check.json"), startingEventCounts(t, f, dir))
		return
	}
	if release := os.Getenv("INFRAPRO_ARCH_RELEASE"); release != "" {
		archWait(t, 2*time.Hour, func() bool { _, e := os.Stat(release); return e == nil }, "exclusive measurement window")
	}
	measurement, e := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ddpPort), f.tls)
	must(t, e)
	defer measurement.Close()
	send.connection = measurement
	must(t, send.ddp(reg, 0x81))
	archWait(t, 90*time.Second, func() bool {
		n := f.archCounts(t, time.Unix(0, 0), tag)
		return n.Receipts == 4 && n.PendingAcks+n.InFlight == 0
	}, "new DDP connection ready")
	f.warmOwnerResults(t, secondActor, secondTenant)
	var stats dbStageCounters
	if epoch != "" {
		// Complete the final warm registration in every transport before
		// taking counters so even a fast PG/MQ handoff cannot leak into the
		// first measured event.
		archWait(t, 30*time.Second, func() bool {
			var pending int
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM event_work WHERE consumer IN ('identify','plan','parse') AND state<>'done'`).Scan(&pending))
			if pending != 0 {
				return false
			}
			if os.Getenv("INFRAPRO_ARCH_TRANSPORT") == "mq" {
				must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE type IN ('ingestion.receipt.stored','ingestion.units.ready','ingestion.plan.ready') AND state<>'sent'`).Scan(&pending))
				if pending != 0 {
					return false
				}
				tail := brokerBacklog(t, dir)
				return tail["identify"]+tail["plan"]+tail["parse"] == 0
			}
			return true
		}, "all warm consumer effects")
		stats = startingEventCounts(t, f, dir)
		archJSON(t, filepath.Join(dir, "event-start.json"), stats)
	}
	started := time.Now().UTC()
	visibility := startVisibility(f.env.Pool, started)
	archJSON(t, filepath.Join(dir, "start.json"), map[string]any{"started": started, "frames": 6000, "rate": 100, "extra_valid_frames": 4, "unparseable": 1, "foreign_terminal": 1, "idempotent_retransmission": 1, "disconnect_after_seconds": 10, "disconnected_seconds": 45})
	inputs := make([]archInput, 0, 4505)
	var mu sync.Mutex
	logInput := func(i archInput) { mu.Lock(); inputs = append(inputs, i); mu.Unlock() }
	var g errgroup.Group
	// Pace writes independently from acknowledgments. Input records keep both
	// clocks so overload and a slow acknowledgement cannot hide a missing frame.
	ddpSent := make(chan archInput, 3000)
	g.Go(func() error {
		defer close(ddpSent)
		for i := 0; i < 3000; i++ {
			due := started.Add(time.Duration(i) * 20 * time.Millisecond)
			time.Sleep(time.Until(due))
			at := time.Now()
			if e := measurement.SetWriteDeadline(at.Add(15 * time.Second)); e != nil {
				return e
			}
			if _, e := measurement.Write(frame); e != nil {
				return e
			}
			ddpSent <- archInput{Source: "ddp", SHA: archHash(frame), Sequence: i + 4, Frames: 1, Due: due, Sent: at}
		}
		return nil
	})
	g.Go(func() error {
		for sample := range ddpSent {
			if e := measurement.SetReadDeadline(time.Now().Add(30 * time.Second)); e != nil {
				return e
			}
			reply, e := ddp.Read(measurement)
			if e != nil {
				return e
			}
			if reply[1] != 0x85 {
				return errors.New("wrong measurement acknowledgment")
			}
			sample.Acknowledged = time.Now()
			logInput(sample)
		}
		return nil
	})
	g.Go(func() error {
		var requests errgroup.Group
		requests.SetLimit(64)
		for i := 0; i < 1500; i++ {
			due := started.Add(time.Duration(i) * 40 * time.Millisecond)
			time.Sleep(time.Until(due))
			requests.Go(func() error {
				at := time.Now()
				key := fmt.Sprintf("load-%04d", i)
				if e := send.send(batch, key); e != nil {
					return e
				}
				logInput(archInput{"https", key, archHash(batch), i + 2, 2, due, at, time.Now()})
				return nil
			})
		}
		return requests.Wait()
	})
	var down, up time.Time
	g.Go(func() error {
		time.Sleep(time.Until(started.Add(10 * time.Second)))
		switcher.offline(true)
		down = time.Now().UTC()
		time.Sleep(45 * time.Second)
		switcher.offline(false)
		up = time.Now().UTC()
		return nil
	})
	must(t, g.Wait())
	sentAt := time.Now()
	var lastOffered time.Time
	for _, i := range inputs {
		if i.Sent.After(lastOffered) {
			lastOffered = i.Sent
		}
	}
	offeredRate := 6000 / lastOffered.Sub(started).Seconds()
	bad := []byte("malformed vendor packet")
	foreign := append([]byte(nil), frame...)
	copy(foreign[4:15], "00000000009")
	foreignBatch, e := json.Marshal(map[string]any{"messages": []map[string]string{{"id": "foreign", "ddp_base64": base64.StdEncoding.EncodeToString(foreign)}}})
	must(t, e)
	for i, v := range []struct {
		key    string
		b      []byte
		frames int
	}{{"duplicate-1", batch, 2}, {"duplicate-2", batch, 2}, {"bad-packet", bad, 0}, {"foreign-terminal", foreignBatch, 0}} {
		at := time.Now()
		must(t, send.send(v.b, v.key))
		logInput(archInput{"https", v.key, archHash(v.b), 1502 + i, v.frames, at, at, time.Now()})
	}
	must(t, send.send(batch, "duplicate-1"))
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Source != inputs[j].Source {
			return inputs[i].Source < inputs[j].Source
		}
		return inputs[i].Sequence < inputs[j].Sequence
	})
	archJSON(t, filepath.Join(dir, "inputs.json"), inputs)
	archJSON(t, filepath.Join(dir, "outage.json"), map[string]any{"down": down, "restored": up, "duration_seconds": up.Sub(down).Seconds(), "production_seconds": sentAt.Sub(started).Seconds(), "offered_fps": offeredRate, "effective_fps": 6000 / sentAt.Sub(started).Seconds()})
	deadline := time.Now().Add(15 * time.Minute)
	var counts archCount
	for {
		counts = f.archCounts(t, started, tag)
		archJSON(t, filepath.Join(dir, "progress.json"), map[string]any{"at": time.Now(), "counts": counts})
		if counts.Receipts == 4504 && counts.Results == 6004 && counts.InFlight+counts.PendingAcks == 0 {
			if epoch == "" {
				break
			}
			var pending, quarantined, unpublished int
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state IN ('awaiting','ready','running')),count(*) FILTER(WHERE state='quarantined') FROM event_work WHERE consumer IN ('identify','plan','parse')`).Scan(&pending, &quarantined))
			if quarantined > 0 {
				t.Fatalf("unexpected dispatch quarantine %d", quarantined)
			}
			if os.Getenv("INFRAPRO_ARCH_TRANSPORT") == "mq" {
				must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE type IN ('ingestion.receipt.stored','ingestion.units.ready','ingestion.plan.ready') AND state<>'sent'`).Scan(&unpublished))
			}
			if pending+unpublished == 0 {
				if os.Getenv("INFRAPRO_ARCH_TRANSPORT") == "mq" {
					backlog := brokerBacklog(t, dir)
					archJSON(t, filepath.Join(dir, "broker-tail.json"), map[string]any{"at": time.Now().UTC(), "unacknowledged_or_waiting": backlog})
					pending = backlog["identify"] + backlog["plan"] + backlog["parse"]
				}
				if pending == 0 {
					break
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("load did not drain: %+v", counts)
		}
		time.Sleep(time.Second)
	}
	drained := time.Now().UTC()
	if epoch != "" {
		archJSON(t, filepath.Join(dir, "event-end.json"), startingEventCounts(t, f, dir))
	}
	close(visibility.stop)
	must(t, <-visibility.done)
	archJSON(t, filepath.Join(dir, "drained.json"), map[string]any{"at": drained, "restored": up})
	f.validateArchitectureLoad(t, dir, started, up, drained, inputs, secondActor, secondTenant, secondSource, stats, visibility.first)
	if offeredRate < 99.5 || offeredRate > 100.5 {
		t.Fatalf("offered frame rate %f outside the target", offeredRate)
	}
}

// JetStream publish acknowledgements do not imply that each durable delivery
// is acknowledged. Query the isolated server for its actual consumer tail.
func brokerBacklog(t *testing.T, dir string) map[string]int {
	t.Helper()
	var cfg struct {
		MonitorPort int `json:"monitorPort"`
	}
	b, e := os.ReadFile(filepath.Join(dir, "private", "events", "lab.json"))
	must(t, e)
	must(t, json.Unmarshal(b, &cfg))
	client := http.Client{Timeout: 2 * time.Second}
	response, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/jsz?accounts=true&streams=true&consumers=true", cfg.MonitorPort))
	must(t, e)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("broker monitoring status", response.StatusCode)
	}
	var report struct {
		Accounts []struct {
			Streams []struct {
				Consumers []struct {
					Name       string `json:"name"`
					Pending    int    `json:"num_pending"`
					AckPending int    `json:"num_ack_pending"`
				} `json:"consumer_detail"`
			} `json:"stream_detail"`
		} `json:"account_details"`
	}
	must(t, json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&report))
	out := map[string]int{}
	for _, account := range report.Accounts {
		for _, stream := range account.Streams {
			for _, consumer := range stream.Consumers {
				switch consumer.Name {
				case "identify", "plan", "parse":
					out[consumer.Name] = consumer.Pending + consumer.AckPending
				}
			}
		}
	}
	if len(out) != 3 {
		t.Fatalf("missing isolated broker consumer metrics: %v", out)
	}
	return out
}

type dbStageCounters struct {
	Scans, Duplicate, PG, Broker, Done, Unsent int64
	ReportAt                                   time.Time
}

func startingEventCounts(t *testing.T, f ingestionFixture, dir string) dbStageCounters {
	t.Helper()
	var v dbStageCounters
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE(sum(duplicates),0),count(*) FILTER(WHERE via='pg'),count(*) FILTER(WHERE via='broker'),
	 (SELECT count(*) FROM event_work WHERE consumer IN ('identify','plan','parse') AND state='done'),
	 (SELECT count(*) FROM event_outbox WHERE type IN ('ingestion.receipt.stored','ingestion.units.ready','ingestion.plan.ready') AND state<>'sent')
	 FROM event_consumptions WHERE consumer IN ('identify','plan','parse')`).Scan(&v.Duplicate, &v.PG, &v.Broker, &v.Done, &v.Unsent))
	var snapshot struct {
		Emitted time.Time `json:"emitted_at"`
		Metrics []struct {
			Key   string `json:"key"`
			Value int64  `json:"value"`
		} `json:"metrics"`
	}
	b, e := os.ReadFile(filepath.Join(dir, "parsing-dispatch-worker.report.json"))
	must(t, e)
	must(t, json.Unmarshal(b, &snapshot))
	v.ReportAt = snapshot.Emitted
	for _, m := range snapshot.Metrics {
		if m.Key == "event.pg_reconciliations" {
			v.Scans = m.Value
			break
		}
	}
	return v
}

// Keep one row per input, cloud confirmation, stored receipt and selected result.
// Tenant and sequence checks rule out a duplicate masking a lost frame.
func (f ingestionFixture) validateArchitectureLoad(t *testing.T, dir string, start, up, drained time.Time, inputs []archInput, secondActor access.Actor, secondTenant, secondSource string, base dbStageCounters, visible map[string]time.Time) {
	t.Helper()
	var pjson struct {
		DatabaseURL string `json:"database_url"`
	}
	b, e := os.ReadFile(os.Getenv("INFRAPRO_INGESTION_TEST_RELAY_CONFIG"))
	must(t, e)
	must(t, json.Unmarshal(b, &pjson))
	cloud, e := pgxpool.New(ctx, pjson.DatabaseURL)
	must(t, e)
	defer cloud.Close()
	cr, e := cloud.Query(ctx, `SELECT r.receipt_id::text,a.local_receipt_id::text,r.spool_epoch::text,r.ingress_id::text,r.sha256,r.envelope_digest,COALESCE(r.producer_delivery_id,''),encode(sha256(r.payload),'hex'),r.source_sequence,r.received_at,a.acknowledged_at FROM relay_receipts r LEFT JOIN relay_ack_receipts a USING(receipt_id) WHERE r.received_at >=$1 ORDER BY r.ingress_id,r.source_sequence`, start)
	must(t, e)
	var clouds []archCloud
	for cr.Next() {
		var v archCloud
		must(t, cr.Scan(&v.CloudID, &v.LocalID, &v.Epoch, &v.Source, &v.SHA, &v.EnvelopeSHA, &v.ProducerKey, &v.PayloadSHA, &v.Sequence, &v.Received, &v.Confirmed))
		clouds = append(clouds, v)
	}
	must(t, cr.Err())
	cr.Close()
	archJSON(t, filepath.Join(dir, "cloud.json"), clouds)
	rr, e := f.env.Pool.Query(ctx, `SELECT r.id::text,r.receipt_id::text,r.epoch::text,r.source_id::text,r.tenant_id::text,r.project_id::text,o.sha256,r.envelope_digest,r.state,r.error_code,a.state,r.source_sequence,r.received_at,r.stored_at FROM ingestion_receipts r JOIN ingestion_objects o ON o.id=r.object_id JOIN ingestion_ack_outbox a ON a.receipt_id=r.id WHERE r.received_at>=$1 ORDER BY r.source_id,r.source_sequence`, start)
	must(t, e)
	all := map[string]archReceipt{}
	byLocal := map[string]archReceipt{}
	var receiptRows []archReceipt
	for rr.Next() {
		var v archReceipt
		must(t, rr.Scan(&v.LocalID, &v.CloudID, &v.Epoch, &v.Source, &v.Tenant, &v.Project, &v.ObjectSHA, &v.EnvelopeSHA, &v.State, &v.Error, &v.Ack, &v.Sequence, &v.Received, &v.Stored))
		all[v.CloudID] = v
		byLocal[v.LocalID] = v
		receiptRows = append(receiptRows, v)
	}
	must(t, rr.Err())
	rr.Close()
	archJSON(t, filepath.Join(dir, "receipts.json"), receiptRows)
	lookup := map[string]archInput{}
	for _, in := range inputs {
		src := f.ddpSource
		if in.Source == "https" {
			src = secondSource
		}
		key := fmt.Sprint(src, ":", in.Sequence)
		if in.Source == "https" {
			key = src + ":" + in.Key
		}
		lookup[key] = in
	}
	if len(clouds) != len(inputs) || len(all) != len(inputs) {
		t.Fatalf("receipt cardinality cloud=%d local=%d inputs=%d", len(clouds), len(all), len(inputs))
	}
	identity := func(cl archCloud) string {
		if cl.Source == secondSource {
			return cl.Source + ":" + cl.ProducerKey
		}
		return fmt.Sprint(cl.Source, ":", cl.Sequence)
	}
	lastSeq := map[string]int64{f.ddpSource: 3, secondSource: 1}
	keyByReceipt := map[string]string{}
	for _, cl := range clouds {
		r, ok := all[cl.CloudID]
		i := lookup[identity(cl)]
		if cl.Sequence != lastSeq[cl.Source]+1 {
			t.Fatal("cloud source sequence gap", cl.Sequence)
		}
		lastSeq[cl.Source] = cl.Sequence
		tenant := f.tenant
		if cl.Source == secondSource {
			tenant = secondTenant
		}
		if !ok || r.LocalID != cl.LocalID || r.Epoch != cl.Epoch || r.Source != cl.Source || r.Sequence != cl.Sequence || r.EnvelopeSHA != cl.EnvelopeSHA || r.ObjectSHA != cl.SHA || r.Ack != "acknowledged" || cl.PayloadSHA != i.SHA || cl.SHA != i.SHA || i.Key != cl.ProducerKey || tenant != r.Tenant {
			t.Fatalf("reconciliation mismatch at %s %d: source=%s input-key=%q cloud-key=%q", i.Source, cl.Sequence, cl.Source, i.Key, cl.ProducerKey)
		}
		keyByReceipt[r.LocalID] = i.Key
	}
	fr, e := f.env.Pool.Query(ctx, `SELECT p.id::text,u.id::text,z.id::text,r.id::text,p.tenant_id::text,coalesce(p.snapshot->'parsing'->'entry'->>'method_version',''),z.status,coalesce(p.snapshot->'parsing'->'entry'->>'method_id',''),r.source_id::text,u.unit_key,p.created_at,first.started,z.created_at FROM ingestion_parse_plans p JOIN ingestion_units u ON u.id=p.unit_id JOIN ingestion_receipts r ON r.id=u.receipt_id JOIN ingestion_results z ON z.plan_id=p.id JOIN (SELECT target_id,min(started_at) started FROM ingestion_attempts WHERE stage='parse' GROUP BY target_id) first ON first.target_id=p.id WHERE r.received_at>=$1 ORDER BY r.source_id,r.source_sequence,u.unit_key`, start)
	must(t, e)
	var frames []archFrame
	perReceipt := map[string]int{}
	for fr.Next() {
		var v archFrame
		must(t, fr.Scan(&v.Plan, &v.Unit, &v.Result, &v.Receipt, &v.Tenant, &v.Version, &v.Status, &v.Method, &v.Source, &v.UnitKey, &v.Ready, &v.FirstClaim, &v.Complete))
		r := byLocal[v.Receipt]
		v.Visible = visible[v.Result]
		if v.Visible.IsZero() || v.Visible.Before(v.Complete) {
			t.Fatal("result never observed committed", v.Result)
		}
		v.E2EMs = float64(v.Visible.Sub(r.Received).Microseconds()) / 1000
		v.ClaimMs = float64(v.FirstClaim.Sub(v.Ready).Microseconds()) / 1000
		if v.Tenant != r.Tenant || v.ClaimMs < 0 || v.E2EMs < 0 {
			t.Fatal("invalid selected result", v.Plan)
		}
		frames = append(frames, v)
		perReceipt[r.LocalID]++
	}
	must(t, fr.Err())
	fr.Close()
	for _, cl := range clouds {
		in := lookup[identity(cl)]
		if perReceipt[cl.LocalID] != in.Frames {
			t.Fatalf("result multiplicity %s seq %d = %d expected %d", in.Key, cl.Sequence, perReceipt[cl.LocalID], in.Frames)
		}
	}
	var formal, unowned int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM monitoring_publications),(SELECT count(*) FROM ingestion_units u JOIN devices d ON d.id=u.device_id WHERE u.tenant_id<>d.tenant_id)`).Scan(&formal, &unowned))
	if formal+unowned != 0 {
		t.Fatal("unexpected formal or cross-scope effect")
	}
	var badKey, foreignState, foreignError, foreignID string
	for _, r := range receiptRows {
		switch keyByReceipt[r.LocalID] {
		case "bad-packet":
			badKey = r.Error
			if badKey == "" {
				t.Fatal("invalid sample has no terminal reason")
			}
		case "foreign-terminal":
			must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,state,error_code FROM ingestion_units WHERE receipt_id=$1 AND device_id IS NULL`, r.LocalID).Scan(&foreignID, &foreignState, &foreignError))
		}
	}
	if foreignID == "" || foreignError == "" {
		t.Fatal("foreign terminal lost quarantine evidence")
	}
	if _, _, err := f.s.Results(ctx, secondActor, c.TenantScope(secondTenant), foreignID, ""); err == nil {
		t.Fatal("foreign terminal result visible")
	}
	if _, _, err := f.s.Results(ctx, f.owner, c.TenantScope(f.tenant), foreignID, ""); err == nil {
		t.Fatal("other tenant can read foreign terminal")
	}
	checks, checkCtx := errgroup.WithContext(ctx)
	checks.SetLimit(6)
	for index := range frames {
		frame := &frames[index]
		checks.Go(func() error {
			actor, scope := f.owner, f.tenant
			if frame.Tenant == secondTenant {
				actor, scope = secondActor, secondTenant
			}
			views, _, err := f.s.Results(checkCtx, actor, c.TenantScope(scope), frame.Unit, "")
			if err != nil {
				return err
			}
			if len(views) != 1 || views[0].Adopted || views[0].ID != frame.Result || views[0].Quality != "not_evaluated" || len(views[0].Mapping) == 0 {
				return errors.New("selected result is not readable by its owner")
			}
			view := views[0]
			if len(view.Response.Records) != 1 || len(view.Response.Records[0].Measurements) != 1 {
				return errors.New("unexpected measurement cardinality")
			}
			m := view.Response.Records[0].Measurements[0]
			if m.Value == nil || *m.Value != "12.34" || m.Unit != "mm" {
				return fmt.Errorf("unexpected measurement for %s: %v %s", frame.Unit, m.Value, m.Unit)
			}
			frame.Value, frame.ValueUnit, frame.OutputKey, frame.Quality = *m.Value, m.Unit, m.OutputKey, view.Quality
			parts := make([]string, 0, len(view.Mapping))
			for _, binding := range view.Mapping {
				parts = append(parts, binding.OutputKey+"/"+binding.SlotKey+"/"+binding.Status)
			}
			sort.Strings(parts)
			encoded, e := json.Marshal(parts)
			if e != nil {
				return e
			}
			frame.Mapping = string(encoded)
			return nil
		})
	}
	must(t, checks.Wait())
	archJSON(t, filepath.Join(dir, "frames.json"), frames)
	archJSON(t, filepath.Join(dir, "verified.json"), map[string]any{"received": len(receiptRows), "cloud_confirmed": len(clouds), "authorized_results": len(frames), "bad_packet": badKey, "foreign_terminal_state": foreignState, "foreign_terminal_reason": foreignError, "foreign_read_denied": 2, "formal_publications": formal, "cross_tenant_effects": unowned, "recovery_seconds": drained.Sub(up).Seconds(), "total_seconds": drained.Sub(start).Seconds(), "completion_rate": float64(len(frames)) / drained.Sub(start).Seconds(), "warm_event_counts": base})
}
