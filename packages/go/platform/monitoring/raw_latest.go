package monitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// LatestRawInput reads the actual newest published observation within the
// declared candidate ranges. It never treats a source change sequence, a chunk
// boundary, or a prior calculated value as an input timestamp.
func (s *Service) LatestRawInput(ctx context.Context, a access.Actor, scope Scope, needs []RawNeed, capability string) (*string, error) {
	if len(needs) == 0 {
		return nil, nil
	}
	var evidence rawEvidence
	e := s.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, needs, false, capability); e != nil {
			return e
		}
		var e error
		evidence, e = prepareRawEvidenceTx(ctx, tx, scope.Tenant, RawFreezeRequest{Needs: needs})
		return e
	})
	if e != nil {
		return nil, e
	}
	_, windows, ids, e := normalizeRawNeeds(needs)
	if e != nil {
		return nil, e
	}
	if len(evidence.Manifest) > 0 {
		if e = s.requireStorage(); e != nil {
			return nil, e
		}
	}
	var latest *int64
	actual := append([]RawNeed{}, needs...)
	for _, id := range ids {
		if len(evidence.Manifest) == 0 {
			break
		}
		e = s.storage.read(ctx, rawRead{Tenant: scope.Tenant, Manifest: evidence.Manifest, Windows: map[string][]Window{id: windows[id]}, Qualities: allQualities, Reverse: true, Order: "channel", EndpointOnly: true, Limit: 1}, func(row rawObservation) error {
			if latest == nil || row.At > *latest {
				at := row.At
				latest = &at
			}
			if row.Start != nil {
				end := row.At
				if end < 1<<63-1 {
					end++
				}
				actual = append(actual, RawNeed{Channel: id, From: row.Start, To: &end})
			}
			return ctx.Err()
		})
		if e != nil {
			return nil, e
		}
	}
	e = s.transact(ctx, a, scope, "details", false, false, func(tx pgx.Tx, auth authorization) error {
		if e := authorizeRawNeedsTx(ctx, tx, auth, actual, false, capability); e != nil {
			return e
		}
		state, e := checkRawEvidenceTx(ctx, tx, scope.Tenant, evidence, false)
		if e != nil {
			return e
		}
		if state.Changed {
			return problem("snapshot_stale", "读取最新输入期间相关数据已变化，请重试。", 409)
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return nsText(latest), nil
}
