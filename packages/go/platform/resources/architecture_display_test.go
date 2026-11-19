package resources_test

import (
	"context"
	"errors"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The coordinator and query owner share only durable authority. In particular,
// pausing after the query returns is late enough to exercise the shell's final
// check rather than merely testing authentication at ingress.
func TestArchitectureBoundDisplayRead(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "BoundRead")
	facility := f.facility(t, "BOUND")
	link := f.accessInput(t, p, facility.ID, "create_link")
	link.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, link)
	scope := displayread.Definition{ID: "project", Domain: "resources", Selector: displayread.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	view := scope
	view.ID = ""
	view.ScopeID = scope.ID
	shell := runtime.Compose(f.env.Service.Access())
	h := createReadDisplay(t, f, p, shell, []displayread.Definition{scope}, map[string]displayread.Definition{"resources": view}, []string{"views.read", "queries.read", "resources.directory", "statistics.resources"})
	owner := runtime.Compose(f.env.Service.Access()).Displays.WithPersonCheck(f.env.Service.CheckDisplaySession)
	h.s.WithReadExecutor(func(ctx context.Context, request display.BoundRead) (display.Envelope, error) {
		if err := f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			full, err := h.s.ReadAuthority(h.session).Check(ctx, tx)
			if err == nil && displayread.Hash([]string{full.Authorization, full.Safety}) != request.Authority {
				t.Fatal("peer proof differs from the authoritative saved-query proof")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		other := request
		other.Binding.Project = uuid.NewString()
		if _, e := owner.ExecuteBoundRead(ctx, other); e == nil {
			t.Fatal("read accepted a different project")
		}
		other = request
		other.Binding.Generation = "old"
		if _, e := owner.ExecuteBoundRead(ctx, other); e == nil {
			t.Fatal("read accepted a retired generation")
		}
		return owner.ExecuteBoundRead(ctx, request)
	})
	query := displayread.ResourcesRequest{Target: displayread.Target{View: "resources"}}
	out := readCall(t, h, "resources.query", query)
	page := readData[displayread.Page[displayread.ResourceSummary]](t, out)
	if len(page.Items) != 1 || page.Items[0].ID != facility.ID || out.Query == nil {
		t.Fatal("bound named query lost authorized rows or snapshot")
	}
	h.s.WithReadExecutor(func(ctx context.Context, req display.BoundRead) (display.Envelope, error) {
		v, e := owner.ExecuteBoundRead(ctx, req)
		if e != nil {
			return v, e
		}
		h.command(t, "grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
		return v, nil
	})
	_, e := h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: displayread.Bytes(displayread.QueryRequest{QueryID: out.Query.ID}), Generation: "read-1"})
	var de *display.Error
	if !errors.As(e, &de) || de.Code != "revoked" {
		t.Fatalf("revoked result reached delivery: %v", e)
	}
	if _, _, e = h.s.Asset(ctx, h.asset, "index.html"); e == nil {
		t.Fatal("old permit still eligible after revocation")
	}
}

func TestArchitectureBoundDisplayReadDelivery(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DeliveredRead")
	facility := f.facility(t, "DELIVERED")
	link := f.accessInput(t, p, facility.ID, "create_link")
	link.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, link)
	h := createDisplayHarness(t, f, p, runtime.Compose(f.env.Service.Access()).Displays,
		map[string]display.Binding{"site": {Kind: "resources", IDs: []string{facility.ID}}},
		display.Grant{Capabilities: []string{"resources.read"}, Scope: display.ReadScope{Resources: []string{facility.ID}}})
	owner := runtime.Compose(f.env.Service.Access()).Displays.WithPersonCheck(f.env.Service.CheckDisplaySession)
	h.s.WithReadExecutor(func(ctx context.Context, req display.BoundRead) (display.Envelope, error) {
		out, err := owner.ExecuteBoundRead(ctx, req)
		if err != nil {
			return out, err
		}
		var recorded bool
		must(t, f.env.Pool.QueryRow(ctx, `SELECT last_data_at IS NOT NULL OR last_query_state <> '' FROM display_sessions WHERE id=$1`, h.session.ID).Scan(&recorded))
		if recorded {
			t.Fatal("private query owner recorded delivery before the runtime checked the response")
		}
		return out, nil
	})
	result := h.query(t, "resources.list", display.Query{Target: display.Target{Binding: "site"}})
	var delivered bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT last_data_at IS NOT NULL AND last_query_state=$2 FROM display_sessions WHERE id=$1`, h.session.ID, result.State).Scan(&delivered))
	if !delivered || result.State != "ready" {
		t.Fatalf("runtime did not record the delivered resource list, state=%s", result.State)
	}
}
