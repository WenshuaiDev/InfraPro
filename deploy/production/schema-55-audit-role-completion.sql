-- Production deployment role completion for schema 55.
-- Required by current audit metadata resolvers; excludes parsing object content.
GRANT SELECT ON device_effective_classifications,engineering_resource_records,
 parsing_runs,parsing_samples,parsing_configs,parsing_config_revisions,
 parsing_method_versions,parsing_product_defaults,parsing_device_heads,parsing_device_adoptions
 TO infrapro_audit_worker;
GRANT SELECT(run_id,generation,state,error_code) ON parsing_run_executions TO infrapro_audit_worker;
