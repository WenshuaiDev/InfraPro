-- +goose Up
-- The scheduler reads one published upper bound per derived primary clock.
-- It remains only a hint; immutable result roots establish actual coverage.
CREATE INDEX derived_generation_clock_horizon ON derived_generations(tenant_id,channel_id,upper(output_range) DESC NULLS FIRST);
CREATE INDEX derived_generation_config_horizon ON derived_generations(tenant_id,channel_id,config_id,upper(output_range) DESC NULLS FIRST);
-- +goose Down
DROP INDEX derived_generation_clock_horizon;
DROP INDEX derived_generation_config_horizon;
