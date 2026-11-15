package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/store"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
)

func fixture(t *testing.T) (*Engine, *store.Store, string, SessionRequest, *time.Time) {
	t.Helper()
	s, err := store.Open(testfixture.Directory(t), "lab", settings.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e, err := New(s, "lab", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	e.now = func() time.Time { return now }
	keys, bearer := testfixture.Keyring()
	m := testfixture.Manifest()
	if err = e.SetConfig(context.Background(), m, keys, settings.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	return e, s, bearer, SessionRequest{1, "lab", "node", "1", "1", "1", uuid.NewString()}, &now
}
func frame(e *Engine, r SessionRequest, n string) Frame {
	f := Frame{SessionRequest: r, Sequence: n, GeneratedAt: e.now(), Observations: []model.Observation{}}
	for _, p := range e.manifest.Probes {
		o := model.New(p, r.BootID, e.now())
		o.Facets = model.NeutralFacets()
		if p.Facet == "presence" {
			o.Facets.Presence = "running"
		} else {
			o.Facets.Readiness = "ready"
			o.Facets.Loop = "idle"
		}
		f.Observations = append(f.Observations, o)
	}
	return f
}
func send(e *Engine, token string, f Frame) (Ack, error) {
	raw, _ := json.Marshal(f)
	return e.Accept(context.Background(), token, raw)
}
func TestStableHeartbeatsDoNotCreateTransitionRows(t *testing.T) {
	e, _, key, request, now := fixture(t)
	session, err := e.Handshake(context.Background(), key, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = send(e, session.Token, frame(e, request, "1")); err != nil {
		t.Fatal(err)
	}
	initial := len(e.Read().Transitions)
	if initial == 0 {
		t.Fatal("initial evidence missing")
	}
	for _, sequence := range []string{"2", "3", "4"} {
		*now = now.Add(5 * time.Second)
		if _, err = send(e, session.Token, frame(e, request, sequence)); err != nil {
			t.Fatal(err)
		}
		e.Tick(context.Background())
	}
	if len(e.Read().Transitions) != initial {
		t.Fatal("stable heartbeat added history rows")
	}
	*now = now.Add(time.Minute)
	e.Tick(context.Background())
	if len(e.Read().Transitions) <= initial {
		t.Fatal("expiry without a new report was omitted")
	}
}
func TestDuplicateDoesNotRenewSamplesOrOnlineTime(t *testing.T) {
	e, _, key, r, now := fixture(t)
	session, err := e.Handshake(context.Background(), key, r)
	if err != nil {
		t.Fatal(err)
	}
	f := frame(e, r, "1")
	a, err := send(e, session.Token, f)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(25 * time.Second)
	dup, err := send(e, session.Token, f)
	if err != nil || !dup.Duplicate || dup.ReceivedAt != a.ReceivedAt || e.Read().Online["node"] {
		t.Fatal("duplicate extended online", err)
	}
	f.Observations[0].Facets.Presence = "stopped"
	if _, err = send(e, session.Token, f); settings.Code(err) != "sequence_conflict" {
		t.Fatal(err)
	}
}

func TestRemoteOnlyStoreRecoveryPreservesFences(t *testing.T) {
	e, _, key, request, now := fixture(t)
	session, err := e.Handshake(context.Background(), key, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = send(e, session.Token, frame(e, request, "3")); err != nil {
		t.Fatal(err)
	}
	e.storeState = "state_unavailable"
	if _, err = e.Handshake(context.Background(), key, request); settings.Code(err) != "state_unavailable" {
		t.Fatal(err)
	}
	if err = e.RetryStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Authorize(session.Token); err == nil {
		t.Fatal("pre-recovery token survived")
	}
	session, err = e.Handshake(context.Background(), key, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = send(e, session.Token, frame(e, request, "2")); settings.Code(err) != "stale_sequence" {
		t.Fatal("fence lost", err)
	}
	*now = now.Add(2 * time.Second)
	if _, err = send(e, session.Token, frame(e, request, "4")); err != nil {
		t.Fatal(err)
	}
}
func TestFullFrameAuthorizationAndFencing(t *testing.T) {
	e, _, key, r, now := fixture(t)
	s, err := e.Handshake(context.Background(), key, r)
	if err != nil {
		t.Fatal(err)
	}
	f := frame(e, r, "1")
	f.Observations = f.Observations[:1]
	if _, err = send(e, s.Token, f); settings.Code(err) != "observer_scope_denied" {
		t.Fatal(err)
	}
	if len(e.Read().Latest) != 0 {
		t.Fatal("partial frame updated state")
	}
	f = frame(e, r, "1")
	f.NodeID = "other"
	if _, err = send(e, s.Token, f); settings.Code(err) != "observer_scope_denied" {
		t.Fatal(err)
	}
	f = frame(e, r, "2")
	if _, err = send(e, s.Token, f); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(5 * time.Second)
	old := frame(e, r, "1")
	if _, err = send(e, s.Token, old); settings.Code(err) != "stale_sequence" {
		t.Fatal(err)
	}
	r2 := r
	r2.Epoch = "2"
	r2.BootID = uuid.NewString()
	if _, err = e.Handshake(context.Background(), key, r2); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Authorize(s.Token); err == nil {
		t.Fatal("old token survived epoch")
	}
	if _, err = e.Handshake(context.Background(), key, r); settings.Code(err) != "epoch_conflict" {
		t.Fatal(err)
	}
	r2.BootID = uuid.NewString()
	if _, err = e.Handshake(context.Background(), key, r2); settings.Code(err) != "epoch_conflict" {
		t.Fatal(err)
	}
}
func TestCachedAttemptAndServerRestartStayHistorical(t *testing.T) {
	e, s, key, r, now := fixture(t)
	session, _ := e.Handshake(context.Background(), key, r)
	f := frame(e, r, "1")
	if _, err := send(e, session.Token, f); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(6 * time.Second)
	f.Sequence = "2"
	f.GeneratedAt = *now
	for i := range f.Observations {
		age := int64(6000)
		f.Observations[i].SampleAgeMS = &age
	}
	if _, err := send(e, session.Token, f); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(s, "lab", "")
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = e.now
	if err = restarted.SetConfig(context.Background(), e.manifest, e.keys, settings.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	session, _ = restarted.Handshake(context.Background(), key, r)
	*now = now.Add(5 * time.Second)
	f.Sequence = "3"
	f.GeneratedAt = *now
	for i := range f.Observations {
		age := int64(11000)
		f.Observations[i].SampleAgeMS = &age
	}
	if _, err = send(restarted, session.Token, f); err != nil {
		t.Fatal(err)
	}
	snap := restarted.Read()
	stored := snap.Latest["presence"]
	if model.View(&stored, e.manifest.Probes[0], restarted.boot, *now, true).Freshness != "historical" {
		t.Fatal("old attempt turned green")
	}
	*now = now.Add(5 * time.Second)
	if _, err = send(restarted, session.Token, frame(restarted, r, "4")); err != nil {
		t.Fatal(err)
	}
	stored = restarted.Read().Latest["presence"]
	if model.View(&stored, e.manifest.Probes[0], restarted.boot, *now, true).Freshness != "fresh" {
		t.Fatal("new sample not fresh")
	}
}
func TestAgeRollbackMutationAndClockSkew(t *testing.T) {
	e, _, key, r, now := fixture(t)
	s, _ := e.Handshake(context.Background(), key, r)
	f := frame(e, r, "1")
	age := int64(1000)
	f.Observations[0].SampleAgeMS = &age
	if _, err := send(e, s.Token, f); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(5 * time.Second)
	f.Sequence = "2"
	f.GeneratedAt = *now
	age = 0
	f.Observations[0].SampleAgeMS = &age
	if _, err := send(e, s.Token, f); settings.Code(err) != "invalid_report" {
		t.Fatal("age rollback", err)
	}
	f = frame(e, r, "2")
	f.GeneratedAt = now.Add(10 * time.Second)
	if _, err := send(e, s.Token, f); err != nil {
		t.Fatal(err)
	}
	got := e.Read().Latest["presence"]
	if !got.ClockUncertain {
		t.Fatal("future clock marked fresh")
	}
}
func TestFailedCommitNeverAcknowledgesOrUpdatesMemory(t *testing.T) {
	e, s, key, r, _ := fixture(t)
	session, _ := e.Handshake(context.Background(), key, r)
	_ = s.Close()
	if _, err := send(e, session.Token, frame(e, r, "1")); settings.Code(err) != "state_unavailable" {
		t.Fatal(err)
	}
	if len(e.Read().Latest) != 0 {
		t.Fatal("uncommitted values visible")
	}
}
