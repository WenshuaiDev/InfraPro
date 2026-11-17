package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/services/ingest-relay/internal/spool"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

var testCtx = context.Background()

func check(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func port(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	check(t, e)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func cloudFixture(t *testing.T) (*Server, *tls.Config, func(string) *http.Client) {
	t.Helper()
	path := os.Getenv("INFRAPRO_INGESTION_TEST_RELAY_CONFIG")
	if path == "" {
		t.Skip("isolated cloud PG and certificates required")
	}
	cfg, e := ReadConfig(path)
	check(t, e)
	cfg.RelayID = uuid.NewString()
	u, e := url.Parse(cfg.DatabaseURL)
	check(t, e)
	schema := "cloud_" + uuid.NewString()
	schema = "cloud_" + c.Digest([]byte(schema))[:16]
	admin, e := pgxpool.New(testCtx, cfg.DatabaseURL)
	check(t, e)
	_, e = admin.Exec(testCtx, "CREATE SCHEMA "+schema)
	check(t, e)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	cfg.DatabaseURL = u.String()
	pool, e := pgxpool.New(testCtx, cfg.DatabaseURL)
	check(t, e)
	store := &spool.Store{Pool: pool, RelayID: cfg.RelayID, Budget: 64 << 20}
	check(t, store.Migrate(testCtx))
	s := New(cfg, store)
	t.Cleanup(func() { s.Close(); pool.Close(); admin.Exec(testCtx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	directory := filepath.Dir(path)
	ca, e := os.ReadFile(cfg.CA)
	check(t, e)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	files := func(name string) *tls.Config {
		cert, e := tls.LoadX509KeyPair(filepath.Join(directory, name+".pem"), filepath.Join(directory, name+".key"))
		check(t, e)
		return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{cert}}
	}
	client := func(name string) *http.Client {
		return &http.Client{Transport: &http.Transport{TLSClientConfig: files(name)}, Timeout: 5 * time.Second}
	}
	return s, files("producer"), client
}
func source(t *testing.T, s *Server, transport string) r.Source {
	t.Helper()
	v := r.Source{IngressID: uuid.NewString(), NamespaceID: uuid.NewString(), Revision: 1, RelayID: s.Config.RelayID, TenantID: uuid.NewString(), ProjectID: uuid.NewString(), ManufacturerID: uuid.NewString(), ManufacturerScope: "public", ProtocolID: "example", ProtocolVersion: "1.0.0", ProtocolDigest: c.Digest([]byte("fixed-test-protocol")), Transport: transport, Address: "127.0.0.1", Port: port(t), SourceCode: "sample", ProducerIDs: []string{"lab-producer"}, Active: true, Synthetic: true, IdleSeconds: 5, Budget: 16 << 20}
	v.Adapter = i.Adapter{ID: ddp.AdapterID, Version: ddp.AdapterVersion, ArtifactDigest: c.Digest([]byte(ddp.AdapterSource))}
	if transport == "https_push" {
		v.Adapter = i.Adapter{ID: "https.raw-body", Version: "1.0.0", ArtifactDigest: c.Digest([]byte("https.raw-body/v1:mtls,4MiB,raw-body,source-scoped-idempotency"))}
	}
	v.Digest = r.SourceDigest(v)
	out, e := s.Apply(testCtx, r.Apply{CommandID: uuid.NewString(), Source: v})
	check(t, e)
	if out.State != "active" {
		t.Fatal(out)
	}
	return v
}
func post(t *testing.T, client *http.Client, url, key string, b []byte) (int, map[string]any) {
	t.Helper()
	req, e := http.NewRequest("POST", url, bytes.NewReader(b))
	check(t, e)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Content-Type", "application/json")
	res, e := client.Do(req)
	check(t, e)
	defer res.Body.Close()
	out := map[string]any{}
	check(t, json.NewDecoder(res.Body).Decode(&out))
	return res.StatusCode, out
}
func TestCloudDurabilityLeasesCapacityAndRestart(t *testing.T) {
	s, tlsConfig, clients := cloudFixture(t)
	httpSource := source(t, s, "https_push")
	tcpSource := source(t, s, "ddp_tcp")
	endpoint := "https://127.0.0.1:" + strconv.Itoa(httpSource.Port) + "/v1/push/sample"
	client := clients("producer")
	defer client.CloseIdleConnections()
	status, first := post(t, client, endpoint, "one", []byte("exact bytes\x00"))
	if status != 202 {
		t.Fatal(status, first)
	}
	status, again := post(t, client, endpoint, "one", []byte("exact bytes\x00"))
	if status != 202 || first["receipt_id"] != again["receipt_id"] {
		t.Fatal("duplicate receipt")
	}
	status, _ = post(t, client, endpoint, "one", []byte("different"))
	if status != 409 {
		t.Fatal(status)
	}
	status, _ = post(t, client, endpoint+"-other", "two", []byte("x"))
	if status != 403 {
		t.Fatal(status)
	}
	status, _ = post(t, clients("consumer"), endpoint, "two", []byte("x"))
	if status != 403 {
		t.Fatal(status)
	}
	status, _ = post(t, clients("outsider"), endpoint, "two", []byte("x"))
	if status != 403 {
		t.Fatal(status)
	}
	// A database rejection must not send a successful HTTP or DDP confirmation.
	_, e := s.Store.Pool.Exec(testCtx, `CREATE FUNCTION refuse_receipt() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'injected write failure';END$$;CREATE TRIGGER refuse_receipt BEFORE INSERT ON relay_receipts FOR EACH ROW EXECUTE FUNCTION refuse_receipt()`)
	check(t, e)
	status, _ = post(t, client, endpoint, "failure", []byte("not committed"))
	if status != 503 {
		t.Fatal("ack before commit", status)
	}
	_, e = s.Store.Pool.Exec(testCtx, `DROP TRIGGER refuse_receipt ON relay_receipts`)
	check(t, e)
	conn, e := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpSource.Port)), tlsConfig)
	check(t, e)
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	reg, e := ddp.Encode(1, "00000000001", make([]byte, 6))
	check(t, e)
	for _, v := range reg {
		_, e = conn.Write([]byte{v})
		check(t, e)
	}
	ack, e := ddp.Read(conn)
	check(t, e)
	if ack[1] != 0x81 {
		t.Fatal("registration")
	}
	frame, e := ddp.Encode(9, "00000000001", []byte("a{b"))
	check(t, e)
	_, e = conn.Write(append(append([]byte{}, frame...), frame...))
	check(t, e)
	for n := 0; n < 2; n++ {
		ack, e = ddp.Read(conn)
		check(t, e)
		if ack[1] != 0x85 {
			t.Fatal("measurement ack")
		}
	}
	pull := r.Pull{RequestID: uuid.NewString(), Limit: 100, MaxBytes: r.MaxPullBytes}
	delivery, e := s.Store.Pull(testCtx, pull)
	check(t, e)
	if len(delivery.Items) != 4 {
		t.Fatal("durable facts", len(delivery.Items))
	}
	for _, v := range delivery.Items {
		b, e := r.Decode(v)
		check(t, e)
		if c.Digest(b) != v.SHA256 {
			t.Fatal("raw drift")
		}
	}
	item := delivery.Items[0]
	a := r.Ack{RelayID: item.RelayID, Epoch: item.Epoch, ReceiptID: item.ReceiptID, LeaseToken: item.LeaseToken, SHA256: item.SHA256, LocalReceiptID: uuid.NewString()}
	result, e := s.Store.Ack(testCtx, a)
	check(t, e)
	if result.State != "acknowledged" {
		t.Fatal(result)
	}
	_, e = s.Store.Ack(testCtx, a)
	check(t, e)
	_, e = s.Store.Pool.Exec(testCtx, `UPDATE relay_deliveries SET lease_until=clock_timestamp()-interval '1 second' WHERE state='leased'`)
	check(t, e)
	redelivery, e := s.Store.Pull(testCtx, pull)
	check(t, e)
	if len(redelivery.Items) != 3 {
		t.Fatal("partial ack lost hole", len(redelivery.Items))
	}
	stale := delivery.Items[1]
	_, e = s.Store.Ack(testCtx, r.Ack{RelayID: stale.RelayID, Epoch: stale.Epoch, ReceiptID: stale.ReceiptID, LeaseToken: stale.LeaseToken, SHA256: stale.SHA256, LocalReceiptID: uuid.NewString()})
	if e == nil {
		t.Fatal("expired lease accepted")
	}
	markIn := r.WatermarkRequest{CommandID: uuid.NewString(), IngressID: tcpSource.IngressID}
	mark, e := s.Store.Watermark(testCtx, markIn)
	check(t, e)
	_, _, e = s.Store.Receive(testCtx, spool.Receive{Source: tcpSource, Principal: "lab-producer", Raw: frame})
	check(t, e)
	mark2, e := s.Store.Watermark(testCtx, markIn)
	check(t, e)
	if mark != mark2 {
		t.Fatal("watermark replay changed")
	}
	check(t, s.Store.Cleanup(testCtx))
	var retained int
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT count(*) FROM relay_receipts WHERE payload IS NOT NULL`).Scan(&retained))
	if retained != 5 {
		t.Fatal("unacked/recent content removed")
	}
	_, e = s.Store.Pool.Exec(testCtx, `UPDATE relay_ack_receipts SET acknowledged_at=clock_timestamp()-interval '8 days'`)
	check(t, e)
	check(t, s.Store.Cleanup(testCtx))
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT count(*) FROM relay_receipts WHERE payload IS NOT NULL`).Scan(&retained))
	if retained != 4 {
		t.Fatal("retention", retained)
	}
	status, again = post(t, client, endpoint, "one", []byte("exact bytes\x00"))
	if status != 202 || first["receipt_id"] != again["receipt_id"] {
		t.Fatal("idempotency index removed")
	}
	oldEpoch := mark.Epoch
	s.Close()
	s = New(s.Config, s.Store)
	defer s.Close()
	check(t, s.Restore(testCtx))
	next, e := s.Store.Watermark(testCtx, r.WatermarkRequest{CommandID: uuid.NewString(), IngressID: tcpSource.IngressID})
	check(t, e)
	if next.Epoch != oldEpoch || next.Sequence != mark.Sequence+1 {
		t.Fatal("restart reset cursor")
	}
	s.Store.Budget = 1
	status, _ = post(t, client, endpoint, "capacity", []byte("new"))
	if status != 503 {
		t.Fatal("capacity false acceptance")
	}
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT count(*) FROM relay_receipts`).Scan(&retained))
	if retained != 5 {
		t.Fatal("capacity deleted facts")
	}
	t.Log("durable commit failure, split/coalesced DDP, exact raw bytes, scoped HTTP keys, partial leases, stable watermark, restart, seven-day retention and capacity rejection verified")
}

func TestCloudConnectionIdentityAndControlRoles(t *testing.T) {
	s, tlsConfig, clients := cloudFixture(t)
	src := source(t, s, "ddp_tcp")
	addr := net.JoinHostPort(src.Address, strconv.Itoa(src.Port))
	dial := func() *tls.Conn {
		conn, e := tls.Dial("tcp", addr, tlsConfig)
		check(t, e)
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	frame, _ := ddp.Encode(9, "00000000001", []byte("x"))
	conn := dial()
	conn.Write(frame)
	if _, e := ddp.Read(conn); e == nil {
		t.Fatal("unregistered data accepted")
	}
	reg, _ := ddp.Encode(1, "00000000001", make([]byte, 6))
	live := dial()
	live.Write(reg)
	_, e := ddp.Read(live)
	check(t, e)
	duplicate := dial()
	duplicate.Write(reg)
	if _, e = ddp.Read(duplicate); e == nil {
		t.Fatal("active session conflict accepted")
	}
	other, _ := ddp.Encode(9, "00000000002", []byte("x"))
	live.Write(other)
	if _, e = ddp.Read(live); e == nil {
		t.Fatal("connection switched identity")
	}
	invalid := dial()
	invalid.Write([]byte{0, 0, 0, 0})
	if _, e = ddp.Read(invalid); e == nil {
		t.Fatal("invalid prefix scanned past")
	}
	tlsServer, e := s.Config.TLS()
	check(t, e)
	listener, e := tls.Listen("tcp", "127.0.0.1:0", tlsServer)
	check(t, e)
	httpServer := &http.Server{Handler: s, ReadHeaderTimeout: time.Second}
	go httpServer.Serve(listener)
	defer httpServer.Close()
	endpoint := "https://" + listener.Addr().String()
	status, _ := post(t, clients("producer"), endpoint+"/v1/consumers/lab/pull", "", []byte(`{"request_id":"x","limit":100,"max_bytes":8388608,"wait_seconds":0}`))
	if status != 401 {
		t.Fatal("producer can consume", status)
	}
	status, _ = post(t, clients("consumer"), endpoint+"/v1/control/source-watermarks", "", []byte(`{}`))
	if status != 403 {
		t.Fatal("consumer can configure", status)
	}
	var count int
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT count(*) FROM relay_receipts WHERE envelope->>'evidence_kind'='invalid_fragment'`).Scan(&count))
	if count < 4 {
		t.Fatal("missing fragment evidence", count)
	}
	// An occupied endpoint is a failed revision, never an active listener.
	occupied, e := net.Listen("tcp", "127.0.0.1:0")
	check(t, e)
	defer occupied.Close()
	conflict := src
	conflict.IngressID = uuid.NewString()
	conflict.NamespaceID = uuid.NewString()
	conflict.Port = occupied.Addr().(*net.TCPAddr).Port
	conflict.Digest = r.SourceDigest(conflict)
	out, e := s.Apply(testCtx, r.Apply{CommandID: uuid.NewString(), Source: conflict})
	check(t, e)
	if out.State != "failed" {
		t.Fatal("bind failure reported active", out)
	}
	_ = io.Discard
}

func TestCloudRevisionFencesCredentialRotationAndInvalidBudget(t *testing.T) {
	s, tlsConfig, clients := cloudFixture(t)
	original, e := os.ReadFile(s.Config.PrincipalsFile)
	check(t, e)
	s.Config.PrincipalsFile = filepath.Join(t.TempDir(), "principals.json")
	check(t, os.WriteFile(s.Config.PrincipalsFile, original, 0600))
	tcp := source(t, s, "ddp_tcp")
	dial := func(key string) *tls.Conn {
		t.Helper()
		conn, e := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(tcp.Port)), tlsConfig)
		check(t, e)
		t.Cleanup(func() { conn.Close() })
		conn.SetDeadline(time.Now().Add(8 * time.Second))
		b, _ := ddp.Encode(1, key, make([]byte, 6))
		_, e = conn.Write(b)
		check(t, e)
		_, e = ddp.Read(conn)
		check(t, e)
		return conn
	}
	old := dial("00000000001")
	next := tcp
	next.Revision = 2
	next.ProtocolVersion = "1.1.0"
	next.Digest = r.SourceDigest(next)
	command := r.Apply{CommandID: uuid.NewString(), ExpectedGeneration: 1, Source: next}
	applied, e := s.Apply(testCtx, command)
	check(t, e)
	replay, e := s.Apply(testCtx, command)
	check(t, e)
	if replay.Generation != applied.Generation || replay.Digest != applied.Digest {
		t.Fatal("lost command response not idempotent")
	}
	altered := command
	altered.Source.SourceCode = "changed"
	altered.Source.Digest = r.SourceDigest(altered.Source)
	if _, e = s.Apply(testCtx, altered); e == nil {
		t.Fatal("same command different body")
	}
	stale := command
	stale.CommandID = uuid.NewString()
	if _, e = s.Apply(testCtx, stale); e == nil {
		t.Fatal("out of order command")
	}
	fresh := dial("00000000002")
	for n, conn := range []*tls.Conn{old, fresh} {
		b, _ := ddp.Encode(9, fmt.Sprintf("%011d", n+1), []byte("exact"))
		_, e = conn.Write(b)
		check(t, e)
		_, e = ddp.Read(conn)
		check(t, e)
	}
	var v1, v2 int
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT count(*) FILTER(WHERE envelope->>'source_revision'='1'),count(*) FILTER(WHERE envelope->>'source_revision'='2') FROM relay_receipts WHERE envelope->>'ingress_id'=$1`, tcp.IngressID).Scan(&v1, &v2))
	if v1 != 2 || v2 != 2 {
		t.Fatal("connection revision drift", v1, v2)
	}
	// Live sockets are checked against the current fingerprint registry on each frame.
	var registry map[string]Principal
	check(t, json.Unmarshal(original, &registry))
	producer := c.Digest(tlsConfig.Certificates[0].Certificate[0])
	p := registry[producer]
	delete(registry, producer)
	outsider := clients("outsider")
	defer outsider.CloseIdleConnections()
	cert := outsider.Transport.(*http.Transport).TLSClientConfig.Certificates[0].Certificate[0]
	registry[c.Digest(cert)] = p
	b, e := json.Marshal(registry)
	check(t, e)
	check(t, os.WriteFile(s.Config.PrincipalsFile, b, 0600))
	frame, _ := ddp.Encode(9, "00000000001", []byte("revoked"))
	_, _ = old.Write(frame)
	if _, e = ddp.Read(old); e == nil {
		t.Fatal("revoked live certificate acknowledged")
	}
	httpSource := source(t, s, "https_push")
	endpoint := "https://127.0.0.1:" + strconv.Itoa(httpSource.Port) + "/v1/push/sample"
	if status, _ := post(t, clients("producer"), endpoint, "revoked", []byte("x")); status != 403 {
		t.Fatal("revoked producer", status)
	}
	if status, _ := post(t, outsider, endpoint, "rotated", []byte("x")); status != 202 {
		t.Fatal("rotated producer", status)
	}
	check(t, os.WriteFile(s.Config.PrincipalsFile, original, 0600))
	// An invalid-fragment storm exhausts only its small separate allowance.
	bad := source(t, s, "ddp_tcp")
	raw := bytes.Repeat([]byte{0}, 8192)
	rejected := false
	for n := 0; n < 100; n++ {
		_, _, err := s.Store.Receive(testCtx, spool.Receive{Source: bad, Principal: "lab-producer", Raw: raw, Invalid: true, Truncated: true})
		if err != nil {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Fatal("unbounded fragment budget")
	}
	frame, _ = ddp.Encode(9, "00000000001", []byte("normal after invalid quota"))
	saved, _, e := s.Store.Receive(testCtx, spool.Receive{Source: bad, Principal: "lab-producer", Raw: frame})
	check(t, e)
	var accounted int64
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT accounted_bytes FROM relay_receipts WHERE receipt_id=$1`, saved.ReceiptID).Scan(&accounted))
	if accounted <= int64(len(frame)) {
		t.Fatal("metadata excluded from physical budget")
	}
	oldEpoch := saved.Epoch
	_, e = s.Store.Pool.Exec(testCtx, `UPDATE relay_state SET spool_epoch=$1 WHERE id=1`, uuid.NewString())
	check(t, e)
	rotated, _, e := s.Store.Receive(testCtx, spool.Receive{Source: bad, Principal: "lab-producer", Raw: frame})
	check(t, e)
	if rotated.Epoch == oldEpoch || rotated.Sequence != 1 {
		t.Fatal("restored sequence reused identity")
	}
	// Pausing closes existing sockets and releases the listener to a new namespace.
	paused := next
	paused.Revision++
	paused.Active = false
	paused.Digest = r.SourceDigest(paused)
	_, e = s.Apply(testCtx, r.Apply{CommandID: uuid.NewString(), ExpectedGeneration: 2, Source: paused})
	check(t, e)
	replacement := paused
	replacement.IngressID = uuid.NewString()
	replacement.NamespaceID = uuid.NewString()
	replacement.ProjectID = uuid.NewString()
	replacement.Revision = 1
	replacement.Active = true
	replacement.Digest = r.SourceDigest(replacement)
	_, e = s.Apply(testCtx, r.Apply{CommandID: uuid.NewString(), Source: replacement})
	check(t, e)
	var oldProject string
	check(t, s.Store.Pool.QueryRow(testCtx, `SELECT envelope->>'source_project_id' FROM relay_receipts WHERE envelope->>'ingress_id'=$1 LIMIT 1`, tcp.IngressID).Scan(&oldProject))
	if oldProject != tcp.ProjectID {
		t.Fatal("port reuse rewrote history")
	}
}
