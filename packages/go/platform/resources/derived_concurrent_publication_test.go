//go:build monitoringintegration

package resources_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

// Hold one real object PUT after the worker has read the current head and
// sealed its result blocks, outside every PostgreSQL transaction. Mutations
// below therefore race with an actual in-flight publication, without changing
// the production clock, executor, source reader or publication implementation.
func TestDerivedConcurrentPublicationRechecksAndMerges(t *testing.T) {
	for _, scenario := range []string{"unrelated_append", "source_revocation", "configuration_correction", "disjoint_publication"} {
		t.Run(scenario, func(t *testing.T) { verifyConcurrentDerivedPublication(t, scenario) })
	}
}

func verifyConcurrentDerivedPublication(t *testing.T, scenario string) {
	var armed atomic.Bool
	arrived, released := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	target, e := url.Parse("http://127.0.0.1:58333")
	must(t, e)
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/derived/history/") && armed.CompareAndSwap(true, false) {
			close(arrived)
			select {
			case <-released:
			case <-r.Context().Done():
				return
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(release)
	f := monitoringSetup(t, func(c *md.StorageConfig) { c.ObjectEndpoint = server.URL })
	service, channel, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	scope := dm.TenantScope(f.tenant)
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(minutes int) string {
		return strconv.FormatInt(base.Add(time.Duration(minutes)*time.Minute).UnixNano(), 10)
	}
	from, end := ns(0), ns(6)
	trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &end}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil {
		t.Fatal("initial trial did not complete", trial.Job.State)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &end, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	enabled, e := service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	channel = enabled.Channel
	drain := func() {
		t.Helper()
		for range 12 {
			_, e := service.ScheduleTenant(ctx, f.tenant)
			must(t, e)
			var jobs []string
			rows, e := f.env.Pool.Query(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued' ORDER BY j.created_at`, f.tenant)
			must(t, e)
			for rows.Next() {
				var id string
				must(t, rows.Scan(&id))
				jobs = append(jobs, id)
			}
			must(t, rows.Err())
			rows.Close()
			if len(jobs) == 0 {
				return
			}
			for _, id := range jobs {
				_, e = f.service.RunTask(ctx, "concurrent-drain", f.tenant, id)
				must(t, e)
			}
		}
		t.Fatal("automatic backlog did not converge")
	}
	_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	drain()
	var originalRoot string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_root FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&originalRoot))
	backfill := func(a, b int) dm.Run {
		t.Helper()
		to := ns(b)
		p, e := service.PreviewBackfill(ctx, f.actor, scope, channel.Resource.ID, dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: dm.NanosecondRange{From: ns(a), To: &to}})
		must(t, e)
		v, e := service.BackfillChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ConfigRevision), PreviewID: p.ID, PreviewHash: p.Hash}, access.Meta{})
		must(t, e)
		if len(v.Runs) != 1 {
			t.Fatal("unexpected backfill targets", v)
		}
		return v.Runs[0]
	}
	firstEnd := 3
	if scenario == "disjoint_publication" {
		firstEnd = 1
	}
	first := backfill(0, firstEnd)
	var correction dm.ConfigPreview
	var correctedID string
	if scenario == "configuration_correction" {
		next, e := service.SaveChannelConfig(ctx, f.actor, scope, channel.Resource.ID, dm.SaveConfigInput{WriteMeta: restoreMeta(channel.ConfigRevision), BaseConfigID: cfg.ID, DefinitionVersionID: cfg.DefinitionVersionID, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters, ChangeKind: "ordinary", NumericContract: &dm.NumericContract{Scale: 24, Rounding: "half_even"}}, access.Meta{})
		must(t, e)
		correctedID = next.Config.ID
		to := ns(2)
		v, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: next.Config.ID, ConfigHash: next.Config.ConfigHash, OutputRange: dm.NanosecondRange{From: ns(1), To: &to}}, access.Meta{})
		must(t, e)
		if v.Trial == nil {
			t.Fatal("correction trial did not complete", v.Job.State)
		}
		correction, e = service.PreviewConfig(ctx, f.actor, scope, channel.Resource.ID, dm.ConfigPreviewInput{Revision: channel.ConfigRevision, ConfigProposal: dm.ConfigProposal{ConfigID: next.Config.ID, Mode: "correction", Range: dm.NanosecondRange{From: ns(1), To: &to}, TrialReceiptID: v.Trial.ReceiptID}})
		must(t, e)
	}
	workerCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	done := make(chan error, 1)
	armed.Store(true)
	go func() {
		_, err := f.service.RunTask(workerCtx, "held-publication", f.tenant, first.Job.ID)
		done <- err
		close(done)
	}()
	defer func() {
		release()
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()
	select {
	case <-arrived:
	case e := <-done:
		t.Fatal("worker ended before the publication barrier", e)
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not reach the publication barrier")
	}
	var second dm.Run
	switch scenario {
	case "unrelated_append":
		f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:04:00Z,20,good\n2024-01-01T00:05:00Z,30,good\n")))
		_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
		must(t, e)
	case "source_revocation":
		p, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
		must(t, e)
		_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: p.Hash})
		must(t, e)
	case "configuration_correction":
		v, e := service.ApplyConfig(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ConfigRevision), PreviewID: correction.ID, PreviewHash: correction.Hash}, access.Meta{})
		must(t, e)
		channel = v.Channel
	case "disjoint_publication":
		second = backfill(1, 3)
		_, e = f.service.RunTask(ctx, "concurrent-second-range", f.tenant, second.Job.ID)
		must(t, e)
	}
	release()
	select {
	case e = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("held publication did not finish after release")
	}
	var state, code, root string
	var publications int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT j.state,COALESCE(j.error_code,''),h.current_root,(SELECT count(*) FROM derived_generations WHERE run_id=$3) FROM monitoring_jobs j CROSS JOIN derived_channel_heads h WHERE j.id=$1 AND h.tenant_id=$2 AND h.channel_id=$4`, first.Job.ID, f.tenant, first.ID, channel.Resource.ID).Scan(&state, &code, &root, &publications))
	if scenario == "source_revocation" || scenario == "configuration_correction" {
		if e == nil || state == "succeeded" || publications != 0 || root != originalRoot {
			t.Fatalf("superseded worker published: state=%s code=%s publications=%d rootChanged=%t err=%v", state, code, publications, root != originalRoot, e)
		}
	} else {
		must(t, e)
		if state != "succeeded" || publications != 1 {
			t.Fatal("valid fixed target failed", state, code, publications)
		}
	}
	if scenario == "unrelated_append" {
		var dirty int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&dirty))
		if dirty == 0 {
			t.Fatal("older completion erased the newly appended target")
		}
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := base.Format(time.RFC3339Nano), base.Add(6*time.Minute).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 20}
	if scenario == "source_revocation" {
		page, e := f.service.Details(ctx, f.actor, f.scope, query)
		must(t, e)
		if len(page.Items) != 0 {
			t.Fatal("revoked source escaped before invalidation consumer")
		}
		return
	}
	if scenario != "disjoint_publication" {
		drain()
	}
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	sort.Slice(page.Items, func(i, j int) bool { return page.Items[i].Calculated.AtNS < page.Items[j].Calculated.AtNS })
	want := 3
	if scenario == "unrelated_append" {
		want = 5
	}
	if len(page.Items) != want {
		t.Fatalf("lost current rows: got %d want %d", len(page.Items), want)
	}
	if scenario == "configuration_correction" {
		middle := page.Items[1].Calculated
		if middle.Config != correctedID || middle.Scale != 24 || *middle.Value != "3.000000000000000000000000" {
			t.Fatal("correction was overwritten", middle)
		}
	}
	if scenario == "disjoint_publication" {
		var firstGeneration, secondGeneration string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT id::text FROM derived_generations WHERE run_id=$1),(SELECT id::text FROM derived_generations WHERE run_id=$2)`, first.ID, second.ID).Scan(&firstGeneration, &secondGeneration))
		if page.Items[0].Calculated.Generation != firstGeneration || page.Items[1].Calculated.Generation != secondGeneration || page.Items[2].Calculated.Generation != secondGeneration {
			t.Fatal("head retry lost one disjoint publication", page.Coverage)
		}
	}
	t.Logf("real sealed publication barrier: %s, old task %s/%s, current rows %d", scenario, state, code, want)
}
