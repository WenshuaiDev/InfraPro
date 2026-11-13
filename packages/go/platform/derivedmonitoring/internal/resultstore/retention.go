package resultstore

import (
	"context"
	"fmt"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func (s *Store) PurgeTemporaryRun(ctx context.Context, tenant, run string, check func(context.Context) error) error {
	return s.transport.PurgeTemporaryRun(ctx, tenant, run, check)
}
func (s *Store) CheckpointObjects(ctx context.Context, tenant, after string) ([]monitoring.CalculationCheckpointObject, error) {
	return s.transport.CheckpointObjects(ctx, tenant, after)
}
func (s *Store) RemoveCheckpoint(ctx context.Context, tenant, hash string) error {
	return s.transport.RemoveCheckpointObject(ctx, tenant, hash)
}

// The domain must first prove this attempt is not selected by any immutable
// publication and record the retention decision under the shared job fence.
func (s *Store) PurgeUnselectedAttempt(ctx context.Context, tenant, shard, chunk, attempt string, check func(context.Context) error) error {
	if !access.ValidID(tenant) || !access.ValidID(chunk) || !access.ValidID(attempt) || check == nil {
		return fmt.Errorf("invalid_attempt_retention")
	}
	g, e := s.transport.Group(shard)
	if e != nil {
		return e
	}
	for _, replica := range g.Replicas {
		if e = check(ctx); e != nil {
			return e
		}
		if e = replica.Exec(s.transport.QueryContext(ctx), "ALTER TABLE "+g.Database+".derived_results_local DELETE WHERE tenant_id=? AND attempt_id=? SETTINGS mutations_sync=2", uuid.MustParse(tenant), uuid.MustParse(attempt)); e != nil {
			return e
		}
	}
	for _, replica := range g.Replicas {
		var count uint64
		if e = replica.QueryRow(s.transport.QueryContext(ctx), "SELECT count() FROM "+g.Database+".derived_results_local WHERE tenant_id=? AND attempt_id=?", uuid.MustParse(tenant), uuid.MustParse(attempt)).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			return fmt.Errorf("attempt_retention_incomplete")
		}
	}
	if e = check(ctx); e != nil {
		return e
	}
	return s.transport.RemoveAttemptObject(ctx, tenant, chunk, attempt)
}
