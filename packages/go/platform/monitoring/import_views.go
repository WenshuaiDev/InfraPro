package monitoring

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

func isHidden(e error) bool {
	var failure *Error
	return errors.As(e, &failure) && failure.Status == 404
}

// A partial project view has its own visible count. File totals, mappings,
// uploader identity and original-file attributes are omitted, never filled
// with zero or with the tenant's complete batch totals.
func (s *Service) partialImport(ctx context.Context, a access.Actor, scope Scope, id string) (ImportBatch, error) {
	out := ImportBatch{}
	if scope.Entry != "project" || !access.ValidID(id) {
		return out, notFound()
	}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	var manifest []selectedChunk
	var windows map[string][]Window
	var authRevision string
	e := s.transact(ctx, a, scope, "imports.detail", false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = scanBatch(tx.QueryRow(ctx, batchSelect+` WHERE tenant_id=$1 AND id=$2 AND state IN ('published','revoked')`, scope.Tenant, id))
		if e != nil {
			return e
		}
		snapshot, e := batchSnapshotTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		windows, e = availableWindowsTx(ctx, tx, auth, actualChannels(snapshot), "read", false)
		if e != nil {
			return e
		}
		if len(windows) == 0 {
			return notFound()
		}
		manifest, e = batchManifestTx(ctx, tx, scope.Tenant, id)
		authRevision = authorizationVersion(auth)
		return e
	})
	if e != nil {
		return out, e
	}
	var count uint64
	coverageByChannel := map[string]coverage{}
	e = s.storage.read(ctx, rawRead{Manifest: manifest, Tenant: scope.Tenant, Windows: windows}, func(row rawObservation) error {
		count++
		c, ok := coverageByChannel[row.Channel]
		if !ok {
			c = coverage{First: row.At, Last: row.At}
		}
		c.Count++
		c.First = min(c.First, row.At)
		c.Last = max(c.Last, row.At)
		coverageByChannel[row.Channel] = c
		return nil
	})
	if e != nil {
		return ImportBatch{}, storageError(e)
	}
	if count == 0 {
		return ImportBatch{}, notFound()
	}
	out.Name = "共享批次（当前授权范围）"
	out.Size = ""
	out.SourceFile = ""
	out.Creator = ""
	out.Counts = nil
	out.ValidationHash = ""
	out.ReadoptBatch = ""
	out.Mapping = nil
	out.Parts = []UploadPart{}
	out.ScopeClipped = true
	out.VisibleAdopted = strconv.FormatUint(count, 10)
	ids := []string{}
	for channel := range coverageByChannel {
		ids = append(ids, channel)
	}
	slices.Sort(ids)
	for _, channel := range ids {
		c := coverageByChannel[channel]
		out.VisibleCoverage = append(out.VisibleCoverage, BatchCoverage{channel, strconv.FormatUint(c.Count, 10), time.Unix(0, c.First).UTC().Format(time.RFC3339Nano), time.Unix(0, c.Last).UTC().Format(time.RFC3339Nano)})
	}
	e = s.transact(ctx, a, scope, "imports.detail", false, false, func(tx pgx.Tx, auth authorization) error {
		if authRevision != authorizationVersion(auth) {
			return revisionChanged()
		}
		var revision string
		if e := tx.QueryRow(ctx, `SELECT revision::text FROM monitoring_import_batches WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&revision); e != nil {
			return e
		}
		if revision != out.Revision {
			return revisionChanged()
		}
		return nil
	})
	return out, e
}
