package derivedmonitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type comparisonPage struct {
	Hash string `json:"hash"`
	Rows uint64 `json:"rows,string"`
}
type comparisonPageDocument struct {
	Schema string                         `json:"schema"`
	Run    string                         `json:"run_id"`
	Items  []AnalysisComparisonDifference `json:"items"`
}
type comparisonReceipt struct {
	AnalysisComparisonResult
	Pages []comparisonPage `json:"_pages"`
}
type AnalysisComparisonQuery struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type AnalysisComparisonRows struct {
	Summary    AnalysisComparisonResult       `json:"summary"`
	Items      []AnalysisComparisonDifference `json:"items"`
	NextCursor string                         `json:"next_cursor"`
}

func (s *Service) AnalysisComparisonRows(ctx context.Context, a access.Actor, scope Scope, id string, in AnalysisComparisonQuery) (AnalysisComparisonRows, error) {
	out := AnalysisComparisonRows{Items: []AnalysisComparisonDifference{}}
	if !access.ValidID(id) {
		return out, notFound()
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return out, invalid("/limit", "每页最多1000个差异。")
	}
	var stored comparisonReceipt
	var run runRecord
	check := func() error {
		return s.transact(ctx, a, scope, "analysis.comparisons.query", false, func(tx pgx.Tx) error {
			var e error
			run, e = scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.scope_key=$2 AND r.id=$3 AND r.mode='comparison'`, scope.Tenant, scope.key(), id))
			if e != nil {
				return e
			}
			if e = s.authorizeComparisonTx(ctx, tx, a, scope, run, false); e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `SELECT result FROM analysis_comparison_receipts WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, id).Scan(&stored)
			if e == pgx.ErrNoRows {
				return problem("comparison_pending", "比较尚未完成。", 409)
			}
			if e != nil {
				return e
			}
			qualified, e := s.readComparisonTx(ctx, tx, a, scope, run)
			if e != nil {
				return e
			}
			out.Summary = *qualified.Comparison
			return nil
		})
	}
	if e := check(); e != nil {
		return out, e
	}
	hash := fingerprint([]any{run.PlanHash, stored.Pages})
	var offset uint64
	if in.Cursor != "" {
		data, e := base64.RawURLEncoding.DecodeString(in.Cursor)
		var cursor artifactCursor
		if e != nil || json.Unmarshal(data, &cursor) != nil || cursor.Hash != hash {
			return out, invalid("/cursor", "比较分页游标无效。")
		}
		offset = cursor.Offset
	}
	if offset > stored.Differences {
		return out, invalid("/cursor", "分页位置超出固定比较。")
	}
	var start uint64
	for _, page := range stored.Pages {
		if start+page.Rows <= offset {
			start += page.Rows
			continue
		}
		if e := check(); e != nil {
			return AnalysisComparisonRows{}, e
		}
		var document comparisonPageDocument
		if e := s.results.ReadDocument(ctx, scope.Tenant, "checkpoints", page.Hash, &document); e != nil {
			return AnalysisComparisonRows{}, e
		}
		if document.Schema != "analysis-comparison-page/1" || document.Run != id || uint64(len(document.Items)) != page.Rows {
			return AnalysisComparisonRows{}, problem("result_integrity_unavailable", "比较差异清单不完整。", 503)
		}
		for n, item := range document.Items {
			if start+uint64(n) < offset {
				continue
			}
			out.Items = append(out.Items, item)
			if len(out.Items) == limit {
				break
			}
		}
		start += page.Rows
		if len(out.Items) == limit {
			break
		}
	}
	if next := offset + uint64(len(out.Items)); next < stored.Differences {
		out.NextCursor = base64.RawURLEncoding.EncodeToString(jsonBytes(artifactCursor{Hash: hash, Offset: next}))
	}
	if e := check(); e != nil {
		return AnalysisComparisonRows{}, e
	}
	return out, nil
}
