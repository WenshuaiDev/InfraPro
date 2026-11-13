package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type BatchCoverage struct {
	Channel string `json:"channel_id"`
	Count   string `json:"count"`
	First   string `json:"earliest"`
	Last    string `json:"latest"`
}
type BatchRevokePreview struct {
	Batch           string            `json:"batch_id"`
	Revision        string            `json:"revision"`
	Hash            string            `json:"preview_hash"`
	Adopted         string            `json:"adopted_count"`
	Ignored         string            `json:"ignored_duplicates"`
	Qualities       map[string]string `json:"quality_counts"`
	Coverage        []BatchCoverage   `json:"coverage"`
	AffectedExports string            `json:"affected_own_exports"`
}
type BatchRevokeResult struct {
	Batch    string `json:"batch_id"`
	Revision string `json:"revision"`
	Sequence string `json:"revoke_sequence"`
	Revoked  string `json:"revoked_count"`
	Replayed bool   `json:"replayed"`
}
type revokeEvidence struct {
	preview       BatchRevokePreview
	authorization string
	windows       map[string][]Window
	channels      []string
	versions      map[string]string
	manifest      []selectedChunk
	publication   string
}

func batchManifestTx(ctx context.Context, tx pgx.Tx, tenant, batch string) ([]selectedChunk, error) {
	out := []selectedChunk{}
	rows, e := tx.Query(ctx, `SELECT c.id::text,m.attempt_id::text,c.payload_hash,c.row_count,m.first_adopted_count,c.batch_id::text,p.sequence,(p.revoke_sequence IS NOT NULL),m.quality_counts FROM monitoring_publication_chunks m JOIN monitoring_chunks c ON c.tenant_id=m.tenant_id AND c.id=m.chunk_id JOIN monitoring_publications p ON p.tenant_id=m.tenant_id AND p.id=m.publication_id WHERE p.tenant_id=$1 AND p.batch_id=$2 ORDER BY c.id LIMIT 20001`, tenant, batch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var m selectedChunk
		var quality []byte
		if e = rows.Scan(&m.Chunk, &m.Attempt, &m.Hash, &m.Count, &m.First, &m.Batch, &m.Sequence, &m.Revoked, &quality); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(quality, &m.Qualities); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 20000 {
		return nil, problem("batch_budget_exceeded", "单批次块数超出维护预算，请联系运维检查导入布局。", 422)
	}
	rows.Close()
	return out, resolveLocationsTx(ctx, tx, tenant, out, 0)
}
func batchSnapshotTx(ctx context.Context, tx pgx.Tx, tenant, id string) (validationSnapshot, error) {
	var out validationSnapshot
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT semantic_snapshot FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&raw)
	if e != nil {
		return out, e
	}
	return out, json.Unmarshal(raw, &out)
}
func actualChannels(snapshot validationSnapshot) []string {
	ids := []string{}
	for id := range snapshot.ActualCoverage {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
func versionsTx(ctx context.Context, tx pgx.Tx, tenant string, ids []string, lock bool) (map[string]string, error) {
	out := map[string]string{}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	for _, id := range ids {
		var revision string
		var quarantined bool
		if e := tx.QueryRow(ctx, `SELECT content_revision::text||':'||revoke_revision::text||':'||integrity_revision::text,quarantined FROM monitoring_channel_versions WHERE tenant_id=$1 AND channel_id=$2`+suffix, tenant, id).Scan(&revision, &quarantined); e != nil {
			return out, e
		}
		if quarantined {
			return out, integrityUnavailable()
		}
		out[id] = revision
	}
	return out, nil
}
func revokeHash(proof revokeEvidence) string {
	return digest([]any{"revoke-v1", proof.preview.Batch, proof.preview.Revision, proof.publication, proof.authorization, proof.windows, proof.versions, proof.manifest})
}

func (s *Service) revokeEvidence(ctx context.Context, a access.Actor, scope Scope, id string) (revokeEvidence, error) {
	out := revokeEvidence{preview: BatchRevokePreview{Batch: id, Qualities: map[string]string{"good": "0", "unmarked": "0", "suspect": "0", "bad": "0"}, Coverage: []BatchCoverage{}}}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, "batches.preview", false, false, func(tx pgx.Tx, auth authorization) error {
		// A shared published batch is governed by its current actual data ranges,
		// not by the project that originally uploaded its file.
		b, e := scanBatch(tx.QueryRow(ctx, batchSelect+` WHERE tenant_id=$1 AND id=$2 AND state='published'`, scope.Tenant, id))
		if e != nil {
			return e
		}
		snapshot, e := batchSnapshotTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		out.channels = actualChannels(snapshot)
		if len(out.channels) == 0 {
			return notFound()
		}
		out.windows, e = authorizedWindowsTx(ctx, tx, auth, out.channels, "revoke")
		if e != nil {
			return e
		}
		out.authorization = authorizationVersion(auth)
		out.preview.Revision = b.Revision
		out.preview.Ignored = b.Counts.Ignored
		out.versions, e = versionsTx(ctx, tx, scope.Tenant, out.channels, false)
		if e != nil {
			return e
		}
		out.manifest, e = batchManifestTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT id::text FROM monitoring_publications WHERE tenant_id=$1 AND batch_id=$2 AND revoke_sequence IS NULL`, scope.Tenant, id).Scan(&out.publication); e != nil {
			return e
		}
		var exports int64
		e = tx.QueryRow(ctx, `SELECT count(*) FROM monitoring_exports e JOIN monitoring_jobs j ON j.id=e.job_id JOIN monitoring_snapshots sn ON sn.id=e.snapshot_id JOIN monitoring_selections sel ON sel.id=sn.selection_id WHERE e.tenant_id=$1 AND j.actor_id=$2 AND j.project_id IS NOT DISTINCT FROM $3::uuid AND e.invalidated_at IS NULL AND e.expires_at>clock_timestamp() AND sel.channel_ids&&$4::uuid[]`, scope.Tenant, a.ID, nullable(scope.Project), out.channels).Scan(&exports)
		out.preview.AffectedExports = countText(exports)
		return e
	})
	if e != nil {
		return out, e
	}
	var expected, count uint64
	for _, entry := range out.manifest {
		expected += entry.Count
	}
	coverageByChannel := map[string]coverage{}
	e = s.storage.read(ctx, rawRead{Manifest: out.manifest, Tenant: scope.Tenant, Windows: out.windows}, func(r rawObservation) error {
		count++
		quality, _ := strconv.ParseUint(out.preview.Qualities[r.Quality], 10, 64)
		out.preview.Qualities[r.Quality] = strconv.FormatUint(quality+1, 10)
		cover, ok := coverageByChannel[r.Channel]
		if !ok {
			cover = coverage{First: r.At, Last: r.At}
		}
		cover.Count++
		cover.First = min(cover.First, r.At)
		cover.Last = max(cover.Last, r.At)
		coverageByChannel[r.Channel] = cover
		return nil
	})
	if e != nil {
		return out, storageError(e)
	}
	if count != expected {
		return out, notFound()
	}
	out.preview.Adopted = strconv.FormatUint(count, 10)
	for _, id := range out.channels {
		cover := coverageByChannel[id]
		out.preview.Coverage = append(out.preview.Coverage, BatchCoverage{id, strconv.FormatUint(cover.Count, 10), time.Unix(0, cover.First).UTC().Format(time.RFC3339Nano), time.Unix(0, cover.Last).UTC().Format(time.RFC3339Nano)})
	}
	out.preview.Hash = revokeHash(out)
	e = s.transact(ctx, a, scope, "batches.preview", false, false, func(tx pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != out.authorization {
			return revisionChanged()
		}
		versions, e := versionsTx(ctx, tx, scope.Tenant, out.channels, false)
		if e != nil {
			return e
		}
		if digest(versions) != digest(out.versions) {
			return revisionChanged()
		}
		return nil
	})
	return out, e
}
func (s *Service) PreviewRevokeBatch(ctx context.Context, a access.Actor, scope Scope, id, revision string) (BatchRevokePreview, error) {
	proof, e := s.revokeEvidence(ctx, a, scope, id)
	if e == nil && proof.preview.Revision != revision {
		e = revisionChanged()
	}
	return proof.preview, e
}

func (s *Service) RevokeBatch(ctx context.Context, a access.Actor, scope Scope, id string, in RevokeInput) (BatchRevokeResult, error) {
	out := BatchRevokeResult{}
	if e := in.validate(); e != nil {
		return out, e
	}
	var replay bool
	e := s.transact(ctx, a, scope, "batches.revoke", false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		replay, e = receiptTx(ctx, tx, auth, "batches.revoke:"+id, in.RequestID, in, &out)
		if e == nil && replay {
			snapshot, e := batchSnapshotTx(ctx, tx, scope.Tenant, id)
			if e != nil {
				return e
			}
			windows, e := authorizedWindowsTx(ctx, tx, auth, actualChannels(snapshot), "revoke")
			if e != nil {
				return e
			}
			return coversBatch(snapshot.ActualCoverage, windows)
		}
		return e
	})
	if e != nil || replay {
		out.Replayed = replay
		return out, e
	}
	proof, e := s.revokeEvidence(ctx, a, scope, id)
	if e != nil {
		return out, e
	}
	if proof.preview.Hash != in.PreviewHash || proof.preview.Revision != in.ExpectedRevision {
		return out, revisionChanged()
	}
	e = s.transact(ctx, a, scope, "batches.revoke", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "batches.revoke:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			out.Replayed = hit
			return e
		}
		if proof.authorization != authorizationVersion(auth) {
			return revisionChanged()
		}
		windows, e := authorizedWindowsTx(ctx, tx, auth, proof.channels, "revoke")
		if e != nil {
			return e
		}
		if digest(windows) != digest(proof.windows) {
			return revisionChanged()
		}
		b, e := scanBatch(tx.QueryRow(ctx, batchSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, scope.Tenant, id))
		if e != nil {
			return e
		}
		if b.Revision != in.ExpectedRevision || b.State != "published" {
			return revisionChanged()
		}
		versions, e := versionsTx(ctx, tx, scope.Tenant, proof.channels, true)
		if e != nil {
			return e
		}
		if digest(versions) != digest(proof.versions) {
			return revisionChanged()
		}
		var seq int64
		if e = tx.QueryRow(ctx, `UPDATE monitoring_tenant_states SET commit_sequence=commit_sequence+1 WHERE tenant_id=$1 RETURNING commit_sequence`, scope.Tenant).Scan(&seq); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_publications SET revoke_sequence=$3,revoked_at=clock_timestamp(),revoked_by=$4,revoke_reason=$5 WHERE tenant_id=$1 AND id=$2 AND revoke_sequence IS NULL`, scope.Tenant, proof.publication, seq, a.ID, in.Reason); e != nil {
			return e
		}
		changes := []ChangeRange{}
		for _, c := range proof.preview.Coverage {
			if _, e = tx.Exec(ctx, `UPDATE monitoring_channel_versions SET content_revision=content_revision+1,revoke_revision=revoke_revision+1 WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, c.Channel); e != nil {
				return e
			}
			first, _ := time.Parse(time.RFC3339Nano, c.First)
			last, _ := time.Parse(time.RFC3339Nano, c.Last)
			lo := first.UnixNano()
			var hi *int64
			if last.UnixNano() < 1<<63-1 {
				value := last.UnixNano() + 1
				hi = &value
			}
			w := Window{From: &lo, To: hi}
			changes = append(changes, ChangeRange{Channel: c.Channel, From: w.From, To: w.To})
			if _, e = tx.Exec(ctx, `INSERT INTO monitoring_dirty_ranges(tenant_id,channel_id,affected,target_sequence) VALUES($1,$2,$3::int8range,$4)`, scope.Tenant, c.Channel, w.pg(), seq); e != nil {
				return e
			}
		}
		if _, e = AppendChangeTx(ctx, tx, scope.Tenant, seq, "raw_revoke", "direct", proof.publication, changes); e != nil {
			return e
		}
		if e = enqueueSummariesTx(ctx, tx, scope.Tenant, proof.channels, seq); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_exports e SET invalidated_at=clock_timestamp() FROM monitoring_snapshots sn JOIN monitoring_selections sel ON sel.id=sn.selection_id WHERE e.tenant_id=$1 AND e.snapshot_id=sn.id AND e.invalidated_at IS NULL AND sel.channel_ids&&$2::uuid[]`, scope.Tenant, proof.channels); e != nil {
			return e
		}
		var revision string
		if e = tx.QueryRow(ctx, `UPDATE monitoring_import_batches SET state='revoked',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 RETURNING revision::text`, scope.Tenant, id).Scan(&revision); e != nil {
			return e
		}
		out = BatchRevokeResult{id, revision, fmt.Sprint(seq), proof.preview.Adopted, false}
		if e = auditTx(ctx, tx, auth, "batches.revoke", id, in.Reason, in.RequestID, out); e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "batches.revoke:"+id, in.RequestID, in, out)
	})
	return out, e
}

func coversBatch(coverageByChannel map[string]coverage, windows map[string][]Window) error {
	for id, c := range coverageByChannel {
		covered := false
		var start *int64
		if c.First < c.Last {
			start = &c.First
		}
		for _, w := range windows[id] {
			if w.Contains(c.Last, start) {
				covered = true
				break
			}
		}
		if !covered {
			return notFound()
		}
	}
	return nil
}
