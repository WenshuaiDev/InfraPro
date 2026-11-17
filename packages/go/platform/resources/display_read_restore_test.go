//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type displayRestoreProof struct {
	harness   *displayHarness
	inventory display.BackupManifest
	directory string
	query     string
	terminal  string
}

func prepareDisplayReadRestore(t *testing.T, f monitoringFixture, p projectFixture) *displayRestoreProof {
	t.Helper()
	b := runtime.Compose(f.env.Service.Access(), f.storage)
	options := b.Displays.Options()
	options.Lease = 15 * time.Minute
	b.Displays.WithOptions(options)
	scopes := []dr.Definition{{ID: "r", StructureModel: 2, Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}, {ID: "c", Domain: "channels", Selector: dr.Selector{Kind: "ids", IDs: []string{f.channel}}, FieldGroups: []string{"resource.metadata", "statistics.counts", "monitoring.values"}}}
	scopes = append(scopes, dr.Definition{ID: "d", StructureModel: 2, Domain: "devices", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"statistics.classification", "statistics.counts"}})
	views := map[string]dr.Definition{}
	for _, d := range scopes {
		v := d
		v.ID = ""
		v.ScopeID = d.ID
		views[d.ID] = v
	}
	h := createReadDisplay(t, f.resourceFixture, p, b, scopes, views, []string{"views.read", "queries.read", "resources.directory", "statistics.resources", "statistics.structure", "statistics.deviceCategories", "statistics.monitoring", "monitoring.details"})
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-source", "display")
	must(t, e)
	for key, value := range h.store.files {
		must(t, store.Put(ctx, key, bytes.NewReader(value), int64(len(value)), "application/octet-stream"))
	}
	t.Cleanup(func() {
		for key := range h.store.files {
			if e := store.Delete(context.Background(), key); e != nil {
				t.Error(e)
			}
		}
	})
	h.s.WithStorage(store)
	proof := &displayRestoreProof{harness: h, directory: filepath.Join(os.Getenv("INFRAPRO_RESTORE_ARTIFACTS"), "display-objects")}
	must(t, os.MkdirAll(proof.directory, 0700))
	proof.inventory, e = h.s.BackupManifest(ctx)
	must(t, e)
	for _, file := range proof.inventory.Files {
		reader, e := store.Get(ctx, file.ObjectKey)
		must(t, e)
		content, e := io.ReadAll(reader)
		reader.Close()
		must(t, e)
		if displayHash(content) != file.SHA256 {
			t.Fatal("source display digest mismatch")
		}
		must(t, os.WriteFile(filepath.Join(proof.directory, file.SHA256), content, 0600))
	}
	r := readCall(t, h, "resources.query", dr.ResourcesRequest{Target: dr.Target{View: "r"}, Paging: dr.Paging{Limit: 1}})
	proof.query = r.Query.ID
	verifyStructureRestoreRead(t, h)
	stat := awaitDisplayRead(t, h, b, readCall(t, h, "statistics.monitoring", dr.MonitoringStatisticsRequest{Targets: []dr.Target{{View: "c"}}, SourceKinds: []string{"direct"}, GroupBy: "none", MonitoringTime: dr.MonitoringTime{AllHistory: true}}))
	counts := readData[dr.MonitoringStatistics](t, stat)
	if counts.Totals.Raw.Active == nil || *counts.Totals.Raw.Active != "10" {
		t.Fatal("pre-restore display count", counts.Totals)
	}
	term := h.command(t, "terminal.create", "", display.Command{Name: "restore fixed terminal", ValidUntil: time.Now().Add(time.Hour)})
	credential, e := h.s.PairTerminal(ctx, term["pair_code"].(string))
	must(t, e)
	_, _, proof.terminal, _, e = h.s.StartTerminal(ctx, credential)
	must(t, e)
	return proof
}
func (p *displayRestoreProof) verify(t *testing.T, pool *pgxpool.Pool, storage *md.Storage, actor access.Actor) {
	t.Helper()
	b := runtime.Compose(access.New(pool, slog.Default()), storage)
	store, e := blobstore.Open("http://127.0.0.1:59333", "restore-source", "restore-source-test-only", "restore-source", "display")
	must(t, e)
	for _, file := range p.inventory.Files {
		content, e := os.ReadFile(filepath.Join(p.directory, file.SHA256))
		must(t, e)
		must(t, store.Put(ctx, file.ObjectKey, bytes.NewReader(content), int64(len(content)), "application/octet-stream"))
	}
	b.Displays.WithStorage(store)
	missing, e := b.Displays.VerifyStorage(ctx)
	must(t, e)
	if len(missing) != 0 {
		t.Fatal("restored display files missing")
	}
	generation, e := b.Displays.InvalidateAfterRestore(ctx)
	must(t, e)
	if generation == p.inventory.Environment {
		t.Fatal("restore reused display environment")
	}
	if _, e = b.Displays.Query(ctx, p.harness.session, display.Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: p.query})}); e == nil {
		t.Fatal("restored cursor survived")
	}
	if _, _, _, _, e = b.Displays.StartTerminal(ctx, p.terminal); e == nil {
		t.Fatal("old fixed terminal survived restore")
	}
	must(t, b.DisplayReads.Cleanup(ctx))
	h := *p.harness
	h.s = b.Displays
	h.actor = actor
	must(t, pool.QueryRow(ctx, `SELECT revision::text FROM display_instances WHERE id=$1`, h.id).Scan(&h.revision))
	term := h.command(t, "terminal.create", "", display.Command{Name: "restored fixed terminal", ValidUntil: time.Now().Add(time.Hour)})
	credential, e := h.s.PairTerminal(ctx, term["pair_code"].(string))
	must(t, e)
	_, _, _, h.session, e = h.s.StartTerminal(ctx, credential)
	must(t, e)
	rows := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, &h, "resources.query", dr.ResourcesRequest{Target: dr.Target{View: "r"}}))
	if rows.Total == "0" {
		t.Fatal("restored fixed resource set empty")
	}
	result := awaitDisplayRead(t, &h, b, readCall(t, &h, "statistics.monitoring", dr.MonitoringStatisticsRequest{Targets: []dr.Target{{View: "c"}}, SourceKinds: []string{"direct"}, GroupBy: "none", MonitoringTime: dr.MonitoringTime{AllHistory: true}}))
	counts := readData[dr.MonitoringStatistics](t, result)
	if counts.Totals.Raw.Active == nil || *counts.Totals.Raw.Active != "10" {
		t.Fatal("restored display widened project data", counts.Totals)
	}
	verifyStructureRestoreRead(t, &h)
	t.Log("display format-2 scopes/config restored; source storage paused; package digests verified; old cursor/terminal rejected; fresh paired terminal reads exact authorized 10")
}
