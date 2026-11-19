-- The three-stage rollback has produced real chunks, channel revisions and
-- installed summary heads. Exercise the installed SQL roles on that data.
BEGIN;
CREATE TEMP TABLE summary_probe ON COMMIT DROP AS
 SELECT h.tenant_id,h.channel_id,h.watermark,gen_random_uuid() AS job_id,
  (SELECT c.id FROM monitoring_chunks c WHERE c.tenant_id=h.tenant_id ORDER BY c.id LIMIT 1) AS chunk_id
 FROM monitoring_summary_heads h WHERE h.level='tree-v1' AND h.watermark>0
 ORDER BY h.watermark DESC,h.tenant_id,h.channel_id LIMIT 1;
GRANT SELECT ON summary_probe TO infrapro_monitoring_history,infrapro_monitoring_read,infrapro_monitoring_incremental;

CREATE FUNCTION pg_temp.summary_check(label text, accepted boolean) RETURNS jsonb LANGUAGE plpgsql AS $$
BEGIN
 IF accepted IS DISTINCT FROM true THEN RAISE EXCEPTION 'summary permission check failed: %',label; END IF;
 RETURN jsonb_build_object('check',label,'allowed',true);
END $$;
CREATE FUNCTION pg_temp.summary_denial(label text, statement text) RETURNS jsonb LANGUAGE plpgsql AS $$
BEGIN
 BEGIN
  EXECUTE statement;
 EXCEPTION WHEN insufficient_privilege THEN
  RETURN jsonb_build_object('check',label,'sqlstate',SQLSTATE);
 END;
 RAISE EXCEPTION 'expected summary permission refusal: %',label;
END $$;

CREATE FUNCTION pg_temp.check_dirty(owner_role text) RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE
 sample summary_probe%ROWTYPE;
 covered uuid := gen_random_uuid();
 newer uuid := gen_random_uuid();
 removed uuid;
 survivor uuid;
BEGIN
 SELECT * INTO STRICT sample FROM summary_probe;
 INSERT INTO monitoring_dirty_ranges(id,tenant_id,channel_id,affected,target_sequence)
 VALUES (covered,sample.tenant_id,sample.channel_id,'[0,1)',sample.watermark),
        (newer,sample.tenant_id,sample.channel_id,'[0,1)',sample.watermark+1);
 PERFORM set_config('role',owner_role,true);
 DELETE FROM monitoring_dirty_ranges WHERE tenant_id=sample.tenant_id AND channel_id=sample.channel_id
  AND id IN (covered,newer) AND target_sequence<=sample.watermark+1 RETURNING id INTO STRICT removed;
 SELECT id INTO STRICT survivor FROM monitoring_dirty_ranges WHERE id IN (covered,newer);
 RESET ROLE;
 RETURN pg_temp.summary_check(owner_role||'/covered-dirty-range',removed=covered AND survivor=newer);
END $$;

SELECT pg_temp.check_dirty('infrapro_monitoring_incremental');
SELECT pg_temp.check_dirty('infrapro_monitoring_history');
SELECT pg_temp.check_dirty('infrapro_monitoring_read');

INSERT INTO monitoring_jobs(id,tenant_id,kind,target_id,target_revision,input_hash,input,state,fence,lease_owner,lease_until)
 SELECT job_id,tenant_id,'summary',channel_id,1,repeat('e',64),'{"mode":"rebuild"}',
  'running',1,'summary-repair-test',clock_timestamp()+interval '5 minutes' FROM summary_probe;
INSERT INTO monitoring_job_steps(tenant_id,job_id,step_key,fence,state,result)
 SELECT tenant_id,job_id,'contribution:'||chunk_id,1,'completed',jsonb_build_object('chunk',chunk_id,'object','verification-object') FROM summary_probe;
UPDATE monitoring_channel_versions v SET integrity_revision=integrity_revision+1,quarantined=true
 FROM summary_probe p WHERE v.tenant_id=p.tenant_id AND v.channel_id=p.channel_id;

SET LOCAL ROLE infrapro_monitoring_history;
SELECT pg_temp.summary_denial('different-contribution-object',$op$
 INSERT INTO monitoring_contribution_replacements(tenant_id,chunk_id,job_id,object_key)
 SELECT tenant_id,chunk_id,job_id,'different-object' FROM summary_probe
$op$);
SELECT pg_temp.summary_denial('release-without-repair-receipt',$op$
 UPDATE monitoring_channel_versions v SET quarantined=false FROM summary_probe p
 WHERE v.tenant_id=p.tenant_id AND v.channel_id=p.channel_id
$op$);
RESET ROLE;
UPDATE monitoring_jobs j SET fence=2 FROM summary_probe p WHERE j.id=p.job_id;
SET LOCAL ROLE infrapro_monitoring_history;
SELECT pg_temp.summary_denial('old-contribution-fence',$op$
 INSERT INTO monitoring_contribution_replacements(tenant_id,chunk_id,job_id,object_key)
 SELECT tenant_id,chunk_id,job_id,'verification-object' FROM summary_probe
$op$);
RESET ROLE;
UPDATE monitoring_jobs j SET fence=1,lease_until=clock_timestamp()-interval '1 minute'
 FROM summary_probe p WHERE j.id=p.job_id;
SET LOCAL ROLE infrapro_monitoring_history;
SELECT pg_temp.summary_denial('expired-rebuild-lease',$op$
 INSERT INTO monitoring_contribution_replacements(tenant_id,chunk_id,job_id,object_key)
 SELECT tenant_id,chunk_id,job_id,'verification-object' FROM summary_probe
$op$);
RESET ROLE;
UPDATE monitoring_jobs j SET lease_until=clock_timestamp()+interval '5 minutes' FROM summary_probe p WHERE j.id=p.job_id;
SET LOCAL ROLE infrapro_monitoring_history;
WITH registered AS (
 INSERT INTO monitoring_contribution_replacements(tenant_id,chunk_id,job_id,object_key)
 SELECT tenant_id,(result->>'chunk')::uuid,job_id,result->>'object' FROM monitoring_job_steps
 WHERE job_id=(SELECT job_id FROM summary_probe) AND fence=1 AND step_key LIKE 'contribution:%' AND state='completed' RETURNING job_id
)
SELECT pg_temp.summary_check('live-matching-contribution',count(*)=1) FROM registered;
INSERT INTO monitoring_summary_replacements(tenant_id,channel_id,content_revision,integrity_revision,root_object,job_id)
 SELECT v.tenant_id,v.channel_id,v.content_revision,v.integrity_revision,'verification-root',p.job_id
 FROM monitoring_channel_versions v JOIN summary_probe p USING(tenant_id,channel_id);
WITH released AS (
 UPDATE monitoring_channel_versions v SET quarantined=false FROM summary_probe p
 WHERE v.tenant_id=p.tenant_id AND v.channel_id=p.channel_id RETURNING v.quarantined
)
SELECT pg_temp.summary_check('release-after-current-repair',count(*)=1 AND bool_and(NOT quarantined)) FROM released;
RESET ROLE;
SET LOCAL ROLE infrapro_monitoring_read;
SELECT pg_temp.summary_denial('head-ahead-of-tenant-position',$op$
 UPDATE monitoring_summary_heads h SET watermark=t.commit_sequence+1 FROM summary_probe p
 JOIN monitoring_tenant_states t USING(tenant_id)
 WHERE h.tenant_id=p.tenant_id AND h.channel_id=p.channel_id AND h.level='tree-v1'
$op$);
ROLLBACK;
