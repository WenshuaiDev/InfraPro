package derivedmonitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, error) {
	action := map[string]string{"derived_channel": "channels.detail", "derived_metric_definition": "metrics.detail", "analysis_config": "analysis.configs.detail", "analysis_artifact": "analysis.artifacts.detail", "derived_run": "runs.list"}[kind]
	if action == "" {
		return "", access.ErrDenied
	}
	op := access.DerivedOperation(scope.Entry, action)
	if scope.Entry == "project" {
		if _, e := access.RequireActorProjectTx(ctx, tx, a, op, scope.Tenant, scope.Project); e != nil {
			return "", e
		}
	} else if e := access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
		return "", e
	}
	switch kind {
	case "derived_channel":
		v, e := s.channelTx(ctx, tx, a, scope, id)
		return v.Resource.Name, e
	case "derived_metric_definition":
		v, e := definitionTx(ctx, tx, scope, id, false)
		return v.Name, e
	case "analysis_config":
		v, e := analysisConfigTx(ctx, tx, scope, id, false)
		return v.Name, e
	case "analysis_artifact":
		v, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, "")
		return v.Detail.Artifact.Name, e
	case "derived_run":
		var channel, job, mode string
		if e := tx.QueryRow(ctx, `SELECT COALESCE(channel_id::text,''),job_id::text,mode FROM derived_runs WHERE tenant_id=$1 AND id::text=$2`, scope.Tenant, id).Scan(&channel, &job, &mode); e != nil {
			return "", e
		}
		// Analysis and comparison runs have no channel. Reuse the source's full
		// metadata qualification without reading trial/result bodies.
		if _, e := s.monitoring.TaskTx(ctx, tx, a, monitoringScope(scope), job, false); e != nil {
			return "", e
		}
		if channel == "" {
			if mode == "comparison" {
				return "比较运行", nil
			}
			return "分析运行", nil
		}
		v, e := s.channelTx(ctx, tx, a, scope, channel)
		return v.Resource.Name, e
	}
	return "", access.ErrDenied
}
