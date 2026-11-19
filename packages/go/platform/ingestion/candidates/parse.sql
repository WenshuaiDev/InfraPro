WITH busy AS MATERIALIZED (
 SELECT tenant_id,count(*) n FROM ingestion_parse_plans WHERE state='parsing' AND lease_until>clock_timestamp() GROUP BY tenant_id
), eligible AS MATERIALIZED (
 SELECT src.id,src.planning_paused FROM ingestion_sources src LEFT JOIN busy ON busy.tenant_id=src.tenant_id WHERE NOT src.blocked AND COALESCE(busy.n,0)<2
)
SELECT candidate.id::text,candidate.tenant_id::text FROM eligible src CROSS JOIN LATERAL (
 SELECT p.id,p.tenant_id,p.available_at FROM ingestion_parse_plans p
 WHERE (p.snapshot->'source'->>'ingress_id')::uuid=src.id
 AND p.state IN ('queued','retry_wait','parsing')
 AND (NOT src.planning_paused OR COALESCE(p.snapshot->>'recovery_id','')<>'')
 AND ((p.state IN ('queued','retry_wait') AND p.available_at<=clock_timestamp()) OR (p.state='parsing' AND p.lease_until<clock_timestamp()))
 /* split event ownership */ ORDER BY p.available_at LIMIT 16
) candidate ORDER BY candidate.available_at LIMIT 16
