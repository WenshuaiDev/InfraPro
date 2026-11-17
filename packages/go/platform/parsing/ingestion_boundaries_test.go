package parsing_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	ig "github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type failIngestionPut struct{ parse.ObjectStore }

func (f failIngestionPut) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("synthetic object outage")
}
func ingestionError(t *testing.T, e error, code string) {
	t.Helper()
	encoded, _ := json.Marshal(e)
	var problem struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(encoded, &problem)
	if e == nil || problem.Code != code && !strings.Contains(e.Error(), code) {
		t.Fatalf("want %s, got %v", code, e)
	}
}
func (f ingestionFixture) cloudPull(t *testing.T) []r.Envelope {
	t.Helper()
	cfg := f.opts.Relays[0]
	cert, e := tls.LoadX509KeyPair(cfg.Consumer.Certificate, cfg.Consumer.Key)
	must(t, e)
	ca, e := os.ReadFile(cfg.Consumer.CA)
	must(t, e)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: roots}}, Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	b, _ := json.Marshal(r.Pull{RequestID: uuid.NewString(), Limit: 100, MaxBytes: r.MaxPullBytes})
	res, e := client.Post(cfg.URL+"/v1/consumers/"+cfg.ConsumerID+"/pull", "application/json", bytes.NewReader(b))
	must(t, e)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("cloud pull", res.StatusCode)
	}
	var v r.Delivery
	must(t, json.NewDecoder(res.Body).Decode(&v))
	return v.Items
}
func (f ingestionFixture) work(t *testing.T, n int) {
	t.Helper()
	for j := 0; j < n; j++ {
		f.tick(t)
	}
}

func TestIngestionStorageIsolationRecoveryAndGovernance(t *testing.T) {
	f := setupIngestion(t)
	scope := c.TenantScope(f.tenant)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	delivery := f.cloudPull(t)
	if len(delivery) != 1 {
		t.Fatal("expected one receipt", len(delivery))
	}
	envelope := delivery[0]
	f.s.WithStorage(failIngestionPut{store})
	_, e = f.s.Accept(ctx, envelope)
	ingestionError(t, e, "storage_unavailable")
	f.s.WithStorage(store)
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_ack_outbox`).Scan(&count))
	if count != 0 {
		t.Fatal("ack before object")
	}
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION ingestion_test_fail_commit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic_receipt_failure'; END$$; CREATE TRIGGER ingestion_test_fail_commit BEFORE INSERT ON ingestion_receipts FOR EACH ROW EXECUTE FUNCTION ingestion_test_fail_commit()`)
	must(t, e)
	_, e = f.s.Accept(ctx, envelope)
	if e == nil {
		t.Fatal("commit fault ignored")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_ack_outbox`).Scan(&count))
	if count != 0 {
		t.Fatal("ack before receipt")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER ingestion_test_fail_commit ON ingestion_receipts;DROP FUNCTION ingestion_test_fail_commit()`)
	must(t, e)
	local, e := f.s.Accept(ctx, envelope)
	must(t, e)
	same, e := f.s.Accept(ctx, envelope)
	must(t, e)
	if same != local {
		t.Fatal("duplicate receipt")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_receive_intents`).Scan(&count))
	if count != 1 {
		t.Fatal("duplicated object reservation")
	}
	changed := envelope
	changed.ProjectID = uuid.NewString()
	changed.EnvelopeDigest = r.EnvelopeDigest(changed)
	_, e = f.s.Accept(ctx, changed)
	ingestionError(t, e, "receipt_conflict")
	events, e := f.s.IntegrityEvents(ctx, f.actor())
	must(t, e)
	if len(events) != 1 {
		t.Fatal("conflict not observable")
	}
	changed.ReceiptID = uuid.NewString()
	changed.EnvelopeDigest = r.EnvelopeDigest(changed)
	_, e = f.s.Accept(ctx, changed)
	ingestionError(t, e, "envelope_config_mismatch")
	f.work(t, 12)
	var unit string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE device_id=$1 AND receipt_id=$2`, f.device, local).Scan(&unit))
	original, e := f.s.Content(ctx, f.owner, scope, unit)
	must(t, e)
	if len(original) != 81 {
		t.Fatal("batch privacy")
	}
	if _, e = f.s.Content(ctx, f.owner, scope, local); e == nil {
		t.Fatal("root body exposed")
	}
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT o.object_key FROM ingestion_receipts r JOIN ingestion_objects o ON o.id=r.object_id WHERE r.id=$1`, local).Scan(&key))
	must(t, store.Delete(ctx, key))
	_, e = f.s.Content(ctx, f.owner, scope, unit)
	ingestionError(t, e, "integrity_failed")
	must(t, store.Put(ctx, key, bytes.NewReader(batch), int64(len(batch)), "application/octet-stream"))
	_, e = f.s.Content(ctx, f.owner, scope, unit)
	must(t, e)
	// A paused automatic planner still permits a separately authorized fixed recovery.
	_, e = f.s.PlanningState(ctx, f.owner, scope, f.httpSource, ig.Command{Meta: f.meta(t), Revision: "1", Action: "pause"})
	must(t, e)
	var config string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT config_revision_id::text FROM parsing_device_adoptions WHERE device_id=$1 AND ended_at IS NULL`, f.device).Scan(&config))
	recovered, e := f.s.Recover(ctx, f.owner, scope, ig.RecoveryInput{Meta: f.meta(t), Kind: "reparse", UnitIDs: []string{unit}, ConfigRevisionID: config}, false)
	must(t, e)
	if recovered.TaskRef == nil {
		t.Fatal("manual recovery task missing")
	}
	for n := 0; n < 12; n++ {
		_, e = f.s.RecoveryOne(ctx)
		must(t, e)
		f.work(t, 2)
	}
	jobs, e := f.s.Recoveries(ctx, f.owner, scope)
	must(t, e)
	if len(jobs) != 1 || jobs[0].State != "succeeded" {
		t.Fatalf("recovery: %+v", jobs)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_identity_decisions WHERE receipt_id=$1`, local).Scan(&count))
	if count != 2 {
		t.Fatal("recovery replaced decision")
	}
	_, e = f.s.Content(ctx, f.owner, scope, unit)
	must(t, e)
	_, e = f.s.PlanningState(ctx, f.owner, scope, f.httpSource, ig.Command{Meta: f.meta(t), Revision: "1", Action: "resume"})
	must(t, e)
	// Historic unknown identity stays closed until an explicit association and fixed backfill.
	raw, e := os.ReadFile("../../../../samples/parsing/measurement.bin")
	must(t, e)
	unknown := append([]byte{}, raw...)
	copy(unknown[4:15], []byte("00000000009"))
	mixed, _ := json.Marshal(map[string]any{"messages": []map[string]string{{"id": "known", "ddp_base64": base64.StdEncoding.EncodeToString(raw)}, {"id": "unknown", "ddp_base64": base64.StdEncoding.EncodeToString(unknown)}}})
	if f.send(t, mixed, uuid.NewString()) != 202 {
		t.Fatal("mixed send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 14)
	var unknownReceipt, unknownUnit string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT receipt_id::text,id::text FROM ingestion_units WHERE terminal_key='00000000009' ORDER BY created_at DESC LIMIT 1`).Scan(&unknownReceipt, &unknownUnit))
	if _, e = f.s.Content(ctx, f.owner, scope, unknownUnit); e == nil {
		t.Fatal("unassigned raw exposed")
	}
	terminal, e := f.s.CreateTerminal(ctx, f.owner, scope, ig.TerminalInput{Meta: f.meta(t), SourceID: f.httpSource, Key: "00000000009", Name: "补登记模拟终端", Mode: "single"})
	must(t, e)
	_, e = f.s.ChangeAssociation(ctx, f.owner, scope, terminal.ID, ig.AssociationInput{Meta: f.meta(t), Revision: "1", DeviceID: f.device, InputView: "complete_frame"}, false)
	must(t, e)
	_, e = f.s.ChangeOne(ctx)
	must(t, e)
	if _, e = f.s.Content(ctx, f.owner, scope, unknownUnit); e == nil {
		t.Fatal("registration retroactively exposed raw")
	}
	links, e := f.s.Associations(ctx, f.owner, scope, terminal.ID)
	must(t, e)
	_, e = f.s.SaveGrant(ctx, f.owner, scope, ig.GrantInput{Meta: f.meta(t), Revision: "0", SourceID: f.httpSource, TerminalID: terminal.ID, Measure: true, Enabled: true, Budget: 100})
	must(t, e)
	_, e = f.s.Recover(ctx, f.owner, scope, ig.RecoveryInput{Meta: f.meta(t), Kind: "backfill", ReceiptIDs: []string{unknownReceipt}, AssociationID: links[0].ID}, false)
	must(t, e)
	for n := 0; n < 12; n++ {
		_, e = f.s.RecoveryOne(ctx)
		must(t, e)
		f.work(t, 2)
	}
	var newUnit string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE terminal_key='00000000009' AND device_id=$1 ORDER BY created_at DESC LIMIT 1`, f.device).Scan(&newUnit))
	_, e = f.s.Content(ctx, f.owner, scope, newUnit)
	must(t, e)
	// Project authorization is checked on every read and on queued execution.
	grants, e := f.b.Devices.Grants(ctx, f.owner, f.tenant, f.project, devices.Filter{Limit: 100})
	must(t, e)
	end := []devices.GrantEnd{}
	for _, g := range grants.Items {
		if g.DeviceID == f.device {
			end = append(end, devices.GrantEnd{ID: g.ID, Revision: g.Revision})
		}
	}
	change := devices.GrantChangeSet{Meta: f.meta(t), AccessRevision: grants.AccessRevision, ProjectRevision: grants.ProjectRevision, End: end}
	gp, e := f.b.Devices.PreviewGrants(ctx, f.owner, f.tenant, f.project, change)
	must(t, e)
	change.PreviewHash = gp.Hash
	_, e = f.b.Devices.ChangeGrants(ctx, f.owner, f.tenant, f.project, change, access.Meta{})
	must(t, e)

	if _, e = f.s.Content(ctx, f.owner, scope, unit); e == nil {
		t.Fatal("revoked project raw")
	}
	if _, _, e = f.s.Results(ctx, f.owner, scope, unit, ""); e == nil {
		t.Fatal("revoked project result")
	}
	summary, e := f.s.DeviceSummary(ctx, f.owner, scope, f.device)
	must(t, e)
	if len(summary) != 0 {
		t.Fatal("revoked project summary")
	}
	t.Log("real S3 failure/loss/repair, PG commit fault/outbox, routing conflicts, fixed recovery task, late binding/backfill, paused planning and current project revocation verified")
}

func TestIngestionGatewayExactViewsAndAdoption(t *testing.T) {
	f := setupIngestion(t)
	scope := c.TenantScope(f.tenant)
	var maker string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT p.manufacturer_id::text FROM devices d JOIN device_products p ON p.id=d.product_id WHERE d.id=$1`, f.device).Scan(&maker))
	gateway, e := f.s.CreateTerminal(ctx, f.owner, scope, ig.TerminalInput{Meta: f.meta(t), SourceID: f.ddpSource, Key: "00000000003", Name: "模拟网关", Mode: "single"})
	must(t, e)
	caps, e := f.ps.Capabilities(ctx, f.actor(), c.PlatformScope(), "identification-methods", c.Filter{Limit: 100})
	must(t, e)
	var demux i.Identification
	for _, v := range caps.Items {
		var m i.Identification
		must(t, json.Unmarshal(v.Entry, &m))
		if m.Purpose == "terminal_demultiplex" {
			demux = m
		}
	}
	_, e = f.s.ReviseTerminal(ctx, f.owner, scope, gateway.ID, ig.TerminalInput{Meta: f.meta(t), Revision: "1", SourceID: f.ddpSource, Key: "00000000003", Name: "模拟网关", Mode: "multiple", Demultiplexer: &demux.Method})
	must(t, e)
	devicesIDs := []string{}
	for n, child := range []string{"A", "B"} {
		product := f.product(t, c.PlatformScope(), f.actor(), maker, "reading"+child, []c.Slot{f.fixed()})
		cfg, e := f.ps.SaveConfig(ctx, f.actor(), c.PlatformScope(), product.ID, product.VersionID, "", "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "独立 reading", MethodID: "example.monitoring-json.reading", MethodVersion: "1.0.0", Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "x", Confirmed: true}}})
		must(t, e)
		def, e := f.ps.SetDefault(ctx, f.actor(), c.PlatformScope(), product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
		must(t, e)
		dev, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: product.VersionID, Code: "G-" + child, Name: "网关子设备 " + child, ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
		must(t, e)
		devicesIDs = append(devicesIDs, dev.ID)
		accessState, e := f.b.Devices.Grants(ctx, f.owner, f.tenant, f.project, devices.Filter{Limit: 100})
		must(t, e)
		in := devices.GrantChangeSet{Meta: f.meta(t), AccessRevision: accessState.AccessRevision, ProjectRevision: accessState.ProjectRevision, Add: []devices.GrantAdd{{Kind: "device", ID: dev.ID}}}
		preview, e := f.b.Devices.PreviewGrants(ctx, f.owner, f.tenant, f.project, in)
		must(t, e)
		in.PreviewHash = preview.Hash
		_, e = f.b.Devices.ChangeGrants(ctx, f.owner, f.tenant, f.project, in, access.Meta{})
		must(t, e)
		current, e := f.s.Terminal(ctx, f.owner, scope, gateway.ID)
		must(t, e)
		_, e = f.s.ChangeAssociation(ctx, f.owner, scope, gateway.ID, ig.AssociationInput{Meta: f.meta(t), Revision: current.Revision, ChildKey: child, DeviceID: dev.ID, InputView: "json_reading"}, false)
		must(t, e)
		_, e = f.s.ChangeOne(ctx)
		must(t, e)
		_ = n
	}
	_, e = f.s.SaveGrant(ctx, f.owner, scope, ig.GrantInput{Meta: f.meta(t), Revision: "0", SourceID: f.ddpSource, TerminalID: gateway.ID, Measure: true, Enabled: true, Budget: 100})
	must(t, e)
	conn, e := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ddpPort), f.tls)
	must(t, e)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	registration, e := ddp.Encode(1, "00000000003", make([]byte, 6))
	must(t, e)
	_, e = conn.Write(registration)
	must(t, e)
	_, e = ddp.Read(conn)
	must(t, e)
	raw, e := os.ReadFile("../../../../samples/ingestion/channels.bin")
	must(t, e)
	_, e = conn.Write(raw)
	must(t, e)
	_, e = ddp.Read(conn)
	must(t, e)
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 20)
	for n, dev := range devicesIDs {
		var id string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE device_id=$1 AND state='parsed'`, dev).Scan(&id))
		body, e := f.s.Content(ctx, f.owner, scope, id)
		must(t, e)
		if len(body) >= len(raw) || bytes.Contains(body, []byte("channels")) {
			t.Fatal("gateway root leaked")
		}
		results, _, e := f.s.Results(ctx, f.owner, scope, id, "")
		must(t, e)
		b, _ := json.Marshal(results)
		want := []string{"12.34", "8.5"}[n]
		if !bytes.Contains(b, []byte(want)) {
			t.Fatal("wrong child measurement", want)
		}
	}
	// A known gateway must not lend its device or sibling methods to an unknown child.
	frame, e := ddp.Decode(raw)
	must(t, e)
	var document map[string]any
	must(t, json.Unmarshal(frame.Payload, &document))
	channels := document["channels"].([]any)
	unknown := map[string]any{}
	for key, value := range channels[0].(map[string]any) {
		unknown[key] = value
	}
	unknown["key"] = "C"
	document["channels"] = append(channels, unknown)
	payload, e := json.Marshal(document)
	must(t, e)
	unknownFrame, e := ddp.Encode(9, "00000000003", payload)
	must(t, e)
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	_, e = conn.Write(unknownFrame)
	must(t, e)
	_, e = ddp.Read(conn)
	must(t, e)
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 20)
	var unknownCount, knownCount int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_units WHERE terminal_key='00000000003' AND child_key='C' AND device_id IS NULL AND state='awaiting_association'`).Scan(&unknownCount))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results r JOIN ingestion_parse_plans p ON p.id=r.plan_id WHERE p.device_id=ANY($1::uuid[])`, devicesIDs).Scan(&knownCount))
	if unknownCount != 1 || knownCount != 4 {
		t.Fatal("unknown gateway child borrowed an identity or blocked valid siblings", unknownCount, knownCount)
	}
	history, e := f.s.TerminalHistory(ctx, f.owner, scope, gateway.ID)
	must(t, e)
	if len(history) != 4 {
		t.Fatal("terminal history", len(history))
	}
	// Incompatible adoption cannot silently invalidate an enabled ingress mapping.
	current, e := f.ps.Device(ctx, f.owner, scope, f.device)
	must(t, e)
	cmd := parse.Command{Meta: f.meta(t), Revision: current.Revision, Action: "clear"}
	preview, e := f.ps.Preview(ctx, f.owner, scope, f.device, cmd)
	must(t, e)
	if preview.CanSubmit {
		t.Fatal("incompatible adoption preview")
	}
	_, e = f.ps.Command(ctx, f.owner, scope, f.device, cmd)
	if e == nil {
		t.Fatal("incompatible adoption accepted")
	}
	links, e := f.s.Associations(ctx, f.owner, scope, f.terminal)
	must(t, e)
	term, e := f.s.Terminal(ctx, f.owner, scope, f.terminal)
	must(t, e)
	_, e = f.s.AssociationState(ctx, f.owner, scope, links[0].ID, ig.Command{Meta: f.meta(t), Revision: term.Revision, Action: "disable"})
	must(t, e)
	// Clearing adoption is permitted after all live entry links are explicitly disabled.
	rows, e := f.env.Pool.Query(ctx, `SELECT id::text,terminal_id::text FROM ingestion_device_associations WHERE device_id=$1 AND enabled AND end_sequence IS NULL`, f.device)
	must(t, e)
	type link struct{ id, terminal string }
	remaining := []link{}
	for rows.Next() {
		var v link
		must(t, rows.Scan(&v.id, &v.terminal))
		remaining = append(remaining, v)
	}
	rows.Close()
	must(t, rows.Err())
	for _, v := range remaining {
		terminal, e := f.s.Terminal(ctx, f.owner, scope, v.terminal)
		must(t, e)
		_, e = f.s.AssociationState(ctx, f.owner, scope, v.id, ig.Command{Meta: f.meta(t), Revision: terminal.Revision, Action: "disable"})
		must(t, e)
	}
	cmd.Meta = f.meta(t)
	preview, e = f.ps.Preview(ctx, f.owner, scope, f.device, cmd)
	must(t, e)
	if !preview.CanSubmit {
		t.Fatal("disabled links still block an explicit adoption change")
	}
	_, e = f.ps.Command(ctx, f.owner, scope, f.device, cmd)
	must(t, e)
	term, e = f.s.Terminal(ctx, f.owner, scope, f.terminal)
	must(t, e)
	_, e = f.s.AssociationState(ctx, f.owner, scope, links[0].ID, ig.Command{Meta: f.meta(t), Revision: term.Revision, Action: "enable"})
	if e == nil {
		t.Fatal("incompatible association restored after adoption changed")
	}
	t.Log("205-byte gateway independent A/B views, unknown C isolation, fixed methods, terminal history and disable-change-restore adoption guard verified")
}

func TestIngestionMachineGrantAndLateResponseFence(t *testing.T) {
	f := setupIngestion(t)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, f.owner.ID)
	must(t, e)
	f.work(t, 12)
	var results int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
	if results != 2 {
		t.Fatal("machine grant borrowed human lifecycle", results)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='active' WHERE id=$1`, f.owner.ID)
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	_, e = f.s.IdentifyOne(ctx)
	must(t, e)
	_, e = f.s.PlanOne(ctx)
	must(t, e)
	_, e = f.s.PlanOne(ctx)
	must(t, e)
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, e)
	f.s.WithStorage(&faultStore{ObjectStore: store, after: func(int) error {
		_, e := f.env.Pool.Exec(ctx, `UPDATE ingestion_parse_plans SET fence=fence+1,lease_until=clock_timestamp()-interval '1 second' WHERE state='parsing'`)
		return e
	}})
	_, e = f.s.ParseOne(ctx)
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
	if results != 2 {
		t.Fatal("late fence committed result")
	}
	f.s.WithStorage(store)
	f.work(t, 6)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
	if results != 4 {
		t.Fatal("expired plan did not recover", results)
	}
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	_, e = f.s.IdentifyOne(ctx)
	must(t, e)
	_, e = f.s.PlanOne(ctx)
	must(t, e)
	_, e = f.s.PlanOne(ctx)
	must(t, e)
	f.s.WithStorage(&faultStore{ObjectStore: store, after: func(int) error {
		grants, e := f.s.Grants(ctx, f.owner, c.TenantScope(f.tenant), ig.Filter{SourceID: f.httpSource})
		if e != nil {
			return e
		}
		for _, g := range grants {
			if g.TerminalID == nil {
				_, e = f.s.SaveGrant(ctx, f.owner, c.TenantScope(f.tenant), ig.GrantInput{Meta: f.meta(t), Revision: g.Generation, SourceID: f.httpSource, Identify: true, Enabled: false, Budget: g.Budget})
				return e
			}
		}
		return errors.New("missing grant")
	}})
	_, e = f.s.ParseOne(ctx)
	must(t, e)
	f.s.WithStorage(store)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
	if results != 4 {
		t.Fatal("revoked machine grant committed result")
	}
	var blocked int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_parse_plans WHERE state='blocked'`).Scan(&blocked))
	if blocked < 1 {
		t.Fatal("revocation not recorded")
	}
	for _, q := range []string{`UPDATE ingestion_identity_decisions SET method='{}'`, `UPDATE ingestion_units SET view='{}'`, `DELETE FROM ingestion_results`, `UPDATE ingestion_parse_plans SET snapshot='{}'`} {
		if _, e = f.env.Pool.Exec(ctx, q); e == nil {
			t.Fatal("mutable frozen evidence")
		}
	}
	t.Log("human disable leaves machine grant valid; expired workers cannot commit; post-call machine revocation blocks result; frozen decisions and plans reject mutation")
}

func TestIngestionRecoveryTaskGenerationsRevocationAndAuditRollback(t *testing.T) {
	f := setupIngestion(t)
	scope := c.TenantScope(f.tenant)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 12)
	var unit string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE device_id=$1`, f.device).Scan(&unit))
	job, e := f.s.Recover(ctx, f.owner, scope, ig.RecoveryInput{Meta: f.meta(t), Kind: "reparse", UnitIDs: []string{unit}}, false)
	must(t, e)
	_, e = f.s.RecoveryOne(ctx)
	must(t, e)
	_, e = f.s.IdentifyOne(ctx)
	must(t, e)
	for n := 0; n < 4; n++ {
		_, e = f.s.PlanOne(ctx)
		must(t, e)
	}
	ref := taskrecord.SourceRef{Module: "ingestion", Kind: "recovery", RootID: job.ID}
	control := func(action, generation string) {
		t.Helper()
		current, e := f.s.RecoveryGeneration(ctx, f.owner, scope, job.ID, generation)
		must(t, e)
		_, e = f.s.TaskControl(ctx, f.owner, ref, tasks.Command{RequestID: f.meta(t).RequestID, Action: action, ExpectedGeneration: generation, ExpectedRevision: current.Revision, Reason: "固定范围代次验收"})
		must(t, e)
	}
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, e)
	f.s.WithStorage(&faultStore{ObjectStore: store, after: func(int) error { control("cancel", "1"); control("retry", "1"); return nil }})
	_, e = f.s.ParseOne(ctx)
	must(t, e)
	f.s.WithStorage(store)
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&count))
	if count != 2 {
		t.Fatal("late cancelled generation committed", count)
	}
	old, e := f.s.RecoveryGeneration(ctx, f.owner, scope, job.ID, "1")
	must(t, e)
	if old.State != "cancelled" {
		t.Fatal("old generation overwritten", old.State)
	}
	for n := 0; n < 12; n++ {
		_, e = f.s.RecoveryOne(ctx)
		must(t, e)
		f.work(t, 2)
	}
	newer, e := f.s.RecoveryGeneration(ctx, f.owner, scope, job.ID, "2")
	must(t, e)
	if newer.State != "succeeded" {
		t.Fatal("new generation failed", newer)
	}
	old, e = f.s.RecoveryGeneration(ctx, f.owner, scope, job.ID, "1")
	must(t, e)
	if old.State != "cancelled" {
		t.Fatal("new attempt overwrote history")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&count))
	if count != 3 {
		t.Fatal("fixed unit set expanded", count)
	}
	revoked, e := f.s.Recover(ctx, f.owner, scope, ig.RecoveryInput{Meta: f.meta(t), Kind: "reparse", UnitIDs: []string{unit}}, false)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, f.owner.ID)
	must(t, e)
	if _, e = f.s.RecoveryOne(ctx); e == nil {
		t.Fatal("disabled operator executed")
	}
	if _, e = f.s.RecoveryGeneration(ctx, f.owner, scope, revoked.ID, "1"); e == nil {
		t.Fatal("disabled operator read deep link")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='active' WHERE id=$1`, f.owner.ID)
	must(t, e)
	var state string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM ingestion_recovery_jobs WHERE id=$1`, revoked.ID).Scan(&state))
	if state != "failed" {
		t.Fatal("revocation not terminal", state)
	}
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION ingestion_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.event_type LIKE 'ingestion.%' THEN RAISE EXCEPTION 'synthetic audit failure';END IF;RETURN NEW;END$$;CREATE TRIGGER ingestion_audit_failure BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION ingestion_audit_failure()`)
	must(t, e)
	_, e = f.s.PlanningState(ctx, f.owner, scope, f.httpSource, ig.Command{Meta: f.meta(t), Revision: "1", Action: "pause"})
	if e == nil {
		t.Fatal("missing audit did not abort")
	}
	var paused bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT planning_paused FROM ingestion_sources WHERE id=$1`, f.httpSource).Scan(&paused))
	if paused {
		t.Fatal("partial business commit")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER ingestion_audit_failure ON account_security_events;DROP FUNCTION ingestion_audit_failure()`)
	must(t, e)
	t.Log("cancel and retry preserve generation 1, late response cannot enter generation 2; one fixed unit yields one new result; revoked actor stops and audit failure rolls back business state")
}

func TestIngestionAssociationWatermarkAndRestoredEpochIsolation(t *testing.T) {
	f := setupIngestion(t)
	scope := c.TenantScope(f.tenant)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	var terminal string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_terminals WHERE source_id=$1 AND terminal_key='00000000001'`, f.httpSource).Scan(&terminal))
	if _, e = f.s.CreateTerminal(ctx, f.owner, scope, ig.TerminalInput{Meta: f.meta(t), SourceID: f.httpSource, Key: "00000000001", Name: "重复", Mode: "single"}); e == nil {
		t.Fatal("duplicate terminal")
	}
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send before switch")
	}
	term, e := f.s.Terminal(ctx, f.owner, scope, terminal)
	must(t, e)
	change, e := f.s.ChangeAssociation(ctx, f.owner, scope, terminal, ig.AssociationInput{Meta: f.meta(t), Revision: term.Revision, DeviceID: f.device2, InputView: "complete_frame"}, false)
	must(t, e)
	if change.State != "pending_watermark" {
		t.Fatal("watermark intent missing", change)
	}
	_, e = f.s.ChangeOne(ctx)
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send after switch")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 20)
	rows, e := f.env.Pool.Query(ctx, `SELECT r.source_sequence,u.device_id::text,p.snapshot->'parsing'->'entry'->>'method_version' FROM ingestion_units u JOIN ingestion_receipts r ON r.id=u.receipt_id JOIN ingestion_parse_plans p ON p.unit_id=u.id WHERE u.terminal_id=$1 ORDER BY r.source_sequence`, terminal)
	must(t, e)
	seen := 0
	for rows.Next() {
		var sequence int
		var device, version string
		must(t, rows.Scan(&sequence, &device, &version))
		seen++
		if sequence == 1 && (device != f.device || version != "1.0.0") || sequence == 2 && (device != f.device2 || version != "1.1.0") {
			t.Fatal("watermark routed old backlog incorrectly", sequence, device, version)
		}
	}
	rows.Close()
	if seen != 2 {
		t.Fatal(seen)
	}
	links, e := f.s.Associations(ctx, f.owner, scope, terminal)
	must(t, e)
	if len(links) != 2 {
		t.Fatal("association history")
	}
	// A restore rotates new source sequences while retaining the prior timeline.
	raw, e := os.ReadFile(os.Getenv("INFRAPRO_INGESTION_TEST_RELAY_CONFIG"))
	must(t, e)
	var cfg struct {
		DatabaseURL string `json:"database_url"`
	}
	must(t, json.Unmarshal(raw, &cfg))
	cloud, e := pgxpool.New(ctx, cfg.DatabaseURL)
	must(t, e)
	defer cloud.Close()
	epoch := uuid.NewString()
	_, e = cloud.Exec(ctx, `UPDATE relay_state SET spool_epoch=$1 WHERE id=1`, epoch)
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send restored")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 3)
	var receipt, state, reason string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,state,error_code FROM ingestion_receipts WHERE epoch=$1`, epoch).Scan(&receipt, &state, &reason))
	if state != "quarantined" || reason != "epoch_unconfirmed" {
		t.Fatal("new epoch auto trusted", state, reason)
	}
	if _, e = f.s.Content(ctx, f.owner, scope, receipt); e == nil {
		t.Fatal("restored root exposed")
	}
	confirmation, e := f.s.Recover(ctx, f.actor(), c.PlatformScope(), ig.RecoveryInput{Meta: f.meta(t), Kind: "confirm_epoch", RelayID: f.opts.Relays[0].ID, Epoch: epoch}, false)
	must(t, e)
	_, e = f.s.RecoveryOne(ctx)
	must(t, e)
	confirmed, e := f.s.RecoveryGeneration(ctx, f.actor(), c.PlatformScope(), confirmation.ID, "1")
	must(t, e)
	if confirmed.State != "succeeded" {
		t.Fatal("epoch task")
	}
	_, e = f.s.Recover(ctx, f.actor(), c.PlatformScope(), ig.RecoveryInput{Meta: f.meta(t), Kind: "reconcile", ReceiptIDs: []string{receipt}}, false)
	must(t, e)
	_, e = f.s.RecoveryOne(ctx)
	must(t, e)
	f.work(t, 8)
	var assigned int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_units WHERE receipt_id=$1 AND device_id IS NOT NULL`, receipt).Scan(&assigned))
	if assigned != 0 {
		t.Fatal("old epoch association leaked into new timeline")
	}
	for _, key := range []string{"00000000001", "00000000002"} {
		var id string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_terminals WHERE source_id=$1 AND terminal_key=$2`, f.httpSource, key).Scan(&id))
		term, e = f.s.Terminal(ctx, f.owner, scope, id)
		must(t, e)
		_, e = f.s.ChangeAssociation(ctx, f.owner, scope, id, ig.AssociationInput{Meta: f.meta(t), Revision: term.Revision, DeviceID: f.device2, InputView: "complete_frame"}, false)
		must(t, e)
		_, e = f.s.ChangeOne(ctx)
		must(t, e)
	}
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send new confirmed range")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 12)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results result JOIN ingestion_parse_plans p ON p.id=result.plan_id JOIN ingestion_units u ON u.id=p.unit_id JOIN ingestion_receipts r ON r.id=u.receipt_id WHERE r.epoch=$1`, epoch).Scan(&assigned))
	if assigned != 2 {
		t.Fatal("new confirmed range", assigned)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_units WHERE receipt_id=$1 AND device_id IS NOT NULL`, receipt).Scan(&assigned))
	if assigned != 0 {
		t.Fatal("registration opened earlier epoch payload")
	}
	t.Log("durable N+1 association boundary routes pre-switch backlog to V1 and later frames to V2; new epoch quarantines, exact task confirms, old timeline cannot authorize new sequence and history remains closed")
}

func TestIngestionSeparatedRawResultPermissionsAndIdentityResponse(t *testing.T) {
	f := setupIngestion(t)
	scope := c.TenantScope(f.tenant)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 12)
	var unit string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE device_id=$1`, f.device).Scan(&unit))
	_, e = f.env.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_code=$1`, access.TenantIngestionRawRead)
	must(t, e)
	detail, e := f.s.Message(ctx, f.owner, scope, unit)
	must(t, e)
	if detail.CanRaw || !detail.CanResults {
		t.Fatal("permission flags")
	}
	if _, e = f.s.Content(ctx, f.owner, scope, unit); e == nil {
		t.Fatal("metadata role read raw")
	}
	views, _, e := f.s.Results(ctx, f.owner, scope, unit, "")
	must(t, e)
	if len(views) != 1 {
		t.Fatal("structured result unavailable")
	}
	for _, field := range views[0].Response.Fields {
		if field.RawText != "" {
			t.Fatal("raw field leaked")
		}
	}
	for _, record := range views[0].Response.Records {
		for _, m := range record.Measurements {
			if m.RawText != "" {
				t.Fatal("raw measurement leaked")
			}
		}
	}
	if _, _, e = f.s.Results(ctx, f.owner, scope, unit, views[0].ID); e == nil {
		t.Fatal("raw result bypass")
	}
	_, e = f.env.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_code IN ($1,$2)`, access.TenantParsingRunRead, access.TenantDeviceParsingManage)
	must(t, e)
	if _, _, e = f.s.Results(ctx, f.owner, scope, unit, ""); e == nil {
		t.Fatal("metadata role read result")
	}
	if _, e = f.ps.Command(ctx, f.owner, scope, f.device, parse.Command{Meta: f.meta(t), Revision: "1", Action: "clear"}); e == nil {
		t.Fatal("ingestion role changed adoption")
	}
	// Production uses the shared validator and an additional frozen external-identity check.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		upstream, e := http.NewRequest(request.Method, os.Getenv("INFRAPRO_PARSING_URL")+request.URL.Path, request.Body)
		if e != nil {
			http.Error(w, "test proxy", 500)
			return
		}
		upstream.Header = request.Header.Clone()
		response, e := http.DefaultClient.Do(upstream)
		if e != nil {
			http.Error(w, "test proxy", 500)
			return
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if request.URL.Path == "/v1/parse" && response.StatusCode == 200 {
			var value wire.Response
			if json.Unmarshal(raw, &value) == nil && len(value.Identities) > 0 {
				value.Identities[0].Text = "00000000099"
				raw, _ = json.Marshal(value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(raw)
	}))
	defer proxy.Close()
	f.ps.WithOptions(f.options(proxy.URL))
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send forged response")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 10)
	var results, mismatches int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ingestion_results),(SELECT count(*) FROM ingestion_parse_plans WHERE error_code='parsed_identity_mismatch')`).Scan(&results, &mismatches))
	if results != 2 || mismatches != 2 {
		t.Fatal("forged identity produced result", results, mismatches)
	}
	t.Log("metadata, raw bytes and parsed results require separate current permissions; ingestion management cannot change adoption; forged production identity produces explicit mismatch and no valid result")
}
