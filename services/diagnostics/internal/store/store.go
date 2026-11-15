// Package store owns the single-writer bounded bbolt cache. Ingest acknowledges
// only after Commit has durably committed fence, latest and history together.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	bolt "go.etcd.io/bbolt"
)

type Meta struct {
	SchemaVersion    int    `json:"schema_version"`
	InstanceID       string `json:"instance_id"`
	ManifestRevision string `json:"last_manifest_revision"`
	ManifestDigest   string `json:"manifest_digest"`
	KeyringRevision  string `json:"last_keyring_revision"`
	KeyringDigest    string `json:"keyring_digest"`
	EvictedCount     uint64 `json:"evicted_count"`
}
type Fence struct {
	NodeID     string     `json:"node_id"`
	Generation string     `json:"generation"`
	Epoch      string     `json:"epoch"`
	BootID     string     `json:"boot_id"`
	Sequence   string     `json:"sequence"`
	Digest     string     `json:"digest"`
	ReceivedAt *time.Time `json:"received_at"`
}
type Data struct {
	Meta        Meta
	Fences      map[string]Fence
	Latest      map[string]model.StoredObservation
	Transitions []model.Transition
	Metrics     []model.MetricPoint
	Access      []model.AccessRecord
}
type Change struct {
	Meta         *Meta
	Fences       []Fence
	Latest       []model.StoredObservation
	DeleteLatest []string
	Transitions  []model.Transition
	Metrics      []model.MetricPoint
	Access       []model.AccessRecord
	Now          time.Time
}
type Store struct {
	db           *bolt.DB
	lock         *securefile.Lock
	path         string
	limits       settings.Limits
	queue        chan struct{}
	mu           sync.Mutex
	beforeCommit func() error
	suspect      bool
}

var buckets = [][]byte{[]byte("meta"), []byte("observer_fences"), []byte("latest"), []byte("transitions"), []byte("metrics"), []byte("access_records")}

func Key(parts ...string) string { b, _ := json.Marshal(parts); return string(b) }
func Open(dir, instance string, limits settings.Limits) (s *Store, err error) {
	if limits.Normalize() != nil || !settings.ValidID(instance) {
		return nil, settings.Invalid
	}
	if e := securefile.CheckDirectory(dir, securefile.Private()); e != nil {
		return nil, settings.Error("state_unavailable")
	}
	lock, e := securefile.Acquire(filepath.Join(dir, "writer.lock"))
	if e != nil {
		if errors.Is(e, securefile.ErrLocked) {
			return nil, settings.Error("file_locked")
		}
		return nil, settings.Error("state_unavailable")
	}
	s = &Store{lock: lock, path: filepath.Join(dir, "state.db"), limits: limits, queue: make(chan struct{}, 32)}
	defer func() {
		if r := recover(); r != nil {
			err = settings.Error("state_unavailable")
		}
		if err != nil {
			if s.db != nil {
				_ = s.db.Close()
			}
			_ = lock.Close()
			s = nil
		}
	}()
	existing := false
	if f, readErr := securefile.Open(s.path, securefile.Private()); readErr == nil {
		info, statErr := f.Stat()
		_ = f.Close()
		existing = true
		if statErr != nil || info.Size() < 16384 || info.Size() > limits.StoreMaxBytes {
			return s, settings.Error("state_unavailable")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return s, settings.Error("state_unavailable")
	}
	db, e := bolt.Open(s.path, 0600, &bolt.Options{Timeout: 100 * time.Millisecond, MaxSize: int(limits.StoreMaxBytes), OpenFile: func(path string, flag int, _ os.FileMode) (*os.File, error) {
		return securefile.OpenMode(path, flag, securefile.Private())
	}})
	if e != nil {
		return s, settings.Error("state_unavailable")
	}
	s.db = db
	if existing {
		if e = validateDB(db, instance); e != nil {
			return s, e
		}
	}
	e = db.View(func(tx *bolt.Tx) error {
		invalid := false
		for issue := range tx.Check() {
			invalid = invalid || issue != nil
		}
		if invalid {
			return settings.Error("state_unavailable")
		}
		b := tx.Bucket([]byte("meta"))
		if b == nil {
			return nil
		}
		var m Meta
		if json.Unmarshal(b.Get([]byte("record")), &m) != nil {
			return settings.Error("state_unavailable")
		}
		if m.SchemaVersion != 1 {
			return settings.Error("state_version_unsupported")
		}
		if m.InstanceID != instance {
			return settings.Error("state_instance_mismatch")
		}
		return nil
	})
	if e != nil {
		return s, e
	}
	e = db.Update(func(tx *bolt.Tx) error {
		for _, name := range buckets {
			if _, e := tx.CreateBucketIfNotExists(name); e != nil {
				return e
			}
		}
		b := tx.Bucket(buckets[0])
		if b.Get([]byte("record")) == nil {
			return put(b, "record", Meta{SchemaVersion: 1, InstanceID: instance, ManifestRevision: "0", KeyringRevision: "0"})
		}
		return nil
	})
	if e != nil {
		return s, settings.Error("state_unavailable")
	}
	return s, nil
}
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.db.Close()
	_ = s.lock.Close()
	return e
}
func put(b *bolt.Bucket, key string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return e
	}
	return b.Put([]byte(key), raw)
}
func (s *Store) Load() (Data, error) {
	d := Data{Fences: map[string]Fence{}, Latest: map[string]model.StoredObservation{}, Transitions: []model.Transition{}, Metrics: []model.MetricPoint{}, Access: []model.AccessRecord{}}
	e := s.db.View(func(tx *bolt.Tx) error {
		if e := json.Unmarshal(tx.Bucket(buckets[0]).Get([]byte("record")), &d.Meta); e != nil {
			return e
		}
		readers := []func([]byte, []byte) error{
			func(k, v []byte) error {
				var x Fence
				if e := json.Unmarshal(v, &x); e != nil {
					return e
				}
				d.Fences[string(k)] = x
				return nil
			},
			func(k, v []byte) error {
				var x model.StoredObservation
				if e := json.Unmarshal(v, &x); e != nil {
					return e
				}
				d.Latest[string(k)] = x
				return nil
			},
			func(_, v []byte) error {
				var x model.Transition
				if e := json.Unmarshal(v, &x); e != nil {
					return e
				}
				d.Transitions = append(d.Transitions, x)
				return nil
			},
			func(_, v []byte) error {
				var x model.MetricPoint
				if e := json.Unmarshal(v, &x); e != nil {
					return e
				}
				d.Metrics = append(d.Metrics, x)
				return nil
			},
			func(_, v []byte) error {
				var x model.AccessRecord
				if e := json.Unmarshal(v, &x); e != nil {
					return e
				}
				d.Access = append(d.Access, x)
				return nil
			},
		}
		for i, read := range readers {
			if e := tx.Bucket(buckets[i+1]).ForEach(read); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return d, settings.Error("state_unavailable")
	}
	return d, nil
}
func (s *Store) Commit(ctx context.Context, c Change) error {
	if s == nil {
		return settings.Error("state_unavailable")
	}
	select {
	case s.queue <- struct{}{}:
	default:
		return settings.Error("state_unavailable")
	}
	defer func() { <-s.queue }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return settings.Error("state_unavailable")
	}
	if s.suspect {
		var instance string
		if err := s.db.View(func(tx *bolt.Tx) error {
			var m Meta
			if json.Unmarshal(tx.Bucket(buckets[0]).Get([]byte("record")), &m) != nil {
				return settings.Invalid
			}
			instance = m.InstanceID
			return nil
		}); err != nil {
			return settings.Error("state_unavailable")
		}
		if validateDB(s.db, instance) != nil {
			return settings.Error("state_unavailable")
		}
		s.suspect = false
	}
	e := s.db.Update(func(tx *bolt.Tx) error {
		if c.Meta != nil {
			var prior Meta
			bucket := tx.Bucket(buckets[0])
			if json.Unmarshal(bucket.Get([]byte("record")), &prior) != nil {
				return settings.Error("state_unavailable")
			}
			meta := *c.Meta
			if prior.EvictedCount > meta.EvictedCount {
				meta.EvictedCount = prior.EvictedCount
			}
			if e := put(bucket, "record", meta); e != nil {
				return e
			}
		}
		for _, v := range c.Fences {
			if e := put(tx.Bucket(buckets[1]), Key(v.NodeID, v.Generation), v); e != nil {
				return e
			}
		}
		for _, v := range c.Latest {
			if e := put(tx.Bucket(buckets[2]), v.ProbeID, v); e != nil {
				return e
			}
		}
		for _, id := range c.DeleteLatest {
			if e := tx.Bucket(buckets[2]).Delete([]byte(id)); e != nil {
				return e
			}
		}
		info, e := os.Stat(s.path)
		if e != nil {
			return e
		}
		reserve := min(int64(32<<20), s.limits.StoreMaxBytes/8)
		historyAllowed := info.Size() < s.limits.StoreMaxBytes-reserve || s.db.Stats().FreePageN*tx.DB().Info().PageSize > int(reserve)
		if historyAllowed {
			for _, v := range c.Transitions {
				if e := put(tx.Bucket(buckets[3]), Key(v.At.UTC().Format(time.RFC3339Nano), v.ID), v); e != nil {
					return e
				}
			}
			for _, v := range c.Metrics {
				if e := put(tx.Bucket(buckets[4]), Key(v.At.UTC().Truncate(time.Minute).Format(time.RFC3339), v.TargetKind, v.TargetID, v.Key, v.DimensionID, v.NodeID), v); e != nil {
					return e
				}
			}
			for _, v := range c.Access {
				if e := put(tx.Bucket(buckets[5]), Key(v.At.UTC().Format(time.RFC3339Nano), v.ID), v); e != nil {
					return e
				}
			}
		}
		now := c.Now
		if now.IsZero() {
			now = time.Now()
		}
		var evicted uint64
		for _, v := range []struct {
			b      []byte
			cutoff time.Time
			count  int
			bytes  int
		}{{buckets[3], now.Add(-24 * time.Hour), 10000, 16 << 20}, {buckets[4], now.Add(-6 * time.Hour), s.limits.HistoryPoints, 64 << 20}, {buckets[5], now.Add(-7 * 24 * time.Hour), 2000, 2 << 20}} {
			b := tx.Bucket(v.b)
			keys := []string{}
			size := 0
			if e := b.ForEach(func(k, raw []byte) error { keys = append(keys, string(k)); size += len(raw) + len(k); return nil }); e != nil {
				return e
			}
			sort.Strings(keys)
			for index, k := range keys {
				var parts []string
				_ = json.Unmarshal([]byte(k), &parts)
				at := time.Time{}
				if len(parts) > 0 {
					at, _ = time.Parse(time.RFC3339Nano, parts[0])
				}
				if !at.Before(v.cutoff) && len(keys)-index <= v.count && size <= v.bytes {
					break
				}
				size -= len(b.Get([]byte(k))) + len(k)
				if e := b.Delete([]byte(k)); e != nil {
					return e
				}
				evicted++
			}
		}
		if !historyAllowed {
			evicted += uint64(len(c.Transitions) + len(c.Metrics) + len(c.Access))
		}
		if evicted > 0 {
			var m Meta
			b := tx.Bucket(buckets[0])
			if json.Unmarshal(b.Get([]byte("record")), &m) != nil {
				return settings.Error("state_unavailable")
			}
			m.EvictedCount += evicted
			if e := put(b, "record", m); e != nil {
				return e
			}
		}
		if s.beforeCommit != nil {
			if e := s.beforeCommit(); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		s.suspect = true
		return settings.Error("state_unavailable")
	}
	return nil
}

func (s *Store) Validate(instance string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if validateDB(s.db, instance) != nil {
		s.suspect = true
		return settings.Error("state_unavailable")
	}
	_, err := s.Load()
	if err != nil {
		s.suspect = true
		return err
	}
	return nil
}
func (s *Store) Info() map[string]any {
	v := map[string]any{"max_bytes": s.limits.StoreMaxBytes, "schema_version": 1}
	if st, e := os.Stat(s.path); e == nil {
		v["bytes"] = st.Size()
	}
	v["free_pages"] = s.db.Stats().FreePageN
	_ = s.db.View(func(tx *bolt.Tx) error {
		var m Meta
		if json.Unmarshal(tx.Bucket(buckets[0]).Get([]byte("record")), &m) == nil {
			v["evicted_count"] = m.EvictedCount
		}
		return nil
	})
	return v
}
