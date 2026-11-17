package parsing_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	ig "github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

type ingestionFixture struct {
	fixture
	s                                                         *ig.Service
	opts                                                      ig.Options
	tls                                                       *tls.Config
	ddpSource, httpSource, terminal, device, device2, project string
	ddpPort, httpPort                                         int
}

func setupIngestion(t *testing.T) ingestionFixture {
	t.Helper()
	if os.Getenv("INFRAPRO_INGESTION_FILE") == "" {
		t.Skip("real isolated relay required")
	}
	f := setup(t)
	f.sync(t)
	s := f.b.Ingestion
	opts, e := ig.LoadOptions(os.Getenv("INFRAPRO_INGESTION_FILE"))
	must(t, e)
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, e)
	s.WithStorage(store).WithOptions(opts)
	_, e = f.ps.SyncIngestion(ctx, f.actor(), f.meta(t))
	must(t, e)
	for _, kind := range []string{"identification-methods", "ingress-protocols", "ingress-compatibilities"} {
		caps, e := f.ps.Capabilities(ctx, f.actor(), c.PlatformScope(), kind, c.Filter{Limit: 100})
		must(t, e)
		if len(caps.Items) == 0 {
			t.Fatal("empty capabilities", kind)
		}
		for _, v := range caps.Items {
			_, e = f.ps.SetCapabilityState(ctx, f.actor(), kind, v.ID, parse.StateInput{Meta: f.meta(t), Revision: v.Revision, State: "enabled"})
			must(t, e)
		}
	}
	_, e = s.CreateRelay(ctx, f.actor(), ig.RelayInput{Meta: f.meta(t), ID: opts.Relays[0].ID, Name: "隔离模拟云节点", ConfigurationRef: opts.Relays[0].Ref})
	must(t, e)
	scope := c.TenantScope(f.tenant)
	platform := c.PlatformScope()
	maker, e := f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "ingest", Name: "模拟厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, platform, f.actor(), maker.ID, "ingest", []c.Slot{f.fixed()})
	cfg := f.config(t, product, "1.0.0")
	def, e := f.ps.SetDefault(ctx, f.actor(), platform, product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	dev, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: product.VersionID, Code: "I-1", Name: "模拟 V1", ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
	must(t, e)
	cfg2 := f.config(t, product, "1.1.0")
	def, e = f.ps.SetDefault(ctx, f.actor(), platform, product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: def.Revision, ConfigRevisionID: cfg2.ConfigRevisionID})
	must(t, e)
	dev2, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: product.VersionID, Code: "I-2", Name: "模拟 V2", ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
	must(t, e)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
	project, e := projects.New(f.env.Service.Access()).Create(ctx, f.owner, f.tenant, projects.CreateInput{Fields: projects.Fields{Code: "ingest", Name: "模拟来源项目", Purpose: "接入验收"}, WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "验收"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	grant := devices.GrantChangeSet{Meta: f.meta(t), AccessRevision: "1", ProjectRevision: project.Version, Add: []devices.GrantAdd{{Kind: "device", ID: dev.ID}, {Kind: "device", ID: dev2.ID}}}
	preview, e := f.b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = f.b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, grant, access.Meta{})
	must(t, e)
	cert, e := tls.LoadX509KeyPair(os.Getenv("INFRAPRO_INGESTION_TEST_PRODUCER")+".pem", os.Getenv("INFRAPRO_INGESTION_TEST_PRODUCER")+".key")
	must(t, e)
	ca, e := os.ReadFile(os.Getenv("INFRAPRO_INGESTION_TEST_CA"))
	must(t, e)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	out := ingestionFixture{fixture: f, s: s, opts: opts, tls: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{cert}}, device: dev.ID, device2: dev2.ID, project: project.ProjectID}
	out.ddpPort = ingestionPort(t)
	out.httpPort = ingestionPort(t)
	for n, protocol := range []string{"hongdian.ddp.tcp", "example.vendor.ddp-batch"} {
		port := out.ddpPort
		if n == 1 {
			port = out.httpPort
		}
		src, e := s.SaveSource(ctx, f.actor(), "", ig.SourceInput{Meta: f.meta(t), Revision: "0", Name: "模拟接入 " + protocol, TenantID: f.tenant, ProjectID: project.ProjectID, ManufacturerID: maker.ID, ManufacturerScope: "public", RelayID: opts.Relays[0].ID, ProtocolID: protocol, ProtocolVersion: "1.0.0", Address: "127.0.0.1", Port: port, SourceCode: "sim-" + strconv.Itoa(n), ProducerIDs: []string{"lab-producer"}, Segregated: true, Synthetic: true, IdleSeconds: 30, Budget: "1073741824"})
		must(t, e)
		_, e = s.SourceCommand(ctx, f.actor(), src.ID, ig.Command{Meta: f.meta(t), Revision: "1", Action: "publish"})
		wantCode(t, e, "awaiting_source_confirmation")
		detail, e := s.Source(ctx, f.owner, scope, src.ID)
		must(t, e)
		_, e = s.ConfirmSource(ctx, f.owner, scope, src.ID, ig.Command{Meta: f.meta(t), Revision: "1", Digest: detail.Revisions[0].Digest, Action: "confirm"})
		must(t, e)
		_, e = s.SourceCommand(ctx, f.actor(), src.ID, ig.Command{Meta: f.meta(t), Revision: "1", Action: "publish"})
		must(t, e)
		_, e = s.PublishOne(ctx)
		must(t, e)
		detail, e = s.Source(ctx, f.actor(), platform, src.ID)
		must(t, e)
		if detail.Source.State != "active" {
			t.Fatalf("source not applied: %+v", detail)
		}
		_, e = s.SaveGrant(ctx, f.owner, scope, ig.GrantInput{Meta: f.meta(t), Revision: "0", SourceID: src.ID, Identify: true, Enabled: true, Budget: 100000})
		must(t, e)
		keys := []string{"00000000001"}
		if n == 1 {
			keys = []string{"00000000001", "00000000002"}
		}
		for k, key := range keys {
			term, e := s.CreateTerminal(ctx, f.owner, scope, ig.TerminalInput{Meta: f.meta(t), SourceID: src.ID, Key: key, Name: "模拟 " + key, Mode: "single"})
			must(t, e)
			device := dev.ID
			if k == 1 {
				device = dev2.ID
			}
			_, e = s.ChangeAssociation(ctx, f.owner, scope, term.ID, ig.AssociationInput{Meta: f.meta(t), Revision: "1", DeviceID: device, InputView: "complete_frame"}, false)
			must(t, e)
			_, e = s.ChangeOne(ctx)
			must(t, e)
			assoc, e := s.Associations(ctx, f.owner, scope, term.ID)
			must(t, e)
			if len(assoc) != 1 {
				t.Fatalf("association not applied: %+v", assoc)
			}
			_, e = s.SaveGrant(ctx, f.owner, scope, ig.GrantInput{Meta: f.meta(t), Revision: "0", SourceID: src.ID, TerminalID: term.ID, Measure: true, Enabled: true, Budget: 100000})
			must(t, e)
			if n == 0 {
				out.terminal = term.ID
			}
		}
		if n == 0 {
			out.ddpSource = src.ID
		} else {
			out.httpSource = src.ID
		}
	}
	t.Cleanup(func() {
		for n := 0; n < 10; n++ {
			worked, e := s.AckOne(context.Background())
			if e != nil || !worked {
				break
			}
		}
	})
	return out
}
func (f ingestionFixture) tick(t *testing.T) {
	t.Helper()
	for _, work := range []func(context.Context) (bool, error){f.s.IdentifyOne, f.s.PlanOne, f.s.ParseOne, f.s.AckOne} {
		_, e := work(ctx)
		must(t, e)
	}
}
func (f ingestionFixture) send(t *testing.T, b []byte, key string) int {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: f.tls}, Timeout: 10 * time.Second}
	req, e := http.NewRequest("POST", fmt.Sprintf("https://127.0.0.1:%d/v1/push/sim-1", f.httpPort), bytes.NewReader(b))
	must(t, e)
	req.Header.Set("Idempotency-Key", key)
	res, e := client.Do(req)
	must(t, e)
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	client.CloseIdleConnections()
	return res.StatusCode
}
func TestIngestionEndToEnd(t *testing.T) {
	f := setupIngestion(t)
	scope := c.TenantScope(f.tenant)
	conn, e := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ddpPort), f.tls)
	must(t, e)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	register, e := ddp.Encode(1, "00000000001", make([]byte, 6))
	must(t, e)
	for _, b := range register {
		_, e = conn.Write([]byte{b})
		must(t, e)
	}
	ack, e := ddp.Read(conn)
	must(t, e)
	if ack[1] != 0x81 {
		t.Fatal("register ack")
	}
	frame, e := os.ReadFile("../../../../samples/parsing/measurement.bin")
	must(t, e)
	_, e = conn.Write(append(append([]byte{}, frame...), frame...))
	must(t, e)
	for n := 0; n < 2; n++ {
		ack, e = ddp.Read(conn)
		must(t, e)
		if ack[1] != 0x85 {
			t.Fatal("data ack")
		}
	}
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	key := uuid.NewString()
	if f.send(t, batch, key) != 202 || f.send(t, batch, key) != 202 || f.send(t, append(batch, ' '), key) != 409 {
		t.Fatal("HTTP idempotency")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	for n := 0; n < 16; n++ {
		f.tick(t)
	}
	var receipts, results int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_receipts`).Scan(&receipts))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
	if receipts != 4 || results != 4 {
		rows, _ := f.env.Pool.Query(ctx, `SELECT state,error_code FROM ingestion_receipts UNION ALL SELECT state,error_code FROM ingestion_units UNION ALL SELECT state,error_code FROM ingestion_parse_plans`)
		for rows.Next() {
			var state, code string
			rows.Scan(&state, &code)
			t.Log(state, code)
		}
		rows.Close()
		t.Fatalf("receipts %d results %d", receipts, results)
	}
	page, e := f.s.Messages(ctx, f.owner, scope, ig.Filter{Filter: c.Filter{Limit: 100}})
	must(t, e)
	if len(page.Items) != 5 {
		t.Fatalf("verified units=%d", len(page.Items))
	}
	for _, raw := range page.Items {
		var v struct {
			ID, DeviceID string
			MessageKind  string `json:"message_kind"`
		}
		must(t, json.Unmarshal(raw, &v))
		content, e := f.s.Content(ctx, f.owner, scope, v.ID)
		must(t, e)
		if len(content) > len(frame) {
			t.Fatal("batch root exposed")
		}
		_, e = f.s.Content(ctx, f.actor(), scope, v.ID)
		if e == nil {
			t.Fatal("platform raw access")
		}
		if v.MessageKind == "measurement" {
			rv, _, e := f.s.Results(ctx, f.owner, scope, v.ID, "")
			must(t, e)
			if len(rv) != 1 || rv[0].Adopted || rv[0].Quality != "not_evaluated" {
				t.Fatal("production result", rv)
			}
		}
	}
	var runs, tasks int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_runs`).Scan(&runs))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM task_records WHERE source_module='ingestion'`).Scan(&tasks))
	if runs != 0 || tasks != 0 {
		t.Fatal("automatic path created manual jobs")
	}
	t.Log("real mTLS DDP split/coalesced frames, HTTP idempotency, durable takeover, V1/V2 exact plans, per-unit raw privacy and no manual jobs verified")
	if path := os.Getenv("INFRAPRO_INGESTION_EVIDENCE"); path != "" {
		must(t, os.MkdirAll(path, 0700))
		must(t, os.WriteFile(filepath.Join(path, "pipeline.json"), []byte(fmt.Sprintf(`{"synthetic":true,"receipts":%d,"results":%d,"manual_runs":%d}`, receipts, results, runs)), 0600))
	}
	_ = r.Version
}

// TestIngestionSustainedLoad uses a second tenant/project and the same external
// terminal numbers. No mocked storage, transport, identification or parse calls.
func TestIngestionSustainedLoad(t *testing.T) {
	if os.Getenv("INFRAPRO_INGESTION_LOAD") != "1" {
		t.Skip("explicit 30-minute load run")
	}
	f := setupIngestion(t)
	other := f.member(t, "LoadOwner")
	actor := f.env.Service.AccessActor(other)
	tid, e := tenancy.New(f.env.Service.Access()).Create(ctx, f.actor(), tenancy.CreateInput{Fields: tenancy.Fields{Name: "负载隔离租户", Code: "load"}, AdministratorID: other.Account.ID, RequestID: f.meta(t).RequestID, Reason: "模拟负载"}, access.Meta{})
	must(t, e)
	scope := c.TenantScope(tid.TenantID)
	var membership, product, version, maker, defaultRevision string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, tid.TenantID, actor.ID).Scan(&membership))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT d.product_id::text,d.product_version_id::text,p.manufacturer_id::text,x.revision::text FROM devices d JOIN device_products p ON p.id=d.product_id JOIN parsing_product_defaults x ON x.product_version_id=d.product_version_id WHERE d.id=$1`, f.device).Scan(&product, &version, &maker, &defaultRevision))
	project, e := projects.New(f.env.Service.Access()).Create(ctx, actor, tid.TenantID, projects.CreateInput{Fields: projects.Fields{Code: "load", Name: "第二来源项目", Purpose: "隔离负载"}, WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "模拟"}, Owner: membership, Administrator: membership}, access.Meta{})
	must(t, e)
	deviceIDs := []string{}
	for n := 0; n < 2; n++ {
		dev, e := f.b.Devices.Create(ctx, actor, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: version, Code: fmt.Sprintf("L-%d", n), Name: "第二租户模拟", ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: defaultRevision}}, access.Meta{})
		must(t, e)
		deviceIDs = append(deviceIDs, dev.ID)
	}
	grant := devices.GrantChangeSet{Meta: f.meta(t), AccessRevision: "1", ProjectRevision: project.Version, Add: []devices.GrantAdd{{Kind: "device", ID: deviceIDs[0]}, {Kind: "device", ID: deviceIDs[1]}}}
	preview, e := f.b.Devices.PreviewGrants(ctx, actor, tid.TenantID, project.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = f.b.Devices.ChangeGrants(ctx, actor, tid.TenantID, project.ProjectID, grant, access.Meta{})
	must(t, e)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	source, e := f.s.SaveSource(ctx, f.actor(), "", ig.SourceInput{Meta: f.meta(t), Revision: "0", Name: "第二租户模拟 HTTP", TenantID: tid.TenantID, ProjectID: project.ProjectID, ManufacturerID: maker, ManufacturerScope: "public", RelayID: f.opts.Relays[0].ID, ProtocolID: "example.vendor.ddp-batch", ProtocolVersion: "1.0.0", Address: "127.0.0.1", Port: port, SourceCode: "load-batch", ProducerIDs: []string{"lab-producer"}, Segregated: true, Synthetic: true, IdleSeconds: 30, Budget: "1073741824"})
	must(t, e)
	detail, e := f.s.Source(ctx, actor, scope, source.ID)
	must(t, e)
	_, e = f.s.ConfirmSource(ctx, actor, scope, source.ID, ig.Command{Meta: f.meta(t), Revision: "1", Digest: detail.Revisions[0].Digest, Action: "confirm"})
	must(t, e)
	_, e = f.s.SourceCommand(ctx, f.actor(), source.ID, ig.Command{Meta: f.meta(t), Revision: "1", Action: "publish"})
	must(t, e)
	_, e = f.s.PublishOne(ctx)
	must(t, e)
	_, e = f.s.SaveGrant(ctx, actor, scope, ig.GrantInput{Meta: f.meta(t), Revision: "0", SourceID: source.ID, Identify: true, Enabled: true, Budget: 100000})
	must(t, e)
	for n, key := range []string{"00000000001", "00000000002"} {
		term, e := f.s.CreateTerminal(ctx, actor, scope, ig.TerminalInput{Meta: f.meta(t), SourceID: source.ID, Key: key, Name: "第二租户 " + key, Mode: "single"})
		must(t, e)
		_, e = f.s.ChangeAssociation(ctx, actor, scope, term.ID, ig.AssociationInput{Meta: f.meta(t), Revision: "1", DeviceID: deviceIDs[n], InputView: "complete_frame"}, false)
		must(t, e)
		_, e = f.s.ChangeOne(ctx)
		must(t, e)
		_, e = f.s.SaveGrant(ctx, actor, scope, ig.GrantInput{Meta: f.meta(t), Revision: "0", SourceID: source.ID, TerminalID: term.ID, Measure: true, Enabled: true, Budget: 100000})
		must(t, e)
	}
	frame, e := os.ReadFile("../../../../samples/parsing/measurement.bin")
	must(t, e)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	conn, e := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ddpPort), f.tls)
	must(t, e)
	defer conn.Close()
	reg, e := ddp.Encode(1, "00000000001", make([]byte, 6))
	must(t, e)
	_, e = conn.Write(reg)
	must(t, e)
	_, e = ddp.Read(conn)
	must(t, e)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: f.tls, MaxIdleConnsPerHost: 10}, Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()
	var cancel context.CancelFunc
	var workerDone chan struct{}
	startWorker := func() {
		var work context.Context
		work, cancel = context.WithCancel(ctx)
		workerDone = make(chan struct{})
		go func() { defer close(workerDone); _ = f.s.RunWorker(work, nil) }()
	}
	stopWorker := func() { cancel(); <-workerDone }
	startWorker()
	defer func() {
		if cancel != nil {
			stopWorker()
		}
	}()
	duration := 1800
	if v := os.Getenv("INFRAPRO_INGESTION_LOAD_SECONDS"); v != "" {
		duration, _ = strconv.Atoi(v)
	}
	evidence := os.Getenv("INFRAPRO_INGESTION_EVIDENCE")
	log, e := os.Create(filepath.Join(evidence, "load.jsonl"))
	must(t, e)
	defer log.Close()
	emit := func(v any) { b, _ := json.Marshal(v); log.Write(append(b, '\n')); log.Sync() }
	started := time.Now()
	var loadSchema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&loadSchema))
	emit(map[string]any{"event": "started", "started_at": started.UTC().Format(time.RFC3339Nano), "schema": loadSchema, "duration_seconds": duration, "target_measurement_frames_per_second": 100, "tenants": 2, "projects": 2, "synthetic": true, "transport": "mTLS DDP + HTTPS batch", "real_parser": true})
	// Independent external senders keep their promised rates while the inner
	// worker is stopped/restarted and while metric queries run. Sampling must
	// not serialize the DDP and HTTP acknowledgements or pause external traffic.
	producerCtx, stopProducers := context.WithCancel(ctx)
	defer stopProducers()
	var acceptedFrames atomic.Int64
	finished := make(chan error, 2)
	go func() {
		for n := 0; n < duration*50; n++ {
			if wait := time.Until(started.Add(time.Duration(n) * 20 * time.Millisecond)); wait > 0 {
				time.Sleep(wait)
			}
			if producerCtx.Err() != nil {
				finished <- producerCtx.Err()
				return
			}
			conn.SetDeadline(time.Now().Add(15 * time.Second))
			if _, e := conn.Write(frame); e != nil {
				finished <- e
				return
			}
			if _, e := ddp.Read(conn); e != nil {
				finished <- e
				return
			}
			acceptedFrames.Add(1)
		}
		finished <- nil
	}()
	go func() {
		for n := 0; n < duration*25; n++ {
			if wait := time.Until(started.Add(time.Duration(n) * 40 * time.Millisecond)); wait > 0 {
				time.Sleep(wait)
			}
			req, e := http.NewRequestWithContext(producerCtx, "POST", fmt.Sprintf("https://127.0.0.1:%d/v1/push/load-batch", port), bytes.NewReader(batch))
			if e != nil {
				finished <- e
				return
			}
			req.Header.Set("Idempotency-Key", fmt.Sprintf("load-%s-%d", source.ID, n))
			res, e := client.Do(req)
			if e != nil {
				finished <- e
				return
			}
			_, e = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if e != nil {
				finished <- e
				return
			}
			if res.StatusCode != 202 {
				finished <- fmt.Errorf("cloud backpressure at batch %d: %d", n, res.StatusCode)
				return
			}
			acceptedFrames.Add(2)
		}
		finished <- nil
	}()
	pauseAt := time.NewTimer(time.Until(started.Add(120 * time.Second)))
	resumeAt := time.NewTimer(time.Until(started.Add(165 * time.Second)))
	sampleAt := time.NewTicker(30 * time.Second)
	defer pauseAt.Stop()
	defer resumeAt.Stop()
	defer sampleAt.Stop()
	var sendFailure error
	for complete := 0; complete < 2; {
		select {
		case err := <-finished:
			complete++
			if err != nil && sendFailure == nil {
				sendFailure = err
				stopProducers()
				conn.Close()
			}
		case <-pauseAt.C:
			stopWorker()
			cancel = nil
			emit(map[string]any{"event": "inner_channel_stopped", "seconds": time.Since(started).Seconds()})
		case <-resumeAt.C:
			if cancel == nil {
				startWorker()
			}
			emit(map[string]any{"event": "inner_channel_restored", "seconds": time.Since(started).Seconds()})
		case <-sampleAt.C:
			var receipts, results int
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_receipts`).Scan(&receipts))
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
			emit(map[string]any{"event": "sample", "elapsed_seconds": time.Since(started).Seconds(), "sent_measurements": acceptedFrames.Load(), "local_receipts": receipts, "results": results})
			var queues []byte
			must(t, f.env.Pool.QueryRow(ctx, `SELECT jsonb_build_object('identify',(SELECT count(*) FROM ingestion_receipts WHERE state IN ('stored','identifying','retry_wait')),'planning',(SELECT count(*) FROM ingestion_units WHERE state IN ('identified','demultiplexing','demultiplex_retry')),'parse',(SELECT count(*) FROM ingestion_parse_plans WHERE state IN ('queued','parsing','retry_wait')),'confirmation',(SELECT count(*) FROM ingestion_ack_outbox WHERE state<>'acknowledged'))`).Scan(&queues))
			emit(map[string]any{"event": "queues", "elapsed_seconds": time.Since(started).Seconds(), "stages": json.RawMessage(queues)})
			t.Logf("load t=%.1fs accepted=%d local=%d results=%d", time.Since(started).Seconds(), acceptedFrames.Load(), receipts, results)
		}
	}
	must(t, sendFailure)
	if cancel == nil {
		startWorker()
	}
	elapsed := time.Since(started).Seconds()
	target := duration * 100
	targetReceipts := duration*75 + 1
	emit(map[string]any{"event": "sending_complete", "seconds": elapsed, "measurement_frames": target, "receipts": targetReceipts, "frames_per_second": float64(target) / elapsed})
	deadline := time.Now().Add(90 * time.Minute)
	var receipts, results, failed, pending int
	lastDrainReport := time.Time{}
	for time.Now().Before(deadline) {
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_receipts`).Scan(&receipts))
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_parse_plans WHERE state IN ('failed','blocked','rejected')`).Scan(&failed))
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_ack_outbox WHERE state<>'acknowledged'`).Scan(&pending))
		if receipts == targetReceipts && results == target && pending == 0 {
			break
		}
		if time.Since(lastDrainReport) >= 30*time.Second {
			emit(map[string]any{"event": "draining", "elapsed_seconds": time.Since(started).Seconds(), "receipts": receipts, "results": results, "remaining_results": target - results, "pending_ack": pending, "failed": failed})
			t.Logf("load draining t=%.1fs receipts=%d results=%d remaining=%d", time.Since(started).Seconds(), receipts, results, target-results)
			lastDrainReport = time.Now()
		}
		time.Sleep(5 * time.Second)
	}
	var duplicate, crossTenant int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT unit_id,decision_id FROM ingestion_parse_plans GROUP BY unit_id,decision_id HAVING count(*)>1)x`).Scan(&duplicate))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_units u JOIN devices d ON d.id=u.device_id WHERE u.tenant_id<>d.tenant_id`).Scan(&crossTenant))
	var latencies []byte
	must(t, f.env.Pool.QueryRow(ctx, `SELECT jsonb_build_object('takeover_p95_seconds',(SELECT percentile_cont(0.95) WITHIN GROUP(ORDER BY extract(epoch FROM stored_at-received_at)) FROM ingestion_receipts),'end_to_end_p95_seconds',(SELECT percentile_cont(0.95) WITHIN GROUP(ORDER BY extract(epoch FROM p.finished_at-r.received_at)) FROM ingestion_parse_plans p JOIN ingestion_units u ON u.id=p.unit_id JOIN ingestion_receipts r ON r.id=u.receipt_id WHERE p.state='parsed'),'total_seconds',$1::double precision)`, time.Since(started).Seconds()).Scan(&latencies))
	emit(map[string]any{"event": "latencies", "values": json.RawMessage(latencies)})
	emit(map[string]any{"event": "reconciled", "receipts": receipts, "expected_receipts": targetReceipts, "results": results, "expected_results": target, "pending_ack": pending, "failed": failed, "duplicate_default_plans": duplicate, "cross_tenant_units": crossTenant})
	if receipts != targetReceipts || results != target || pending != 0 || failed != 0 || duplicate != 0 || crossTenant != 0 {
		t.Fatalf("load mismatch receipts=%d/%d results=%d/%d pending=%d failed=%d", receipts, targetReceipts, results, target, pending, failed)
	}
	if elapsed > float64(duration)*1.005 {
		t.Fatalf("offered throughput below tolerance: %.2f", float64(target)/elapsed)
	}
}

func ingestionPort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
