//go:build monitoringintegration

package resources_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	goruntime "runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type displaySQLCounter struct{ count atomic.Int64 }

func (c *displaySQLCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}
func (*displaySQLCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Synthetic metadata is bulk inserted through the real schema constraints;
// all measured reads use the runtime, and observations use formal CSV adoption.
func displayBulkPoints(t *testing.T, f resourceFixture, p projectFixture, facility string, count int, offset int) {
	t.Helper()
	_, e := f.env.Pool.Exec(ctx, `WITH inserted AS (INSERT INTO engineering_points(tenant_id,facility_id,code,name,created_by,updated_by) SELECT $1,$2,'LOAD-P-'||n,'SDK 合成负载点 '||n,$3,$3 FROM generate_series($4::int,$5::int) n RETURNING id) INSERT INTO project_resource_grants(tenant_id,project_id,facility_id,link_id,resource_kind,point_id,source,granted_by) SELECT $1,$6,$2,l.id,'point',i.id,'tenant_grant',$3 FROM inserted i CROSS JOIN project_facility_links l WHERE l.project_id=$6 AND l.facility_id=$2 AND l.status='active'`, f.tenant, facility, f.actor.ID, offset+1, offset+count, p.id)
	must(t, e)
}
func TestDisplayReadFiftyThousandMemberBoundary(t *testing.T) {
	if os.Getenv("INFRAPRO_DISPLAY_READ_LOAD") != "1" {
		t.Skip("explicit load laboratory required")
	}
	f := resourcesSetup(t)
	p := f.project(t, "SDKMemberLimit")
	facility := f.facility(t, "LIMIT")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	b := runtime.Compose(f.env.Service.Access())
	options := b.Displays.Options()
	options.Lease = 15 * time.Minute
	b.Displays.WithOptions(options)
	d := dr.Definition{ID: "r", Domain: "resources", Selector: dr.Selector{Kind: "project"}, IncludeFuture: true, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	v := d
	v.ID = ""
	v.ScopeID = "r"
	h := createReadDisplay(t, f, p, b, []dr.Definition{d}, map[string]dr.Definition{"r": v}, []string{"views.read", "queries.read", "resources.directory", "statistics.resources"})
	displayBulkPoints(t, f, p, facility.ID, 49999, 0)
	start := time.Now()
	preview, e := h.s.ReadScopePreview(ctx, p.actor, h.scope, h.id, display.ReadScopePreviewInput{Purpose: "grant", Definition: d})
	must(t, e)
	if preview.MemberCount != "50000" {
		t.Fatal(preview.MemberCount)
	}
	t.Logf("member boundary 50000 resolved in %s", time.Since(start))
	complete := readData[dr.ResourceStatistics](t, readCall(t, h, "statistics.resources", dr.ResourceStatisticsRequest{Target: dr.Target{View: "r"}, GroupBy: "none"}))
	if complete.Totals.PointCount != "49999" {
		t.Fatal("large total truncated", complete.Totals)
	}
	displayBulkPoints(t, f, p, facility.ID, 1, 49999)
	_, e = h.s.ReadScopePreview(ctx, p.actor, h.scope, h.id, display.ReadScopePreviewInput{Purpose: "grant", Definition: d})
	var problem *dr.Error
	if !errors.As(e, &problem) || problem.Code != "budget_exceeded" {
		t.Fatal("50001 must be rejected", e)
	}
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "statistics.resources", Params: dr.Bytes(dr.ResourceStatisticsRequest{Target: dr.Target{View: "r"}, GroupBy: "none"})})
	if !errors.As(e, &problem) || problem.Code != "budget_exceeded" {
		t.Fatal("50001 runtime query must be rejected", e)
	}
	t.Log("50000 accepted with exact total; 50001 preview and runtime rejected")
}
func TestDisplayReadLoadTenInstances(t *testing.T) {
	if os.Getenv("INFRAPRO_DISPLAY_READ_LOAD") != "1" {
		t.Skip("explicit load laboratory required")
	}
	f := monitoringSetup(t)
	p := f.project(t, "SDKLoad")
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	g := f.accessInput(t, p, facility, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Point, PointID: &point, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, p, facility, g)
	displayBulkPoints(t, f.resourceFixture, p, facility, 1998, 0)
	_, e := f.env.Pool.Exec(ctx, `WITH inserted AS (INSERT INTO engineering_channels(tenant_id,facility_id,point_id,code,name,metric_version_id,unit_version_id,timestamp_meaning,created_by,updated_by) SELECT tenant_id,facility_id,point_id,'LOAD-C-'||n,'SDK 合成负载通道 '||n,metric_version_id,unit_version_id,timestamp_meaning,created_by,updated_by FROM engineering_channels CROSS JOIN generate_series(1,599) n WHERE id=$1 RETURNING id) INSERT INTO project_resource_grants(tenant_id,project_id,facility_id,link_id,resource_kind,channel_id,source,granted_by) SELECT $2,$3,$4,l.id,'channel',i.id,'tenant_grant',$5 FROM inserted i CROSS JOIN project_facility_links l WHERE l.project_id=$3 AND l.facility_id=$4 AND l.status='active'`, f.channel, f.tenant, p.id, facility, f.actor.ID)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO monitoring_data_grants(tenant_id,project_id,channel_id,resource_grant_id,permitted,capabilities,granted_by,reason) SELECT tenant_id,project_id,channel_id,id,'(,)'::int8range,ARRAY['read','history'],$2,'SDK disposable load fixture' FROM project_resource_grants WHERE project_id=$1 AND resource_kind='channel'`, p.id, f.actor.ID)
	must(t, e)
	rows, e := f.env.Pool.Query(ctx, `SELECT id::text FROM engineering_channels WHERE tenant_id=$1 ORDER BY id`, f.tenant)
	must(t, e)
	channels := []string{}
	for rows.Next() {
		var id string
		must(t, rows.Scan(&id))
		channels = append(channels, id)
	}
	must(t, rows.Err())
	rows.Close()
	if len(channels) != 600 {
		t.Fatal(len(channels))
	}
	mapping := md.Mapping{Format: "csv", Mode: "long", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, ChannelColumn: "channel", TimeColumn: "observed_at", ValueColumn: "value", QualityColumn: "quality", Timezone: "UTC", EmptyWide: "error"}
	for i, id := range channels {
		mapping.Channels = append(mapping.Channels, md.ChannelMapping{Alias: fmt.Sprint(i), Channel: id, Unit: "mm"})
	}
	var content strings.Builder
	content.WriteString("channel,observed_at,value,quality\n")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range 100000 {
		fmt.Fprintf(&content, "%d,%s,%d,good\n", i%600, base.Add(time.Duration(i/600)*time.Minute).Format(time.RFC3339), i)
	}
	start := time.Now()
	f.publish(t, f.validate(t, f.uploadBytes(t, "load.csv", []byte(content.String()), mapping)))
	t.Logf("100000 formal observations adopted in %s", time.Since(start))
	content.Reset()
	tracer := &displaySQLCounter{}
	cfg, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
	must(t, e)
	cfg.ConnConfig.Tracer = tracer
	cfg.MaxConns = 40
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	must(t, e)
	defer pool.Close()
	b := runtime.Compose(access.New(pool, slog.Default()), f.storage)
	options := b.Displays.Options()
	options.Lease = 15 * time.Minute
	b.Displays.WithOptions(options)
	scopes := []dr.Definition{{ID: "r", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}, {ID: "c", Domain: "channels", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts", "monitoring.values"}}}
	views := map[string]dr.Definition{}
	for _, d := range scopes {
		v := d
		v.ID = ""
		v.ScopeID = d.ID
		views[d.ID] = v
	}
	screens := []*displayHarness{}
	for range 10 {
		screens = append(screens, createReadDisplay(t, f.resourceFixture, p, b, scopes, views, []string{"views.read", "queries.read", "resources.directory", "channels.read", "statistics.monitoring"}))
	}
	// Complete the normal publication maintenance before measuring display
	// queries. Persistent per-channel indexes are formal monitoring output;
	// every display query/cache is still cold. Record this setup separately.
	maintenanceStarted := time.Now()
	var maintenance sync.WaitGroup
	maintenanceErrors := make(chan error, 4)
	for i := range 4 {
		maintenance.Add(1)
		go func() {
			defer maintenance.Done()
			for {
				worked, err := b.Monitoring.RunOne(ctx, fmt.Sprintf("load-publication-%d", i))
				if err != nil {
					maintenanceErrors <- err
					return
				}
				if !worked {
					return
				}
			}
		}()
	}
	maintenance.Wait()
	close(maintenanceErrors)
	for err := range maintenanceErrors {
		must(t, err)
	}
	maintenanceMS := float64(time.Since(maintenanceStarted).Microseconds()) / 1000
	t.Logf("formal publication indexes completed in %.0fms; display cache empty", maintenanceMS)
	// The reference backend runs a small read pool beside the owning summary
	// workers, as in the deployed monitoring-worker process.
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	workerErrors := make(chan error, 8)
	for i := range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for workCtx.Err() == nil {
				did, e := b.Monitoring.RunOne(workCtx, fmt.Sprintf("load-owning-%d", i))
				if e != nil && workCtx.Err() == nil {
					workerErrors <- e
					return
				}
				if !did {
					select {
					case <-workCtx.Done():
						return
					case <-time.After(30 * time.Millisecond):
					}
				}
			}
		}()
	}
	for i := range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if e := b.DisplayReads.RunWorker(workCtx, fmt.Sprintf("load-display-%d", i)); e != nil && workCtx.Err() == nil {
				workerErrors <- e
			}
		}()
	}
	defer func() {
		cancel()
		workers.Wait()
		close(workerErrors)
		for e := range workerErrors {
			must(t, e)
		}
	}()
	var memory goruntime.MemStats
	var peak atomic.Uint64
	stopMem := make(chan struct{})
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stopMem:
				return
			case <-tick.C:
				goruntime.ReadMemStats(&memory)
				for old := peak.Load(); memory.HeapAlloc > old && !peak.CompareAndSwap(old, memory.HeapAlloc); old = peak.Load() {
				}
			}
		}
	}()
	defer close(stopMem)
	statsQuery := dr.Bytes(dr.MonitoringStatisticsRequest{Targets: []dr.Target{{View: "c"}}, SourceKinds: []string{"direct"}, GroupBy: "none", MonitoringTime: dr.MonitoringTime{AllHistory: true}})
	sqlStart := tracer.count.Load()
	cold := make([]float64, 10)
	var wg sync.WaitGroup
	failures := make(chan error, 20)
	for i, h := range screens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			r, e := h.s.Query(ctx, h.session, display.Call{Method: "statistics.monitoring", Params: statsQuery, Generation: "load"})
			for e == nil && r.State == "pending" && time.Since(started) < 120*time.Second {
				time.Sleep(100 * time.Millisecond)
				r, e = h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: r.Query.ID}), Generation: "load"})
			}
			if e != nil {
				failures <- e
				return
			}
			if r.State != "ready" {
				failures <- fmt.Errorf("cold state %s", r.State)
				return
			}
			var s dr.MonitoringStatistics
			if e = json.Unmarshal(dr.Bytes(r.Data), &s); e != nil {
				failures <- e
				return
			}
			if s.Totals.Raw.Active == nil || *s.Totals.Raw.Active != "100000" || s.Totals.RawChannels != "600" {
				failures <- fmt.Errorf("incorrect load total %s", dr.Bytes(s.Totals))
				return
			}
			cold[i] = float64(time.Since(started).Microseconds()) / 1000
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		must(t, e)
	}
	// Warm all sessions together immediately before each measured batch, so
	// sequential setup cannot consume another session's ten-second cache TTL.
	// Keep the real workers available throughout; pending is never a hot pass.
	hot := []float64{}
	directory := []float64{}
	for iteration := range 3 {
		warming := make(chan error, 10)
		for _, h := range screens {
			wg.Add(1)
			go func() {
				defer wg.Done()
				started := time.Now()
				r, e := h.s.Query(ctx, h.session, display.Call{Method: "statistics.monitoring", Params: statsQuery, Generation: "warm"})
				for e == nil && r.State == "pending" && time.Since(started) < 120*time.Second {
					time.Sleep(100 * time.Millisecond)
					r, e = h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: r.Query.ID}), Generation: "warm"})
				}
				if e != nil {
					warming <- e
				} else if r.State != "ready" {
					warming <- fmt.Errorf("prewarm state %s", r.State)
				}
			}()
		}
		wg.Wait()
		close(warming)
		for e := range warming {
			must(t, e)
		}
		times := make(chan [2]float64, 10)
		errs := make(chan error, 10)
		for _, h := range screens {
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := time.Now()
				r, e := h.s.Query(ctx, h.session, display.Call{Method: "statistics.monitoring", Params: statsQuery, Generation: "hot"})
				if e != nil {
					errs <- e
					return
				}
				if r.State != "ready" {
					errs <- fmt.Errorf("warm result was %s", r.State)
					return
				}
				s := float64(time.Since(start).Microseconds()) / 1000
				start = time.Now()
				r, e = h.s.Query(ctx, h.session, display.Call{Method: "resources.query", Params: dr.Bytes(dr.ResourcesRequest{Target: dr.Target{View: "r"}, Paging: dr.Paging{Limit: 50}}), Generation: "page"})
				if e != nil {
					errs <- e
					return
				}
				var page dr.Page[dr.ResourceSummary]
				e = json.Unmarshal(dr.Bytes(r.Data), &page)
				if e != nil || page.Total != "2600" {
					errs <- fmt.Errorf("directory total invalid %s: %v", page.Total, e)
					return
				}
				times <- [2]float64{s, float64(time.Since(start).Microseconds()) / 1000}
			}()
		}
		wg.Wait()
		close(times)
		close(errs)
		for e := range errs {
			must(t, e)
		}
		for pair := range times {
			hot = append(hot, pair[0])
			directory = append(directory, pair[1])
		}
		_ = iteration
	}
	// Page all channels through the immutable query, independently of the old
	// explicit binding limit; both total and unique IDs must be exactly 600.
	page := readData[dr.Page[dr.ChannelSummary]](t, readCall(t, screens[0], "channels.list", dr.ChannelsRequest{Target: dr.Target{View: "c"}, Paging: dr.Paging{Limit: 200}}))
	seen := map[string]bool{}
	for {
		for _, v := range page.Items {
			if seen[v.ID] {
				t.Fatal("duplicate channel page")
			}
			seen[v.ID] = true
		}
		if !page.HasMore {
			break
		}
		page = readData[dr.Page[dr.ChannelSummary]](t, readCall(t, screens[0], "queries.get", dr.QueryRequest{QueryID: page.QueryID, Cursor: page.NextCursor, Limit: 200}))
	}
	if len(seen) != 600 || page.Total != "600" {
		t.Fatal("600-channel pagination truncated")
	}
	resourcesPage := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, screens[0], "resources.query", dr.ResourcesRequest{Target: dr.Target{View: "r"}, Paging: dr.Paging{Limit: 200}}))
	resourceIDs := map[string]bool{}
	for {
		for _, v := range resourcesPage.Items {
			if resourceIDs[v.ID] {
				t.Fatal("duplicate resource page")
			}
			resourceIDs[v.ID] = true
		}
		if !resourcesPage.HasMore {
			break
		}
		resourcesPage = readData[dr.Page[dr.ResourceSummary]](t, readCall(t, screens[0], "queries.get", dr.QueryRequest{QueryID: resourcesPage.QueryID, Cursor: resourcesPage.NextCursor}))
	}
	if len(resourceIDs) != 2600 || resourcesPage.Total != "2600" {
		t.Fatal("resource pagination truncated")
	}
	coldCancel := readCall(t, screens[0], "statistics.monitoring", dr.MonitoringStatisticsRequest{Targets: []dr.Target{{View: "c"}}, SourceKinds: []string{"direct"}, GroupBy: "none", MonitoringTime: dr.MonitoringTime{AllHistory: true, Qualities: []string{"bad"}}})
	start = time.Now()
	readCall(t, screens[0], "queries.cancel", dr.QueryControlRequest{QueryID: coldCancel.Query.ID})
	cancelMS := float64(time.Since(start).Microseconds()) / 1000
	metrics, e := b.Monitoring.MaintenanceStatus(ctx)
	must(t, e)
	queueWait := []float64{}
	queueRows, e := pool.Query(ctx, `SELECT extract(epoch FROM (first_started_at-created_at))*1000 FROM display_read_jobs WHERE first_started_at IS NOT NULL ORDER BY created_at`)
	must(t, e)
	for queueRows.Next() {
		var ms float64
		must(t, queueRows.Scan(&ms))
		queueWait = append(queueWait, ms)
	}
	must(t, queueRows.Err())
	queueRows.Close()
	quantiles := func(values []float64) map[string]any {
		v := slices.Clone(values)
		slices.Sort(v)
		return map[string]any{"samples_ms": values, "p50_ms": v[(len(v)-1)/2], "p95_ms": v[(len(v)*95+99)/100-1]}
	}
	report := map[string]any{"at": time.Now().UTC(), "objects": 2600, "channels": 600, "observations": 100000, "instances": 10, "go": goruntime.Version(), "os": goruntime.GOOS, "arch": goruntime.GOARCH, "cpus": goruntime.NumCPU(), "pg_pool_max": 40, "display_workers": 4, "owning_workers": 4, "cold": quantiles(cold), "hot": quantiles(hot), "directory": quantiles(directory), "sql_statements": tracer.count.Load() - sqlStart, "heap_peak_bytes": peak.Load(), "cancel_ms": cancelMS, "replicas": metrics.Replicas}
	report["publication_index_setup_ms"] = maintenanceMS
	report["cold_definition"] = "display queries and result cache empty; formal publication indexes ready"
	if len(queueWait) > 0 {
		report["queue_wait"] = quantiles(queueWait)
	} else {
		report["queue_wait"] = map[string]any{"samples_ms": queueWait, "note": "all measured queries completed synchronously"}
	}
	t.Logf("DISPLAY_READ_LOAD %s", dr.Bytes(report))
	if path := os.Getenv("INFRAPRO_DISPLAY_READ_LOAD_REPORT"); path != "" {
		must(t, os.WriteFile(path, dr.Bytes(report), 0600))
	}
	for _, values := range [][]float64{directory, hot} {
		slices.Sort(values)
		if values[(len(values)*95+99)/100-1] > 2000 {
			t.Fatal("p95 exceeded 2 seconds")
		}
	}
	for _, v := range cold {
		if v > 120000 {
			t.Fatal("cold deadline exceeded")
		}
	}
}
