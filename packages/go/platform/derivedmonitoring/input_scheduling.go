package derivedmonitoring

import (
	"context"
	"math"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

// This indexed metadata bound is only a scheduling hint, never evidence of
// current/complete coverage. The fixed source reader still verifies roots,
// assignments, holes, revocations and recursive candidate ranges before use.
func primaryClockHorizonTx(ctx context.Context, tx pgx.Tx, tenant, channel string, output planning.Range) (planning.Range, error) {
	var derived bool
	if e := tx.QueryRow(ctx, `SELECT source_kind='derived' FROM engineering_channels WHERE tenant_id=$1 AND id=$2`, tenant, channel).Scan(&derived); e != nil || !derived {
		return output, e
	}
	var through int64
	e := tx.QueryRow(ctx, `SELECT COALESCE(upper(output_range),$3) FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2 ORDER BY upper(output_range) DESC NULLS FIRST LIMIT 1`, tenant, channel, int64(math.MaxInt64)).Scan(&through)
	if e == pgx.ErrNoRows {
		through = output.From
	} else if e != nil {
		return output, e
	}
	if output.To == nil || through < *output.To {
		output.To = &through
	}
	return output, nil
}

// Sparse appends must also establish the absence of clock events between two
// publications. Otherwise a downstream reader correctly rejects that unproven
// gap forever. Extend only a genuinely uncomputed gap, within this assignment
// and mandate; late corrections inside previously computed coverage stay local.
func primaryClockGapTx(ctx context.Context, tx pgx.Tx, tenant, channel, config string, boundary int64, output planning.Range) (planning.Range, error) {
	var covered bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2 AND config_id=$3 AND output_range @> $4::bigint)`, tenant, channel, config, output.From).Scan(&covered); e != nil || covered {
		return output, e
	}
	var previous int64
	e := tx.QueryRow(ctx, `SELECT upper(output_range) FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2 AND config_id=$3 AND upper(output_range)<=$4 AND upper(output_range)>=$5 ORDER BY upper(output_range) DESC LIMIT 1`, tenant, channel, config, output.From, boundary).Scan(&previous)
	if e == pgx.ErrNoRows {
		return output, nil
	}
	if e == nil {
		output.From = previous
	}
	return output, e
}

// The scheduler only inspects bounded PostgreSQL dependency metadata. Physical
// coverage and exact rank prefixes are verified by the fixed input reader.
func (s *Service) upstreamWorkPendingTx(ctx context.Context, tx pgx.Tx, tenant string, needs []monitoring.RawNeed, prefixes []monitoring.RawPrefix) (bool, error) {
	plan, e := s.inputRequirementsTx(ctx, tx, tenant, needs)
	if e != nil {
		return false, e
	}
	candidates := append([]monitoring.RawNeed{}, plan.Derived...)
	for _, prefix := range append(prefixes, plan.Prefixes...) {
		minimum := monitoringvalue.MinTime.UnixNano()
		if prefix.Before > minimum {
			candidates = append(candidates, monitoring.RawNeed{Channel: prefix.Channel, From: &minimum, To: &prefix.Before})
		}
	}
	candidates, e = normalizedNeeds(candidates)
	if e != nil {
		return false, e
	}
	for _, need := range candidates {
		var pending bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND affected && $3::int8range)`, tenant, need.Channel, rangeSQL(planning.Range{From: *need.From, To: need.To})).Scan(&pending)
		if e != nil || pending {
			return pending, e
		}
	}
	return false, nil
}
