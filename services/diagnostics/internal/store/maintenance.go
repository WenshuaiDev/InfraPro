package store

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sys/unix"
)

type Inspection struct {
	SchemaVersion    int            `json:"schema_version"`
	InstanceID       string         `json:"instance_id"`
	FileBytes        int64          `json:"file_bytes"`
	MaxBytes         int64          `json:"max_bytes"`
	Buckets          map[string]int `json:"buckets"`
	ManifestRevision string         `json:"manifest_revision"`
	KeyringRevision  string         `json:"keyring_revision"`
	Fences           []FenceSummary `json:"fences"`
}
type FenceSummary struct {
	NodeID     string `json:"node_id"`
	Generation string `json:"generation"`
	Epoch      string `json:"epoch"`
	Sequence   string `json:"sequence"`
}

func validateDB(db *bolt.DB, instance string) error {
	return db.View(func(tx *bolt.Tx) error {
		invalid := false
		for issue := range tx.Check() {
			invalid = invalid || issue != nil
		}
		if invalid {
			return settings.Error("state_unavailable")
		}
		for _, name := range buckets {
			if tx.Bucket(name) == nil {
				return settings.Error("state_unavailable")
			}
		}
		var m Meta
		if settings.Strict(tx.Bucket(buckets[0]).Get([]byte("record")), &m) != nil {
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
}
func openReadOnly(path, instance string, maxBytes int64) (*bolt.DB, error) {
	f, e := securefile.Open(path, securefile.Private())
	if e != nil {
		return nil, settings.Error("state_unavailable")
	}
	info, e := f.Stat()
	_ = f.Close()
	if e != nil || info.Size() < 16384 || info.Size() > maxBytes {
		return nil, settings.Error("state_unavailable")
	}
	db, e := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: 100 * time.Millisecond, MaxSize: int(maxBytes), OpenFile: func(p string, flags int, _ os.FileMode) (*os.File, error) {
		return securefile.OpenMode(p, flags, securefile.Private())
	}})
	if e != nil {
		if errors.Is(e, bolt.ErrTimeout) {
			return nil, settings.Error("file_locked")
		}
		return nil, settings.Error("state_unavailable")
	}
	if e = validateDB(db, instance); e != nil {
		_ = db.Close()
		return nil, e
	}
	return db, nil
}
func Inspect(dir, instance string, limits settings.Limits) (result Inspection, err error) {
	defer func() {
		if recover() != nil {
			err = settings.Error("state_unavailable")
		}
	}()
	if limits.Normalize() != nil || securefile.CheckDirectory(dir, securefile.Private()) != nil {
		return result, settings.Invalid
	}
	db, e := openReadOnly(filepath.Join(dir, "state.db"), instance, limits.StoreMaxBytes)
	if e != nil {
		return result, e
	}
	defer db.Close()
	result = Inspection{SchemaVersion: 1, InstanceID: instance, MaxBytes: limits.StoreMaxBytes, Buckets: map[string]int{}, Fences: []FenceSummary{}}
	err = db.View(func(tx *bolt.Tx) error {
		result.FileBytes = tx.Size()
		var m Meta
		if json.Unmarshal(tx.Bucket(buckets[0]).Get([]byte("record")), &m) != nil {
			return settings.Error("state_unavailable")
		}
		result.ManifestRevision, result.KeyringRevision = m.ManifestRevision, m.KeyringRevision
		for _, name := range buckets {
			result.Buckets[string(name)] = tx.Bucket(name).Stats().KeyN
		}
		return tx.Bucket(buckets[1]).ForEach(func(_, value []byte) error {
			var f Fence
			if json.Unmarshal(value, &f) != nil {
				return settings.Error("state_unavailable")
			}
			result.Fences = append(result.Fences, FenceSummary{f.NodeID, f.Generation, f.Epoch, f.Sequence})
			return nil
		})
	})
	return result, err
}
func Compact(dir, instance string, limits settings.Limits) (err error) {
	defer func() {
		if recover() != nil {
			err = settings.Error("state_unavailable")
		}
	}()
	if limits.Normalize() != nil || securefile.CheckDirectory(dir, securefile.Private()) != nil {
		return settings.Invalid
	}
	lock, e := securefile.Acquire(filepath.Join(dir, "writer.lock"))
	if e != nil {
		return settings.Error("file_locked")
	}
	defer lock.Close()
	path := filepath.Join(dir, "state.db")
	source, e := openReadOnly(path, instance, limits.StoreMaxBytes)
	if e != nil {
		return e
	}
	defer source.Close()
	var size int64
	_ = source.View(func(tx *bolt.Tx) error { size = tx.Size(); return nil })
	var stat unix.Statfs_t
	if unix.Statfs(dir, &stat) != nil || uint64(stat.Bavail)*uint64(stat.Bsize) < uint64(size+32<<20) {
		return settings.Error("state_unavailable")
	}
	temp := filepath.Join(dir, ".compact-"+rand.Text())
	f, e := securefile.OpenMode(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, securefile.Private())
	if e != nil {
		return settings.Error("state_unavailable")
	}
	_ = f.Close()
	defer os.Remove(temp)
	destination, e := bolt.Open(temp, 0600, &bolt.Options{MaxSize: int(limits.StoreMaxBytes), OpenFile: func(p string, flags int, _ os.FileMode) (*os.File, error) {
		return securefile.OpenMode(p, flags, securefile.Private())
	}})
	if e != nil {
		return settings.Error("state_unavailable")
	}
	defer destination.Close()
	if e = bolt.Compact(destination, source, 4<<20); e != nil {
		return settings.Error("state_unavailable")
	}
	if e = validateDB(destination, instance); e != nil {
		return e
	}
	// Compact copies every key verbatim, including generation/epoch/sequence.
	if e = destination.Sync(); e != nil {
		return settings.Error("state_unavailable")
	}
	if e = destination.Close(); e != nil {
		return settings.Error("state_unavailable")
	}
	if e = source.Close(); e != nil {
		return settings.Error("state_unavailable")
	}
	if e = securefile.Replace(temp, path); e != nil {
		return settings.Error("state_unavailable")
	}
	return nil
}
