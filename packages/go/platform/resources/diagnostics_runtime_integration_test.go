//go:build diagnosticsintegration && monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	br "github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/google/uuid"
)

type diagnosticChild struct {
	cmd    *exec.Cmd
	done   chan error
	report string
}

func diagnosticWait(t *testing.T, description string, fn func() bool) {
	t.Helper()
	end := time.Now().Add(45 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("actual process condition timed out: " + description)
}
func diagnosticReport(path string) diagnosticreport.Report {
	var out diagnosticreport.Report
	raw, _ := os.ReadFile(path)
	_ = json.Unmarshal(raw, &out)
	return out
}
func diagnosticJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, e := json.Marshal(v)
	must(t, e)
	must(t, os.WriteFile(path, raw, 0600))
}
func diagnosticAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	a := l.Addr().String()
	_ = l.Close()
	return a
}
func (c *diagnosticChild) stop(t *testing.T) {
	t.Helper()
	select {
	case <-c.done:
		return
	default:
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		<-c.done
	}
}
func TestDiagnosticsActualOwningProcesses(t *testing.T) {
	labDir := os.Getenv("INFRAPRO_DIAGNOSTICS_LAB_DIRECTORY")
	if labDir == "" {
		t.Skip("explicit owned diagnostics lab required")
	}
	var lab struct {
		RunID          string `json:"run_id"`
		BusinessConfig string `json:"business_config"`
	}
	raw, e := os.ReadFile(filepath.Join(labDir, "lab.json"))
	must(t, e)
	must(t, json.Unmarshal(raw, &lab))
	if !strings.HasPrefix(lab.RunID, "diag-") {
		t.Fatal("unowned lab")
	}
	f := resourcesSetup(t)
	raw, e = os.ReadFile(lab.BusinessConfig)
	must(t, e)
	base := map[string]any{}
	must(t, json.Unmarshal(raw, &base))
	base["database_url"] = f.env.Config.DatabaseURL
	base["redis_key_prefix"] = f.env.Config.RedisKeyPrefix
	dir, e := os.MkdirTemp("/tmp", "dg-own-")
	must(t, e)
	dir, e = filepath.EvalSymlinks(dir)
	must(t, e)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	for _, name := range []string{"checker", "files", "audit", "spatial", "monitoring", "reports"} {
		must(t, os.Mkdir(filepath.Join(dir, name), 0700))
	}
	base["http_addr"] = diagnosticAddress(t)
	displayCfg := base["display"].(map[string]any)
	for _, kind := range []string{"runtime", "asset"} {
		a := diagnosticAddress(t)
		displayCfg[kind+"_addr"] = a
		displayCfg[kind+"_origin"] = "http://" + a
	}
	base["spatial_temp_directory"] = filepath.Join(dir, "spatial")
	base["monitoring_temp_directory"] = filepath.Join(dir, "monitoring")
	fileCfg := base["files"].(map[string]any)
	fileCfg["temp_directory"] = filepath.Join(dir, "files")
	fileCfg["checker_socket"] = filepath.Join(dir, "files/check.sock")
	fileCfg["cleanup_interval"] = "2s"
	base["audit"].(map[string]any)["temp_directory"] = filepath.Join(dir, "audit")
	configPath := filepath.Join(dir, "business.json")
	diagnosticJSON(t, configPath, base)
	cfg, e := config.Load(configPath)
	must(t, e)
	host, e := br.Open(ctx, cfg, slog.Default(), false)
	must(t, e)
	t.Cleanup(host.Close)
	business := br.Compose(f.env.Service.Access(), host.Monitoring).Configure(cfg)
	root, e := filepath.Abs("../../../..")
	must(t, e)
	start := func(name, binary, path string, args ...string) *diagnosticChild {
		t.Helper()
		out := filepath.Join(dir, "reports", name+".json")
		log, e := os.OpenFile(filepath.Join(dir, name+".log"), os.O_CREATE|os.O_WRONLY, 0600)
		must(t, e)
		cmd := exec.Command(filepath.Join(labDir, "bin", binary), args...)
		cmd.Dir = root
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "INFRAPRO_") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, "INFRAPRO_CONFIG="+path, "INFRAPRO_DIAGNOSTICS_INSTANCE_ID="+lab.RunID, "INFRAPRO_DIAGNOSTICS_NODE_ID=owning-process-test", "INFRAPRO_DIAGNOSTICS_REPLICA_ID="+name+"-1", "INFRAPRO_DIAGNOSTICS_REPORT_FILE="+out)
		cmd.Stdout = log
		cmd.Stderr = log
		must(t, cmd.Start())
		_ = log.Close()
		child := &diagnosticChild{cmd, make(chan error, 1), out}
		go func() { child.done <- cmd.Wait(); close(child.done) }()
		t.Cleanup(func() { child.stop(t) })
		return child
	}
	children := map[string]*diagnosticChild{}
	names := []string{"files-checker", "api", "monitoring-worker", "notification-worker", "task-index-worker", "spatial-worker", "display-package-worker", "display-runtime", "files-worker", "audit-worker"}
	records := []map[string]any{}
	record := func(name string) {
		reports := map[string]diagnosticreport.Report{}
		for id, child := range children {
			reports[id] = diagnosticReport(child.report)
		}
		records = append(records, map[string]any{"case": name, "result": "passed", "evidence_type": "actual-owning-process + isolated PG/Redis/CH/S3", "reports": reports})
		t.Log(name)
	}
	for _, name := range names {
		args := []string{}
		if name == "files-checker" {
			args = []string{"--directory", filepath.Join(dir, "files"), "--socket", filepath.Join(dir, "files/check.sock")}
		}
		c := start(name, name, configPath, args...)
		children[name] = c
		diagnosticWait(t, name+" ready", func() bool { return diagnosticReport(c.report).Phase == "ready" })
	}
	record("all-ten-owning-processes-initialized")
	required := map[string][]string{"monitoring-worker": {"scheduler", "foreground", "historical"}, "notification-worker": {"delivery"}, "task-index-worker": {"reconcile", "backfill"}, "spatial-worker": {"converter", "convert"}, "display-package-worker": {"process", "maintain"}, "display-runtime": {"runtime-http", "assets-http"}, "files-worker": {"prepare", "cleanup", "heartbeat"}, "files-checker": {"socket"}, "audit-worker": {"reconcile", "export"}, "api": {"http"}}
	for name, lanes := range required {
		for _, id := range lanes {
			diagnosticWait(t, name+" idle "+id, func() bool {
				for _, l := range diagnosticReport(children[name].report).Lanes {
					if l.ID == id && l.State == "idle" {
						return true
					}
				}
				return false
			})
		}
	}
	record("all-declared-lanes-idle-with-actual-next-attempt")
	// Real queue locking keeps the three actual monitoring lanes in a visible
	// working state. It changes only this test's private PostgreSQL schema.
	lock, e := f.env.Pool.Begin(ctx)
	must(t, e)
	_, e = lock.Exec(ctx, "LOCK TABLE monitoring_jobs, derived_channels IN ACCESS EXCLUSIVE MODE")
	must(t, e)
	for _, id := range []string{"scheduler", "foreground", "historical"} {
		diagnosticWait(t, "monitoring working "+id, func() bool {
			for _, l := range diagnosticReport(children["monitoring-worker"].report).Lanes {
				if l.ID == id && l.State == "working" {
					return true
				}
			}
			return false
		})
	}
	must(t, lock.Rollback(ctx))
	record("monitoring-three-lanes-real-working")
	// Seed a real CSV through the owning service; only the external monitoring
	// process may claim, validate and publish it.
	facility := f.facility(t, "DIAGNOSTIC")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "合成测点"})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	channel, e := f.write(t, ct, "create", map[string]any{"code": "C", "name": "合成监测通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	mf := monitoringFixture{f, business.Monitoring, md.TenantScope(f.tenant), channel.ID, host.Monitoring, cfg.Monitoring.Database}
	batch := mf.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,1,good\n2024-01-01T00:00:01Z,2,good\n")
	_, e = mf.service.QueueImport(ctx, f.actor, mf.scope, batch.ID, "validate", mf.meta(batch.Revision))
	must(t, e)
	diagnosticWait(t, "monitoring validate", func() bool {
		b, err := mf.service.Import(ctx, f.actor, mf.scope, batch.ID)
		if err == nil && b.State == "ready" {
			batch = b
			return true
		}
		return false
	})
	_, e = mf.service.QueueImport(ctx, f.actor, mf.scope, batch.ID, "publish", mf.meta(batch.Revision))
	must(t, e)
	diagnosticWait(t, "monitoring publish", func() bool {
		b, err := mf.service.Import(ctx, f.actor, mf.scope, batch.ID)
		return err == nil && b.State == "published"
	})
	record("monitoring-actual-CSV-validation-publication")
	queuedExport, e := mf.service.CreateExport(ctx, f.actor, mf.scope, md.ExportInput{WriteMeta: mf.meta("0"), Query: mf.query(t), Format: "csv", Kind: "details"})
	must(t, e)
	diagnosticWait(t, "monitoring historical export", func() bool {
		job, err := mf.service.Job(ctx, f.actor, mf.scope, queuedExport.JobID)
		return err == nil && job.State == "succeeded"
	})
	record("monitoring-historical-lane-real-export")
	sf := spatialFixture{f, business.Spatial, sp.Tenant(f.tenant), root}
	asset := sf.newAsset(t, facility.ID, "layout", "DIAG-PLAN")
	version := sf.newVersion(t, asset.ID, "", true)
	job := sf.upload(t, version, "bridge-plan.png")
	diagnosticWait(t, "actual Python conversion", func() bool {
		j, err := sf.s.Job(ctx, f.actor, sf.scope, job.ID)
		return err == nil && j.State == "succeeded"
	})
	record("spatial-owning-process-real-python-conversion")
	// A valid upload envelope containing an invalid PNG exercises the real child
	// converter failure path, without replacing that converter with a stub.
	badAsset := sf.newAsset(t, facility.ID, "layout", "DIAG-BAD")
	bad := sf.newVersion(t, badAsset.ID, "", true)
	data := []byte("invalid-png-diagnostics-fixture")
	up, e := sf.s.StartUpload(ctx, f.actor, sf.scope, sp.UploadInput{Meta: spatialMeta(), Revision: bad.Revision, VersionID: bad.ID, Filename: "invalid.png", Size: int64(len(data)), SHA256: displayHash(data)})
	must(t, e)
	part, e := sf.s.UploadPart(ctx, f.actor, sf.scope, up.ID, 1, displayHash(data), bytes.NewReader(data))
	must(t, e)
	_, e = sf.s.CompleteUpload(ctx, f.actor, sf.scope, up.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: part.Revision})
	must(t, e)
	v, e := sf.s.Manifest(ctx, f.actor, sf.scope, bad.ID)
	must(t, e)
	badJob, e := sf.s.Process(ctx, f.actor, sf.scope, bad.ID, sp.RevisionInput{Meta: spatialMeta(), Revision: v.Revision})
	must(t, e)
	diagnosticWait(t, "conversion failed", func() bool {
		j, err := sf.s.Job(ctx, f.actor, sf.scope, badJob.ID)
		return err == nil && j.State == "failed"
	})
	record("spatial-real-converter-rejects-invalid-file")
	project := f.project(t, "DiagnosticDisplay")
	scope := display.Scope{Tenant: f.tenant, Project: project.id}
	ds := business.Displays
	meta := func(rev string) display.Meta {
		return display.Meta{Revision: rev, RequestID: uuid.NewString(), Reason: "独立诊断进程自验"}
	}
	_, e = ds.Command(ctx, project.actor, scope, "", "settings", "", display.Command{Meta: meta("0"), Enabled: true})
	must(t, e)
	created, e := ds.Command(ctx, project.actor, scope, "", "create", "", display.Command{Meta: meta("0"), Name: "合成诊断大屏"})
	must(t, e)
	instance := created["id"].(string)
	zip := displayZIP(t)
	upload, e := ds.Command(ctx, project.actor, scope, instance, "upload", "", display.Command{Meta: meta(created["revision"].(string)), Size: int64(len(zip)), SHA256: displayHash(zip)})
	must(t, e)
	uploadID := upload["upload_id"].(string)
	must(t, ds.UploadPart(ctx, project.actor, scope, instance, uploadID, 0, displayHash(zip), zip))
	_, e = ds.Command(ctx, project.actor, scope, instance, "upload.complete", uploadID, display.Command{Meta: meta(upload["revision"].(string))})
	must(t, e)
	diagnosticWait(t, "display package processed", func() bool {
		d, err := ds.Detail(ctx, project.actor, scope, instance)
		return err == nil && len(d["packages"].([]json.RawMessage)) == 1
	})
	record("display-package-owning-process-validates-real-package")
	fs := business.Files
	fileScope := files.Scope{Kind: "tenant", TenantID: f.tenant}
	caps, e := fs.Capabilities(ctx, f.actor, fileScope)
	must(t, e)
	wm := files.WriteMeta{RequestID: uuid.NewString(), Reason: "诊断接入合成文件", Epoch: caps.RestoreEpoch}
	body := []byte("diagnostics real checker and attachment\n")
	started, e := fs.StartUpload(ctx, f.actor, fileScope, files.StartUploadInput{WriteMeta: wm, Host: fileScope.Host(), Intent: "add", Filename: "sample.txt", DisplayName: "合成样本", SizeBytes: strconv.Itoa(len(body)), SHA256: displayHash(body), DeclaredMediaType: "text/plain"})
	must(t, e)
	_, e = fs.PutPart(ctx, f.actor, fileScope, *started.UploadID, files.PartInput{Part: 1, Size: int64(len(body)), SHA256: displayHash(body), Epoch: caps.RestoreEpoch}, bytes.NewReader(body))
	must(t, e)
	wm.RequestID = uuid.NewString()
	submitted, e := fs.CompleteUpload(ctx, f.actor, fileScope, *started.UploadID, files.RevisionCommand{WriteMeta: wm, ExpectedRevision: started.Revision})
	must(t, e)
	diagnosticWait(t, "file prepared by actual worker and checker", func() bool {
		var state string
		err := f.env.Pool.QueryRow(ctx, "SELECT state FROM file_job_runs WHERE job_id=$1 AND generation=1", *submitted.JobID).Scan(&state)
		return err == nil && state == "succeeded"
	})
	record("files-worker-and-real-checker-child-complete-attachment")
	as := business.Audit
	ascope := audit.Scope{Kind: "tenant", Tenant: f.tenant}
	ac, e := as.Capabilities(ctx, f.actor, ascope)
	must(t, e)
	ex, e := as.CreateExport(ctx, f.actor, ascope, uuid.NewString(), audit.ExportInput{Filter: audit.Filter{From: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), To: time.Now().UTC().Format(time.RFC3339Nano)}, FieldProfile: "standard", Reason: "独立进程审计导出自验", Epoch: ac.Epoch, AuthorizationRevision: ac.AuthorizationRevision, ProjectionVersion: ac.ProjectionVersion})
	must(t, e)
	diagnosticWait(t, "actual audit export", func() bool {
		v, err := as.Export(ctx, f.actor, ascope, ex.ID, "1")
		return err == nil && v.State == "succeeded"
	})
	record("audit-owning-process-reconcile-and-real-export")
	// Fixed SQL fault is limited to this schema. It lets all three lanes return
	// actual database errors while the report writer remains independently alive.
	_, e = f.env.Pool.Exec(ctx, "ALTER TABLE monitoring_jobs RENAME TO diagnostic_hidden_jobs")
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, "ALTER TABLE derived_channels RENAME TO diagnostic_hidden_channels")
	must(t, e)
	t.Cleanup(func() {
		_, _ = f.env.Pool.Exec(ctx, "ALTER TABLE diagnostic_hidden_jobs RENAME TO monitoring_jobs")
		_, _ = f.env.Pool.Exec(ctx, "ALTER TABLE diagnostic_hidden_channels RENAME TO derived_channels")
	})
	for _, id := range []string{"scheduler", "foreground", "historical"} {
		diagnosticWait(t, "monitoring error "+id, func() bool {
			for _, l := range diagnosticReport(children["monitoring-worker"].report).Lanes {
				if l.ID == id && l.State == "blocked" {
					return true
				}
			}
			return false
		})
	}
	_, e = f.env.Pool.Exec(ctx, "ALTER TABLE diagnostic_hidden_jobs RENAME TO monitoring_jobs")
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, "ALTER TABLE diagnostic_hidden_channels RENAME TO derived_channels")
	must(t, e)
	record("monitoring-three-lanes-real-database-error-and-recovery")
	for name, c := range children {
		c.stop(t)
		if diagnosticReport(c.report).Phase != "stopping" {
			t.Fatalf("%s did not retain normal exit semantics", name)
		}
	}
	record("all-ten-owning-processes-graceful-exit")
	// Every parent reports before initialization, including its failure and exit.
	invalid := map[string]any{}
	raw, _ = json.Marshal(base)
	_ = json.Unmarshal(raw, &invalid)
	invalid["database_url"] = "postgres://unused@127.0.0.1:1/diagnostic_unavailable"
	invalidPath := filepath.Join(dir, "invalid.json")
	diagnosticJSON(t, invalidPath, invalid)
	for _, name := range names {
		args := []string{}
		if name == "files-checker" {
			args = []string{"--directory", filepath.Join(dir, "files"), "--socket", filepath.Join(dir, "outside.sock")}
		}
		c := start(name+"-failure", name, invalidPath, args...)
		diagnosticWait(t, name+" initialization failure report", func() bool { return diagnosticReport(c.report).Phase == "failed" })
		select {
		case err := <-c.done:
			if err == nil {
				t.Fatal("startup failure exited successfully")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("startup failure did not exit", name)
		}
	}
	record("all-ten-owning-processes-initialization-failure-before-exit")
	unconfigured := map[string]any{}
	raw, _ = json.Marshal(base)
	_ = json.Unmarshal(raw, &unconfigured)
	unconfigured["files"] = map[string]any{}
	unconfiguredPath := filepath.Join(dir, "unconfigured.json")
	diagnosticJSON(t, unconfiguredPath, unconfigured)
	uc := start("files-unconfigured", "files-worker", unconfiguredPath, "--allow-unconfigured")
	diagnosticWait(t, "files unconfigured", func() bool { return diagnosticReport(uc.report).Phase == "unconfigured" })
	uc.stop(t)
	record("files-unconfigured-preserves-opt-in-idle")
	invalid["database_url"] = base["database_url"]
	invalid["spatial_python"] = "/nonexistent/diagnostic-python"
	diagnosticJSON(t, invalidPath, invalid)
	pc := start("converter-preflight", "spatial-worker", invalidPath)
	diagnosticWait(t, "real converter preflight failure", func() bool {
		for _, l := range diagnosticReport(pc.report).Lanes {
			if l.ID == "converter" && l.State == "blocked" {
				return true
			}
		}
		return false
	})
	record("spatial-converter-preflight-failure")
	diagnosticJSON(t, filepath.Join(labDir, "evidence/owning-processes.json"), map[string]any{"run_id": lab.RunID, "result": "passed", "command": "node scripts/diagnostics-business-check.mjs", "evidence_type": "real_local/real-container", "cases": records, "recovery_result": "all owned children stopped; private schema cleanup scheduled"})
}
