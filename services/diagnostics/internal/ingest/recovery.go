package ingest

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/store"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

// RetryStore also covers a server with remote observers only: no local sample
// is required to recover a temporarily failed writer. AttachStore validates the
// entire database and commits metadata before reopening ingestion.
func (e *Engine) RetryStore(ctx context.Context) error {
	e.mu.RLock()
	db, state := e.db, e.storeState
	e.mu.RUnlock()
	if state == "ok" {
		return nil
	}
	return e.AttachStore(ctx, db)
}

// AttachStore only accepts a fully checked database; a recovery must never roll
// acknowledged generations/epochs/sequences backwards or recreate their fences.
func (e *Engine) AttachStore(ctx context.Context, db *store.Store) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if db == nil || db.Validate(e.meta.InstanceID) != nil {
		return settings.Error("state_unavailable")
	}
	d, err := db.Load()
	if err != nil {
		return err
	}
	for key, old := range e.fences {
		candidate, ok := d.Fences[key]
		if !ok || settings.Number(candidate.Epoch) < settings.Number(old.Epoch) || candidate.Epoch == old.Epoch && (candidate.BootID != old.BootID || settings.Number(candidate.Sequence) < settings.Number(old.Sequence)) {
			return settings.Error("state_unavailable")
		}
	}
	// Offline rollback checks still apply even if the server initially had no DB.
	if settings.Number(d.Meta.ManifestRevision) > settings.Number(e.meta.ManifestRevision) && e.meta.ManifestRevision != "0" || settings.Number(d.Meta.KeyringRevision) > settings.Number(e.meta.KeyringRevision) && e.meta.KeyringRevision != "0" {
		return settings.Error("config_rollback")
	}
	meta := e.meta
	if meta.ManifestRevision == "0" {
		meta = d.Meta
	} else {
		meta.EvictedCount = d.Meta.EvictedCount
		if meta.KeyringRevision == "0" {
			meta.KeyringRevision = d.Meta.KeyringRevision
			meta.KeyringDigest = d.Meta.KeyringDigest
		}
	}
	if err = db.Commit(ctx, store.Change{Meta: &meta, Now: e.now()}); err != nil {
		return err
	}
	e.db = db
	e.meta = meta
	e.fences = d.Fences
	for id, sample := range d.Latest {
		if old, ok := e.latest[id]; !ok || old.Persistence != "volatile" {
			e.latest[id] = sample
		}
	}
	e.transitions = d.Transitions
	e.metrics = d.Metrics
	e.sessions = map[string]session{}
	e.storeState = "ok"
	return nil
}
