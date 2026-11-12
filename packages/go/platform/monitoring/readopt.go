package monitoring

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReadoptSelection struct {
	All      bool     `json:"all_originally_adopted"`
	Channels []string `json:"channel_ids"`
	Range    *Range   `json:"range"`
}
type ReadoptInput struct {
	WriteMeta
	ReadoptSelection
}

func (s *Service) Readopt(ctx context.Context, a access.Actor, scope Scope, id string, in ReadoptInput) (ImportBatch, error) {
	out := ImportBatch{}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if e := in.validate(); e != nil {
		return out, e
	}
	if !access.ValidID(id) {
		return out, notFound()
	}
	if in.All && (len(in.Channels) > 0 || in.Range != nil) || !in.All && len(in.Channels) == 0 && in.Range == nil {
		return out, invalid("selection", "请明确选择全部原采用记录，或限制通道／时间。")
	}
	for _, channel := range in.Channels {
		if !access.ValidID(channel) {
			return out, notFound()
		}
	}
	if in.Range != nil {
		if _, e := in.Range.window(); e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "imports.readopt", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "imports.readopt:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		original, e := scanBatch(tx.QueryRow(ctx, batchSelect+` WHERE tenant_id=$1 AND id=$2 AND state='revoked'`, scope.Tenant, id))
		if e != nil {
			return e
		}
		if original.Revision != in.ExpectedRevision {
			return revisionChanged()
		}
		prior, e := batchSnapshotTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		ids := slices.Clone(in.Channels)
		if len(ids) == 0 {
			ids = actualChannels(prior)
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		for _, channel := range ids {
			if _, ok := prior.ActualCoverage[channel]; !ok {
				return notFound()
			}
		}
		windows, e := authorizedWindowsTx(ctx, tx, auth, ids, "import")
		if e != nil {
			return e
		}
		if _, e = semanticsTx(ctx, tx, scope.Tenant, ids, true); e != nil {
			return e
		}
		selected := map[string]coverage{}
		for _, id := range ids {
			selected[id] = prior.ActualCoverage[id]
		}
		if in.Range == nil {
			if e = coversBatch(selected, windows); e != nil {
				return e
			}
		} else {
			w, _ := in.Range.window()
			for _, channel := range ids {
				allowed := false
				for _, grant := range windows[channel] {
					if grant.Covers(w) {
						allowed = true
						break
					}
				}
				if !allowed {
					return notFound()
				}
			}
		}
		var raw []byte
		if e = tx.QueryRow(ctx, `SELECT mapping FROM monitoring_import_mappings WHERE tenant_id=$1 AND batch_id=$2 AND revision=$3`, scope.Tenant, id, original.MappingRevision).Scan(&raw); e != nil {
			return e
		}
		var mapping Mapping
		if e = json.Unmarshal(raw, &mapping); e != nil {
			return e
		}
		mapping.Channels = slices.DeleteFunc(mapping.Channels, func(c ChannelMapping) bool { return !slices.Contains(ids, c.Channel) })
		selection := in.ReadoptSelection
		selection.Channels = ids
		selection.All = false
		snapshot := validationSnapshot{Readopt: &selection}
		newID := uuid.NewString()
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_import_batches(id,tenant_id,project_id,created_by,state,source_file_id,original_name,expected_size,mapping_revision,readopt_batch_id,semantic_snapshot) VALUES($1,$2,$3,$4,'received',$5,$6,$7,1,$8,$9)`, newID, scope.Tenant, nullable(scope.Project), a.ID, original.SourceFile, original.Name, original.Size, id, encodeJSON(snapshot)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_import_mappings(tenant_id,batch_id,revision,mapping,sha256,created_by) VALUES($1,$2,1,$3,$4,$5)`, scope.Tenant, newID, encodeJSON(mapping), digest(mapping), a.ID); e != nil {
			return e
		}
		out, e = batchTx(ctx, tx, auth, newID, false)
		if e != nil {
			return e
		}
		if e = auditTx(ctx, tx, auth, "imports.readopt", newID, in.Reason, in.RequestID, map[string]any{"source_batch": id, "selection": selection}); e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "imports.readopt:"+id, in.RequestID, in, out)
	})
	return out, e
}

func (s *Service) readoptCandidates(ctx context.Context, j Job, in validationInput, consume func(rawObservation) error) error {
	selection := in.snapshot.Readopt
	if selection == nil {
		return invalid("readopt", "再次采用缺少固定选择。")
	}
	var manifest []selectedChunk
	e := s.db(ctx, func(tx pgx.Tx) error {
		var e error
		manifest, e = batchManifestTx(ctx, tx, j.tenant, in.batch.ReadoptBatch)
		return e
	})
	if e != nil {
		return e
	}
	windows := map[string][]Window{}
	for _, id := range selection.Channels {
		w := Window{}
		if selection.Range != nil {
			w, e = selection.Range.window()
			if e != nil {
				return e
			}
		}
		windows[id] = []Window{w}
	}
	return s.storage.read(ctx, rawRead{Manifest: manifest, Tenant: j.tenant, Windows: windows}, func(r rawObservation) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		sem, ok := in.snapshot.Semantics[r.Channel]
		if !ok || sem.Hash != r.Semantic {
			return problem("semantic_changed", "原观测语义与当前固定通道语义不一致。", 409)
		}
		r.Batch = j.Target
		r.Mapping, _ = strconv.ParseUint(in.batch.MappingRevision, 10, 64)
		r.Adoption = uuid.NewSHA1(uuid.MustParse(j.Target), []byte("adoption-v1:"+r.Observation)).String()
		r.First = false
		// Keep original observation identity, source coordinates, quality and exact
		// semantic payload. Only the new adoption and its batch are new identities.
		return consume(r)
	})
}

func readoptAccessTx(ctx context.Context, tx pgx.Tx, tenant, original string, selection *ReadoptSelection, windows map[string][]Window) error {
	if selection == nil {
		return invalid("readopt", "再次采用缺少固定选择。")
	}
	if selection.Range != nil {
		w, e := selection.Range.window()
		if e != nil {
			return e
		}
		for _, id := range selection.Channels {
			covered := false
			for _, allowed := range windows[id] {
				if allowed.Covers(w) {
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
	prior, e := batchSnapshotTx(ctx, tx, tenant, original)
	if e != nil {
		return e
	}
	selected := map[string]coverage{}
	for _, id := range selection.Channels {
		c, ok := prior.ActualCoverage[id]
		if !ok {
			return notFound()
		}
		selected[id] = c
	}
	return coversBatch(selected, windows)
}
