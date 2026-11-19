WITH busy AS MATERIALIZED (
 SELECT br.tenant_id,count(*) n FROM ingestion_identity_decisions d JOIN ingestion_receipts br ON br.id=d.receipt_id WHERE d.state='identifying' AND d.lease_until>clock_timestamp() GROUP BY br.tenant_id
), eligible AS MATERIALIZED (
 SELECT s.id,s.tenant_id,s.planning_paused FROM ingestion_sources s LEFT JOIN busy ON busy.tenant_id=s.tenant_id
 WHERE COALESCE(busy.n,0)<2 AND NOT s.blocked AND NOT EXISTS(SELECT 1 FROM ingestion_change_intents c WHERE c.source_id=s.id AND c.state='pending')
), fresh AS MATERIALIZED (
 SELECT picked.* FROM eligible s CROSS JOIN LATERAL (
  SELECT r.id,r.tenant_id,r.stored_at available_at FROM ingestion_receipts r
  WHERE NOT s.planning_paused AND r.source_id=s.id AND r.tenant_id IS NOT NULL AND r.state='stored'
  AND NOT EXISTS(SELECT 1 FROM ingestion_identity_decisions d WHERE d.receipt_id=r.id AND d.generation=r.current_generation)/* split event ownership */
  ORDER BY r.stored_at LIMIT 16
 ) picked
), retries AS MATERIALIZED (
 SELECT r.id,r.tenant_id,d.available_at FROM ingestion_identity_decisions d JOIN ingestion_receipts r ON r.id=d.receipt_id AND r.current_generation=d.generation JOIN eligible s ON s.id=r.source_id
 WHERE r.tenant_id IS NOT NULL AND r.state IN ('stored','identifying','retry_wait') AND (NOT s.planning_paused OR d.recovery_id IS NOT NULL)
 AND (d.state IN ('queued','retry_wait') AND d.available_at<=clock_timestamp() OR d.state='identifying' AND d.lease_until<clock_timestamp())/* split event ownership */ ORDER BY d.available_at LIMIT 16
)
SELECT id::text,tenant_id::text FROM (SELECT * FROM fresh UNION ALL SELECT * FROM retries) candidates ORDER BY available_at LIMIT 16
