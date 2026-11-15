package resources_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestTaskServerSortingAndMultiTypePagination(t *testing.T) {
	f := resourcesSetup(t)
	source := &taskVisibilitySource{values: map[string]tasks.View{}}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for n := 0; n < 16; n++ {
		ref := taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: uuid.NewString()}
		kind := "monitoring.export"
		if n%2 == 1 {
			kind = "monitoring.query"
		}
		change := taskrecord.Change{SourceRef: ref, Type: kind, Scope: taskrecord.Scope{Kind: "tenant", Tenant: f.tenant}, Originator: f.actor.ID, OriginKind: "person", RequestedBy: f.actor.ID, ExecutorKind: "person", ExecutorRef: f.actor.ID, Generation: "1", Revision: "1", ControlRevision: "1", State: "queued", SourceState: "queued", Phase: "queued", InputRef: map[string]string{"job_id": ref.RootID}, InputHash: "fixed", CreatedAt: now.Add(-time.Duration(n/2+1) * time.Minute), ObservedAt: now, Fence: "0", Event: "queued"}
		must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := taskrecord.ApplyTx(ctx, tx, change); return e }))
		if n%5 != 0 {
			source.values[ref.ID()] = tasks.Present(change, "排序与筛选契约")
		}
	}
	s := tasks.New(f.env.Service.Access(), map[string]tasks.Source{"monitoring": source})
	for _, direction := range []string{"newest", "oldest"} {
		for _, kinds := range []string{"monitoring.export", "monitoring.query,monitoring.export"} {
			filter := tasks.Filter{Sort: direction, Type: kinds, Limit: 3}
			expected := []tasks.View{}
			for _, item := range source.values {
				if kinds != "monitoring.export" || item.Type == kinds {
					expected = append(expected, item)
				}
			}
			sort.Slice(expected, func(i, j int) bool {
				less := expected[i].CreatedAt.Before(expected[j].CreatedAt)
				if expected[i].CreatedAt.Equal(expected[j].CreatedAt) {
					less = expected[i].ID < expected[j].ID
				}
				if direction == "newest" {
					return !less
				}
				return less
			})
			got := []tasks.View{}
			for {
				page, e := s.List(ctx, f.actor, filter)
				must(t, e)
				got = append(got, page.Items...)
				if page.NextCursor == "" {
					break
				}
				if len(got) > len(expected) {
					t.Fatal("pagination loop")
				}
				wrong := filter
				wrong.Cursor = page.NextCursor
				if direction == "newest" {
					wrong.Sort = "oldest"
				} else {
					wrong.Sort = "newest"
				}
				if _, e = s.List(ctx, f.actor, wrong); e == nil {
					t.Fatal("cursor accepted changed ordering")
				}
				filter.Cursor = page.NextCursor
				if kinds != "monitoring.export" {
					filter.Type = "monitoring.export,monitoring.query"
				}
			}
			if len(got) != len(expected) {
				t.Fatalf("%s %s: got %d expected %d", direction, kinds, len(got), len(expected))
			}
			for i := range expected {
				if expected[i].ID != got[i].ID {
					t.Fatalf("%s %s: order or authorization mismatch at %d", direction, kinds, i)
				}
			}
		}
	}
	options := tasks.DefaultOptions()
	// A specific ID search must not spend the live-source budget on unrelated
	// rows. Both task and source prefixes retain post-filter authorization.
	options.CandidateBudget = 2
	options.CandidateBatch = 2
	s.WithOptions(options)
	for _, item := range source.values {
		for _, term := range []string{strings.ToUpper(item.ID[:8]), item.Source.RootID} {
			page, err := s.List(ctx, f.actor, tasks.Filter{Q: term, Limit: 10})
			must(t, err)
			if len(page.Items) != 1 || page.Items[0].ID != item.ID || page.NextCursor != "" {
				t.Fatal("ID search scanned unrelated sources or lost its match", term)
			}
		}
	}
	for _, term := range []string{"%", "_", "absent-title"} {
		page, err := s.List(ctx, f.actor, tasks.Filter{Q: term, Limit: 10})
		must(t, err)
		if len(page.Items) != 0 || page.NextCursor != "" {
			t.Fatal("search treated literal text as a wildcard")
		}
	}
	s.WithOptions(tasks.DefaultOptions())
	page, err := s.List(ctx, f.actor, tasks.Filter{Q: "导出", Limit: 100})
	must(t, err)
	for _, item := range page.Items {
		if item.Type != "monitoring.export" {
			t.Fatal("title search admitted another type")
		}
	}
	if len(page.Items) != 6 {
		t.Fatal("title search lost authorized exports", len(page.Items))
	}
	options.CandidateBudget = 2
	options.CandidateBatch = 2
	s.WithOptions(options)
	if _, e := s.List(ctx, f.actor, tasks.Filter{Sort: "oldest", Limit: 10}); e == nil {
		t.Fatal("ascending budget exhaustion claimed complete results")
	}
}
