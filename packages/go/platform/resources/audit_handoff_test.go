package resources_test

import (
	"log/slog"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuditRealDisplayHandoffExhaustionAndMaintenanceReplay(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "AuditHandoff")
	b := runtime.Compose(f.env.Service.Access())
	s := b.Displays.WithStorage(&displayMemoryStore{files: map[string][]byte{}})
	scope := display.Scope{Tenant: f.tenant, Project: p.id}
	meta := func(revision string) display.Meta {
		return display.Meta{Revision: revision, RequestID: uuid.NewString(), Reason: "真实包检查审计交接恢复"}
	}
	_, e := s.Command(ctx, p.actor, scope, "", "settings", "", display.Command{Meta: meta("0"), Enabled: true})
	must(t, e)
	v, e := s.Command(ctx, p.actor, scope, "", "create", "", display.Command{Meta: meta("0"), Name: "审计交接合成实例"})
	must(t, e)
	id, revision := v["id"].(string), v["revision"].(string)
	data := displayZIP(t)
	v, e = s.Command(ctx, p.actor, scope, id, "upload", "", display.Command{Meta: meta(revision), Size: int64(len(data)), SHA256: displayHash(data)})
	must(t, e)
	upload := v["upload_id"].(string)
	revision = v["revision"].(string)
	must(t, s.UploadPart(ctx, p.actor, scope, id, upload, 0, displayHash(data), data))
	_, e = s.Command(ctx, p.actor, scope, id, "upload.complete", upload, display.Command{Meta: meta(revision)})
	must(t, e)
	worked, e := s.ProcessOne(ctx)
	must(t, e)
	if !worked {
		t.Fatal("actual package not checked")
	}
	var before string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(string_agg(row_to_json(j)::text,',' ORDER BY id)) FROM display_package_jobs j WHERE upload_id IN (SELECT id FROM display_uploads WHERE tenant_id=$1)`, f.tenant).Scan(&before))
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_display_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_module='display' THEN RAISE EXCEPTION 'injected_handoff_failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_display_audit BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION reject_display_audit()`)
	must(t, e)
	for range 8 {
		_, e = b.Audit.Reconcile(ctx)
		must(t, e)
	}
	rows, e := f.env.Pool.Query(ctx, `SELECT id::text FROM display_audit_handoffs WHERE translated_at IS NULL AND attempts=8`)
	must(t, e)
	ids := []string{}
	for rows.Next() {
		var id string
		must(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	must(t, rows.Err())
	rows.Close()
	if len(ids) < 2 {
		t.Fatal("accepted and terminal handoffs were not durably retained", len(ids))
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER reject_display_audit ON account_security_events; DROP FUNCTION reject_display_audit()`)
	must(t, e)
	pc, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
	must(t, e)
	pc.ConnConfig.RuntimeParams["role"] = "infrapro_audit_maintenance"
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	must(t, e)
	defer pool.Close()
	maintenance := runtime.Compose(access.New(pool, slog.Default())).Audit
	for _, id := range ids {
		request := uuid.NewString()
		first, e := maintenance.RetryHandoff(ctx, id, 8, request, "恢复已耗尽的交接翻译")
		must(t, e)
		again, e := maintenance.RetryHandoff(ctx, id, 8, request, "恢复已耗尽的交接翻译")
		must(t, e)
		if first["intent"] != again["intent"] {
			t.Fatal("maintenance receipt changed")
		}
		if _, e = maintenance.RetryHandoff(ctx, id, 9, request, "恢复已耗尽的交接翻译"); e == nil {
			t.Fatal("changed intent replayed")
		}
	}
	for range 2 {
		_, e = b.Audit.Reconcile(ctx)
		must(t, e)
	}
	var count, pending int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE source_module='display' AND evidence_quality='source_handoff'`).Scan(&count))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM display_audit_handoffs WHERE translated_at IS NULL`).Scan(&pending))
	if count != len(ids) || pending != 0 {
		t.Fatalf("bridge lost or duplicated facts: facts=%d pending=%d handoffs=%d", count, pending, len(ids))
	}
	var after string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(string_agg(row_to_json(j)::text,',' ORDER BY id)) FROM display_package_jobs j WHERE upload_id IN (SELECT id FROM display_uploads WHERE tenant_id=$1)`, f.tenant).Scan(&after))
	if before != after {
		t.Fatal("audit replay re-executed package processing")
	}
	t.Log("actual package accepted and checked; eight audit write failures preserve handoffs; restricted maintenance retries exact receipts; bridge publishes once without changing source jobs")
}
