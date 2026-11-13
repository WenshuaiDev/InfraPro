-- +goose Up
CREATE TABLE derived_run_dependencies (
 tenant_id uuid NOT NULL, run_id uuid NOT NULL, dependency_run_id uuid NOT NULL,
 PRIMARY KEY(tenant_id,run_id,dependency_run_id),
 CHECK(run_id<>dependency_run_id),
 FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),
 FOREIGN KEY(tenant_id,dependency_run_id) REFERENCES derived_runs(tenant_id,id)
);
CREATE INDEX derived_run_dependency_wakeup ON derived_run_dependencies(tenant_id,dependency_run_id,run_id);
CREATE TRIGGER derived_run_dependencies_immutable BEFORE UPDATE OR DELETE ON derived_run_dependencies FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
-- +goose StatementBegin
CREATE FUNCTION derived_run_dependency_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM derived_runs p JOIN derived_runs c ON c.tenant_id=p.tenant_id WHERE p.tenant_id=NEW.tenant_id AND p.id=NEW.run_id AND c.id=NEW.dependency_run_id AND p.actor_id IS NOT NULL AND p.actor_id=c.actor_id AND p.project_id IS NOT DISTINCT FROM c.project_id AND c.mode='backfill') THEN
  RAISE EXCEPTION 'explicit child runs must use the same current user scope' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER derived_run_dependency_guard BEFORE INSERT ON derived_run_dependencies FOR EACH ROW EXECUTE FUNCTION derived_run_dependency_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'explicit run dependency evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
