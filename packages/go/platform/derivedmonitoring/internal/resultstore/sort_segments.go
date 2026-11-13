package resultstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
)

func (s *Store) TempDirectory() string { return s.transport.TempDirectory() }

func sortSegmentKey(tenant, run string, segment exactsort.Segment) (string, error) {
	if !access.ValidID(tenant) || !access.ValidID(run) || len(segment.SHA256) != 64 {
		return "", fmt.Errorf("invalid_sort_identity")
	}
	if _, e := hex.DecodeString(segment.SHA256); e != nil {
		return "", e
	}
	return tenant + "/derived/temp/" + run + "/" + segment.SHA256 + ".rat", nil
}

func (s *Store) ReadSortSegment(ctx context.Context, tenant, run string, segment exactsort.Segment) (io.ReadCloser, error) {
	key, e := sortSegmentKey(tenant, run, segment)
	if e != nil {
		return nil, e
	}
	return s.transport.Get(ctx, key)
}

// Sealed sort runs have a separate temporary prefix. They are never referenced
// as formal result blocks and cannot be confused with permanent source evidence.
func (s *Store) SealSortSegment(ctx context.Context, tenant, run, path string, segment exactsort.Segment) (string, error) {
	key, e := sortSegmentKey(tenant, run, segment)
	if e != nil {
		return "", e
	}
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	putErr := s.transport.Put(ctx, key, f, int64(segment.Bytes), "application/x-infrapro-exact-rationals")
	object, e := s.transport.Get(ctx, key)
	if e != nil {
		if putErr != nil {
			return "", putErr
		}
		return "", e
	}
	defer object.Close()
	hash := sha256.New()
	count, e := io.Copy(hash, io.LimitReader(object, int64(segment.Bytes)+1))
	if e != nil {
		return "", e
	}
	if uint64(count) != segment.Bytes || hex.EncodeToString(hash.Sum(nil)) != segment.SHA256 {
		return "", fmt.Errorf("sort_segment_integrity_unavailable")
	}
	return key, nil
}
