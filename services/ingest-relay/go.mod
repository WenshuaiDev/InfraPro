module github.com/WenshuaiDev/InfraPro/services/ingest-relay

go 1.27.0

require (
	github.com/WenshuaiDev/InfraPro/packages/go/foundation v0.0.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.12.0
)

replace github.com/WenshuaiDev/InfraPro/packages/go/foundation => ../../packages/go/foundation
