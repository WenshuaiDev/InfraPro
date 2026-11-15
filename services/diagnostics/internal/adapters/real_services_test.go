package adapters

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// These tests require the explicitly created, owned acceptance lab. They never
// inspect business configuration or alter resources discovered on the host.
func TestRealFixedProtocols(t *testing.T) {
	dir := os.Getenv("INFRAPRO_DIAGNOSTICS_LAB_DIRECTORY")
	if dir == "" {
		t.Skip("explicit owned lab required")
	}
	var lab struct {
		RunID        string   `json:"run_id"`
		ManifestFile string   `json:"manifest_file"`
		Faults       []string `json:"faults"`
	}
	load := func(path string, out any) {
		t.Helper()
		b, e := os.ReadFile(path)
		if e != nil || json.Unmarshal(b, out) != nil {
			t.Fatal("cannot read owned fixture")
		}
	}
	load(filepath.Join(dir, "lab.json"), &lab)
	if !strings.HasPrefix(lab.RunID, "diag-") || len(lab.Faults) > 0 {
		t.Fatal("lab not recovered")
	}
	var m settings.Manifest
	var bindings settings.Bindings
	load(lab.ManifestFile, &m)
	load(filepath.Join(dir, "native-b/bindings.json"), &bindings)
	runtime := New(m, bindings, uuid.NewString())
	results := []map[string]any{}
	probe := func(b settings.Binding, expected string) model.Observation {
		t.Helper()
		p := settings.Probe{ID: b.ID, NodeID: bindings.NodeID, Adapter: b.Adapter, Facet: "readiness"}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		o := runtime.Collect(ctx, p, b)
		if o.ReasonCode != expected {
			t.Fatalf("%s expected %s, received %s/%s", b.Adapter, expected, o.Outcome, o.ReasonCode)
		}
		results = append(results, map[string]any{"adapter": b.Adapter, "expected": expected, "observation": o})
		return o
	}
	temp := testfixture.Directory(t)
	secret := func(name string, c settings.Credential) string {
		t.Helper()
		p := filepath.Join(temp, name)
		if e := settings.Save(p, c, true); e != nil {
			t.Fatal(e)
		}
		return p
	}
	wrong := secret("wrong.json", settings.Credential{SchemaVersion: 1, Kind: "password", Password: "diagnostics-invalid-fixed-fixture"})
	byID := map[string]settings.Binding{}
	for _, b := range bindings.Bindings {
		byID[b.ID] = b
	}
	for _, id := range []string{"postgres-protocol", "redis-protocol", "clickhouse-protocol", "s3-protocol"} {
		b := byID[id]
		if b.ID == "" {
			t.Fatal("missing lab binding", id)
		}
		probe(b, "ok")
		bad := b
		if b.Adapter == "s3" {
			bad.CredentialFile = secret("bad-s3.json", settings.Credential{SchemaVersion: 1, Kind: "s3", AccessKeyID: "diagnostics-missing-key", SecretAccessKey: "diagnostics-invalid-fixed-fixture"})
			o := runtime.Collect(context.Background(), settings.Probe{ID: b.ID, NodeID: bindings.NodeID, Adapter: b.Adapter}, bad)
			if o.Outcome == "ok" {
				t.Fatal("S3 accepted invalid key")
			}
			results = append(results, map[string]any{"adapter": "s3", "case": "invalid-key HEAD intentionally cannot disambiguate 403", "observation": o})
		} else {
			bad.CredentialFile = wrong
			probe(bad, "authentication_failed")
		}
		bad = b
		switch b.Adapter {
		case "postgres":
			bad.Port = 1
		case "redis", "clickhouse":
			bad.Address = "127.0.0.1:1"
		case "s3":
			bad.Endpoint = "http://127.0.0.1:1"
		}
		probe(bad, "connection_refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Permission fixtures are created through admin setup, never by the probes.
	pg := byID["postgres-protocol"]
	cred, e := credential(pg.CredentialFile, "password")
	if e != nil {
		t.Fatal(e)
	}
	pc, e := pgx.ParseConfig("postgres://127.0.0.1/postgres?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	pc.Host = pg.Host
	pc.Port = uint16(pg.Port)
	pc.User = pg.Username
	pc.Password = cred.Password
	pc.Database = pg.Database
	conn, e := pgx.ConnectConfig(ctx, pc)
	if e != nil {
		t.Fatal("PG fixture connect")
	}
	defer conn.Close(context.Background())
	name := "diag_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := "probe-" + uuid.NewString()
	if _, e = conn.Exec(ctx, "CREATE ROLE "+name+" LOGIN PASSWORD '"+password+"'"); e != nil {
		t.Fatal("PG role fixture")
	}
	defer conn.Exec(context.Background(), "DROP ROLE IF EXISTS "+name)
	if _, e = conn.Exec(ctx, "CREATE DATABASE "+name); e != nil {
		t.Fatal("PG database fixture")
	}
	defer conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	if _, e = conn.Exec(ctx, "REVOKE CONNECT ON DATABASE "+name+" FROM PUBLIC"); e != nil {
		t.Fatal("PG grant fixture")
	}
	pg.Username = name
	pg.Database = name
	pg.CredentialFile = secret("pg-denied.json", settings.Credential{SchemaVersion: 1, Kind: "password", Password: password})
	probe(pg, "permission_denied")
	rb := byID["redis-protocol"]
	rc, _ := credential(rb.CredentialFile, "password")
	redisAdmin := redis.NewClient(&redis.Options{Addr: rb.Address, Password: rc.Password, Protocol: 2})
	defer redisAdmin.Close()
	if e = redisAdmin.Do(ctx, "ACL", "SETUSER", name, "on", ">"+password, "-@all").Err(); e != nil {
		t.Fatal("Redis ACL fixture")
	}
	defer redisAdmin.Do(context.Background(), "ACL", "DELUSER", name)
	rb.Username = name
	rb.CredentialFile = pg.CredentialFile
	probe(rb, "permission_denied")
	cb := byID["clickhouse-protocol"]
	cc, _ := credential(cb.CredentialFile, "password")
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{cb.Address}, Auth: clickhouse.Auth{Username: cb.Username, Password: cc.Password, Database: cb.Database}})
	if e != nil {
		t.Fatal(e)
	}
	defer ch.Close()
	if e = ch.Exec(ctx, "CREATE USER "+name+" IDENTIFIED WITH plaintext_password BY '"+password+"' SETTINGS readonly=0 CONST"); e != nil {
		t.Fatal("CH constraint fixture")
	}
	defer ch.Exec(context.Background(), "DROP USER IF EXISTS "+name)
	cb.Username = name
	cb.CredentialFile = pg.CredentialFile
	probe(cb, "permission_denied")
	sb := byID["s3-protocol"]
	sc, _ := credential(sb.CredentialFile, "s3")
	sc.AccessKeyID = "diagnostics-denied"
	sb.CredentialFile = secret("s3-denied.json", sc)
	probe(sb, "permission_denied")
	keeper := byID["keeper-protocol"]
	keeper.Enabled = false
	probe(keeper, "not_configured")
	out, _ := json.MarshalIndent(map[string]any{"run_id": lab.RunID, "result": "passed", "evidence_type": "real-container", "command": "go test -run TestRealFixedProtocols ./services/diagnostics/internal/adapters", "cases": results, "recovery_result": "temporary PG/Redis/CH identities removed; no application records written"}, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "evidence/fixed-protocols.json"), out, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestRealDockerReadOnlyObservations(t *testing.T) {
	dir := os.Getenv("INFRAPRO_DIAGNOSTICS_LAB_DIRECTORY")
	if dir == "" {
		t.Skip("explicit owned lab required")
	}
	var lab struct {
		RunID string `json:"run_id"`
	}
	raw, e := os.ReadFile(filepath.Join(dir, "lab.json"))
	if e != nil || json.Unmarshal(raw, &lab) != nil || !strings.HasPrefix(lab.RunID, "diag-") {
		t.Fatal("owned lab required")
	}
	var bindings settings.Bindings
	raw, e = os.ReadFile(filepath.Join(dir, "native-b/bindings.json"))
	if e != nil || json.Unmarshal(raw, &bindings) != nil {
		t.Fatal("owned binding")
	}
	socket := ""
	for _, b := range bindings.Bindings {
		if b.Adapter == "docker_container" {
			socket = b.SocketPath
			break
		}
	}
	if socket == "" {
		t.Fatal("explicit socket required")
	}
	command := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if e != nil {
			t.Fatalf("owned Docker fixture command failed: %s", args[0])
		}
		return strings.TrimSpace(string(out))
	}
	key := "diagnostics-" + uuid.NewString()
	ids := []string{}
	defer func() {
		for _, id := range ids {
			labels := command("inspect", "--format", "{{index .Config.Labels \"io.infrapro.fixture\"}}", id)
			if labels != key {
				t.Error("fixture ownership changed")
				continue
			}
			command("rm", "-f", id)
		}
	}()
	b := settings.Binding{ID: "docker-test", Adapter: "docker_container", SocketPath: socket, ExpectedReplicaID: "test-1", SelectorLabels: map[string]string{"io.infrapro.fixture": key}}
	p := settings.Probe{ID: b.ID, NodeID: "test", Adapter: b.Adapter}
	r := New(settings.Manifest{}, settings.Bindings{}, uuid.NewString())
	records := []map[string]any{}
	observe := func(name, code string) model.Observation {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		o := r.Collect(ctx, p, b)
		if o.ReasonCode != code {
			t.Fatalf("%s: %s/%s", name, o.Outcome, o.ReasonCode)
		}
		records = append(records, map[string]any{"case": name, "observation": o})
		return o
	}
	run := func(args ...string) string {
		base := []string{"run", "-d", "--network", "none", "--memory", "32m", "--memory-swap", "32m", "--cpus", "0.25", "--pids-limit", "16", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--label", "io.infrapro.lab.run_id=" + lab.RunID, "--label", "io.infrapro.fixture=" + key}
		id := command(append(base, args...)...)
		ids = append(ids, id)
		return id
	}
	observe("exact-selector-zero", "process_not_found")
	one := run("alpine:3.22", "sleep", "120")
	o := observe("exact-selector-one-no-healthcheck", "ok")
	if o.Evidence.Health != "none" || o.Process.ContainerID != one {
		t.Fatal("wrong container or health")
	}
	boot := o.Process.BootID
	command("restart", one)
	o = observe("restart-generation", "ok")
	if o.Process.BootID == boot {
		t.Fatal("restart boot unchanged")
	}
	two := run("alpine:3.22", "sleep", "120")
	observe("exact-selector-multiple", "ambiguous_identity")
	command("rm", "-f", two)
	ids = ids[:1]
	command("rm", "-f", one)
	ids = nil
	run("alpine:3.22", "sh", "-c", "exit 17")
	time.Sleep(200 * time.Millisecond)
	o = observe("exit-code", "process_exited")
	if o.Evidence.ExitCode == nil || *o.Evidence.ExitCode != 17 {
		t.Fatal("exit code")
	}
	command("rm", "-f", ids[0])
	ids = nil
	run("alpine:3.22", "awk", "BEGIN { s=sprintf(\"%65536s\",\"x\"); for (i=0;i<100000;i++) a[i]=s i; }")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		o = r.Collect(context.Background(), p, b)
		if o.Evidence.OOMKilled != nil && *o.Evidence.OOMKilled {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if o.Evidence.OOMKilled == nil || !*o.Evidence.OOMKilled {
		t.Fatal("bounded OOM fixture not observed")
	}
	records = append(records, map[string]any{"case": "actual-cgroup-oom", "observation": o})
	command("rm", "-f", ids[0])
	ids = nil
	run("--health-cmd", "exit 1", "--health-interval", "1s", "--health-timeout", "1s", "--health-retries", "1", "alpine:3.22", "sleep", "120")
	time.Sleep(2200 * time.Millisecond)
	o = observe("unhealthy", "response_mismatch")
	if o.Evidence.Health != "unhealthy" {
		t.Fatal("health state")
	}
	output, _ := json.MarshalIndent(map[string]any{"run_id": lab.RunID, "result": "passed", "evidence_type": "real-container", "command": "go test -run TestRealDockerReadOnlyObservations ./services/diagnostics/internal/adapters", "cases": records, "recovery_result": "exact owned fixture IDs removed"}, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "evidence/docker-contract.json"), output, 0600); e != nil {
		t.Fatal(e)
	}
}
