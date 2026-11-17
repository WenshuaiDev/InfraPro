package resources_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5"
)

func TestDisplayReadWorkerFencingCancellationDeadlineAndRestore(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ReadLifecycle")
	facility := f.facility(t, "LIFE")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	a := displayReadPlanAccess(t, f)
	b := runtime.Compose(a)
	o := b.Displays.Options()
	o.Lease = 15 * time.Minute
	b.Displays.WithOptions(o)
	d := dr.Definition{ID: "r", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	v := d
	v.ID = ""
	v.ScopeID = "r"
	h := createReadDisplay(t, f, p, b, []dr.Definition{d}, map[string]dr.Definition{"r": v}, []string{"views.read", "queries.read", "resources.directory", "statistics.resources"})
	// Fault-controlled executor exercises the production durable coordinator and
	// real authorization callbacks. Domain correctness is covered separately.
	coord := dr.New(a)
	authority := h.s.ReadAuthority(h.session)
	coord.WithAuthority(func(context.Context, dr.Owner) (dr.Authority, error) { return authority, nil })
	var run atomic.Int32
	started := make(chan int, 8)
	unblock := make(chan struct{})
	coord.RegisterWork("statistics.resources", func(context.Context, pgx.Tx, dr.Plan) (dr.Materialized, error) {
		return dr.Materialized{Work: json.RawMessage(`{"pending":true}`)}, nil
	}, func(ctx context.Context, _ dr.Plan, _ json.RawMessage) (dr.Materialized, error) {
		n := int(run.Add(1))
		started <- n
		if n == 1 {
			select {
			case <-unblock:
			case <-ctx.Done():
				return dr.Materialized{}, ctx.Err()
			}
		}
		return dr.Materialized{Single: map[string]any{"winner": n}}, nil
	}, nil)
	raw := dr.Bytes(dr.ResourceStatisticsRequest{Target: dr.Target{View: "r"}, GroupBy: "none"})
	first, e := coord.Read(ctx, authority, "statistics.resources", raw, "one")
	must(t, e)
	if first.State != "pending" {
		t.Fatal(first)
	}
	done := make(chan error, 1)
	go func() { _, e := coord.ProcessOne(ctx, "old"); done <- e }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never started")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_read_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE query_id=$1`, first.Query.ID)
	must(t, e)
	worked, e := coord.ProcessOne(ctx, "replacement")
	must(t, e)
	if !worked {
		t.Fatal("expired lease was not reclaimed")
	}
	close(unblock)
	must(t, <-done)
	result, e := coord.Read(ctx, authority, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: first.Query.ID}), "one")
	must(t, e)
	if string(dr.Bytes(result.Data)) != `{"winner":2}` {
		t.Fatal("stale worker overwrote replacement", string(dr.Bytes(result)))
	}
	var attempts, fence int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT attempts,fence FROM display_read_jobs WHERE query_id=$1`, first.Query.ID).Scan(&attempts, &fence))
	if attempts != 2 || fence != 2 {
		t.Fatal(attempts, fence)
	}
	// Released callers cannot cancel an independently acquired cached consumer.
	other, e := coord.Read(ctx, authority, "statistics.resources", raw, "other")
	must(t, e)
	if other.Query.ID == first.Query.ID || other.Query.SnapshotID != first.Query.SnapshotID {
		t.Fatal("bad cache sharing")
	}
	for range 2 {
		_, e = coord.Read(ctx, authority, "queries.release", dr.Bytes(dr.QueryControlRequest{QueryID: first.Query.ID}), "one")
		must(t, e)
	}
	_, e = coord.Read(ctx, authority, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: other.Query.ID}), "other")
	must(t, e)
	nextRaw := dr.Bytes(dr.ResourceStatisticsRequest{Target: dr.Target{View: "r"}, GroupBy: "facility"})
	timed, e := coord.Read(ctx, authority, "statistics.resources", nextRaw, "timeout")
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_read_jobs SET deadline=clock_timestamp()-interval '1 second' WHERE query_id=$1`, timed.Query.ID)
	must(t, e)
	_, e = coord.ProcessOne(ctx, "deadline")
	must(t, e)
	_, e = coord.Read(ctx, authority, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: timed.Query.ID}), "timeout")
	var problem *dr.Error
	if !errors.As(e, &problem) || problem.Code != "query_timeout" {
		t.Fatalf("deadline: %v", e)
	}
	for _, failure := range []string{"dependency_unavailable", "query_failed"} {
		q := dr.New(f.env.Service.Access())
		q.WithAuthority(func(context.Context, dr.Owner) (dr.Authority, error) { return authority, nil })
		q.RegisterWork("resources.query", func(context.Context, pgx.Tx, dr.Plan) (dr.Materialized, error) {
			return dr.Materialized{Work: json.RawMessage(`{}`)}, nil
		}, func(context.Context, dr.Plan, json.RawMessage) (dr.Materialized, error) {
			return dr.Materialized{}, dr.Problem(failure, "injected owning dependency failure", 503)
		}, nil)
		pending, e := q.Read(ctx, authority, "resources.query", dr.Bytes(dr.ResourcesRequest{Target: dr.Target{View: "r"}, Search: failure}), failure)
		must(t, e)
		_, e = q.ProcessOne(ctx, "dependency")
		must(t, e)
		_, e = q.Read(ctx, authority, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: pending.Query.ID}), failure)
		if !errors.As(e, &problem) || problem.Code != failure {
			t.Fatal("dependency must not be empty", e)
		}
	}
	// Query cleanup after environment recovery cannot delete business records.
	before, e := h.s.BackupManifest(ctx)
	must(t, e)
	generation, e := h.s.InvalidateAfterRestore(ctx)
	must(t, e)
	if generation == before.Environment {
		t.Fatal("environment generation unchanged")
	}
	_, e = coord.Read(ctx, authority, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: other.Query.ID}), "other")
	if e == nil {
		t.Fatal("restored environment accepted old subject")
	}
	must(t, coord.Cleanup(ctx))
	var queries, resources int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM display_read_queries),(SELECT count(*) FROM engineering_facilities WHERE id=$1)`, facility.ID).Scan(&queries, &resources))
	if queries != 0 || resources != 1 {
		t.Fatal("cleanup changed business data", queries, resources)
	}
	t.Log("expired worker fenced; independent release; deadline and dependency failures; environment recovery and cleanup passed")
}

func TestDisplayReadWorkerInstanceConcurrencyAndRevocation(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ReadConcurrency")
	facility := f.facility(t, "QUEUE")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	b := runtime.Compose(f.env.Service.Access())
	d := dr.Definition{ID: "r", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	v := d
	v.ID = ""
	v.ScopeID = "r"
	h := createReadDisplay(t, f, p, b, []dr.Definition{d}, map[string]dr.Definition{"r": v}, []string{"views.read", "queries.read", "resources.directory"})
	coord := dr.New(f.env.Service.Access())
	authority := h.s.ReadAuthority(h.session)
	coord.WithAuthority(func(context.Context, dr.Owner) (dr.Authority, error) { return authority, nil })
	entered := make(chan bool, 4)
	release := make(chan struct{})
	coord.RegisterWork("resources.query", func(context.Context, pgx.Tx, dr.Plan) (dr.Materialized, error) {
		return dr.Materialized{Work: json.RawMessage(`{}`)}, nil
	}, func(c context.Context, _ dr.Plan, _ json.RawMessage) (dr.Materialized, error) {
		entered <- true
		select {
		case <-release:
			return dr.Materialized{Single: map[string]bool{"secret": true}}, nil
		case <-c.Done():
			return dr.Materialized{}, c.Err()
		}
	}, nil)
	ids := []string{}
	for i := range 4 {
		r, e := coord.Read(ctx, authority, "resources.query", dr.Bytes(dr.ResourcesRequest{Target: dr.Target{View: "r"}, Search: fmt.Sprint(i)}), "queue")
		must(t, e)
		ids = append(ids, r.Query.ID)
	}
	_, e := coord.Read(ctx, authority, "resources.query", dr.Bytes(dr.ResourcesRequest{Target: dr.Target{View: "r"}, Search: "5"}), "queue")
	var problem *dr.Error
	if !errors.As(e, &problem) || problem.Code != "budget_exceeded" {
		t.Fatal("queue limit", e)
	}
	done := make(chan error, 2)
	for i := range 2 {
		go func() { _, e := coord.ProcessOne(ctx, fmt.Sprint(i)); done <- e }()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not enter")
		}
	}
	worked, e := coord.ProcessOne(ctx, "third")
	must(t, e)
	if worked {
		t.Fatal("third active calculation exceeded per-instance limit")
	}
	// Revoke while both executors are outside the database; final delivery must
	// recheck the current grant even though it was valid when each job started.
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
	close(release)
	for range 2 {
		must(t, <-done)
	}
	for _, id := range ids {
		_, e = coord.Read(ctx, authority, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: id}), "queue")
		if e == nil {
			t.Fatal("revoked result delivered", id)
		}
	}
}
