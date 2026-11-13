package exactsort

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExternalRanksExactAcrossPassesAndBudget(t *testing.T) {
	ctx := context.Background()
	budget := &Budget{Limit: 1 << 20}
	sealed := 0
	s, e := New(Options{Directory: t.TempDir(), MemoryBytes: 16 << 10, Budget: budget, ChunkRows: 17, FanIn: 4, Seal: func(_ context.Context, path string, meta Segment) error {
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(body)
		if hex.EncodeToString(hash[:]) != meta.SHA256 || uint64(len(body)) != meta.Bytes {
			t.Fatal("unsealed run")
		}
		sealed++
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := 999; i >= 0; i-- {
		v := new(big.Rat).SetFrac(big.NewInt(int64(i/2)), big.NewInt(1000000000000))
		if e = s.Add(ctx, v); e != nil {
			t.Fatal(e)
		}
	}
	ranks := []uint64{0, 1, 499, 500, 998, 999}
	got, e := s.Ranks(ctx, ranks)
	if e != nil {
		t.Fatal(e)
	}
	for _, rank := range ranks {
		want := new(big.Rat).SetFrac(new(big.Int).SetUint64(rank/2), big.NewInt(1000000000000))
		if got[rank].Cmp(want) != 0 {
			t.Fatal("inexact rank", rank, got[rank], want)
		}
	}
	if sealed <= 60 || budget.Peak > budget.Limit {
		t.Fatal("fan-in or quota not exercised", sealed, budget)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if budget.Used != 0 {
		t.Fatal("temporary bytes leaked", budget)
	}
	s, e = New(Options{Directory: t.TempDir(), MemoryBytes: 16 << 10, Budget: &Budget{Limit: 16 << 10}, ChunkRows: 3})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := range 10000 {
		e = s.Add(ctx, big.NewRat(int64(i), 3))
		if e != nil {
			break
		}
	}
	if e == nil || !strings.Contains(e.Error(), "budget_exceeded") {
		t.Fatal("disk budget ignored", e)
	}
}

func TestExternalSortRestoresSealedPrefixWithoutResorting(t *testing.T) {
	objects := map[string]string{}
	options := Options{Directory: t.TempDir(), MemoryBytes: 16 << 10, Budget: &Budget{Limit: 1 << 20}, ChunkRows: 17, FanIn: 4, Seal: func(_ context.Context, path string, segment Segment) error {
		body, e := os.ReadFile(path)
		objects[segment.SHA256] = string(body)
		return e
	}}
	s, e := New(options)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := range 123 {
		if e = s.Add(t.Context(), big.NewRat(int64(599-i), 1000000000000)); e != nil {
			t.Fatal(e)
		}
	}
	checkpoint, e := s.Checkpoint(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	seals := 0
	options.Seal = func(context.Context, string, Segment) error { seals++; return nil }
	restored, e := New(options)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if e = restored.Restore(t.Context(), checkpoint, func(_ context.Context, v Segment) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(objects[v.SHA256])), nil
	}); e != nil {
		t.Fatal(e)
	}
	if seals != 0 {
		t.Fatal("restored runs were sealed again")
	}
	for i := 123; i < 600; i++ {
		if e = restored.Add(t.Context(), big.NewRat(int64(599-i), 1000000000000)); e != nil {
			t.Fatal(e)
		}
	}
	values, e := restored.Ranks(t.Context(), []uint64{0, 299, 300, 599})
	if e != nil {
		t.Fatal(e)
	}
	for rank, value := range values {
		if value.Cmp(big.NewRat(int64(rank), 1000000000000)) != 0 {
			t.Fatal("resumed sort lost or counted prefix twice", rank, value)
		}
	}
}

func TestExternalSortCancellationAndFullDisk(t *testing.T) {
	for _, code := range []error{syscall.ENOSPC, syscall.EDQUOT} {
		e := temporaryError(&os.PathError{Op: "write", Path: "private-run", Err: code})
		if !strings.Contains(e.Error(), "temporary_budget_exceeded") || !errors.Is(e, code) {
			t.Fatal("full disk did not retain actionable budget classification", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	merging := false
	budget := &Budget{Limit: 1 << 20}
	s, e := New(Options{Directory: t.TempDir(), MemoryBytes: 16 << 10, Budget: budget, ChunkRows: 16, FanIn: 2, Seal: func(context.Context, string, Segment) error {
		if merging {
			cancel()
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := range 1024 {
		if e = s.Add(ctx, big.NewRat(int64(1024-i), 7)); e != nil {
			t.Fatal(e)
		}
	}
	merging = true
	start := time.Now()
	if _, e = s.Ranks(ctx, []uint64{512}); !errors.Is(e, context.Canceled) || time.Since(start) > 2*time.Second {
		t.Fatal("external merge did not stop within cancellation target", e, time.Since(start))
	}
	if e = s.Close(); e != nil || budget.Used != 0 {
		t.Fatal("cancelled external merge leaked temporary files", e, budget)
	}
}
func TestExternalRunCorruptionNeverBecomesMissingRank(t *testing.T) {
	s, e := New(Options{Directory: t.TempDir(), MemoryBytes: 16 << 10, Budget: &Budget{Limit: 1 << 20}, ChunkRows: 2})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := range 4 {
		if e = s.Add(context.Background(), big.NewRat(int64(i), 1)); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.WriteFile(filepath.Clean(s.files[0].path), []byte("0\n9\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Ranks(context.Background(), []uint64{0}); e == nil {
		t.Fatal("corrupt unselected tail accepted")
	}
}
