package resources_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/pressly/goose/v3"
)

func TestUploadLimitsMigrationPreservesEvidenceAndRejectsUnsafeDowngrade(t *testing.T) {
	f := filesFixtureForResources(t, resourcesFromFixture(t, setupAtMigration(t, 54)))
	job := f.run(t, f.upload(t, []byte("retained historical attachment"), nil, nil))
	if job.State != "succeeded" {
		t.Fatal(job)
	}
	_, _ = f.start(t, []byte("accepted unfinished upload"))
	snapshot := func(table string) string {
		t.Helper()
		var value string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]')::text FROM `+table+` r`).Scan(&value))
		return value
	}
	before := map[string]string{}
	for _, table := range []string{"file_records", "file_uploads", "file_upload_parts", "file_objects", "file_jobs", "file_attachment_revisions"} {
		before[table] = snapshot(table)
	}
	must(t, migrations.Up(ctx, f.env.Config.DatabaseURL))
	must(t, migrations.Check(ctx, f.env.Pool))
	for table, want := range before {
		if snapshot(table) != want {
			t.Fatal("upload upgrade changed retained evidence", table)
		}
	}
	f.options.MaxFileBytes = 101 << 20
	f.s.WithOptions(f.options)
	_, e := f.s.StartUpload(ctx, f.actor, f.scope, files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: "add", Filename: "larger.txt", DisplayName: "larger", SizeBytes: "105906176", SHA256: fileDigest([]byte("declaration only")), DeclaredMediaType: "text/plain"})
	must(t, e)
	accepted := snapshot("file_uploads")
	db, e := sql.Open("pgx", f.env.Config.DatabaseURL)
	must(t, e)
	defer db.Close()
	dir, e := filepath.Abs("../migrations")
	must(t, e)
	provider, e := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	must(t, e)
	if _, e = provider.DownTo(ctx, 54); e == nil {
		t.Fatal("downgrade silently discarded an accepted larger upload")
	}
	must(t, migrations.Check(ctx, f.env.Pool))
	if snapshot("file_uploads") != accepted {
		t.Fatal("failed downgrade changed uploads")
	}
	t.Log("schema 54→55 retained completed and unfinished attachments; unsafe rollback failed atomically")
}
