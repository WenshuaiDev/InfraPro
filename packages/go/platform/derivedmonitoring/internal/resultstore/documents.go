package resultstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/google/uuid"
)

const MaxDocumentBytes = 8 << 20

func documentKey(tenant, kind, hash string) (string, error) {
	if id, e := uuid.Parse(tenant); e != nil || id.String() != tenant {
		return "", fmt.Errorf("invalid_result_document")
	}
	if !slices.Contains([]string{"manifests", "lineage", "ranges", "checkpoints", "history"}, kind) || len(hash) != 64 {
		return "", fmt.Errorf("invalid_result_document")
	}
	if _, e := hex.DecodeString(hash); e != nil {
		return "", fmt.Errorf("invalid_result_document")
	}
	return tenant + "/derived/" + kind + "/" + hash + ".json", nil
}

// Documents are content-addressed and bounded. Large result sets use immutable
// tree nodes and manifest pages, never an ever-growing PostgreSQL JSON value.
func (s *Store) SealDocument(ctx context.Context, tenant, kind string, value any) (string, error) {
	body, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	if len(body) > MaxDocumentBytes {
		return "", fmt.Errorf("manifest_budget_exceeded")
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	key, e := documentKey(tenant, kind, hash)
	if e != nil {
		return "", e
	}
	putError := s.transport.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "application/json")
	// An existing identical object is a legitimate retry. Existence alone is
	// insufficient: a failed write is accepted only after a complete hash check.
	var verified json.RawMessage
	if e = s.ReadDocument(ctx, tenant, kind, hash, &verified); e != nil {
		if putError != nil {
			return "", putError
		}
		return "", e
	}
	return hash, nil
}
func (s *Store) ReadDocument(ctx context.Context, tenant, kind, hash string, out any) error {
	key, e := documentKey(tenant, kind, hash)
	if e != nil {
		return e
	}
	object, e := s.transport.Get(ctx, key)
	if e != nil {
		return e
	}
	defer object.Close()
	body, e := io.ReadAll(io.LimitReader(object, MaxDocumentBytes+1))
	if e != nil {
		return e
	}
	if len(body) > MaxDocumentBytes {
		return fmt.Errorf("manifest_budget_exceeded")
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != hash {
		return fmt.Errorf("manifest_hash_mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}
