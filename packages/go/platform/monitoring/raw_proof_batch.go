package monitoring

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// RawSnapshotPart contains server-generated dependencies of an immutable
// result. Batching preserves each snapshot's source position and exact candidate
// intervals; it never widens a partial reader's required authorization.
type RawSnapshotPart struct {
	Snapshot string
	Needs    []RawNeed
}

// CheckRawSnapshotPartsTx performs the same current-only qualification as
// CheckRawSnapshotPartTx for a collection, with shared catalog reads. All checks
// remain inside the caller's transaction and are repeated on every invocation.
func (s *Service) CheckRawSnapshotPartsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, parts []RawSnapshotPart, capability string) error {
	if len(parts) == 0 {
		return nil
	}
	bySnapshot := map[string][]RawNeed{}
	for _, part := range parts {
		if !access.ValidID(part.Snapshot) || len(part.Needs) == 0 {
			return integrityUnavailable()
		}
		bySnapshot[part.Snapshot] = append(bySnapshot[part.Snapshot], part.Needs...)
	}
	ids := make([]string, 0, len(bySnapshot))
	for id, needs := range bySnapshot {
		var e error
		bySnapshot[id], _, _, e = normalizeRawNeeds(needs)
		if e != nil {
			return e
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	auth, e := s.rawAuthorizationTx(ctx, tx, a, scope)
	if e != nil {
		return e
	}
	allWindows, historicalWindows := map[string][]Window{}, map[string][]Window{}
	channels, ranges, positions := []string{}, []string{}, []int64{}
	expectedIntegrity := map[string]string{}
	for offset := 0; offset < len(ids); offset += 128 {
		chunk := ids[offset:min(offset+128, len(ids))]
		rows, e := tx.Query(ctx, rawEvidenceSelect+` WHERE tenant_id=$1 AND id=ANY($2::uuid[]) ORDER BY id`, scope.Tenant, chunk)
		if e != nil {
			return e
		}
		loaded := 0
		for rows.Next() {
			original, e := scanRawEvidence(rows)
			if e != nil {
				rows.Close()
				return e
			}
			part, e := rawDependencyPart(original, bySnapshot[original.Snapshot.ID])
			if e != nil {
				rows.Close()
				return e
			}
			loaded++
			for _, need := range part.Needs {
				allWindows[need.Channel] = append(allWindows[need.Channel], Window{need.From, need.To})
				if part.History {
					historicalWindows[need.Channel] = append(historicalWindows[need.Channel], Window{need.From, need.To})
				}
			}
			for channel, revision := range part.Integrity {
				if previous, exists := expectedIntegrity[channel]; exists && previous != revision {
					rows.Close()
					return integrityUnavailable()
				}
				expectedIntegrity[channel] = revision
			}
			for _, need := range part.Candidates {
				channels = append(channels, need.Channel)
				ranges = append(ranges, (Window{need.From, need.To}).pg())
				positions = append(positions, mustRevision(part.Snapshot.SourcePosition))
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if loaded != len(chunk) {
			return notFound()
		}
	}
	needsOf := func(windows map[string][]Window) []RawNeed {
		needs := []RawNeed{}
		for channel, parts := range windows {
			for _, part := range UnionWindows(parts) {
				needs = append(needs, RawNeed{Channel: channel, From: part.From, To: part.To})
			}
		}
		return needs
	}
	allNeeds, historicalNeeds := needsOf(allWindows), needsOf(historicalWindows)
	if e = authorizeRawNeedsTx(ctx, tx, auth, allNeeds, false, capability); e != nil {
		return e
	}
	if len(historicalNeeds) > 0 {
		if e = authorizeRawNeedsTx(ctx, tx, auth, historicalNeeds, true, capability); e != nil {
			return e
		}
	}
	_, _, channelIDs, e := normalizeRawNeeds(allNeeds)
	if e != nil {
		return e
	}
	integrity, e := rawIntegrityTx(ctx, tx, scope.Tenant, channelIDs)
	if e != nil {
		return e
	}
	if digest(integrity) != digest(expectedIntegrity) {
		return integrityUnavailable()
	}
	var changed, revoked bool
	e = tx.QueryRow(ctx, `SELECT count(*)>0,COALESCE(bool_or(e.change_type='raw_revoke'),false)
 FROM unnest($2::uuid[],$3::int8range[],$4::bigint[]) n(channel_id,affected,source_position)
 JOIN monitoring_change_ranges r ON r.tenant_id=$1 AND r.channel_id=n.channel_id AND r.affected&&n.affected
 JOIN monitoring_changes e ON e.tenant_id=r.tenant_id AND e.id=r.event_id AND e.source_position>n.source_position`, scope.Tenant, channels, ranges, positions).Scan(&changed, &revoked)
	if e != nil {
		return e
	}
	if revoked {
		return problem("snapshot_stale", "固定输入范围的采用已被撤销，需要重新建立输入快照。", 409)
	}
	if changed {
		return problem("export_invalidated", "结果输入已变化。", 409)
	}
	return nil
}
