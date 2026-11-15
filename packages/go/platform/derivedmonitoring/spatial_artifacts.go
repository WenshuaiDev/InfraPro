package derivedmonitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

// RelatedArtifactQuery selects actual saved analysis inputs, never inferred
// dependencies or another project's artifact. Each returned link is fixed.
type RelatedArtifactQuery struct {
	OwnerKind, OwnerID, From, To, Cursor string
	Descendants                          bool
	Limit                                int
}

func (s *Service) RelatedArtifacts(ctx context.Context, a access.Actor, scope Scope, in RelatedArtifactQuery) (AnalysisArtifactPage, error) {
	out := AnalysisArtifactPage{Items: []AnalysisArtifactDetail{}}
	if !slices.Contains([]string{"facility", "component", "point"}, in.OwnerKind) || !access.ValidID(in.OwnerID) {
		return out, invalid("/owner", "需要明确所属对象。")
	}
	if in.Limit == 0 {
		in.Limit = 25
	}
	if in.Limit < 1 || in.Limit > 100 {
		return out, invalid("/limit", "每页为 1–100 项。")
	}
	var from, to *int64
	for _, item := range []struct {
		raw  string
		dest **int64
	}{{in.From, &from}, {in.To, &to}} {
		raw, dest := item.raw, item.dest
		if raw != "" {
			at, e := time.Parse(time.RFC3339Nano, raw)
			if e != nil || at.Year() < 1678 || at.Year() > 2261 {
				return out, invalid("/range", "需要可表示的带时区时刻。")
			}
			n := at.UnixNano()
			*dest = &n
		}
	}
	if from != nil && to != nil && *from >= *to {
		return out, invalid("/range", "结束时刻必须晚于开始时刻。")
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.list", false, func(tx pgx.Tx) error {
		rs := resources.Scope{Entry: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}
		if _, e := s.resources.SpatialTargetTx(ctx, tx, a, rs, resources.Target{Kind: resources.Kind(in.OwnerKind), ID: in.OwnerID}); e != nil {
			return e
		}
		revision, e := access.Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		type cursor struct{ Context, ID string }
		query := in
		query.Cursor = ""
		query.Limit = 0
		key := fingerprint([]any{scope, a.ID, query, revision})
		after := cursor{Context: key}
		if in.Cursor != "" {
			raw, e := base64.RawURLEncoding.DecodeString(in.Cursor)
			if e != nil || json.Unmarshal(raw, &after) != nil || after.Context != key || !access.ValidID(after.ID) {
				return invalid("/cursor", "查询或授权已变化，请重新读取。")
			}
		}
		rows, e := tx.Query(ctx, `WITH RECURSIVE owners AS(SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$3 AND id=$4 UNION ALL SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN owners o ON n.parent_kind=o.kind AND n.parent_id=o.id WHERE n.tenant_id=$1 AND $5 AND n.kind<>'channel' AND n.deleted_at IS NULL)
 SELECT DISTINCT v.id::text,v.artifact_id::text FROM analysis_artifact_versions v JOIN analysis_artifacts ar ON ar.tenant_id=v.tenant_id AND ar.id=v.artifact_id JOIN analysis_version_inputs i ON i.tenant_id=v.tenant_id AND i.version_id=v.config_version_id JOIN engineering_channels c ON c.tenant_id=i.tenant_id AND c.id=i.channel_id JOIN owners o ON o.kind=c.owner_kind AND o.id=CASE c.owner_kind WHEN 'facility' THEN c.facility_id WHEN 'component' THEN c.component_id ELSE c.point_id END WHERE v.tenant_id=$1 AND ar.scope_key=$2 AND ($6::uuid IS NULL OR v.id>$6) AND ($7::bigint IS NULL OR upper(v.output_range)>$7) AND ($8::bigint IS NULL OR lower(v.output_range)<$8) AND ($9::uuid IS NULL OR EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=c.tenant_id AND g.project_id=$9 AND g.resource_kind=o.kind AND COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)=o.id AND g.ended_at IS NULL)) ORDER BY v.id::text LIMIT 201`, scope.Tenant, scope.key(), in.OwnerKind, in.OwnerID, in.Descendants, nullable(after.ID), from, to, nullable(scope.Project))
		if e != nil {
			return e
		}
		type match struct{ version, artifact string }
		matches := []match{}
		for rows.Next() {
			var m match
			if e = rows.Scan(&m.version, &m.artifact); e != nil {
				rows.Close()
				return e
			}
			matches = append(matches, m)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for i, m := range matches {
			if i == 200 {
				after.ID = matches[i-1].version
				out.NextCursor = base64.RawURLEncoding.EncodeToString(jsonBytes(after))
				break
			}
			proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, m.artifact, m.version)
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
			if len(out.Items) == in.Limit {
				if i+1 < len(matches) {
					after.ID = m.version
					out.NextCursor = base64.RawURLEncoding.EncodeToString(jsonBytes(after))
				}
				break
			}
		}
		return nil
	})
	return out, e
}
