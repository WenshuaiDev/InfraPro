-- Run as maintenance owner in an isolated architecture test database.
-- The caller starts the transaction. All data and temporary helpers roll back.
-- Each role in production has a separate connection. Discard cached SQL plans
-- when impersonating consecutive roles in this single test transaction.
\set ON_ERROR_STOP on
SELECT set_config('arc_test.instance',gen_random_uuid()::text,true);
UPDATE event_protocol SET frozen=false;
UPDATE dispatch_ownership SET mode='compat',generation=1
 WHERE role IN ('monitoring-publish-worker','ingest-custody-worker','monitoring-incremental-worker');

CREATE FUNCTION pg_temp.acquire_compat(target text) RETURNS bigint LANGUAGE sql AS $$
 INSERT INTO dispatch_leases(role,partition,instance,generation,restore_epoch,lease_until)
 SELECT o.role,o.partition,current_setting('arc_test.instance')::uuid,o.generation,o.restore_epoch,clock_timestamp()+interval '30 seconds'
 FROM dispatch_ownership o JOIN event_protocol p USING(restore_epoch)
 WHERE o.role=target AND o.partition='all' AND o.mode='compat' AND NOT p.frozen FOR SHARE OF o,p
 ON CONFLICT(role,partition,instance) DO UPDATE SET generation=EXCLUDED.generation,
   restore_epoch=EXCLUDED.restore_epoch,lease_until=EXCLUDED.lease_until RETURNING generation
$$;
CREATE FUNCTION pg_temp.compat_commit(target text, expected bigint) RETURNS boolean LANGUAGE sql AS $$
 SELECT EXISTS(SELECT FROM event_protocol p
   JOIN dispatch_ownership o ON o.restore_epoch=p.restore_epoch AND o.role=target AND o.partition='all'
     AND o.generation=expected AND o.mode='compat'
   JOIN dispatch_leases l ON l.role=o.role AND l.partition=o.partition AND l.restore_epoch=o.restore_epoch
     AND l.generation=o.generation AND l.instance=current_setting('arc_test.instance')::uuid
     AND l.lease_until>clock_timestamp()
   WHERE p.id=1 AND NOT p.frozen FOR SHARE OF p,o,l)
$$;
CREATE FUNCTION pg_temp.assert_lease(target text, expected bigint, denied text) RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE actual bigint;
BEGIN
 actual:=pg_temp.acquire_compat(target);
 IF actual IS DISTINCT FROM expected THEN RAISE EXCEPTION 'expected generation %, got % for % as %',expected,actual,target,current_user; END IF;
 IF pg_temp.acquire_compat(denied) IS NOT NULL THEN RAISE EXCEPTION 'claimed another role %',denied; END IF;
 IF pg_temp.compat_commit(target,1) IS DISTINCT FROM (expected IS NOT DISTINCT FROM 1) THEN RAISE EXCEPTION 'incorrect old fence %',target; END IF;
 IF expected IS NOT NULL AND NOT pg_temp.compat_commit(target,expected) THEN RAISE EXCEPTION 'current fence refused %',target; END IF;
 RETURN jsonb_build_object('role',current_user,'target',target,'stage',CASE WHEN expected=1 THEN 'compat-before'
   WHEN expected=5 THEN 'compat-after' ELSE 'split' END,'claimedGeneration',actual,
   'gen1Commit',pg_temp.compat_commit(target,1),'crossRole',denied,'crossRoleDenied',true);
END
$$;
CREATE FUNCTION pg_temp.assert_forged_lease_denied(target text) RETURNS jsonb LANGUAGE plpgsql AS $$
BEGIN
 BEGIN
  UPDATE dispatch_leases SET generation=1 WHERE role=target AND instance=current_setting('arc_test.instance')::uuid;
  RAISE EXCEPTION 'wrote stale generation for %',target;
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 BEGIN
  UPDATE dispatch_leases SET restore_epoch=gen_random_uuid() WHERE role=target AND instance=current_setting('arc_test.instance')::uuid;
  RAISE EXCEPTION 'wrote unrelated restore epoch for %',target;
 EXCEPTION WHEN insufficient_privilege THEN NULL;
 END;
 RETURN jsonb_build_object('role',current_user,'target',target,'stage','invalid-ticket',
   'staleGenerationDenied',true,'otherRestoreDenied',true);
END
$$;
CREATE FUNCTION pg_temp.assert_frozen(target text) RETURNS jsonb LANGUAGE plpgsql AS $$
BEGIN
 IF pg_temp.acquire_compat(target) IS NOT NULL THEN RAISE EXCEPTION 'frozen claim for %',target; END IF;
 IF pg_temp.compat_commit(target,5) THEN RAISE EXCEPTION 'frozen commit for %',target; END IF;
 RETURN jsonb_build_object('role',current_user,'target',target,'stage','frozen',
   'currentClaimDenied',true,'currentCommitDenied',true);
END
$$;

SET LOCAL ROLE infrapro_compat_monitoring_worker;
DISCARD PLANS;
SELECT pg_temp.assert_lease('monitoring-publish-worker',1,'ingest-custody-worker');
SET LOCAL ROLE infrapro_compat_ingest_worker;
DISCARD PLANS;
SELECT pg_temp.assert_lease('ingest-custody-worker',1,'monitoring-publish-worker');
SET LOCAL ROLE infrapro_compat_api;
DISCARD PLANS;
SELECT pg_temp.assert_lease('monitoring-incremental-worker',1,'monitoring-publish-worker');
RESET ROLE;
\echo three limited compatibility roles hold generation 1

UPDATE dispatch_ownership SET mode='draining',generation=2;
UPDATE dispatch_ownership SET mode='split',generation=3;
SET LOCAL ROLE infrapro_compat_monitoring_worker;
DISCARD PLANS;
SELECT pg_temp.assert_lease('monitoring-publish-worker',NULL,'ingest-custody-worker');
SET LOCAL ROLE infrapro_compat_ingest_worker;
DISCARD PLANS;
SELECT pg_temp.assert_lease('ingest-custody-worker',NULL,'monitoring-publish-worker');
SET LOCAL ROLE infrapro_compat_api;
DISCARD PLANS;
SELECT pg_temp.assert_lease('monitoring-incremental-worker',NULL,'monitoring-publish-worker');
RESET ROLE;
\echo split generation 3 excluded the old compatibility tickets and denied all three compatibility claims

UPDATE dispatch_ownership SET mode='draining',generation=4;
UPDATE dispatch_ownership SET mode='compat',generation=5;
SET LOCAL ROLE infrapro_compat_monitoring_worker;
DISCARD PLANS;
SELECT pg_temp.assert_lease('monitoring-publish-worker',5,'ingest-custody-worker');
SELECT pg_temp.assert_forged_lease_denied('monitoring-publish-worker');
SET LOCAL ROLE infrapro_compat_ingest_worker;
DISCARD PLANS;
SELECT pg_temp.assert_lease('ingest-custody-worker',5,'monitoring-publish-worker');
SELECT pg_temp.assert_forged_lease_denied('ingest-custody-worker');
SET LOCAL ROLE infrapro_compat_api;
DISCARD PLANS;
SELECT pg_temp.assert_lease('monitoring-incremental-worker',5,'monitoring-publish-worker');
SELECT pg_temp.assert_forged_lease_denied('monitoring-incremental-worker');
RESET ROLE;
\echo the same three instances reacquired generation 5, with older generations and other restores denied

UPDATE event_protocol SET frozen=true;
SET LOCAL ROLE infrapro_compat_monitoring_worker;
DISCARD PLANS;
SELECT pg_temp.assert_frozen('monitoring-publish-worker');
SET LOCAL ROLE infrapro_compat_ingest_worker;
DISCARD PLANS;
SELECT pg_temp.assert_frozen('ingest-custody-worker');
SET LOCAL ROLE infrapro_compat_api;
DISCARD PLANS;
SELECT pg_temp.assert_frozen('monitoring-incremental-worker');
RESET ROLE;
\echo frozen protocol denied the current generation claim and commit
ROLLBACK;
