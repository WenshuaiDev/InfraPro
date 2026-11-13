package derivedmonitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CopyAnalysis(ctx context.Context, a access.Actor, scope Scope, id string, in CopyAnalysisInput, audit access.Meta) (AnalysisConfigResult, error) {
	var out AnalysisConfigResult
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !codePattern.MatchString(in.Code) {
		return out, invalid("/code", "分析编号格式不正确。")
	}
	if e := checkName(in.Name, ""); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.configs.copy", true, func(tx pgx.Tx) error {
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.configs.copy", in.WriteMeta, payload, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			return s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out.Config)
		}
		source, e := analysisVersionTx(ctx, tx, scope, in.SourceVersionID)
		if e != nil {
			return e
		}
		if source.ConfigID != id {
			return notFound()
		}
		if _, e = s.analysisBindingsTx(ctx, tx, a, scope, source.Body); e != nil {
			return e
		}
		created := uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO analysis_configs(id,tenant_id,project_id,code,name,draft,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, created, scope.Tenant, nullable(scope.Project), in.Code, in.Name, jsonBytes(source.Body), a.ID)
		if e != nil {
			return e
		}
		out.Config, e = analysisConfigTx(ctx, tx, scope, created, false)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.configs.copy", "analysis_config", created, in.WriteMeta, payload, map[string]string{"source_config_id": id, "source_version_id": source.ID}, out, audit)
	})
	return out, e
}
func (s *Service) ArchiveAnalysis(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta, audit access.Meta) (AnalysisConfigResult, error) {
	var out AnalysisConfigResult
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.configs.archive", true, func(tx pgx.Tx) error {
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.configs.archive", in, payload, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			return s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out.Config)
		}
		before, e := analysisConfigTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return conflict()
		}
		if before.Status != "archived" {
			if _, e = tx.Exec(ctx, `UPDATE analysis_configs SET status='archived',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id); e != nil {
				return e
			}
		}
		out.Config, e = analysisConfigTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if e = s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out.Config); e != nil {
			return e
		}
		before.Draft = nil
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.configs.archive", "analysis_config", id, in, payload, before, out, audit)
	})
	return out, e
}
func analysisBaselineIdentity(p *compute.Plan, cfg ChannelConfig) string {
	rules := map[string]compute.Baseline{}
	for _, node := range p.Rule.Nodes {
		if node.Params.Baseline != nil && node.Params.Baseline.Kind != "constant" {
			rules[node.Params.Baseline.Reference] = *node.Params.Baseline
		}
	}
	return fingerprint([]any{rules, cfg.BaselineBindings, baselineSelections(p, cfg)})
}
