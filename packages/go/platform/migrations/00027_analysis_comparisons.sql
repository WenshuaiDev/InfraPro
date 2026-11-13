-- +goose Up
CREATE TABLE analysis_comparisons (
 tenant_id uuid NOT NULL,run_id uuid NOT NULL,left_version_id uuid NOT NULL,right_version_id uuid NOT NULL,
 output_mapping jsonb NOT NULL CHECK(jsonb_typeof(output_mapping)='array' AND jsonb_array_length(output_mapping) BETWEEN 1 AND 32),
 comparison_range int8range,comparison_hash text NOT NULL CHECK(comparison_hash ~ '^[0-9a-f]{64}$'),
 PRIMARY KEY(tenant_id,run_id),FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),
 FOREIGN KEY(tenant_id,left_version_id) REFERENCES analysis_artifact_versions(tenant_id,id),FOREIGN KEY(tenant_id,right_version_id) REFERENCES analysis_artifact_versions(tenant_id,id)
);
CREATE TABLE analysis_comparison_receipts (
 tenant_id uuid NOT NULL,run_id uuid NOT NULL,result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=4194304),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(tenant_id,run_id),FOREIGN KEY(tenant_id,run_id) REFERENCES analysis_comparisons(tenant_id,run_id)
);
CREATE TRIGGER analysis_comparison_immutable BEFORE UPDATE OR DELETE ON analysis_comparisons FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TRIGGER analysis_comparison_receipt_immutable BEFORE UPDATE OR DELETE ON analysis_comparison_receipts FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
-- +goose StatementBegin
CREATE FUNCTION analysis_comparison_scope_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM derived_runs r JOIN analysis_artifact_versions lv ON lv.tenant_id=r.tenant_id AND lv.id=NEW.left_version_id JOIN analysis_artifacts la ON la.tenant_id=lv.tenant_id AND la.id=lv.artifact_id JOIN analysis_artifact_versions rv ON rv.tenant_id=r.tenant_id AND rv.id=NEW.right_version_id JOIN analysis_artifacts ra ON ra.tenant_id=rv.tenant_id AND ra.id=rv.artifact_id WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.run_id AND r.mode='comparison' AND r.project_id IS NOT DISTINCT FROM la.project_id AND r.project_id IS NOT DISTINCT FROM ra.project_id) THEN
  RAISE EXCEPTION 'comparison and both artifacts must belong to one workspace' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER analysis_comparison_scope_guard BEFORE INSERT ON analysis_comparisons FOR EACH ROW EXECUTE FUNCTION analysis_comparison_scope_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'analysis comparison evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
