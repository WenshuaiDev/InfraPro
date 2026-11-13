package derivedmonitoring

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

func analysisRestricted(e error) bool {
	var de *Error
	var raw *monitoring.Error
	return errors.Is(e, access.ErrDenied) || errors.As(e, &de) && (de.Status == 403 || de.Status == 404) || errors.As(e, &raw) && (raw.Status == 403 || raw.Status == 404)
}
func (s *Service) AnalysisArtifacts(ctx context.Context, a access.Actor, scope Scope, filter MetricFilter) (AnalysisArtifactPage, error) {
	out := AnalysisArtifactPage{Items: []AnalysisArtifactDetail{}}
	if filter.Cursor != "" && !access.ValidID(filter.Cursor) {
		return out, invalid("/cursor", "分页游标无效。")
	}
	if filter.State != "" && filter.State != "active" && filter.State != "archived" {
		return out, invalid("/state", "不支持该成果状态。")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.list", false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id::text FROM analysis_artifacts WHERE tenant_id=$1 AND scope_key=$2 AND latest_version>0 AND ($3::uuid IS NULL OR id>$3) AND ($4='' OR name ILIKE '%'||$4||'%') AND ($5='' OR status=$5) ORDER BY id LIMIT 201`, scope.Tenant, scope.key(), nullable(filter.Cursor), strings.TrimSpace(filter.Query), filter.State)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i, id := range ids {
			if i == 200 {
				if i > 0 {
					out.NextCursor = ids[i-1]
				}
				break
			}
			proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, "")
			if analysisRestricted(e) {
				continue
			}
			if e != nil {
				return e
			}
			var rule Rule
			if e = compute.Decode(proof.Detail.Config.Body.Rule, &rule); e != nil {
				return e
			}
			proof.Detail.Outputs = []Output{}
			for _, output := range rule.Outputs {
				if slices.Contains(proof.Detail.Run.Outputs, output.Key) {
					proof.Detail.Outputs = append(proof.Detail.Outputs, output)
				}
			}
			out.Items = append(out.Items, proof.Detail)
			if len(out.Items) == limit {
				if i+1 < len(ids) {
					out.NextCursor = id
				}
				break
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) AnalysisArtifactVersions(ctx context.Context, a access.Actor, scope Scope, id, cursor string) (AnalysisArtifactVersions, error) {
	out := AnalysisArtifactVersions{Items: []AnalysisArtifactVersion{}}
	before := int64(1<<63 - 1)
	if cursor != "" {
		var e error
		before, e = revisionNumber(cursor)
		if e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.versions", false, func(tx pgx.Tx) error {
		if _, e := analysisArtifactTx(ctx, tx, scope, id); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT v.id::text,v.version_no FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE v.tenant_id=$1 AND a.scope_key=$2 AND a.id=$3 AND v.version_no<$4 ORDER BY v.version_no DESC LIMIT 201`, scope.Tenant, scope.key(), id, before)
		if e != nil {
			return e
		}
		type item struct {
			id     string
			number int64
		}
		items := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.number); e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i, v := range items {
			if i == 200 {
				out.NextCursor = strconv.FormatInt(items[i-1].number, 10)
				break
			}
			proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, v.id)
			if analysisRestricted(e) {
				continue
			}
			if e != nil {
				return e
			}
			out.Items = append(out.Items, proof.Detail.Version)
			if len(out.Items) == 50 {
				if i+1 < len(items) {
					out.NextCursor = strconv.FormatInt(v.number, 10)
				}
				break
			}
		}
		return nil
	})
	return out, e
}

type AnalysisArtifactArchiveResult struct {
	Artifact AnalysisArtifact `json:"artifact"`
	Replayed bool             `json:"replayed"`
}

func (s *Service) ArchiveAnalysisArtifact(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta, audit access.Meta) (AnalysisArtifactArchiveResult, error) {
	var out AnalysisArtifactArchiveResult
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.archive", true, func(tx pgx.Tx) error {
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.artifacts.archive", in, payload, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			_, e = analysisArtifactTx(ctx, tx, scope, id)
			return e
		}
		before, e := analysisArtifactTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return conflict()
		}
		if before.Status != "archived" {
			tag, e := tx.Exec(ctx, `UPDATE analysis_artifacts SET status='archived',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3`, scope.Tenant, id, in.Revision)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return conflict()
			}
		}
		out.Artifact, e = analysisArtifactTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.artifacts.archive", "analysis_artifact", id, in, payload, before, out, audit)
	})
	return out, e
}
func (s *Service) AnalysisArtifactLineage(ctx context.Context, a access.Actor, scope Scope, id string, in AnalysisArtifactQuery) (map[string]any, error) {
	proof, e := s.loadSelectedAnalysis(ctx, a, scope, id, in, "analysis.artifacts.lineage")
	if e != nil {
		return nil, e
	}
	var document map[string]any
	if e = s.results.ReadDocument(ctx, scope.Tenant, "lineage", proof.Version.Lineage, &document); e != nil {
		return nil, e
	}
	if document["schema"] != "analysis-lineage/1" || document["run_id"] != proof.Version.RunID || document["config_version_id"] != proof.Version.ConfigVersionID {
		return nil, problem("result_integrity_unavailable", "分析来源清单身份不一致。", 503)
	}
	e = s.transact(ctx, a, scope, "analysis.artifacts.lineage", false, func(tx pgx.Tx) error {
		_, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, id, proof.Version.ID, proof.Selection)
		return e
	})
	if e != nil {
		return nil, e
	}
	if proof.Selection != nil {
		needs, e := sourceCandidates(proof.Selection.Direct, proof.Selection.Sources)
		if e != nil {
			return nil, e
		}
		document["input_ranges"] = needs
		document["upstream_inputs"] = proof.Selection.Sources
		document["output_range"] = proof.Selection.Range
		document["outputs"] = proof.Selection.Outputs
		document["baselines"] = slices.DeleteFunc(slices.Clone(proof.Manifest.Baselines), func(b BaselineProof) bool { return !slices.Contains(proof.Selection.Baselines, b.Reference) })
		document["partial"] = proof.Selection.Partial
		document["lineage_root"] = proof.Version.Lineage
	}
	return document, nil
}
