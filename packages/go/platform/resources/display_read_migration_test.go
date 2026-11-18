package resources_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

func TestDisplayReadMigrationFormatsAndDowngrade(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DisplayUpgrade")
	facility := f.facility(t, "UPGRADE")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	b := runtime.Compose(f.env.Service.Access())
	h := createDisplayHarness(t, f, p, b.Displays, map[string]display.Binding{"site": {Kind: "resources", IDs: []string{facility.ID}}}, display.Grant{Capabilities: []string{"resources.read"}, Scope: display.ReadScope{Resources: []string{facility.ID}}})
	db, e := sql.Open("pgx", f.env.Config.DatabaseURL)
	must(t, e)
	defer db.Close()
	dir, e := filepath.Abs("../migrations")
	must(t, e)
	provider, e := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	must(t, e)
	// The structure migrations deliberately cannot be downgraded in place.
	// Historical v46→current row preservation is covered by the real historical
	// schema fixture in TestStructureUpgradePreservesLegacyFactsAndEverySectionOccurrence.
	var before string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT md5((SELECT jsonb_agg(to_jsonb(c)-'config_format'-'read_views' ORDER BY c.id)::text FROM display_configs c)||(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id)::text FROM display_releases r))`).Scan(&before))
	must(t, migrations.Check(ctx, f.env.Pool))
	if binary := os.Getenv("INFRAPRO_DISPLAY_LEGACY_PROBE"); binary != "" {
		probe := exec.CommandContext(ctx, binary)
		probe.Env = append(os.Environ(), "INFRAPRO_PROBE_DATABASE="+f.env.Config.DatabaseURL)
		out, e := probe.CombinedOutput()
		must(t, e)
		if !strings.Contains(string(out), "legacy binary rejected newer schema") {
			t.Fatal("legacy readiness did not fail closed")
		}
		t.Log(strings.TrimSpace(string(out)))
	}
	var after string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT md5((SELECT jsonb_agg(to_jsonb(c)-'config_format'-'read_views' ORDER BY c.id)::text FROM display_configs c)||(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id)::text FROM display_releases r))`).Scan(&after))
	if before != after {
		t.Fatal("upgrade changed legacy immutable config/release")
	}
	var format int
	var fields string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT config_format,read_views::text FROM display_configs WHERE id=$1`, h.config).Scan(&format, &fields))
	if format != 1 || fields != "{}" {
		t.Fatal("legacy config did not default to format 1")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT scope_format,read_scopes::text FROM display_grants WHERE instance_id=$1`, h.id).Scan(&format, &fields))
	if format != 1 || fields != "[]" {
		t.Fatal("legacy grant expanded on upgrade")
	}
	result := h.query(t, "resources.list", display.Query{Target: display.Target{Binding: "site"}})
	if !strings.Contains(string(dr.Bytes(result.Data)), facility.ID) {
		t.Fatal("old binding failed after upgrade")
	}
	for _, format := range []int{-1, 3, 999} {
		if _, e := h.s.Command(ctx, p.actor, h.scope, h.id, "grant", "", display.Command{Meta: display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "format boundary"}, Grant: &display.Grant{Format: format}}); e == nil {
			t.Fatal("unknown grant format accepted")
		}
	}
	if _, e = f.env.Pool.Exec(ctx, `UPDATE display_grants SET scope_format=3 WHERE instance_id=$1`, h.id); e == nil {
		t.Fatal("database accepted unknown format")
	}
	var packageID string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT package_id::text FROM display_configs WHERE id=$1`, h.config).Scan(&packageID))
	for _, format := range []int{-1, 3, 999} {
		if _, e := h.s.Command(ctx, p.actor, h.scope, h.id, "config", "", display.Command{Meta: display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "config format boundary"}, ConfigFormat: format, PackageID: packageID, Bindings: map[string]display.Binding{"site": {Kind: "resources", IDs: []string{facility.ID}}}, Parameters: map[string]any{}, Mode: "fixed"}); e == nil {
			t.Fatal("unknown config format accepted")
		}
	}
	d := dr.Definition{ID: "r", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata"}}
	v := d
	v.ID = ""
	v.ScopeID = "r"
	named := createReadDisplay(t, f, p, b, []dr.Definition{d}, map[string]dr.Definition{"r": v}, []string{"views.read", "queries.read", "resources.directory"})
	// Later migrations can block a full downgrade before it reaches the display guard.
	_, e = provider.ApplyVersion(ctx, 46, false)
	var rejection *pgconn.PgError
	if !errors.As(e, &rejection) || rejection.Code != "P0001" || !strings.Contains(rejection.Message, "format-2 display data must be retained") {
		t.Fatal("display migration did not protect the named view", e)
	}
	_, e = provider.DownTo(ctx, 45)
	if !errors.As(e, &rejection) || rejection.Code != "P0001" {
		t.Fatal("destructive rollback was not refused", e)
	}
	must(t, migrations.Up(ctx, f.env.Config.DatabaseURL))
	must(t, migrations.Check(ctx, f.env.Pool))
	page := readCall(t, named, "resources.query", dr.ResourcesRequest{Target: dr.Target{View: "r"}})
	var data dr.Page[dr.ResourceSummary]
	must(t, json.Unmarshal(dr.Bytes(page.Data), &data))
	if data.Total != "1" {
		t.Fatal("failed rollback damaged fixed scope")
	}
	t.Log("Legacy format-1 publication remains readable; unknown formats rejected; retained structure/file evidence prevents destructive rollback; re-upgrade preserves named reads")
}
