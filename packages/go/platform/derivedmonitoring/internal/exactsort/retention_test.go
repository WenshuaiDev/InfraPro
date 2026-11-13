package exactsort

import (
	"math/big"
	"os"
	"testing"
	"time"
)

func TestExpiredPrivateRunsKeepLiveLocksAndCollectOrphans(t *testing.T) {
	directory := t.TempDir()
	s, e := New(Options{Directory: directory, MemoryBytes: 16 << 10, Budget: &Budget{Limit: 1 << 20}, ChunkRows: 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Add(t.Context(), big.NewRat(3, 7)); e != nil {
		t.Fatal(e)
	}
	old := time.Now().Add(-48 * time.Hour)
	if e = os.Chtimes(s.directory, old, old); e != nil {
		t.Fatal(e)
	}
	n, e := PruneOrphans(t.Context(), directory, time.Now().Add(-24*time.Hour))
	if e != nil || n != 0 {
		t.Fatal("live sort was collected", n, e)
	}
	// Closing only the OS lock models a process exit with private files left.
	if e = s.lock.Close(); e != nil {
		t.Fatal(e)
	}
	n, e = PruneOrphans(t.Context(), directory, time.Now().Add(-24*time.Hour))
	if e != nil || n != 1 {
		t.Fatal("expired orphan was retained", n, e)
	}
	if _, e = os.Stat(s.directory); !os.IsNotExist(e) {
		t.Fatal("orphan files remain", e)
	}
}
