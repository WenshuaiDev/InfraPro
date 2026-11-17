package resources_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
)

func createReadDisplay(t *testing.T, f resourceFixture, p projectFixture, b runtime.Business, scopes []displayread.Definition, views map[string]displayread.Definition, caps []string) *displayHarness {
	t.Helper()
	s := b.Displays
	h := &displayHarness{s: s, scope: display.Scope{Tenant: f.tenant, Project: p.id}, actor: p.actor, store: &displayMemoryStore{files: map[string][]byte{}}}
	s.WithStorage(h.store).WithPersonCheck(f.env.Service.CheckDisplaySession)
	var enabled bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT enabled FROM display_settings WHERE tenant_id=$1 AND project_id=$2),false)`, f.tenant, p.id).Scan(&enabled))
	if !enabled {
		_, e := s.Command(ctx, p.actor, h.scope, "", "settings", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "read fixture"}, Enabled: true})
		must(t, e)
	}
	created, e := s.Command(ctx, p.actor, h.scope, "", "create", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "read fixture"}, Name: "命名读取验收"})
	must(t, e)
	h.id, h.revision = created["id"].(string), created["revision"].(string)
	old := displayZIPCustom(t, caps, map[string]display.Binding{})
	validated, e := display.ValidateZIP(ctx, bytes.NewReader(old), int64(len(old)))
	must(t, e)
	m := validated.Manifest
	m.Version = 2
	m.ReadViews = map[string]displayread.ViewSchema{}
	for name, v := range views {
		m.ReadViews[name] = displayread.ViewSchema{StructureModel: v.StructureModel, Domain: v.Domain, Required: true, AllowDynamic: true, Purpose: "合成读取验收"}
	}
	validated.Files["manifest.json"] = displayread.Bytes(m)
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for name, content := range validated.Files {
		w, e := z.Create(name)
		must(t, e)
		_, e = w.Write(content)
		must(t, e)
	}
	must(t, z.Close())
	data := archive.Bytes()
	upload := h.command(t, "upload", "", display.Command{Size: int64(len(data)), SHA256: displayHash(data)})["upload_id"].(string)
	must(t, s.UploadPart(ctx, p.actor, h.scope, h.id, upload, 0, displayHash(data), data))
	h.command(t, "upload.complete", upload, display.Command{})
	_, e = s.ProcessOne(ctx)
	must(t, e)
	detail, e := s.Detail(ctx, p.actor, h.scope, h.id)
	must(t, e)
	var pack display.Package
	must(t, json.Unmarshal(detail["packages"].([]json.RawMessage)[0], &pack))
	t.Log("read harness: grant")
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Format: 2, ReadScopes: scopes, Capabilities: caps, ValidUntil: time.Now().Add(time.Hour)}})
	t.Log("read harness: config")
	h.config = h.command(t, "config", "", display.Command{PackageID: pack.ID, ConfigFormat: 2, ReadViews: views, Mode: "follow", Bindings: map[string]display.Binding{}, Parameters: map[string]any{}})["config_id"].(string)
	t.Log("read harness: prepare")
	prep, e := s.Prepare(ctx, p.actor, h.scope, h.id, display.Command{Meta: display.Meta{Revision: h.revision}, ConfigID: h.config})
	must(t, e)
	h.command(t, "publish", "", display.Command{ConfigID: h.config, Preparation: prep.Hash})
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL LIMIT 1`, p.actor.ID).Scan(&h.login))
	verifier := "read-fixture-verifier-012345678901234567890123456789"
	launch, e := s.Launch(ctx, p.actor, h.scope, h.id, display.Command{SessionID: h.login, Challenge: displayHash([]byte(verifier)), Audience: s.Options().RuntimeOrigin}, false)
	must(t, e)
	h.token, h.asset, h.session, e = s.Exchange(ctx, launch["code"].(string), verifier, s.Options().RuntimeOrigin)
	must(t, e)
	return h
}
func readCall(t *testing.T, h *displayHarness, method string, params any) display.Envelope {
	t.Helper()
	result, e := h.s.Query(ctx, h.session, display.Call{Method: method, Params: displayread.Bytes(params), Generation: "read-1"})
	if e != nil {
		t.Fatalf("%s: %v (%s)", method, e, displayread.Bytes(e))
	}
	return result
}
func readData[T any](t *testing.T, result display.Envelope) T {
	t.Helper()
	var out T
	must(t, json.Unmarshal(displayread.Bytes(result.Data), &out))
	return out
}
func TestDisplayReadDirectorySnapshotAndIsolation(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ReadDirectory")
	one := f.facility(t, "READ1")
	two := f.facility(t, "READ2")
	hidden := f.facility(t, "SECRET_F4")
	for _, facility := range []res.Result{one, two} {
		in := f.accessInput(t, p, facility.ID, "create_link")
		in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
		f.grant(t, p, facility.ID, in)
	}
	fields := []string{"resource.metadata", "resource.attributes", "resource.position", "statistics.counts"}
	scope := displayread.Definition{ID: "project", Domain: "resources", Selector: displayread.Selector{Kind: "project"}, FieldGroups: fields}
	view := scope
	view.ID = ""
	view.ScopeID = "project"
	b := runtime.Compose(displayReadPlanAccess(t, f))
	h := createReadDisplay(t, f, p, b, []displayread.Definition{scope}, map[string]displayread.Definition{"project_resources": view}, []string{"views.read", "queries.read", "resources.directory", "resources.detail", "statistics.resources"})
	// Both management purposes use current actor eligibility and server paging.
	for _, purpose := range []string{"grant", "config"} {
		definition := scope
		if purpose == "config" {
			definition = view
		}
		preview, e := h.s.ReadScopePreview(ctx, p.actor, h.scope, h.id, display.ReadScopePreviewInput{Purpose: purpose, Definition: definition})
		must(t, e)
		if preview.MemberCount != "2" || len(preview.Sample) != 2 || bytes.Contains(displayread.Bytes(preview), []byte(hidden.ID)) {
			t.Fatalf("unsafe scope preview: %+v", preview)
		}
		options := display.ReadScopeOptionsInput{Purpose: purpose, Domain: "resources", Kind: "resource", Limit: 1}
		if purpose == "config" {
			options.ScopeID = "project"
		}
		first, e := h.s.ReadScopeOptions(ctx, p.actor, h.scope, h.id, options)
		must(t, e)
		if first.Total != "2" || len(first.Items) != 1 || !first.HasMore {
			t.Fatalf("options: %+v", first)
		}
		options.Cursor = first.NextCursor
		second, e := h.s.ReadScopeOptions(ctx, p.actor, h.scope, h.id, options)
		must(t, e)
		if second.Total != "2" || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID || second.HasMore {
			t.Fatalf("paged options: %+v", second)
		}
		options.Cursor = ""
		options.Search = "SECRET_F4"
		none, e := h.s.ReadScopeOptions(ctx, p.actor, h.scope, h.id, options)
		must(t, e)
		if none.Total != "0" || len(none.Items) != 0 {
			t.Fatal("hidden scope search leaked")
		}
	}
	invalid := view
	invalid.Selector = displayread.Selector{Kind: "ids", IDs: []string{hidden.ID}}
	if _, e := h.s.ReadScopePreview(ctx, p.actor, h.scope, h.id, display.ReadScopePreviewInput{Purpose: "config", Definition: invalid}); e == nil {
		t.Fatal("config preview expanded authority")
	}
	for _, purpose := range []string{"grant", "config", "view"} {
		if _, e := h.s.ReadScopeOptions(ctx, f.actor, h.scope, h.id, display.ReadScopeOptionsInput{Purpose: purpose, Domain: "resources"}); e == nil {
			t.Fatal("tenant administrator bypassed purpose/business permissions")
		}
	}
	query := displayread.ResourcesRequest{Target: displayread.Target{View: "project_resources"}, Paging: displayread.Paging{Limit: 1}}
	first := readCall(t, h, "resources.query", query)
	page := readData[displayread.Page[displayread.ResourceSummary]](t, first)
	if first.Query == nil || page.Total != "2" || len(page.Items) != 1 || page.Items[0].ID != one.ID || page.NextCursor == "" {
		t.Fatalf("first page: %+v %+v", first.Query, page)
	}
	if bytes.Contains(displayread.Bytes(first), []byte(hidden.ID)) || bytes.Contains(displayread.Bytes(first), []byte("SECRET_F4")) {
		t.Fatal("hidden facility leaked")
	}
	polled := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, h, "queries.get", displayread.QueryRequest{QueryID: page.QueryID}))
	if len(polled.Items) != 1 || polled.NextCursor != page.NextCursor {
		t.Fatal("polling changed the requested page size", polled)
	}
	// A query created with default sizing may be paged at a smaller size;
	// its continuation inherits the issued cursor, not the original request.
	wide := readCall(t, h, "resources.query", displayread.ResourcesRequest{Target: query.Target})
	small := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, h, "queries.get", displayread.QueryRequest{QueryID: wide.Query.ID, Limit: 1}))
	continued := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, h, "queries.get", displayread.QueryRequest{QueryID: wide.Query.ID, Cursor: small.NextCursor}))
	if len(continued.Items) != 1 || continued.Items[0].ID == small.Items[0].ID || continued.HasMore {
		t.Fatal("cursor did not retain explicit page size", continued)
	}
	stats := displayread.ResourceStatisticsRequest{Target: query.Target, GroupBy: "facility", ReadFields: displayread.ReadFields{FieldGroups: []string{"resource.metadata", "statistics.counts"}}, Paging: displayread.Paging{Limit: 1}}
	sub, e := h.s.Subscribe(ctx, h.session, display.SubscriptionInput{Kind: "statistics.resources", Query: displayread.Bytes(stats), Generation: "resource-sub"})
	must(t, e)
	events, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Result == nil {
		t.Fatal("resource statistics subscription missing", events)
	}
	oldQuery := events[0].Result.Query.ID
	must(t, h.s.Ack(ctx, h.session, sub, events[0].Generation, events[0].Sequence))
	events, e = h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 0 {
		t.Fatal("unchanged resource statistics rescanned")
	}
	// Independent panels share one personnel session. Snapshot transactions may
	// run concurrently without turning a session-touch conflict into logout.
	var concurrent sync.WaitGroup
	errorsOut := make(chan error, 4)
	for range 4 {
		concurrent.Add(1)
		go func() {
			defer concurrent.Done()
			_, err := h.s.Query(ctx, h.session, display.Call{Method: "resources.query", Params: displayread.Bytes(displayread.ResourcesRequest{Target: query.Target})})
			errorsOut <- err
		}()
	}
	concurrent.Wait()
	close(errorsOut)
	for err := range errorsOut {
		must(t, err)
	}
	// An ordinary rename must not rewrite an existing query projection.
	_, e = f.write(t, target(two, res.Facility, two.ID), "update", map[string]any{"expected_revision": 1, "name": "新设施名称", "values": map[string]any{"length": 0}})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_subscriptions SET updated_at=clock_timestamp()-interval '31 seconds' WHERE id=$1`, sub)
	must(t, e)
	events, e = h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Result == nil || events[0].Result.Query.ID == oldQuery {
		t.Fatal("resource update did not replace the whole result", events)
	}
	updated := readData[displayread.ResourceStatistics](t, *events[0].Result)
	if updated.Totals.FacilityCount != "2" || len(updated.Groups) != 1 || !updated.HasMore {
		t.Fatal("subscription mixed totals/groups", updated)
	}
	lastGroup := readData[displayread.ResourceStatistics](t, readCall(t, h, "queries.get", displayread.QueryRequest{QueryID: updated.QueryID, Cursor: updated.NextCursor}))
	if lastGroup.QueryID != updated.QueryID || lastGroup.Totals.FacilityCount != "2" || len(lastGroup.Groups) != 1 {
		t.Fatal("subscription pagination lost snapshot", lastGroup)
	}
	must(t, h.s.CancelSubscription(ctx, h.session, sub))
	query.QueryID, query.Cursor = page.QueryID, page.NextCursor
	next := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, h, "resources.query", query))
	if len(next.Items) != 1 || next.Items[0].ID != two.ID || next.Items[0].Name == "新设施名称" || next.HasMore {
		t.Fatalf("unstable next page: %+v", next)
	}
	fresh := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, h, "resources.query", displayread.ResourcesRequest{Target: query.Target, Search: "新设施名称"}))
	if len(fresh.Items) != 1 {
		t.Fatalf("new query missed rename: %+v", fresh)
	}
	query.Search = "changed"
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "resources.query", Params: displayread.Bytes(query)})
	var problem *display.Error
	if !errors.As(e, &problem) || problem.Code != "invalid_cursor" {
		t.Fatalf("changed filter cursor: %v", e)
	}
	detail := readData[displayread.ResourceDetail](t, readCall(t, h, "resources.detail", displayread.ResourceDetailRequest{Target: query.Target, ID: one.ID, ReadFields: displayread.ReadFields{FieldGroups: []string{"resource.metadata", "resource.attributes"}}}))
	if detail.Values["length"] != "0" || len(detail.Definitions) != 1 {
		t.Fatalf("fixed schema/exact values: %+v", detail)
	}
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "resources.detail", Params: displayread.Bytes(displayread.ResourceDetailRequest{Target: query.Target, ID: hidden.ID})})
	if e == nil {
		t.Fatal("explicit inaccessible ID succeeded")
	}
	// A new resource after freezing the view does not become a fixed member.
	futureScope, futureView := scope, view
	futureScope.IncludeFuture, futureView.IncludeFuture = true, true
	future := createReadDisplay(t, f, p, b, []displayread.Definition{futureScope}, map[string]displayread.Definition{"project_resources": futureView}, []string{"views.read", "queries.read", "resources.directory"})
	three := f.facility(t, "READ3")
	in := f.accessInput(t, p, three.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, three.ID, in)
	fixed := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, h, "resources.query", displayread.ResourcesRequest{Target: query.Target}))
	if fixed.Total != "2" {
		t.Fatalf("fixed view expanded: %+v", fixed)
	}
	expanded := readData[displayread.Page[displayread.ResourceSummary]](t, readCall(t, future, "resources.query", displayread.ResourcesRequest{Target: query.Target}))
	if expanded.Total != "3" {
		t.Fatalf("joint future policy missed newly authorized resource: %+v", expanded)
	}
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: displayread.Bytes(displayread.QueryRequest{QueryID: page.QueryID})})
	if e == nil {
		t.Fatal("revoked stored result delivered")
	}
}
