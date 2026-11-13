package monitoring

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// A fixed reference retains its original adoption set. Later publications do
// not change it, and readoption cannot revive a revoked original adoption.
// This checks provenance and current authority, without reading revoked data.
func fixedRawStateTx(ctx context.Context, tx pgx.Tx, tenant string, evidence rawEvidence) (RawSnapshotState, error) {
	state, e := checkRawEvidenceTx(ctx, tx, tenant, evidence, true)
	if e != nil {
		return state, e
	}
	sequences := []int64{}
	for _, chunk := range evidence.Manifest {
		sequences = append(sequences, chunk.Sequence)
	}
	slices.Sort(sequences)
	sequences = slices.Compact(sequences)
	channels, ranges := []string{}, []string{}
	for _, n := range evidence.Candidates {
		channels = append(channels, n.Channel)
		ranges = append(ranges, (Window{n.From, n.To}).pg())
	}
	e = tx.QueryRow(ctx, `SELECT count(*)>0,COALESCE(max(e.source_position),$2)::text FROM monitoring_changes e JOIN monitoring_publications p ON p.tenant_id=e.tenant_id AND p.id=e.source_id JOIN monitoring_change_ranges r ON r.tenant_id=e.tenant_id AND r.event_id=e.id JOIN unnest($3::uuid[],$4::int8range[]) n(channel_id,affected) ON n.channel_id=r.channel_id AND n.affected&&r.affected WHERE e.tenant_id=$1 AND e.source_position>$2 AND e.change_type='raw_revoke' AND p.sequence=ANY($5::bigint[])`, tenant, mustRevision(evidence.Snapshot.SourcePosition), channels, ranges, sequences).Scan(&state.Revoked, &state.SourcePosition)
	state.Changed = state.Revoked
	return state, e
}
func (s *Service) CheckFixedRawUserTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (RawSnapshotState, error) {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return RawSnapshotState{}, e
	}
	evidence, e := loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = authorizeRawNeedsTx(ctx, tx, auth, evidence.Needs, false); e != nil {
		return RawSnapshotState{}, e
	}
	return fixedRawStateTx(ctx, tx, scope.Tenant, evidence)
}
func (s *Service) CheckFixedRawSystemTx(ctx context.Context, tx pgx.Tx, p RawSystemPermit, id string) (RawSnapshotState, error) {
	if s.rawSystem == nil {
		return RawSnapshotState{}, access.ErrDenied
	}
	evidence, e := loadRawEvidenceTx(ctx, tx, p.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = s.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, evidence.Needs, false); e != nil {
		return RawSnapshotState{}, e
	}
	return fixedRawStateTx(ctx, tx, p.Tenant, evidence)
}

// A fixed derived baseline may reference only part of an older generation.
// Qualify that original adoption subset, without letting later append/readopt
// replace it or requiring permission to the rest of the upstream generation.
func (s *Service) CheckFixedRawPartTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, needs []RawNeed, capabilities ...string) (RawSnapshotState, error) {
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return RawSnapshotState{}, e
	}
	original, e := loadRawEvidenceTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	part, e := rawDependencyPart(original, needs)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = authorizeRawNeedsTx(ctx, tx, auth, part.Needs, false, capabilities...); e != nil {
		return RawSnapshotState{}, e
	}
	return fixedRawStateTx(ctx, tx, scope.Tenant, part)
}
func (s *Service) CheckFixedRawDependencySystemTx(ctx context.Context, tx pgx.Tx, p RawSystemPermit, id string, needs []RawNeed) (RawSnapshotState, error) {
	if s.rawSystem == nil {
		return RawSnapshotState{}, access.ErrDenied
	}
	original, e := loadRawEvidenceTx(ctx, tx, p.Tenant, id)
	if e != nil {
		return RawSnapshotState{}, e
	}
	part, e := rawDependencyPart(original, needs)
	if e != nil {
		return RawSnapshotState{}, e
	}
	if e = s.rawSystem.AuthorizeRawSystemTx(ctx, tx, p, part.Needs, false); e != nil {
		return RawSnapshotState{}, e
	}
	return fixedRawStateTx(ctx, tx, p.Tenant, part)
}
