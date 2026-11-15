package resources_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type displayMemoryStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

type displayGatedStore struct {
	*displayMemoryStore
	once            sync.Once
	entered, resume chan struct{}
}

func (b *displayGatedStore) Get(c context.Context, key string) (io.ReadCloser, error) {
	b.once.Do(func() {
		close(b.entered)
		select {
		case <-b.resume:
		case <-c.Done():
		}
	})
	return b.displayMemoryStore.Get(c, key)
}

func (b *displayMemoryStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, e := io.ReadAll(r)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files[key] = data
	return e
}
func (b *displayMemoryStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.files[key]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (b *displayMemoryStore) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.files, key)
	return nil
}
func displayHash(b []byte) string    { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func displayZIP(t *testing.T) []byte { return displayZIPCustom(t, nil, nil) }
func displayZIPCustom(t *testing.T, caps []string, bindings map[string]display.Binding) []byte {
	body := []byte(`<!doctype html><title>SDK fixture</title>`)
	manifest := map[string]any{"manifest_version": 1, "app_key": "integration", "version": "1.0.0", "entry": "index.html", "routing": map[string]any{"mode": "hash"}, "sdk_protocol": map[string]any{"major": 1}, "required_capabilities": map[string]int{"resources.read": 1}, "config_schema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"title": map[string]any{"type": "string", "default": "合成测试"}}}, "binding_schema": map[string]any{"site": map[string]any{"kind": "resources", "required": true, "min_items": 1}}, "files": []any{map[string]any{"path": "index.html", "size": len(body), "sha256": displayHash(body), "media_type": "text/html"}}}
	if caps != nil {
		required := map[string]int{}
		for _, cap := range caps {
			required[cap] = 1
		}
		manifest["required_capabilities"] = required
		schemas := map[string]any{}
		for name, b := range bindings {
			schemas[name] = map[string]any{"kind": b.Kind, "required": true, "min_items": 1}
		}
		manifest["binding_schema"] = schemas
	}
	raw, _ := json.Marshal(manifest)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, b := range map[string][]byte{"manifest.json": raw, "index.html": body} {
		f, e := w.Create(name)
		must(t, e)
		_, e = f.Write(b)
		must(t, e)
	}
	must(t, w.Close())
	return buf.Bytes()
}
func TestDisplayLifecycleAndSubjectBoundary(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DisplayProject")
	facility := f.facility(t, "DISPLAY")
	grant := f.accessInput(t, p, facility.ID, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, grant)
	business := runtime.Compose(f.env.Service.Access())
	s := business.Displays.WithStorage(&displayMemoryStore{files: map[string][]byte{}}).WithPersonCheck(f.env.Service.CheckDisplaySession)
	scope := display.Scope{Tenant: f.tenant, Project: p.id}
	meta := func(rev string) display.Meta {
		return display.Meta{Revision: rev, RequestID: uuid.NewString(), Reason: "大屏完整机制合成自验"}
	}
	list, e := s.List(ctx, p.actor, scope)
	must(t, e)
	if list["settings"].(display.Settings).Enabled {
		t.Fatal("display enabled by default")
	}
	_, e = s.Command(ctx, p.actor, scope, "", "settings", "", display.Command{Meta: meta("0"), Enabled: true})
	must(t, e)
	created, e := s.Command(ctx, p.actor, scope, "", "create", "", display.Command{Meta: meta("0"), Name: "技术接入"})
	must(t, e)
	id := created["id"].(string)
	rev := created["revision"].(string)
	command := func(action, target string, in display.Command) map[string]any {
		t.Helper()
		in.Meta = meta(rev)
		out, e := s.Command(ctx, p.actor, scope, id, action, target, in)
		must(t, e)
		if value, ok := out["revision"].(string); ok {
			rev = value
		}
		return out
	}
	data := displayZIP(t)
	up := command("upload", "", display.Command{Size: int64(len(data)), SHA256: displayHash(data)})
	upload := up["upload_id"].(string)
	must(t, s.UploadPart(ctx, p.actor, scope, id, upload, 0, displayHash(data), data))
	must(t, s.UploadPart(ctx, p.actor, scope, id, upload, 0, displayHash(data), data))
	command("upload.complete", upload, display.Command{})
	worked, e := s.ProcessOne(ctx)
	must(t, e)
	if !worked {
		t.Fatal("worker did not process")
	}
	detail, e := s.Detail(ctx, p.actor, scope, id)
	must(t, e)
	packages := detail["packages"].([]json.RawMessage)
	var pack display.Package
	must(t, json.Unmarshal(packages[0], &pack))
	cfg := command("config", "", display.Command{PackageID: pack.ID, Mode: "follow", Parameters: map[string]any{}, Bindings: map[string]display.Binding{"site": {Kind: "resources", IDs: []string{facility.ID}}}})
	config := cfg["config_id"].(string)
	command("grant", "", display.Command{Grant: &display.Grant{Capabilities: []string{"resources.read"}, Scope: display.ReadScope{Resources: []string{facility.ID}}, ValidUntil: time.Now().Add(time.Hour)}})
	prep, e := s.Prepare(ctx, p.actor, scope, id, display.Command{Meta: meta(rev), ConfigID: config})
	must(t, e)
	published := command("publish", "", display.Command{ConfigID: config, Preparation: prep.Hash})
	if published["release_id"] == "" {
		t.Fatal("no release")
	}
	var login string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL LIMIT 1`, p.actor.ID).Scan(&login))
	verifier := "0123456789012345678901234567890123456789"
	launch, e := s.Launch(ctx, p.actor, scope, id, display.Command{SessionID: login, Challenge: displayHash([]byte(verifier)), Audience: s.Options().RuntimeOrigin}, false)
	must(t, e)
	_, _, _, e = s.Exchange(ctx, launch["code"].(string), "wrongwrongwrongwrongwrongwrongwrong", s.Options().RuntimeOrigin)
	if e == nil {
		t.Fatal("pairing verifier ignored")
	}
	token, asset, session, e := s.Exchange(ctx, launch["code"].(string), verifier, s.Options().RuntimeOrigin)
	must(t, e)
	_, _, _, e = s.Exchange(ctx, launch["code"].(string), verifier, s.Options().RuntimeOrigin)
	if e == nil {
		t.Fatal("code replay")
	}
	_, e = s.Session(ctx, token)
	must(t, e)
	navigation, e := s.Navigation(ctx, session, "resource", facility.ID)
	must(t, e)
	if navigation != s.Options().ConsoleOrigin+"/t/"+f.tenant+"/p/"+p.id+"/resources/facilities/"+facility.ID {
		t.Fatal("resource navigation missed the facility route", navigation)
	}
	_, e = s.Navigation(ctx, session, "resource", uuid.NewString())
	if e == nil {
		t.Fatal("navigation allowed an object outside the configured scope")
	}
	q, _ := json.Marshal(map[string]any{"target": map[string]string{"binding": "site"}})
	result, e := s.Query(ctx, session, display.Call{Method: "resources.list", Params: q, Generation: "1"})
	must(t, e)
	raw, _ := json.Marshal(result)
	if !bytes.Contains(raw, []byte(facility.ID)) {
		t.Fatal("missing real resource", string(raw))
	}
	_, body, e := s.Asset(ctx, asset, "index.html")
	must(t, e)
	if !bytes.Contains(body, []byte("SDK fixture")) {
		t.Fatal("wrong asset")
	}
	subject, e := s.ReadSubject(ctx, session.SubjectID)
	must(t, e)
	_, e = business.Resources.Mutate(ctx, subject.ReadActor(), res.ProjectScope(f.tenant, p.id), res.Target{Kind: res.Facility, ID: facility.ID}, "update", []byte(`{}`), access.Meta{})
	if e == nil {
		t.Fatal("read-only subject entered write")
	}
	command("grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
	_, e = s.Session(ctx, token)
	if e == nil {
		t.Fatal("revoked session survived")
	}
	_, _, e = s.Asset(ctx, asset, "index.html")
	if e == nil {
		t.Fatal("revoked asset permit survived")
	}
}

type displayHarness struct {
	store                              *displayMemoryStore
	s                                  *display.Service
	session                            display.Session
	token, asset, id, config, revision string
	scope                              display.Scope
	actor                              access.Actor
	login                              string
}

func createDisplayHarness(t *testing.T, f resourceFixture, p projectFixture, s *display.Service, bindings map[string]display.Binding, grant display.Grant) *displayHarness {
	t.Helper()
	h := &displayHarness{s: s, scope: display.Scope{Tenant: f.tenant, Project: p.id}, actor: p.actor}
	h.store = &displayMemoryStore{files: map[string][]byte{}}
	s.WithStorage(h.store).WithPersonCheck(f.env.Service.CheckDisplaySession)
	_, e := s.Command(ctx, p.actor, h.scope, "", "settings", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "synthetic"}, Enabled: true})
	must(t, e)
	c, e := s.Command(ctx, p.actor, h.scope, "", "create", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "synthetic"}, Name: "SDK contract"})
	must(t, e)
	h.id = c["id"].(string)
	h.revision = c["revision"].(string)
	data := displayZIPCustom(t, grant.Capabilities, bindings)
	up := h.command(t, "upload", "", display.Command{Size: int64(len(data)), SHA256: displayHash(data)})
	upload := up["upload_id"].(string)
	must(t, s.UploadPart(ctx, p.actor, h.scope, h.id, upload, 0, displayHash(data), data))
	h.command(t, "upload.complete", upload, display.Command{})
	worked, e := s.ProcessOne(ctx)
	must(t, e)
	if !worked {
		t.Fatal("package worker idle")
	}
	detail, e := s.Detail(ctx, p.actor, h.scope, h.id)
	must(t, e)
	var pack display.Package
	must(t, json.Unmarshal(detail["packages"].([]json.RawMessage)[0], &pack))
	cfg := h.command(t, "config", "", display.Command{PackageID: pack.ID, Parameters: map[string]any{}, Bindings: bindings, Mode: "follow"})
	h.config = cfg["config_id"].(string)
	grant.ValidUntil = time.Now().Add(time.Hour)
	h.command(t, "grant", "", display.Command{Grant: &grant})
	prep, e := s.Prepare(ctx, p.actor, h.scope, h.id, display.Command{Meta: display.Meta{Revision: h.revision}, ConfigID: h.config})
	must(t, e)
	h.command(t, "publish", "", display.Command{ConfigID: h.config, Preparation: prep.Hash})
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL LIMIT 1`, p.actor.ID).Scan(&h.login))
	verifier := "0123456789012345678901234567890123456789"
	launch, e := s.Launch(ctx, p.actor, h.scope, h.id, display.Command{SessionID: h.login, Challenge: displayHash([]byte(verifier)), Audience: s.Options().RuntimeOrigin}, false)
	must(t, e)
	h.token, h.asset, h.session, e = s.Exchange(ctx, launch["code"].(string), verifier, s.Options().RuntimeOrigin)
	must(t, e)
	return h
}
func (h *displayHarness) command(t *testing.T, action, target string, in display.Command) map[string]any {
	t.Helper()
	in.Meta = display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "synthetic display verification"}
	r, e := h.s.Command(ctx, h.actor, h.scope, h.id, action, target, in)
	if e != nil {
		t.Fatalf("display %s: %+v", action, e)
		return nil
	}
	h.revision = r["revision"].(string)
	return r
}
func (h *displayHarness) query(t *testing.T, method string, q display.Query) display.Envelope {
	t.Helper()
	raw, _ := json.Marshal(q)
	r, e := h.s.Query(ctx, h.session, display.Call{Method: method, Params: raw, Generation: "1"})
	must(t, e)
	return r
}
func TestDisplayTerminalPublicationAndRestore(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DisplayRuntime")
	facility := f.facility(t, "DISPLAY-RUNTIME")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	h := createDisplayHarness(t, f, p, runtime.Compose(f.env.Service.Access()).Displays, map[string]display.Binding{"site": {Kind: "resources", IDs: []string{facility.ID}}}, display.Grant{Capabilities: []string{"resources.read"}, Scope: display.ReadScope{Resources: []string{facility.ID}}})
	term := h.command(t, "terminal.create", "", display.Command{Name: "Wall", ValidUntil: time.Now().Add(time.Hour)})
	credential, e := h.s.PairTerminal(ctx, term["pair_code"].(string))
	must(t, e)
	_, e = h.s.PairTerminal(ctx, term["pair_code"].(string))
	if e == nil {
		t.Fatal("pair replay accepted")
	}
	tok, _, rotated, session, e := h.s.StartTerminal(ctx, credential)
	must(t, e)
	_, _, _, _, e = h.s.StartTerminal(ctx, credential)
	if e == nil {
		t.Fatal("old terminal credential accepted")
	}
	if rotated == credential {
		t.Fatal("credential did not rotate")
	}
	must(t, h.s.RuntimeControl(ctx, session, "ready", ""))
	prep, e := h.s.Prepare(ctx, p.actor, h.scope, h.id, display.Command{Meta: display.Meta{Revision: h.revision}, ConfigID: h.config})
	must(t, e)
	pub := h.command(t, "publish", "", display.Command{ConfigID: h.config, Preparation: prep.Hash})
	current, e := h.s.Session(ctx, tok)
	must(t, e)
	if current.ReleaseID == current.TargetRelease {
		t.Fatal("release changed before app ready")
	}
	newToken, _, next, e := h.s.Switch(ctx, current)
	must(t, e)
	if next.ReleaseID != pub["release_id"] {
		t.Fatal("switch target wrong")
	}
	must(t, h.s.RuntimeControl(ctx, next, "error", "loading_failed"))
	var actual string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT actual_release_id::text FROM display_terminals WHERE id=$1`, term["terminal_id"]).Scan(&actual))
	if actual == next.ReleaseID {
		t.Fatal("failed new app replaced actual release")
	}
	must(t, h.s.RuntimeControl(ctx, next, "ready", ""))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT actual_release_id::text FROM display_terminals WHERE id=$1`, term["terminal_id"]).Scan(&actual))
	if actual != next.ReleaseID {
		t.Fatal("ready not recorded")
	}
	_, e = h.s.InvalidateAfterRestore(ctx)
	must(t, e)
	if _, e = h.s.Session(ctx, newToken); e == nil {
		t.Fatal("restored session survived")
	}
	if _, _, _, _, e = h.s.StartTerminal(ctx, rotated); e == nil {
		t.Fatal("restored terminal survived")
	}
	inventory, e := h.s.BackupManifest(ctx)
	must(t, e)
	if len(inventory.Files) != 1 {
		t.Fatal("backup omitted package")
	}
	missing, e := h.s.VerifyStorage(ctx)
	must(t, e)
	if len(missing) != 0 {
		t.Fatal("intact package missing")
	}
}
func TestDisplayUploadToolOneTimeAndRestricted(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DisplayTool")
	s := runtime.Compose(f.env.Service.Access()).Displays.WithPersonCheck(f.env.Service.CheckDisplaySession)
	scope := display.Scope{Tenant: f.tenant, Project: p.id}
	meta := func() display.Meta {
		return display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "synthetic tool test"}
	}
	_, e := s.Command(ctx, p.actor, scope, "", "settings", "", display.Command{Meta: meta(), Enabled: true})
	must(t, e)
	instance, e := s.Command(ctx, p.actor, scope, "", "create", "", display.Command{Meta: meta(), Name: "Empty instance"})
	must(t, e)
	var login string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL LIMIT 1`, p.actor.ID).Scan(&login))
	verifier := "0123456789012345678901234567890123456789"
	code, e := s.AuthorizeTool(ctx, p.actor, scope, instance["id"].(string), display.Command{SessionID: login, Challenge: displayHash([]byte(verifier)), Audience: "http://127.0.0.1:5190"})
	must(t, e)
	pair, e := s.ExchangeTool(ctx, code["code"].(string), verifier, "http://127.0.0.1:5190")
	must(t, e)
	_, e = s.ExchangeTool(ctx, code["code"].(string), verifier, "http://127.0.0.1:5190")
	if e == nil {
		t.Fatal("tool code replay")
	}
	_, e = s.ToolCommand(ctx, pair["id"], pair["token"], "inspect", "", display.Command{})
	must(t, e)
	_, e = s.ToolCommand(ctx, pair["id"], pair["token"], "publish", "", display.Command{})
	if e == nil {
		t.Fatal("tool escalated into publication")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE account_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, login)
	must(t, e)
	_, e = s.ToolCommand(ctx, pair["id"], pair["token"], "inspect", "", display.Command{})
	if e == nil {
		t.Fatal("tool ignored personnel revocation")
	}
}

func TestDisplayConcurrentPublishArchiveAndStorageRecovery(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DisplayRecovery")
	facility := f.facility(t, "DISPLAY-RECOVERY")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	h := createDisplayHarness(t, f, p, runtime.Compose(f.env.Service.Access()).Displays, map[string]display.Binding{"site": {Kind: "resources", IDs: []string{facility.ID}}}, display.Grant{Capabilities: []string{"resources.read"}, Scope: display.ReadScope{Resources: []string{facility.ID}}})
	prep, e := h.s.Prepare(ctx, p.actor, h.scope, h.id, display.Command{Meta: display.Meta{Revision: h.revision}, ConfigID: h.config})
	must(t, e)
	inputs := []display.Command{{Meta: display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "CAS a"}, ConfigID: h.config, Preparation: prep.Hash}, {Meta: display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "CAS b"}, ConfigID: h.config, Preparation: prep.Hash}}
	type outcome struct {
		index int
		value map[string]any
		err   error
	}
	results := make(chan outcome, 2)
	for i, in := range inputs {
		go func() {
			v, e := h.s.Command(ctx, p.actor, h.scope, h.id, "publish", "", in)
			results <- outcome{i, v, e}
		}()
	}
	var winner outcome
	success := 0
	for range 2 {
		r := <-results
		if r.err == nil {
			success++
			winner = r
		}
	}
	if success != 1 {
		t.Fatalf("CAS winners %d", success)
	}
	replay, e := h.s.Command(ctx, p.actor, h.scope, h.id, "publish", "", inputs[winner.index])
	must(t, e)
	if replay["release_id"] != winner.value["release_id"] {
		t.Fatal("retry duplicated publication")
	}
	h.revision = winner.value["revision"].(string)
	if _, e = h.s.Command(ctx, p.actor, h.scope, h.id, "package.delete", h.session.PackageID, display.Command{Meta: display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "reference guard"}}); e == nil {
		t.Fatal("referenced package reclaimed")
	}
	inventory, e := h.s.BackupManifest(ctx)
	must(t, e)
	file := inventory.Files[0]
	original := append([]byte{}, h.store.files[file.ObjectKey]...)
	must(t, h.store.Delete(ctx, file.ObjectKey))
	if _, _, e = h.s.Asset(ctx, h.asset, file.Path); e == nil {
		t.Fatal("missing package served")
	}
	var state string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM display_packages WHERE id=$1`, file.PackageID).Scan(&state))
	if state != "missing" {
		t.Fatal(state)
	}
	missing, e := h.s.VerifyStorage(ctx)
	must(t, e)
	if len(missing) != 1 {
		t.Fatal(missing)
	}
	must(t, h.store.Put(ctx, file.ObjectKey, bytes.NewReader(original), int64(len(original)), "text/html"))
	missing, e = h.s.VerifyStorage(ctx)
	must(t, e)
	if len(missing) != 0 {
		t.Fatal(missing)
	}
	_, _, e = h.s.Asset(ctx, h.asset, file.Path)
	must(t, e)
	inbox := runtime.ComposeNotifications(f.env.Service.Access())
	for i := 0; i < 3; i++ {
		_, e = inbox.Tick(ctx)
		must(t, e)
	}
	var conditions, recoveries int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE outcome='missing'),count(*) FILTER(WHERE outcome='recovered') FROM notification_events WHERE event_type='display.condition' AND target_id=$1`, h.id).Scan(&conditions, &recoveries))
	if conditions != 1 || recoveries != 1 {
		t.Fatal("display condition duplicate or absent", conditions, recoveries)
	}
	orphan := "display/orphan/test"
	must(t, h.store.Put(ctx, orphan, bytes.NewReader([]byte("orphan")), 6, "text/plain"))
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO display_object_intents(object_key,expires_at) VALUES($1,clock_timestamp()-interval '1 second')`, orphan)
	must(t, e)
	must(t, h.s.Maintain(ctx))
	if _, e = h.store.Get(ctx, orphan); e == nil {
		t.Fatal("orphan survived")
	}
	if _, e = h.store.Get(ctx, file.ObjectKey); e != nil {
		t.Fatal("referenced file reclaimed")
	}
	ps := projects.New(f.env.Service.Access())
	change := projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "archive display"}}}
	preview, e := ps.Preview(ctx, p.actor, f.tenant, p.id, "", access.OpProjectArchive, change)
	must(t, e)
	change.ProjectVersion, change.Version, change.Hash = preview.ProjectVersion, preview.Version, preview.Hash
	_, e = ps.Change(ctx, p.actor, f.tenant, p.id, "", access.OpProjectArchive, change, access.Meta{})
	must(t, e)
	// Existing content remains readable; archive cannot create or broaden displays.
	h.query(t, "resources.list", display.Query{Target: display.Target{Binding: "site"}})
	if _, e = h.s.Command(ctx, p.actor, h.scope, "", "create", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "archived create"}, Name: "must fail"}); e == nil {
		t.Fatal("archive allowed new instance")
	}
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
	if _, e = h.s.Session(ctx, h.token); e == nil {
		t.Fatal("archive revocation failed")
	}
}

func TestDisplayPackageWorkerLeastPrivilege(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "DisplayWorker")
	s := runtime.Compose(f.env.Service.Access()).Displays
	scope := display.Scope{Tenant: f.tenant, Project: p.id}
	meta := func(rev string) display.Meta {
		return display.Meta{Revision: rev, RequestID: uuid.NewString(), Reason: "worker role"}
	}
	_, e := s.Command(ctx, p.actor, scope, "", "settings", "", display.Command{Meta: meta("0"), Enabled: true})
	must(t, e)
	v, e := s.Command(ctx, p.actor, scope, "", "create", "", display.Command{Meta: meta("0"), Name: "Worker scope"})
	must(t, e)
	id, rev := v["id"].(string), v["revision"].(string)
	store := &displayMemoryStore{files: map[string][]byte{}}
	s.WithStorage(store)
	data := displayZIP(t)
	u, e := s.Command(ctx, p.actor, scope, id, "upload", "", display.Command{Meta: meta(rev), Size: int64(len(data)), SHA256: displayHash(data)})
	must(t, e)
	upload := u["upload_id"].(string)
	rev = u["revision"].(string)
	must(t, s.UploadPart(ctx, p.actor, scope, id, upload, 0, displayHash(data), data))
	_, e = s.Command(ctx, p.actor, scope, id, "upload.complete", upload, display.Command{Meta: meta(rev)})
	must(t, e)
	role := "display_worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	sql, e := os.ReadFile("../../../../deploy/display/package-worker-role.sql")
	must(t, e)
	grants := strings.ReplaceAll(strings.ReplaceAll(string(sql), "infrapro_display_packager", role), "USAGE ON SCHEMA public", "USAGE ON SCHEMA "+schema)
	_, e = f.env.Pool.Exec(ctx, grants)
	must(t, e)
	defer func() {
		_, _ = f.env.Pool.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = f.env.Pool.Exec(ctx, "DROP ROLE "+role)
	}()
	pc, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
	must(t, e)
	pc.ConnConfig.RuntimeParams["role"] = role
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	must(t, e)
	defer pool.Close()
	worker := display.New(access.New(pool, slog.Default())).WithStorage(store)
	if _, e = pool.Exec(ctx, `SELECT id FROM accounts LIMIT 1`); e == nil {
		t.Fatal("worker can read accounts")
	}
	for _, table := range []string{"task_records", "task_executions", "task_attempts", "notification_events", "notification_recipients", "notification_deliveries", "account_notifications", "notification_preferences"} {
		if _, e = pool.Exec(ctx, `SELECT * FROM `+table+` LIMIT 1`); e == nil {
			t.Fatalf("restricted packager can read %s", table)
		}
	}
	worked, e := worker.ProcessOne(ctx)
	must(t, e)
	if !worked {
		t.Fatal("worker idle")
	}
	must(t, worker.Maintain(ctx))
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM display_packages WHERE instance_id=$1 AND state='ready'`, id).Scan(&count))
	if count != 1 {
		t.Fatal(count)
	}
	// Both the package fact and its durable bridge receipt survive retention.
	if _, e = f.env.Pool.Exec(ctx, `UPDATE display_events SET created_at=clock_timestamp()-interval '365 days' WHERE instance_id=$1`, id); e != nil {
		t.Fatal(e)
	}
	must(t, worker.Maintain(ctx))
	var handoff string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM display_notification_handoffs WHERE instance_id=$1 AND kind='package'`, id).Scan(&handoff))
	inbox := runtime.ComposeNotifications(f.env.Service.Access())
	for i := 0; i < 2; i++ {
		_, e = inbox.Tick(ctx)
		must(t, e)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events e JOIN account_notifications n ON n.event_id=e.id WHERE e.source_module='display' AND e.target_id=$1 AND e.event_type='display.package'`, id).Scan(&count))
	if count != 1 {
		t.Fatal("duplicate or absent package notification", count)
	}
	var translated bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT translated_at IS NOT NULL FROM display_notification_handoffs WHERE id=$1`, handoff).Scan(&translated))
	if !translated {
		t.Fatal("bridge receipt absent")
	}
	// Cancel while a worker owns the lease but has not sealed the package. Its
	// stale fence cannot publish, even if it finishes reading/writing objects.
	must(t, f.env.Pool.QueryRow(ctx, `SELECT revision::text FROM display_instances WHERE id=$1`, id).Scan(&rev))
	u, e = s.Command(ctx, p.actor, scope, id, "upload", "", display.Command{Meta: meta(rev), Size: int64(len(data)), SHA256: displayHash(data)})
	must(t, e)
	upload = u["upload_id"].(string)
	rev = u["revision"].(string)
	must(t, s.UploadPart(ctx, p.actor, scope, id, upload, 0, displayHash(data), data))
	u, e = s.Command(ctx, p.actor, scope, id, "upload.complete", upload, display.Command{Meta: meta(rev)})
	must(t, e)
	rev = u["revision"].(string)
	gate := &displayGatedStore{displayMemoryStore: store, entered: make(chan struct{}), resume: make(chan struct{})}
	worker.WithStorage(gate)
	done := make(chan error, 1)
	go func() { _, e := worker.ProcessOne(ctx); done <- e }()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never entered read")
	}
	_, e = s.Command(ctx, p.actor, scope, id, "upload.cancel", upload, display.Command{Meta: meta(rev)})
	close(gate.resume)
	must(t, e)
	if e = <-done; e == nil {
		t.Fatal("canceled worker committed")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM display_packages WHERE instance_id=$1`, id).Scan(&count))
	if count != 1 {
		t.Fatal("stale fence created package")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_object_intents SET expires_at=clock_timestamp()-interval '1 second'`)
	must(t, e)
	must(t, worker.Maintain(ctx))
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT f.object_key FROM display_package_files f JOIN display_packages p ON p.id=f.package_id WHERE p.instance_id=$1`, id).Scan(&key))
	if _, e = store.Get(ctx, key); e != nil {
		t.Fatal("maintenance deleted sealed object")
	}
	// A process dying on its fifth lease is finalized by the restricted
	// maintenance path, including its durable personal result handoff.
	must(t, f.env.Pool.QueryRow(ctx, `SELECT revision::text FROM display_instances WHERE id=$1`, id).Scan(&rev))
	u, e = s.Command(ctx, p.actor, scope, id, "upload", "", display.Command{Meta: meta(rev), Size: int64(len(data)), SHA256: displayHash(data)})
	must(t, e)
	upload, rev = u["upload_id"].(string), u["revision"].(string)
	must(t, s.UploadPart(ctx, p.actor, scope, id, upload, 0, displayHash(data), data))
	_, e = s.Command(ctx, p.actor, scope, id, "upload.complete", upload, display.Command{Meta: meta(rev)})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_package_jobs SET state='validating',attempt=5,lease_until=clock_timestamp()-interval '1 second' WHERE upload_id=$1`, upload)
	must(t, e)
	must(t, worker.Maintain(ctx))
	for i := 0; i < 2; i++ {
		_, e = inbox.Tick(ctx)
		must(t, e)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events e JOIN account_notifications n ON n.event_id=e.id WHERE e.source_module='display' AND e.target_id=$1 AND e.event_type='display.package' AND e.outcome='failed'`, id).Scan(&count))
	if count != 1 {
		t.Fatal("expired final package lease result missing or duplicated", count)
	}
}
