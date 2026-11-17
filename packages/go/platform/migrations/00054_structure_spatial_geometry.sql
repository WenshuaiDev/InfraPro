-- +goose Up
ALTER TABLE spatial_point_placements ADD COLUMN geometry jsonb;
ALTER TABLE spatial_point_placements ADD CONSTRAINT spatial_placement_geometry_shape CHECK (
 geometry IS NULL OR (target_kind IN ('monitoring_section','borehole') AND jsonb_typeof(geometry)='object' AND geometry->>'kind' IN ('region','path','plane'))
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM spatial_point_placements WHERE geometry IS NOT NULL) THEN
  RAISE EXCEPTION 'Retained section/borehole geometry requires compatible software; restore a verified backup';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE spatial_point_placements DROP COLUMN geometry;
