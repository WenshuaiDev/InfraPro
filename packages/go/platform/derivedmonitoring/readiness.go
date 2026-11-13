package derivedmonitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/jackc/pgx/v5"
)

// Composition supplies this gate to real API and worker entry points. Pure
// domain fixtures may exercise authorization without a physical storage stack.
func (s *Service) WithReadiness(check func(context.Context) error) *Service {
	s.readiness = check
	return s
}
func (s *Service) requireReady(ctx context.Context) error {
	if s.readiness != nil {
		return s.readiness(ctx)
	}
	return nil
}

func (s *Service) Ready(ctx context.Context) error {
	fail := func() error {
		return problem("dependency_unavailable", "计算存储、规则引擎或变化日志尚未就绪。", 503)
	}
	if s.results == nil || s.monitoring == nil {
		return fail()
	}
	if e := s.monitoring.StorageReady(ctx); e != nil {
		return fail()
	}
	if e := s.results.Check(ctx); e != nil {
		return fail()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var valid bool
		if e := tx.QueryRow(ctx, `SELECT to_regclass('monitoring_changes') IS NOT NULL AND to_regclass('monitoring_change_consumers') IS NOT NULL AND to_regclass('monitoring_queue_fairness') IS NOT NULL AND NOT EXISTS(SELECT 1 FROM derived_metric_versions WHERE engine_version<>$1 OR compiler_version<>$2 OR schema_version<>$3) AND NOT EXISTS(SELECT 1 FROM analysis_config_versions WHERE engine_version<>$1 OR compiler_version<>$2)`, compute.EngineVersion, compute.CompilerVersion, compute.SchemaVersion).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return fail()
		}
		return nil
	})
	if e != nil {
		return fail()
	}
	return nil
}
