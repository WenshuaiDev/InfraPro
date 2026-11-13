-- +goose Up
CREATE TABLE monitoring_queue_fairness (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 queue text NOT NULL CHECK(queue IN ('foreground','historical')),
 last_claimed_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,queue)
);
-- +goose Down
DROP TABLE monitoring_queue_fairness;
